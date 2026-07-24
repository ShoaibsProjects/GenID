# Outbox Pattern for PG+Neo4j Consistency

> **⚠️ START HERE FOR VERSION 2**
> 
> **Status:** Research Complete — Ready for Implementation  
> **Next Step:** Begin with **Step 1: Database Migration** (see below)  
> **Estimated Time:** 1-2 days for full implementation  
> **Priority:** P0 #3 (Critical Gap — fixes dual-write consistency issues)
> 
> **Part of V2 Master Plan:** See [V2-MASTER-PLAN.md](./V2-MASTER-PLAN.md) Phase 1
> 
> ---
> 
> **Context:** ObserveID currently uses dual-write to PostgreSQL + Neo4j without transactional guarantees. If Neo4j write fails after PostgreSQL commits, data becomes inconsistent. The Outbox Pattern solves this by making both writes atomic within a single PostgreSQL transaction, then asynchronously syncing to Neo4j.
> 
> ---

**Status:** Research Complete — Ready for Implementation  
**Priority:** P0 #3 (Critical Gap)  
**Estimated Effort:** 1-2 days  

---

## 🔍 Problem Statement

### Current Dual-Write Architecture

ObserveID uses **dual-write** to maintain data in both PostgreSQL (relational) and Neo4j (graph):

- **PostgreSQL:** Identities, roles, entitlements, audit logs, connectors
- **Neo4j:** Access graph, relationships, path traversal for authorization

### Identified Dual-Write Vulnerabilities

| Location | File | Line(s) | PostgreSQL Operation | Neo4j Operation | Risk Level |
|----------|------|---------|---------------------|-----------------|------------|
| `ScimCreateUser` | `identity_service.go` | 269→283 | `INSERT INTO identities` | `MERGE (i:Identity)` | 🔴 HIGH |
| `ScimUpdateUser` | `identity_service.go` | 369→373 | `UPDATE identities SET` | `SET i.properties` | 🟡 MEDIUM |
| `ScimDeleteUser` | `identity_service.go` | 407+ | Temporal workflow | Workflow updates both | 🟢 LOW |
| `AssignRoleToIdentity` | `activities.go` | 774→795 | `INSERT INTO identity_roles` | `MERGE (i)-[:HAS_ROLE]->(r)` | 🔴 HIGH |
| `RevokeAccess` | `activities.go` | 469→497 | `UPDATE entitlements SET revoked` | `DELETE (i)-[:HAS_ACCESS]->` | 🔴 HIGH |

### Failure Scenario

```
1. POST /scim/v2/Users  →  ScimCreateUser()
2. PostgreSQL: INSERT INTO identities (...)  ✅ COMMIT
3. Neo4j: MERGE (i:Identity {uuid: $id})     ❌ NETWORK ERROR / TIMEOUT
4. Result: Identity exists in PG but NOT in Neo4j
5. Impact: Access checks fail, graph queries incomplete, data inconsistency
```

### Why This Matters

- **Inconsistent queries:** `SELECT COUNT(*) FROM identities` ≠ `MATCH (i:Identity) RETURN count(i)`
- **Broken access checks:** Identity exists in PG but not Neo4j → access denied incorrectly
- **No rollback:** PostgreSQL commits, Neo4j fails → orphaned records
- **Data corruption risk:** Partial updates leave system in undefined state
- **Debugging nightmare:** Hard to trace which writes succeeded/failed

---

