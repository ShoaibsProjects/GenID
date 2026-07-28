package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GenesisHash is the prev_hash of the first entry in the chain.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// DefaultTenant is used when an event has no explicit tenant.
const DefaultTenant = "00000000-0000-0000-0000-000000000001"

// ChainEntry carries the fields written to a chained audit_log row.
// Details is mutable post-insert and therefore deliberately NOT hashed.
type ChainEntry struct {
	TenantID  string
	EventType string
	ActorID   string
	ActorType string
	Action    string
	Resource  string
	Details   json.RawMessage
	IPAddress string
}

// Chain is a tamper-evident audit ledger writer. A single shared instance
// serializes every audit insert through a mutex, so each row's prev_hash
// always points at the committed chain tail — no forks.
//
// Hash input (pipe-delimited, immutable fields only):
//
//	prevHash | created_at(UTC, µs, RFC3339Nano) | tenant | event_type | actor | action | resource
type Chain struct {
	pool *pgxpool.Pool
	mu   sync.Mutex
}

func NewChain(pool *pgxpool.Pool) *Chain {
	return &Chain{pool: pool}
}

// sanitizeIP converts a raw remote address ("1.2.3.4:5678") into a *string
// suitable for the INET column. Returns nil for non-IP values ("internal").
func sanitizeIP(raw string) *string {
	if raw == "" {
		return nil
	}
	host := raw
	if h, _, err := net.SplitHostPort(raw); err == nil {
		host = h
	}
	if net.ParseIP(host) == nil {
		return nil
	}
	return &host
}

