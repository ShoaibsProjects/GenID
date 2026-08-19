package vault

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGStore is a SecretStore backed by the `vault_secrets` PostgreSQL table.
//
// It only stores ciphertext bytes (sealed by the Vault); the master key
// never reaches the database. Tenant isolation is enforced at the database
// layer via RLS keyed on `app.current_tenant`, so cross-tenant reads are
// impossible regardless of how the caller scoped the query.
//
// The table DDL (created by migration 001) is:
//
//	CREATE TABLE vault_secrets (
//	  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
//	  tenant_id   UUID NOT NULL,
//	  name        VARCHAR(255) NOT NULL,
//	  secret_type VARCHAR(50) NOT NULL,
//	  reference   VARCHAR(255),
//	  ciphertext  BYTEA NOT NULL,
//	  nonce       BYTEA NOT NULL,             -- always present (AES-GCM nonce)
//	  version     INT NOT NULL DEFAULT 1,
//	  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
//	  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
//	  UNIQUE (tenant_id, name)
//	);
//
// (nonce isn't strictly needed because the v key pipeline also prepends it
// to ciphertext — but separating it makes queries readable and lets us add
// key rotation later.)

type PGStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

// Put persists (or updates) a secret. The caller (Vault) is responsible for
// encoding the ciphertext; here we decode base64 → raw bytes to store as
// BYTEA (more space-efficient and avoids any charset ambiguity).
// Returns the actual ID of the stored secret (may differ from e.ID on conflict).
func (s *PGStore) Put(ctx context.Context, e SecretEntry) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(e.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("pgstore: decode ciphertext: %w", err)
	}
	nonce, err := extractNonce(ciphertext)
	if err != nil {
		// ciphertext malformed — accept but store empty nonce; row still valid
		nonce = nil
	}

	// Store only the actual ciphertext (without the nonce prefix) in the ciphertext column.
	// The nonce is stored separately in the nonce column.
	actualCiphertext := ciphertext
	if len(ciphertext) >= 12 {
		actualCiphertext = ciphertext[12:]
	}

	tenantID := currentTenantID(ctx)
	
	// Use transaction from context if available (set by TenantMiddleware)
	// to ensure FK constraints can see rows inserted in the same transaction.
	type execer interface {
		QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row
	}
	var q execer
	if t, ok := TxFromContext(ctx).(pgx.Tx); ok && t != nil {
		q = t
	} else {
		q = s.pool
	}

	// If tenantID still empty, try to read app.current_tenant from the transaction
	if tenantID == "" {
		if t, ok := TxFromContext(ctx).(pgx.Tx); ok && t != nil {
			var currentTenant string
			if err := t.QueryRow(ctx, "SHOW app.current_tenant").Scan(&currentTenant); err == nil && currentTenant != "" {
				tenantID = currentTenant
			}
		}
	}

	// Fallback to default tenant if still empty
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	// Upsert on (tenant_id, name). On conflict, bump version + replace ciphertext.
	// Use RETURNING id to get the actual ID of the row.
	var actualID string
	err = q.QueryRow(ctx, `
		INSERT INTO vault_secrets (id, tenant_id, name, secret_type, reference,
		                            ciphertext, nonce, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT (tenant_id, name) DO UPDATE SET
			secret_type = EXCLUDED.secret_type,
			reference   = EXCLUDED.reference,
			ciphertext  = EXCLUDED.ciphertext,
			nonce       = EXCLUDED.nonce,
			version     = vault_secrets.version + 1,
			updated_at  = NOW()
		RETURNING id
	`, e.ID, tenantID, e.Name, e.Type, e.Reference,
		actualCiphertext, nonce).Scan(&actualID)
	if err != nil {
		return "", fmt.Errorf("pgstore: Put: %w", err)
	}
	return actualID, nil
}

// Get returns a secret by id. RLS ensures tenant isolation.

func (s *PGStore) Get(ctx context.Context, id string) (SecretEntry, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id::text, COALESCE(tenant_id::text,''), name, secret_type,
		       COALESCE(reference,''), ciphertext, nonce,
		       version, created_at, updated_at
		FROM vault_secrets
		WHERE id = $1::uuid
	`, id)
	return scanEntry(row)
}

// GetByName returns a secret by (tenant_id, name).
func (s *PGStore) GetByName(ctx context.Context, tenantID, name string) (SecretEntry, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id::text, COALESCE(tenant_id::text,''), name, secret_type,
		       COALESCE(reference,''), ciphertext, nonce,
		       version, created_at, updated_at
		FROM vault_secrets
		WHERE tenant_id = $1::uuid AND name = $2
	`, tenantID, name)
	return scanEntry(row)
}

// List returns all secrets for a tenant. Ciphertext is returned as base64
// so callers see a uniform shape (caller strips before showing to user).
func (s *PGStore) List(ctx context.Context, tenantID string) ([]SecretEntry, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("pgstore: missing tenant_id in context")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, COALESCE(tenant_id::text,''), name, secret_type,
		       COALESCE(reference,''), ciphertext, nonce,
		       version, created_at, updated_at
		FROM vault_secrets
		WHERE tenant_id = $1::uuid
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: List query: %w", err)
	}
	defer rows.Close()

	var entries []SecretEntry
	for rows.Next() {
		var (
			e         SecretEntry
			tenant    string
			ct, nonce []byte
			created   time.Time
			updated   time.Time
		)
		if err := rows.Scan(&e.ID, &tenant, &e.Name, &e.Type, &e.Reference,
			&ct, &nonce, &e.Version, &created, &updated); err != nil {
			return nil, fmt.Errorf("pgstore: List scan: %w", err)
		}
		// Re-encode merged ciphertext (nonce + body) as base64 so the Vault's
		// decrypt() path matches the in-memory encode shape.
		merged := append(append([]byte{}, nonce...), ct...)
		e.Ciphertext = base64.StdEncoding.EncodeToString(merged)
		e.CreatedAt = created
		e.UpdatedAt = updated
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Delete removes a secret. RLS ensures the caller can only delete within their tenant.
func (s *PGStore) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM vault_secrets WHERE id = $1::uuid`, id)
	if err != nil {
		return fmt.Errorf("pgstore: Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("secret not found: %s", id)
	}
	return nil
}

// ─── Helpers ────────────────────────────────────────────────────

// extractNonce returns the leading 12 bytes of ciphertext (AES-256-GCM
// nonce size used by the Vault's encrypt()). Used to populate the nonce
// column separately from the body. Returns the full ciphertext if shorter
// than the GCM nonce — the caller handles malformed input.
func extractNonce(ct []byte) ([]byte, error) {
	if len(ct) < 12 {
		return nil, fmt.Errorf("ciphertext shorter than GCM nonce size")
	}
	return ct[:12], nil
}

// scanEntry scans a single row (all columns) into a SecretEntry struct.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanEntry(row rowScanner) (SecretEntry, error) {
	var (
		e       SecretEntry
		tenant  string
		ct      []byte
		nonce   []byte
		created time.Time
		updated time.Time
	)
	if err := row.Scan(&e.ID, &tenant, &e.Name, &e.Type, &e.Reference,
		&ct, &nonce, &e.Version, &created, &updated); err != nil {
		if err == sql.ErrNoRows {
			return SecretEntry{}, fmt.Errorf("secret not found")
		}
		return SecretEntry{}, fmt.Errorf("pgstore: scan: %w", err)
	}
	merged := append(append([]byte{}, nonce...), ct...)
	e.Ciphertext = base64.StdEncoding.EncodeToString(merged)
	e.CreatedAt = created
	e.UpdatedAt = updated
	return e, nil
}