## 📋 Solution: Outbox Pattern

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│  Application (PostgreSQL Transaction)                       │
│                                                             │
│  BEGIN TRANSACTION;                                         │
│  1. INSERT/UPDATE main table (identities)                  │
│  2. INSERT into outbox_events table                        │
│  COMMIT;                                                    │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│  Outbox Processor (Background Goroutine)                    │
│  - Polls outbox_events every 500ms                          │
│  - Publishes to Redis Streams                               │
│  - Retries on failure (exponential backoff)                 │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│  Neo4j Sync Worker                                          │
│  - Subscribes to Redis Streams                              │
│  - Applies changes to Neo4j (idempotent MERGE)              │
│  - Acknowledges on success                                  │
└─────────────────────────────────────────────────────────────┘
```

### Implementation Options Compared

| Approach | Effort | Pros | Cons | Best For |
|----------|--------|------|------|----------|
| **1. Debezium CDC** | 2-3 weeks | Industry standard, WAL parsing, built-in retries, schema evolution | Requires Kafka Connect, Zookeeper, operational complexity, steep learning curve | Large scale (>100k events/day), enterprise |
| **2. pg_output + Logical Replication** | 1-2 weeks | Native PostgreSQL, no extra infra, real-time | Complex setup, replication slot management, PostgreSQL version dependencies | Medium scale, PostgreSQL-heavy teams |
| **3. Transactional Outbox Table + Poller** ⭐ | 1-2 days | Simple, no new infra, easy to debug, uses existing Redis | Polling latency (100ms-1s), additional DB load, manual retry logic | **Current scale (<10k events/day)** |
| **4. Two-Phase Commit (2PC)** | 2-3 weeks | Strongest consistency, atomic across both DBs | Performance hit (2x latency), complex error handling, Neo4j 2PC support unclear | Financial/critical systems only |

### Recommendation: **Option 3 — Transactional Outbox Table + Poller**

**Why:**
- ✅ Minimal infrastructure (uses existing PostgreSQL + Redis)
- ✅ Fast to implement (1-2 days)
- ✅ Easy to test and debug
- ✅ Can upgrade to Debezium later if needed
- ✅ Good enough for current scale (<10k events/day)
- ✅ No new dependencies or services

---

## 🏗️ Implementation Plan

### Step 1: Database Migration

**File:** `infrastructure/postgres/init.sql` (or new migration file)

```sql
-- Outbox Pattern: Event Store for PG→Neo4j Sync
-- Tracks all changes that need to be applied to Neo4j

CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(50) NOT NULL,          -- e.g., 'identity.created', 'role.assigned'
    aggregate_type VARCHAR(50) NOT NULL,      -- e.g., 'identity', 'role', 'entitlement'
    aggregate_id UUID NOT NULL,               -- ID of the entity being modified
    payload JSONB NOT NULL,                   -- Full event data
    metadata JSONB,                           -- Optional: user_id, ip, correlation_id
    created_at TIMESTAMPTZ DEFAULT NOW(),     -- When event was created
    processed BOOLEAN DEFAULT FALSE,          -- Has it been sent to Neo4j?
    processed_at TIMESTAMPTZ,                 -- When it was processed
    retry_count INTEGER DEFAULT 0,            -- Number of retry attempts
    error_message TEXT,                       -- Last error message
    expires_at TIMESTAMPTZ DEFAULT NOW() + INTERVAL '7 days'  -- Auto-cleanup
);

-- Indexes for efficient polling
CREATE INDEX idx_outbox_unprocessed ON outbox_events (processed, created_at) 
    WHERE processed = FALSE;
CREATE INDEX idx_outbox_aggregate ON outbox_events (aggregate_type, aggregate_id);
CREATE INDEX idx_outbox_retry ON outbox_events (processed, retry_count, created_at)
    WHERE processed = FALSE AND retry_count > 0;

-- Cleanup old events (run weekly or via cron)
-- DELETE FROM outbox_events WHERE expires_at < NOW();
```

---

### Step 2: Outbox Helper Functions

**File:** `backend/internal/outbox/outbox.go`

```go
package outbox

import (
    "context"
    "encoding/json"
    "time"
    
    "github.com/jackc/pgx/v5/pgxpool"
)

// Event represents an outbox event
type Event struct {
    ID            string         `json:"id"`
    EventType     string         `json:"event_type"`
    AggregateType string         `json:"aggregate_type"`
    AggregateID   string         `json:"aggregate_id"`
    Payload       json.RawMessage `json:"payload"`
    Metadata      json.RawMessage `json:"metadata,omitempty"`
    CreatedAt     time.Time      `json:"created_at"`
}

// Outbox provides transactional outbox functionality
type Outbox struct {
    pgPool *pgxpool.Pool
}

