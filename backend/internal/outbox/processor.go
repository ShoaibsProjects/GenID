package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/observeid/genid/internal/eventbus"
	"github.com/observeid/genid/pkg/telemetry"
)

// ProcessorConfig holds configuration for the outbox processor.
type ProcessorConfig struct {
	PollInterval time.Duration // How often to check for new events (default: 500ms)
	BatchSize    int           // Max events to process per batch (default: 100)
	MaxRetries   int           // Max retry attempts before dead-letter (default: 5)
	Neo4jTimeout time.Duration // Timeout for Neo4j operations (default: 30s)
}

// DefaultConfig returns sensible defaults for the processor.
//
// PollInterval defaults to 100ms (was 500ms) so the outbox-to-Neo4j lag stays
// well under 200ms at the loads this platform targets. Batch fetch + per-event
// Neo4j writes mean a 100ms tick rarely finds more than a handful of rows at
// moderate event rates, and at high rates the loop just processes bigger
// batches per tick — bounded by BatchSize.
func DefaultConfig() ProcessorConfig {
	return ProcessorConfig{
		PollInterval: 100 * time.Millisecond,
		BatchSize:    100,
		MaxRetries:   5,
		Neo4jTimeout: 30 * time.Second,
	}
}

// ConfigFromEnv returns a ProcessorConfig populated from environment
// variables, falling back to DefaultConfig() for any unset value. Recognised
// env vars: OUTBOX_POLL_INTERVAL_MS, OUTBOX_BATCH_SIZE,
// OUTBOX_MAX_RETRIES, OUTBOX_NEO4J_TIMEOUT_MS.
func ConfigFromEnv() ProcessorConfig {
	cfg := DefaultConfig()
	if v, err := strconv.Atoi(os.Getenv("OUTBOX_POLL_INTERVAL_MS")); err == nil && v > 0 {
		cfg.PollInterval = time.Duration(v) * time.Millisecond
	}
	if v, err := strconv.Atoi(os.Getenv("OUTBOX_BATCH_SIZE")); err == nil && v > 0 {
		cfg.BatchSize = v
	}
	if v, err := strconv.Atoi(os.Getenv("OUTBOX_MAX_RETRIES")); err == nil && v > 0 {
		cfg.MaxRetries = v
	}
	if v, err := strconv.Atoi(os.Getenv("OUTBOX_NEO4J_TIMEOUT_MS")); err == nil && v > 0 {
		cfg.Neo4jTimeout = time.Duration(v) * time.Millisecond
	}
	return cfg
}