// computeHash derives the SHA-256 chain hash for one entry.
func computeHash(prevHash string, ts time.Time, e ChainEntry) string {
	ts = ts.UTC().Truncate(time.Microsecond)
	payload := strings.Join([]string{
		prevHash,
		ts.Format(time.RFC3339Nano),
		e.TenantID,
		e.EventType,
		e.ActorID,
		e.Action,
		e.Resource,
	}, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// Append inserts one chained audit row and returns its hash.
// The row id is returned as well for correlation with in-memory logs.
func (c *Chain) Append(ctx context.Context, e ChainEntry) (id, hash string, err error) {
	if e.TenantID == "" {
		e.TenantID = DefaultTenant
	}
	if e.EventType == "" {
		e.EventType = "event"
	}
	if len(e.Details) == 0 {
		e.Details = json.RawMessage(`{}`)
	}
	ts := time.Now().UTC().Truncate(time.Microsecond)

	c.mu.Lock()
	defer c.mu.Unlock()

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("audit chain begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// RLS: scope this transaction to the entry's tenant (no-op for owner).
	// Parameterized via set_config — never interpolate into SQL.
	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_tenant', $1, true)`, e.TenantID); err != nil {
		return "", "", fmt.Errorf("audit chain set tenant: %w", err)
	}

	// Read the committed chain tail inside the tx.
	var prevHash string
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT hash FROM audit_log WHERE hash IS NOT NULL
			 ORDER BY created_at DESC, id DESC LIMIT 1),
			$1)`, GenesisHash).Scan(&prevHash)
	if err != nil {
		return "", "", fmt.Errorf("audit chain tail: %w", err)
	}

	hash = computeHash(prevHash, ts, e)

	err = tx.QueryRow(ctx, `
		INSERT INTO audit_log
			(tenant_id, event_type, actor_id, actor_type, action, resource,
			 details, ip_address, prev_hash, hash, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`,
		e.TenantID, e.EventType, e.ActorID, e.ActorType, e.Action, e.Resource,
		e.Details, sanitizeIP(e.IPAddress), prevHash, hash, ts,
	).Scan(&id)
	if err != nil {
		return "", "", fmt.Errorf("audit chain insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", fmt.Errorf("audit chain commit: %w", err)
	}
	return id, hash, nil
}

// VerifyResult is returned by GET /api/v1/audit/verify.
type VerifyResult struct {
	Status    string `json:"status"`             // "intact" | "tampered"
	Checked   int    `json:"checked"`            // rows replayed
	BrokenAt  string `json:"broken_at,omitempty"`
	Reason    string `json:"reason,omitempty"`
	VerifiedAt string `json:"verified_at"`
}

// Verify replays the entire chain and recomputes every hash.
func (c *Chain) Verify(ctx context.Context) (VerifyResult, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT id, tenant_id, event_type, COALESCE(actor_id, ''), action,
		       COALESCE(resource, ''), prev_hash, hash, created_at
		FROM audit_log
		WHERE hash IS NOT NULL
		ORDER BY created_at, id`)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("audit verify query: %w", err)
	}
	defer rows.Close()

	prevHash := GenesisHash
	checked := 0
	for rows.Next() {
		var id, tenantID, eventType, actorID, action, resource, storedPrev, storedHash string
		var createdAt time.Time
		if err := rows.Scan(&id, &tenantID, &eventType, &actorID, &action, &resource, &storedPrev, &storedHash, &createdAt); err != nil {
			return VerifyResult{}, fmt.Errorf("audit verify scan: %w", err)
		}
		checked++

		if storedPrev != prevHash {
			return VerifyResult{
				Status: "tampered", Checked: checked, BrokenAt: id,
				Reason:     "prev_hash does not match previous row's hash",
				VerifiedAt: time.Now().UTC().Format(time.RFC3339),
			}, nil
		}

		recomputed := computeHash(prevHash, createdAt, ChainEntry{
			TenantID: tenantID, EventType: eventType,
			ActorID: actorID, Action: action, Resource: resource,
		})
		if recomputed != storedHash {
			return VerifyResult{
				Status: "tampered", Checked: checked, BrokenAt: id,
				Reason:     "recomputed hash mismatch — row content was altered",
				VerifiedAt: time.Now().UTC().Format(time.RFC3339),
			}, nil
		}

		prevHash = storedHash
	}
	if err := rows.Err(); err != nil {
		return VerifyResult{}, fmt.Errorf("audit verify rows: %w", err)
	}

	return VerifyResult{
		Status: "intact", Checked: checked,
		VerifiedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// Backfill computes the chain for any legacy rows that predate the ledger
// (hash IS NULL), oldest first. Runs once at startup under the mutex.
func (c *Chain) Backfill(ctx context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("audit backfill begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var prevHash string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT hash FROM audit_log WHERE hash IS NOT NULL
			 ORDER BY created_at DESC, id DESC LIMIT 1),
			$1)`, GenesisHash).Scan(&prevHash); err != nil {
		return 0, fmt.Errorf("audit backfill tail: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT id, tenant_id, event_type, COALESCE(actor_id, ''), action,
		       COALESCE(resource, ''), created_at
		FROM audit_log WHERE hash IS NULL
		ORDER BY created_at, id`)
	if err != nil {
		return 0, fmt.Errorf("audit backfill query: %w", err)
	}

	type legacyRow struct {
		id, tenantID, eventType, actorID, action, resource string
		createdAt                                          time.Time
	}
	var legacy []legacyRow
	for rows.Next() {
		var r legacyRow
		if err := rows.Scan(&r.id, &r.tenantID, &r.eventType, &r.actorID, &r.action, &r.resource, &r.createdAt); err != nil {
			rows.Close()
			return 0, fmt.Errorf("audit backfill scan: %w", err)
		}
		legacy = append(legacy, r)
	}
	rows.Close()

	for _, r := range legacy {
		hash := computeHash(prevHash, r.createdAt, ChainEntry{
			TenantID: r.tenantID, EventType: r.eventType,
			ActorID: r.actorID, Action: r.action, Resource: r.resource,
		})
		if _, err := tx.Exec(ctx,
			`UPDATE audit_log SET prev_hash = $2, hash = $3 WHERE id = $1`,
			r.id, prevHash, hash); err != nil {
			return 0, fmt.Errorf("audit backfill update: %w", err)
		}
		prevHash = hash
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("audit backfill commit: %w", err)
	}
	return len(legacy), nil
}