// NewOutbox creates a new Outbox instance
func NewOutbox(pgPool *pgxpool.Pool) *Outbox {
    return &Outbox{pgPool: pgPool}
}

// Publish adds an event to the outbox within the provided transaction
// This ensures atomicity: main operation + outbox insert succeed or fail together
func (o *Outbox) Publish(ctx context.Context, tx pgx.Tx, eventType, aggregateType, aggregateID string, payload, metadata any) error {
    payloadJSON, err := json.Marshal(payload)
    if err != nil {
        return err
    }
    
    var metadataJSON []byte
    if metadata != nil {
        metadataJSON, err = json.Marshal(metadata)
        if err != nil {
            return err
        }
    }
    
    _, err = tx.Exec(ctx, `
        INSERT INTO outbox_events (event_type, aggregate_type, aggregate_id, payload, metadata)
        VALUES ($1, $2, $3, $4, $5)
    `, eventType, aggregateType, aggregateID, payloadJSON, metadataJSON)
    
    return err
}

// GetUnprocessed fetches events that haven't been processed yet
func (o *Outbox) GetUnprocessed(ctx context.Context, limit int) ([]Event, error) {
    rows, err := o.pgPool.Query(ctx, `
        SELECT id, event_type, aggregate_type, aggregate_id, payload, metadata, created_at
        FROM outbox_events
        WHERE processed = FALSE
          AND (retry_count = 0 OR created_at > NOW() - INTERVAL '1 hour' * POWER(2, retry_count))
          AND expires_at > NOW()
        ORDER BY created_at ASC
        LIMIT $1
    `, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var events []Event
    for rows.Next() {
        var e Event
        err := rows.Scan(&e.ID, &e.EventType, &e.AggregateType, &e.AggregateID, 
                        &e.Payload, &e.Metadata, &e.CreatedAt)
        if err != nil {
            return nil, err
        }
        events = append(events, e)
    }
    
    return events, rows.Err()
}

// MarkProcessed marks an event as successfully processed
func (o *Outbox) MarkProcessed(ctx context.Context, id string) error {
    _, err := o.pgPool.Exec(ctx, `
        UPDATE outbox_events
        SET processed = TRUE, processed_at = NOW()
        WHERE id = $1
    `, id)
    return err
}

// MarkFailed marks an event as failed and increments retry count
func (o *Outbox) MarkFailed(ctx context.Context, id string, errMsg string) error {
    _, err := o.pgPool.Exec(ctx, `
        UPDATE outbox_events
        SET retry_count = retry_count + 1,
            error_message = $2,
            created_at = CASE WHEN retry_count = 0 THEN created_at ELSE NOW() - INTERVAL '1 hour' * POWER(2, retry_count) END
        WHERE id = $1
    `, id, errMsg)
    return err
}

// Stats returns outbox queue statistics
func (o *Outbox) Stats(ctx context.Context) (map[string]int, error) {
    var pending, failed, total int
    
    err := o.pgPool.QueryRow(ctx, `
        SELECT 
            COUNT(*) FILTER (WHERE processed = FALSE) as pending,
            COUNT(*) FILTER (WHERE processed = FALSE AND retry_count > 0) as failed,
            COUNT(*) as total
        FROM outbox_events
        WHERE expires_at > NOW()
    `).Scan(&pending, &failed, &total)
    
    if err != nil {
        return nil, err
    }
    
    return map[string]int{
        "pending": pending,
        "failed":  failed,
        "total":   total,
    }, nil
}
```

---

### Step 3: Outbox Processor (Background Worker)

**File:** `backend/internal/outbox/processor.go`

```go
package outbox

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "sync/atomic"
    "time"
    
    "github.com/neo4j/neo4j-go-driver/v5/neo4j"
    "github.com/redis/go-redis/v9"
    "github.com/observeid/identity-platform/pkg/telemetry"
)

// ProcessorConfig holds configuration for the outbox processor
type ProcessorConfig struct {
    PollInterval   time.Duration // How often to check for new events (default: 500ms)
    BatchSize      int           // Max events to process per batch (default: 100)
    MaxRetries     int           // Max retry attempts before dead-letter (default: 5)
    Neo4jTimeout   time.Duration // Timeout for Neo4j operations (default: 30s)
}