// Processor handles outbox event processing and Neo4j sync.
// It runs as a background goroutine, polling for unprocessed events and applying them to Neo4j.
type Processor struct {
	config  ProcessorConfig
	outbox  *Outbox
	neo4j   neo4j.DriverWithContext
	natsBus *eventbus.NatsBus
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

// WithNatsBus sets the NATS event bus for post-sync event publishing.
func (p *Processor) WithNatsBus(bus *eventbus.NatsBus) *Processor {
	p.natsBus = bus
	return p
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
//
// Same-type events are grouped and applied together via UNWIND-style batch
// Cypher when the group is large enough to benefit (≥ batchBatchSize, default
// 5), turning N Neo4j round-trips into one. For small or mixed-type batches
// the original per-event path is used — correctness preserved. On any batched
// failure the whole group falls back to per-event processing so individual
// failing events can still be MarkFailed precisely without losing the others.
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

	// Drop over-retry events up front (no Neo4j call).
	var live []Event
	for _, ev := range events {
		if ev.RetryCount >= p.config.MaxRetries {
			log.Printf("[OUTBOX] event %s exceeded max retries (%d), dead-lettering", ev.ID, ev.RetryCount)
			p.outbox.MarkFailed(ctx, ev.ID, "max retries exceeded")
			telemetry.OutboxEventsFailed.Inc()
			continue
		}
		live = append(live, ev)
	}
	events = live

	// Group by event_type so we can apply batched Cypher per group.
	groups := make(map[string][]Event, 4)
	for _, ev := range events {
		groups[ev.EventType] = append(groups[ev.EventType], ev)
	}

	successCount := 0
	failCount := 0

	// Order types so identity.created (high cardinality on bulk onboarding) is
	// processed first. Keeps log output stable for human readers.
	typeOrder := []string{
		"identity.created",
		"role.assigned",
		"entitlement.provisioned",
		"identity.updated",
		"identity.deleted",
		"role.revoked",
		"entitlement.revoked",
	}
	seen := make(map[string]bool)
	for _, t := range typeOrder {
		seen[t] = true
		if g, ok := groups[t]; ok && len(g) >= p.batchThreshold() {
			s, f := p.applyBatched(ctx, t, g)
			successCount += s
			failCount += f
			delete(groups, t)
		}
	}
	// Anything remaining (small groups, unknown types) → per-event path.
	for t, g := range groups {
		for _, ev := range g {
			if !seen[t] && t != "" {
				// preserve original "unknown type just log+skip" behaviour
			}
			eventStart := time.Now()
			if err := p.applyToNeo4j(ctx, ev); err != nil {
				log.Printf("[OUTBOX] event %s failed (retry %d/%d): %v",
					ev.ID, ev.RetryCount+1, p.config.MaxRetries, err)
				p.outbox.MarkFailed(ctx, ev.ID, err.Error())
				failCount++
				telemetry.OutboxEventsFailed.Inc()
				continue
			}
			p.outbox.MarkProcessed(ctx, ev.ID)
			successCount++
			telemetry.OutboxEventsProcessed.Inc()
			telemetry.OutboxProcessingLatency.
				WithLabelValues(ev.EventType).
				Observe(float64(time.Since(eventStart).Milliseconds()))
			p.publishToNATS(ctx, ev)
		}
	}

	// Update queue size metric
	stats, _ := p.outbox.Stats(ctx)
	telemetry.OutboxQueueSize.Set(float64(stats["pending"]))

	log.Printf("[OUTBOX] batch complete: %d success, %d failed in %v",
		successCount, failCount, time.Since(startTime))
}

// batchThreshold is the minimum group size for the UNWIND fast path.
// Below this, per-event Cypher is cheaper than the overhead of the batched query.
const batchThreshold = 5

func (p *Processor) batchThreshold() int { return batchThreshold }

// publishToNATS republishes an event onto the JetStream as a secondary
// notification (downstream risk engine, audit, etc.). Non-blocking.
func (p *Processor) publishToNATS(ctx context.Context, event Event) {
	if p.natsBus == nil {
		return
	}
	natsEvent := eventbus.Event{
		ID:          event.ID,
		EventType:   event.EventType,
		AggregateID: event.AggregateID,
		Payload:     event.Payload,
		Timestamp:  event.CreatedAt,
	}
	_ = p.natsBus.Publish(ctx, natsEvent)
}

// applyBatched dispatches a homogeneous group of events to the per-type
// batched Neo4j handler. For event types without a batched implementation
// it falls back to per-event processing.
func (p *Processor) applyBatched(ctx context.Context, eventType string, group []Event) (success, fail int) {
	session := p.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	cctx, cancel := context.WithTimeout(ctx, p.config.Neo4jTimeout)
	defer cancel()

	var batchErr error
	switch eventType {
	case "identity.created":
		batchErr = p.handleIdentityCreatedBatch(cctx, session, group)
	default:
		// No batched impl for this type — fall back to per-event.
		for _, ev := range group {
			if err := p.applyToNeo4j(ctx, ev); err != nil {
				log.Printf("[OUTBOX] event %s failed (retry %d/%d): %v",
					ev.ID, ev.RetryCount+1, p.config.MaxRetries, err)
				p.outbox.MarkFailed(ctx, ev.ID, err.Error())
				fail++
				telemetry.OutboxEventsFailed.Inc()
				continue
			}
			p.outbox.MarkProcessed(ctx, ev.ID)
			success++
			telemetry.OutboxEventsProcessed.Inc()
			p.publishToNATS(ctx, ev)
		}
		return success, fail
	}

	if batchErr != nil {
		log.Printf("[OUTBOX] batched %s failed (%d events), falling back to per-event: %v",
			eventType, len(group), batchErr)
		// Fall back to per-event so individual failing events can be marked.
		for _, ev := range group {
			if err := p.applyToNeo4j(ctx, ev); err != nil {
				log.Printf("[OUTBOX] event %s failed (retry %d/%d): %v",
					ev.ID, ev.RetryCount+1, p.config.MaxRetries, err)
				p.outbox.MarkFailed(ctx, ev.ID, err.Error())
				fail++
				telemetry.OutboxEventsFailed.Inc()
				continue
			}
			p.outbox.MarkProcessed(ctx, ev.ID)
			success++
			telemetry.OutboxEventsProcessed.Inc()
			p.publishToNATS(ctx, ev)
		}
		return success, fail
	}

	// Whole batch succeeded.
	for _, ev := range group {
		p.outbox.MarkProcessed(ctx, ev.ID)
		success++
		telemetry.OutboxEventsProcessed.Inc()
		p.publishToNATS(ctx, ev)
	}
	log.Printf("[OUTBOX] batched %d %s events in one Cypher (UNWIND)", len(group), eventType)
	return success, fail
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
		"id":           id,
		"tenant_id":    getStr(payload, "tenant_id"),
		"email":        getStr(payload, "email"),
		"display_name": getStr(payload, "display_name"),
		"status":       getStr(payload, "status"),
		"type":         getStr(payload, "type"),
		"source":       getStr(payload, "source"),
		"department":   getStr(payload, "department"),
		"employee_id":  getStr(payload, "employee_id"),
	})
	if err != nil {
		return fmt.Errorf("identity.created: %w", err)
	}
	return nil
}

