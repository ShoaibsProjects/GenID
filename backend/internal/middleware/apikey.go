package middleware

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// APIKeyRecord is a row from the api_keys table, loaded at auth time.
type APIKeyRecord struct {
	ID        string
	Name      string
	KeyHash   string
	Scopes    []string
	TenantID  string
	ExpiresAt *time.Time
}

// ErrAPIKeyNotFound is returned when no enabled key row matches.
var ErrAPIKeyNotFound = errors.New("api key not found")

// ErrAPIKeyExpired is returned when the matched key has expired.
var ErrAPIKeyExpired = errors.New("api key expired")

// APIKeyStore resolves a key ID to its record. Implemented by PGAPIKeyStore;
// an in-memory fake can be used in tests.
type APIKeyStore interface {
	FindEnabledByID(ctx context.Context, id string) (*APIKeyRecord, error)
}

// PGAPIKeyStore reads api_keys rows from PostgreSQL (migration 007).
type PGAPIKeyStore struct {
	pool *pgxpool.Pool
}

// NewPGAPIKeyStore wires the store to the shared pool.
func NewPGAPIKeyStore(pool *pgxpool.Pool) *PGAPIKeyStore {
	return &PGAPIKeyStore{pool: pool}
}

// FindEnabledByID returns a non-expired, enabled key row by its public ID.
func (s *PGAPIKeyStore) FindEnabledByID(ctx context.Context, id string) (*APIKeyRecord, error) {
	var rec APIKeyRecord
	var scopes []string
	var expiresAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, name, key_hash, scopes, tenant_id::text, expires_at
		FROM api_keys
		WHERE id = $1 AND enabled = true
	`, id).Scan(&rec.ID, &rec.Name, &rec.KeyHash, &scopes, &rec.TenantID, &expiresAt)
	if err != nil {
		return nil, ErrAPIKeyNotFound
	}
	rec.Scopes = scopes
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return nil, ErrAPIKeyExpired
	}
	rec.ExpiresAt = expiresAt
	if rec.TenantID == "" {
		rec.TenantID = "00000000-0000-0000-0000-000000000001"
	}
	return &rec, nil
}

// VerifyAPIKey checks the presented key against a store with bcrypt.
// Keys are formatted "genid_<id>_<secret>"; only the secret is hashed in
// the table, so the public ID stays indexable and the hash stays secret-safe.
func VerifyAPIKey(ctx context.Context, store APIKeyStore, key string) (*APIKeyRecord, error) {
	parts := splitAPIKey(key)
	if len(parts) != 3 || parts[0] != "genid" || parts[1] == "" || parts[2] == "" {
		return nil, ErrAPIKeyNotFound
	}
	rec, err := store.FindEnabledByID(ctx, parts[1])
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(rec.KeyHash), []byte(parts[2])); err != nil {
		return nil, ErrAPIKeyNotFound
	}
	return rec, nil
}

func splitAPIKey(key string) []string {
	out := []string{}
	start := 0
	for i := 0; i <= len(key); i++ {
		if i == len(key) || key[i] == '_' {
			out = append(out, key[start:i])
			start = i + 1
			if len(out) == 3 {
				return out
			}
		}
	}
	return out
}