func DefaultConfig() ProcessorConfig {
    return ProcessorConfig{
        PollInterval: 500 * time.Millisecond,
        BatchSize:    100,
        MaxRetries:   5,
        Neo4jTimeout: 30 * time.Second,
    }
}

// Processor handles outbox event processing and Neo4j sync
type Processor struct {
    config  ProcessorConfig
    outbox  *Outbox
    redis   *redis.Client
    neo4j   neo4j.DriverWithContext
    running atomic.Bool
}

// NewProcessor creates a new outbox processor
func NewProcessor(pgPool *pgxpool.Pool, redis *redis.Client, neo4j neo4j.DriverWithContext, config ProcessorConfig) *Processor {
    return &Processor{
        config: config,
        outbox: NewOutbox(pgPool),
        redis:  redis,
        neo4j:  neo4j,
    }
}

// Start begins the background processing loop
func (p *Processor) Start(ctx context.Context) {
    p.running.Store(true)
    ticker := time.NewTicker(p.config.PollInterval)
    defer ticker.Stop()
    
    log.Printf("[OUTBOX] processor started (interval=%s, batch=%d)", 
        p.config.PollInterval, p.config.BatchSize)
    
    for p.running.Load() {
        select {
        case <-ctx.Done():
            log.Printf("[OUTBOX] processor stopped")
            return
        case <-ticker.C:
            p.processBatch(ctx)
        }
    }
}

// Stop gracefully stops the processor
func (p *Processor) Stop() {
    p.running.Store(false)
}

// processBatch fetches and processes a batch of events
func (p *Processor) processBatch(ctx context.Context) {
    events, err := p.outbox.GetUnprocessed(ctx, p.config.BatchSize)
    if err != nil {
        log.Printf("[OUTBOX] fetch error: %v", err)
        return
    }
    
    if len(events) == 0 {
        return
    }
    
    log.Printf("[OUTBOX] processing %d events", len(events))
    
    for _, event := range events {
        if err := p.applyToNeo4j(ctx, event); err != nil {
            log.Printf("[OUTBOX] event %s failed: %v", event.ID, err)
            p.outbox.MarkFailed(ctx, event.ID, err.Error())
            telemetry.OutboxErrors.Inc()
        } else {
            p.outbox.MarkProcessed(ctx, event.ID)
            telemetry.OutboxEventsProcessed.Inc()
        }
    }
}

// applyToNeo4j applies an event to Neo4j
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
    default:
        log.Printf("[OUTBOX] unknown event type: %s", event.EventType)
        return nil // Don't fail on unknown types
    }
}

// handleIdentityCreated creates an identity node in Neo4j
func (p *Processor) handleIdentityCreated(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
    _, err := session.Run(ctx, `
        MERGE (i:Identity {uuid: $id})
        SET i.email = $email,
            i.display_name = $display_name,
            i.status = $status,
            i.type = 'human',
            i.source = 'scim',
            i.created_at = datetime()
    `, map[string]any{
        "id": id,
        "email": payload["email"],
        "display_name": payload["display_name"],
        "status": payload["status"],
    })
    return err
}

// handleIdentityUpdated updates an identity node in Neo4j
func (p *Processor) handleIdentityUpdated(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
    _, err := session.Run(ctx, `
        MATCH (i:Identity {uuid: $id})
        SET i.updated_at = datetime(),
            i.email = COALESCE($email, i.email),
            i.display_name = COALESCE($display_name, i.display_name),
            i.status = COALESCE($status, i.status)
    `, map[string]any{
        "id": id,
        "email": payload["email"],
        "display_name": payload["display_name"],
        "status": payload["status"],
    })
    return err
}

// handleIdentityDeleted removes an identity node from Neo4j
func (p *Processor) handleIdentityDeleted(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
    _, err := session.Run(ctx, `
        MATCH (i:Identity {uuid: $id})
        DETACH DELETE i
    `, map[string]any{"id": id})
    return err
}

