package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/observeid/identity-platform/pkg/telemetry"
)

// ProcessorConfig holds configuration for the outbox processor.
type ProcessorConfig struct {
	PollInterval time.Duration // How often to check for new events (default: 500ms)
	BatchSize    int           // Max events to process per batch (default: 100)
	MaxRetries   int           // Max retry attempts before dead-letter (default: 5)
	Neo4jTimeout time.Duration // Timeout for Neo4j operations (default: 30s)
}

// DefaultConfig returns sensible defaults for the processor.
func DefaultConfig() ProcessorConfig {
	return ProcessorConfig{
		PollInterval: 500 * time.Millisecond,
		BatchSize:    100,
		MaxRetries:   5,
		Neo4jTimeout: 30 * time.Second,
	}
}

// Processor handles outbox event processing and Neo4j sync.
// It runs as a background goroutine, polling for unprocessed events and applying them to Neo4j.
type Processor struct {
	config  ProcessorConfig
	outbox  *Outbox
	neo4j   neo4j.DriverWithContext
	running atomic.Bool
}

// NewProcessor creates a new outbox processor.
func NewProcessor(outbox *Outbox, neo4jDriver neo4j.DriverWithContext, config ProcessorConfig) *Processor {
	return &Processor{
		config: config,
		outbox:  outbox,
		neo4j:   neo4jDriver,
	}
}

// Start begins the background processing loop.
// It runs until ctx is cancelled or Stop() is called.
func (p *Processor) Start(ctx context.Context) {
	p.running.Store(true)
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	log.Printf("[OUTBOX] processor started (interval=%s, batch=%d, maxRetries=%d)",
		p.config.PollInterval, p.config.BatchSize, p.config.MaxRetries)

	for p.running.Load() {
		select {
		case <-ctx.Done():
			log.Printf("[OUTBOX] processor stopped (context cancelled)")
			return
		case <-ticker.C:
			p.processBatch(ctx)
		}
	}
}

// Stop gracefully stops the processor.
func (p *Processor) Stop() {
	p.running.Store(false)
	log.Printf("[OUTBOX] processor stopping")
}

// processBatch fetches and processes a batch of unprocessed events.
func (p *Processor) processBatch(ctx context.Context) {
	startTime := time.Now()
	
	events, err := p.outbox.GetUnprocessed(ctx, p.config.BatchSize)
	if err != nil {
		log.Printf("[OUTBOX] fetch error: %v", err)
		return
	}

	if len(events) == 0 {
		return
	}

	log.Printf("[OUTBOX] processing %d events", len(events))

	successCount := 0
	failCount := 0

	for _, event := range events {
		eventStart := time.Now()
		
		// Skip events that have exceeded max retries
		if event.RetryCount >= p.config.MaxRetries {
			log.Printf("[OUTBOX] event %s exceeded max retries (%d), dead-lettering", event.ID, event.RetryCount)
			failCount++
			telemetry.OutboxEventsFailed.Inc()
			continue
		}

		if err := p.applyToNeo4j(ctx, event); err != nil {
			log.Printf("[OUTBOX] event %s failed (retry %d/%d): %v",
				event.ID, event.RetryCount+1, p.config.MaxRetries, err)
			p.outbox.MarkFailed(ctx, event.ID, err.Error())
			failCount++
			telemetry.OutboxEventsFailed.Inc()
		} else {
			p.outbox.MarkProcessed(ctx, event.ID)
			successCount++
			telemetry.OutboxEventsProcessed.Inc()
			telemetry.OutboxProcessingLatency.
				WithLabelValues(event.EventType).
				Observe(float64(time.Since(eventStart).Milliseconds()))
		}
	}

	// Update queue size metric
	stats, _ := p.outbox.Stats(ctx)
	telemetry.OutboxQueueSize.Set(float64(stats["pending"]))

	log.Printf("[OUTBOX] batch complete: %d success, %d failed in %v",
		successCount, failCount, time.Since(startTime))
}

// applyToNeo4j applies an event to Neo4j based on its event type.
func (p *Processor) applyToNeo4j(ctx context.Context, event Event) error {
	session := p.neo4j.NewSession(ctx, neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeWrite,
	})
	defer session.Close(ctx)

	// Set timeout for Neo4j operations
	ctx, cancel := context.WithTimeout(ctx, p.config.Neo4jTimeout)
	defer cancel()

	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	switch event.EventType {
	case "identity.created":
		return p.handleIdentityCreated(ctx, session, event.AggregateID, payload)
	case "identity.updated":
		return p.handleIdentityUpdated(ctx, session, event.AggregateID, payload)
	case "identity.deleted":
		return p.handleIdentityDeleted(ctx, session, event.AggregateID, payload)
	case "role.assigned":
		return p.handleRoleAssigned(ctx, session, event.AggregateID, payload)
	case "role.revoked":
		return p.handleRoleRevoked(ctx, session, event.AggregateID, payload)
	case "entitlement.provisioned":
		return p.handleEntitlementProvisioned(ctx, session, event.AggregateID, payload)
	case "entitlement.revoked":
		return p.handleEntitlementRevoked(ctx, session, event.AggregateID, payload)
	default:
		log.Printf("[OUTBOX] unknown event type: %s (id=%s)", event.EventType, event.ID)
		return nil // Don't fail on unknown types — just skip
	}
}