// handleIdentityCreatedBatch MERGEs N identity nodes in a single Cypher
// (UNWIND over $rows), collapsing N round-trips into one. On success, all
// identities in the batch are applied atomically. On failure, the caller
// falls back to per-event applyToNeo4j so individual failing events can be
// marked failed precisely without losing the successes.
//
// Each row of $rows is an {id, payload} map. payload carries tenant_id,
// email, display_name, status, type, source, department, employee_id.
func (p *Processor) handleIdentityCreatedBatch(ctx context.Context, session neo4j.SessionWithContext, events []Event) error {
	rows := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		var payload map[string]any
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			// Skip malformed — per-event fallback will pick it up and MarkFailed.
			continue
		}
		rows = append(rows, map[string]any{
			"id":          ev.AggregateID,
			"tenant_id":   getStr(payload, "tenant_id"),
			"email":       getStr(payload, "email"),
			"display_name": getStr(payload, "display_name"),
			"status":      coalesceStr(getStr(payload, "status"), "active"),
			"type":        coalesceStr(getStr(payload, "type"), "human"),
			"source":      coalesceStr(getStr(payload, "source"), "manual"),
			"department":  getStr(payload, "department"),
			"employee_id": getStr(payload, "employee_id"),
		})
	}
	if len(rows) == 0 {
		return fmt.Errorf("identity.created.batch: no well-formed rows")
	}
	_, err := session.Run(ctx, `
		UNWIND $rows AS row
		MERGE (i:Identity {uuid: row.id})
		SET i.tenant_id    = row.tenant_id,
		    i.email        = row.email,
		    i.display_name = row.display_name,
		    i.status       = row.status,
		    i.type         = row.type,
		    i.source       = row.source,
		    i.department   = row.department,
		    i.employee_id  = row.employee_id,
		    i.created_at   = datetime()
	`, map[string]any{"rows": rows})
	if err != nil {
		return fmt.Errorf("identity.created.batch: %w", err)
	}
	return nil
}

// handleIdentityUpdated updates an identity node in Neo4j.
//
// Uses MERGE+SET (upsert semantics) so that "reinstated" identities — whose
// node was DETACH-DELETED by an earlier identity.deleted — get re-created
// the next time they reappear in an identity.updated event (e.g. HR sync
// flips status from terminated back to active).
func (p *Processor) handleIdentityUpdated(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
	_, err := session.Run(ctx, `
		MERGE (i:Identity {uuid: $id})
		SET i.tenant_id    = COALESCE($tenant_id, i.tenant_id),
		    i.email        = COALESCE(NULLIF($email,''), i.email),
		    i.display_name = COALESCE(NULLIF($display_name,''), i.display_name),
		    i.status       = COALESCE(NULLIF($status,''), i.status),
		    i.department   = COALESCE(NULLIF($department,''), i.department),
		    i.employee_id  = COALESCE(NULLIF($employee_id,''), i.employee_id),
		    i.type         = COALESCE(i.type, 'human'),
		    i.source       = COALESCE(i.source, 'hris'),
		    i.risk_score   = COALESCE(i.risk_score, 0.0),
		    i.updated_at   = datetime(),
		    i.created_at   = COALESCE(i.created_at, datetime())
	`, map[string]any{
		"id":           id,
		"tenant_id":    getStr(payload, "tenant_id"),
		"email":        getStr(payload, "email"),
		"display_name": getStr(payload, "display_name"),
		"status":       getStr(payload, "status"),
		"department":   getStr(payload, "department"),
		"employee_id":  getStr(payload, "employee_id"),
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

// coalesceStr returns the first non-empty argument, falling back to the default.
// Used by the batched identity.created handler to set platform defaults inside
// the UNWIND Cypher (COALESCE in Cypher handles null, not empty strings).
func coalesceStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	if n := len(vals); n > 0 {
		return vals[n-1]
	}
	return ""
}