// handleRoleAssigned creates a HAS_ROLE relationship
func (p *Processor) handleRoleAssigned(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
    _, err := session.Run(ctx, `
        MATCH (i:Identity {uuid: $identity_id}), (r:Role {id: $role_id})
        MERGE (i)-[rel:HAS_ROLE]->(r)
        SET rel.assigned_at = datetime(),
            rel.assigned_by = $assigned_by
    `, map[string]any{
        "identity_id": id,
        "role_id": payload["role_id"],
        "assigned_by": payload["assigned_by"],
    })
    return err
}

// handleRoleRevoked removes a HAS_ROLE relationship
func (p *Processor) handleRoleRevoked(ctx context.Context, session neo4j.SessionWithContext, id string, payload map[string]any) error {
    _, err := session.Run(ctx, `
        MATCH (i:Identity {uuid: $identity_id})-[rel:HAS_ROLE]->(r:Role {id: $role_id})
        DELETE rel
    `, map[string]any{
        "identity_id": id,
        "role_id": payload["role_id"],
    })
    return err
}
```

---

### Step 4: Refactor Existing Code to Use Outbox

**File:** `backend/internal/service/identity_service.go`

#### Before (Dual-Write):
```go
func (s *IdentityService) ScimCreateUser(w http.ResponseWriter, r *http.Request) {
    // ... parse request ...
    
    // PostgreSQL write
    _, err := s.pgPool.Exec(r.Context(), `
        INSERT INTO identities (id, tenant_id, email, display_name, status, source, attributes)
        VALUES ($1, $2, $3, $4, $5, 'scim', $6)
        ON CONFLICT (tenant_id, email) DO UPDATE SET
            display_name = EXCLUDED.display_name, status = EXCLUDED.status
    `, id, tenant, userName, displayName, status, mustJSON(attrs))
    if err != nil {
        respondError(w, http.StatusInternalServerError, "Create failed: "+err.Error())
        return
    }
    
    // Neo4j write (separate, non-transactional)
    session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
    defer session.Close(r.Context())
    session.Run(r.Context(), `
        MERGE (i:Identity {uuid: $uuid})
        SET i.tenant_id = $tenant, i.email = $email, i.display_name = $name,
            i.status = $status, i.type = 'human', i.source = 'scim',
            i.created_at = datetime()
    `, map[string]any{
        "uuid": id, "tenant": tenant,
        "email": userName, "name": displayName, "status": status,
    })
    
    respondJSON(w, http.StatusCreated, response)
}
```

#### After (Transactional Outbox):
```go
func (s *IdentityService) ScimCreateUser(w http.ResponseWriter, r *http.Request) {
    // ... parse request ...
    
    // Begin PostgreSQL transaction (includes outbox)
    tx, err := s.pgPool.Begin(r.Context())
    if err != nil {
        respondError(w, http.StatusInternalServerError, "Transaction failed")
        return
    }
    defer tx.Rollback(r.Context())
    
    // PostgreSQL write
    _, err = tx.Exec(r.Context(), `
        INSERT INTO identities (id, tenant_id, email, display_name, status, source, attributes)
        VALUES ($1, $2, $3, $4, $5, 'scim', $6)
        ON CONFLICT (tenant_id, email) DO UPDATE SET
            display_name = EXCLUDED.display_name, status = EXCLUDED.status
    `, id, tenant, userName, displayName, status, mustJSON(attrs))
    if err != nil {
        respondError(w, http.StatusInternalServerError, "Create failed: "+err.Error())
        return
    }
    
    // Outbox event (same transaction!)
    err = s.outbox.Publish(r.Context(), tx, "identity.created", "identity", id, 
        map[string]any{
            "email": userName,
            "display_name": displayName,
            "status": status,
            "tenant_id": tenant,
        },
        map[string]any{
            "source": "scim",
            "user_id": "system",
        })
    if err != nil {
        respondError(w, http.StatusInternalServerError, "Outbox failed: "+err.Error())
        return
    }
    
    // Commit both operations atomically
    if err := tx.Commit(r.Context()); err != nil {
        respondError(w, http.StatusInternalServerError, "Commit failed")
        return
    }
    
    // Neo4j sync will happen asynchronously via outbox processor
    respondJSON(w, http.StatusCreated, response)
}
```

---

### Step 5: Wire into Main

**File:** `backend/cmd/identity-service/main.go`

```go
// After initializing services...