// handleIdentityCreated creates an identity node in Neo4j.
func (p *Processor) handleIdentityCreated(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
	_, err := session.Run(ctx, `
		MERGE (i:Identity {uuid: $id})
		SET i.tenant_id = $tenant_id,
			i.email = $email,
			i.display_name = $display_name,
			i.status = $status,
			i.type = COALESCE($type, 'human'),
			i.source = COALESCE($source, 'manual'),
			i.department = $department,
			i.employee_id = $employee_id,
			i.created_at = datetime()
	`, map[string]any{
		"id":            id,
		"tenant_id":     getStr(payload, "tenant_id"),
		"email":         getStr(payload, "email"),
		"display_name":  getStr(payload, "display_name"),
		"status":        getStr(payload, "status"),
		"type":          getStr(payload, "type"),
		"source":        getStr(payload, "source"),
		"department":    getStr(payload, "department"),
		"employee_id":   getStr(payload, "employee_id"),
	})
	if err != nil {
		return fmt.Errorf("identity.created: %w", err)
	}
	return nil
}

// handleIdentityUpdated updates an identity node in Neo4j.
func (p *Processor) handleIdentityUpdated(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
	_, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $id})
		SET i.updated_at = datetime(),
			i.email = COALESCE($email, i.email),
			i.display_name = COALESCE($display_name, i.display_name),
			i.status = COALESCE($status, i.status),
			i.department = COALESCE($department, i.department)
	`, map[string]any{
		"id":           id,
		"email":        getStr(payload, "email"),
		"display_name": getStr(payload, "display_name"),
		"status":       getStr(payload, "status"),
		"department":   getStr(payload, "department"),
	})
	if err != nil {
		return fmt.Errorf("identity.updated: %w", err)
	}
	return nil
}

// handleIdentityDeleted removes an identity node from Neo4j.
func (p *Processor) handleIdentityDeleted(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
	_, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $id})
		DETACH DELETE i
	`, map[string]any{"id": id})
	if err != nil {
		return fmt.Errorf("identity.deleted: %w", err)
	}
	return nil
}

// handleRoleAssigned creates a HAS_ROLE relationship.
func (p *Processor) handleRoleAssigned(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
	_, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $identity_id}), (r:Role {id: $role_id})
		MERGE (i)-[rel:HAS_ROLE]->(r)
		SET rel.assigned_at = timestamp(),
			rel.assigned_by = $assigned_by,
			rel.source = 'outbox'
	`, map[string]any{
		"identity_id": id,
		"role_id":     getStr(payload, "role_id"),
		"assigned_by": getStr(payload, "assigned_by"),
	})
	if err != nil {
		return fmt.Errorf("role.assigned: %w", err)
	}
	return nil
}

// handleRoleRevoked removes a HAS_ROLE relationship.
func (p *Processor) handleRoleRevoked(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
	_, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $identity_id})-[rel:HAS_ROLE]->(r:Role {id: $role_id})
		DELETE rel
	`, map[string]any{
		"identity_id": id,
		"role_id":     getStr(payload, "role_id"),
	})
	if err != nil {
		return fmt.Errorf("role.revoked: %w", err)
	}
	return nil
}

// handleEntitlementProvisioned creates a HAS_DIRECT_ACCESS relationship.
func (p *Processor) handleEntitlementProvisioned(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
	_, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $identity_id}), (res:Resource {id: $resource_id})
		MERGE (i)-[rel:HAS_DIRECT_ACCESS]->(res)
		SET rel.granted_at = timestamp(),
			rel.granted_by = $granted_by,
			rel.reason = $reason,
			rel.source = 'outbox'
	`, map[string]any{
		"identity_id": id,
		"resource_id": getStr(payload, "resource_id"),
		"granted_by":  getStr(payload, "granted_by"),
		"reason":      getStr(payload, "reason"),
	})
	if err != nil {
		return fmt.Errorf("entitlement.provisioned: %w", err)
	}
	return nil
}

// handleEntitlementRevoked removes a HAS_DIRECT_ACCESS relationship and marks the entitlement as revoked.
func (p *Processor) handleEntitlementRevoked(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
	_, err := session.Run(ctx, `
		MATCH (e:Entitlement {id: $entitlement_id})
		SET e.status = 'revoked', e.revoked_at = timestamp(), e.revoked_by = $revoked_by
	`, map[string]any{
		"entitlement_id": getStr(payload, "entitlement_id"),
		"revoked_by":     getStr(payload, "revoked_by"),
	})
	if err != nil {
		return fmt.Errorf("entitlement.revoked: %w", err)
	}
	return nil
}

// getStr safely extracts a string value from a map.
func getStr(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