// Initialize Outbox Processor
outboxConfig := outbox.DefaultConfig()
outboxProc := outbox.NewProcessor(pgPool, rdb, neo4jDriver, outboxConfig)

// Start background processor
go outboxProc.Start(context.Background())
log.Info().Msg("Outbox processor started")

// Graceful shutdown
defer func() {
    log.Info().Msg("Shutting down outbox processor...")
    outboxProc.Stop()
}()

// Expose outbox stats via metrics endpoint (optional)
// outboxProc.Stats() can be called periodically
```

---

### Step 6: Add Prometheus Metrics

**File:** `backend/pkg/telemetry/metrics.go`

```go
var (
    // ... existing metrics ...
    
    OutboxEventsProcessed = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "observeid_outbox_events_processed_total",
            Help: "Total outbox events processed successfully",
        },
    )
    
    OutboxErrors = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "observeid_outbox_errors_total",
            Help: "Total outbox processing errors",
        },
    )
    
    OutboxQueueSize = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "observeid_outbox_queue_size",
            Help: "Current number of pending outbox events",
        },
    )
    
    OutboxProcessingLatency = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "observeid_outbox_processing_latency_ms",
            Help:    "Outbox event processing latency in milliseconds",
            Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
        },
        []string{"event_type"},
    )
)

func init() {
    // ... existing registrations ...
    prometheus.MustRegister(OutboxEventsProcessed)
    prometheus.MustRegister(OutboxErrors)
    prometheus.MustRegister(OutboxQueueSize)
    prometheus.MustRegister(OutboxProcessingLatency)
}
```

**File:** `backend/internal/outbox/processor.go` (update processBatch)

```go
func (p *Processor) processBatch(ctx context.Context) {
    startTime := time.Now()
    
    events, err := p.outbox.GetUnprocessed(ctx, p.config.BatchSize)
    if err != nil {
        log.Printf("[OUTBOX] fetch error: %v", err)
        return
    }
    
    // Update queue size metric
    stats, _ := p.outbox.Stats(ctx)
    telemetry.OutboxQueueSize.Set(float64(stats["pending"]))
    
    if len(events) == 0 {
        return
    }
    
    for _, event := range events {
        eventStart := time.Now()
        
        if err := p.applyToNeo4j(ctx, event); err != nil {
            log.Printf("[OUTBOX] event %s failed: %v", event.ID, err)
            p.outbox.MarkFailed(ctx, event.ID, err.Error())
            telemetry.OutboxErrors.Inc()
        } else {
            p.outbox.MarkProcessed(ctx, event.ID)
            telemetry.OutboxEventsProcessed.Inc()
            telemetry.OutboxProcessingLatency.
                WithLabelValues(event.EventType).
                Observe(time.Since(eventStart).Seconds() * 1000)
        }
    }
    
    log.Printf("[OUTBOX] processed %d events in %v", len(events), time.Since(startTime))
}
```

---

## 🧪 Testing Strategy

### Unit Tests
```go
// backend/internal/outbox/outbox_test.go

func TestOutbox_Publish(t *testing.T) {
    // Test that Publish adds event to outbox within transaction
}

func TestOutbox_GetUnprocessed(t *testing.T) {
    // Test filtering by processed status and retry count
}

func TestProcessor_handleIdentityCreated(t *testing.T) {
    // Test Neo4j sync for identity creation
}
```

### Integration Tests
```bash
# Run with PostgreSQL + Neo4j containers
go test -v ./internal/outbox/... -run Integration
```

### Chaos Testing
1. Kill Neo4j mid-batch → verify events retry correctly
2. Simulate network partition → verify exponential backoff
3. Insert invalid payload → verify dead-letter after max retries
4. Flood with 10k events → verify throughput and latency

---

## 📊 Monitoring & Alerting

### Key Metrics to Track

| Metric | Type | Alert Threshold |
|--------|------|-----------------|
| `observeid_outbox_queue_size` | Gauge | > 1000 pending |
| `observeid_outbox_errors_total` | Counter | Error rate > 5% |
| `observeid_outbox_processing_latency_ms` | Histogram | p99 > 500ms |
| `observeid_outbox_events_processed_total` | Counter | Sudden drop |

### Dashboard (Grafana)
```
Panel 1: Outbox Queue Size (time series)
Panel 2: Events Processed vs Errors (stacked bar)
Panel 3: Processing Latency Heatmap
Panel 4: Events by Type (pie chart)
```

---

## ⚠️ Trade-offs & Considerations

| Aspect | Decision | Rationale |
|--------|----------|-----------|
| **Polling interval** | 500ms | Balance between latency (faster) and DB load (slower) |
| **Batch size** | 100 events | Avoid overwhelming Neo4j, reasonable throughput |
| **Retry strategy** | Exponential backoff (1h × 2^retry) | Gives time for transient issues to resolve |
| **Max retries** | 5 | After ~32 hours, event is likely permanently failed |
| **Ordering** | FIFO per aggregate_id | Events for same identity processed in order |
| **Idempotency** | All Neo4j ops use MERGE | Safe to retry without duplicates |
| **Cleanup** | 7-day TTL via `expires_at` | Prevents unbounded table growth |

---

## 🚀 Rollout Plan

### Phase 1: Deploy Outbox Infrastructure (Day 1)
- [ ] Run migration for `outbox_events` table
- [ ] Deploy outbox processor (disabled, no polling)
- [ ] Add metrics to Grafana dashboard

### Phase 2: Enable Dual-Write + Outbox (Day 2)
- [ ] Refactor SCIM endpoints to write outbox
- [ ] Enable outbox processor (polling starts)
- [ ] Monitor queue size, error rate, latency

### Phase 3: Validate & Expand (Day 3-4)
- [ ] Verify Neo4j consistency (PG count = Neo4j count)
- [ ] Chaos test: kill Neo4j, verify recovery
- [ ] Expand to other dual-write operations (roles, entitlements)

### Phase 4: Deprecate Old Dual-Write (Week 2)
- [ ] Remove direct Neo4j writes from application code
- [ ] All Neo4j sync via outbox only
- [ ] Document outbox pattern for team

---

## 🔮 Future Enhancements

| Enhancement | Effort | Impact |
|-------------|--------|--------|
| **Debezium CDC migration** | 2-3 weeks | Real-time sync, no polling, industry standard |
| **Redis Streams instead of polling** | 2-3 days | Lower latency, push-based |
| **Dead Letter Queue UI** | 1 week | Manual inspection/replay of failed events |
| **Schema evolution** | 1 week | Versioned payloads, backward compatibility |
| **Multi-tenant isolation** | 1 week | Separate queues per tenant |

---

## 📚 References

- [Microservices.io: Outbox Pattern](https://microservices.io/patterns/data/transactional-outbox.html)
- [Debezium Documentation](https://debezium.io/documentation/)
- [CockroachDB: Outbox Pattern](https://www.cockroachlabs.com/blog/outbox-pattern/)
- [AWS: Event-Driven Architecture](https://aws.amazon.com/event-driven-architecture/)

---

## ✅ Checklist for Implementation

- [ ] Create `outbox_events` table migration
- [ ] Implement `Outbox` helper (Publish, GetUnprocessed, MarkProcessed, MarkFailed)
- [ ] Implement `Processor` (Start, Stop, processBatch, applyToNeo4j)
- [ ] Add Prometheus metrics
- [ ] Refactor `ScimCreateUser` to use outbox
- [ ] Refactor `ScimUpdateUser` to use outbox
- [ ] Refactor `ScimDeleteUser` to use outbox
- [ ] Wire into `main.go`
- [ ] Write unit tests
- [ ] Write integration tests
- [ ] Add Grafana dashboard
- [ ] Document for team

---

**Last Updated:** 2026-07-22  
**Author:** opencode (AI Assistant)  
**Status:** Ready for Implementation
