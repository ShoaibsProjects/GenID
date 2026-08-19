package vault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ─── Credential Vault ────────────────────────────────────────
// Encrypted secret storage for connector credentials and sensitive config.
// Uses AES-256-GCM with a master key derived from an environment variable.
//
// Storage backends are pluggable via the SecretStore interface:
//   - In-memory + file persistence (default): legacy behaviour, master key
//     in env, ciphertext base64-encoded in a JSON file.
//   - Postgres-backed (PGStore): secrets live in the `vault_secrets` table
//     with tenant isolation (RLS). Used in production so secrets survive
//     container restarts and are shared across replicas.
//
// The Vault always performs the AES-256-GCM seal/open itself; stores handle
// only ciphertext storage. The master key never leaves process memory.

type Vault struct {
	mu        sync.RWMutex
	masterKey  []byte
	secrets    map[string]SecretEntry
	vaultPath  string // file path for persistent storage (in-memory backend)
	store      SecretStore // optional pluggable storage backend (nil = in-memory)
}

// SecretStore is the storage interface a Vault can use. Implementations
// handle only ciphertext persistence; the Vault performs all encryption.
type SecretStore interface {
	// Put persists a secret entry. Must be idempotent on (name) within a tenant.
	// Returns the actual ID of the stored row (may differ from e.ID on conflict).
	Put(ctx context.Context, e SecretEntry) (string, error)
	// Get returns a secret entry by id, including its stored ciphertext.
	Get(ctx context.Context, id string) (SecretEntry, error)
	// GetByName returns a secret entry by (tenant_id, name).
	GetByName(ctx context.Context, tenantID, name string) (SecretEntry, error)
	// List returns all secret entries for a tenant (ciphertext stripped by caller).
	List(ctx context.Context, tenantID string) ([]SecretEntry, error)
	// Delete removes a secret entry by id.
	Delete(ctx context.Context, id string) error
}

type SecretEntry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`       // connector_password, client_secret, api_key, tls_cert
	Reference string    `json:"reference"`  // connector_id or identity_id
	Ciphertext string   `json:"ciphertext"` // AES-256-GCM encrypted
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int       `json:"version"`
}

// NewVault creates a vault with a key derived from VAULT_MASTER_KEY env var.
// vaultPath is an optional file path for persistent storage. Set to "" for in-memory only.
func NewVault(masterKey string, vaultPath string) (*Vault, error) {
	if masterKey == "" {
		return nil, fmt.Errorf("vault: VAULT_MASTER_KEY is not set")
	}
	if len(masterKey) < 32 {
		return nil, fmt.Errorf("vault: VAULT_MASTER_KEY too short (%d chars, minimum 32)", len(masterKey))
	}

	key := deriveKey(masterKey)
	v := &Vault{
		masterKey: key,
		secrets:   make(map[string]SecretEntry),
		vaultPath: vaultPath,
	}
	// Auto-load from file on startup
	if vaultPath != "" {
		if data, err := os.ReadFile(vaultPath); err == nil && len(data) > 0 {
			if err := v.Import(data); err != nil {
				log.Printf("[VAULT] Failed to load from %s: %v", vaultPath, err)
			} else {
				log.Printf("[VAULT] Loaded %d secrets from %s", len(v.secrets), vaultPath)
			}
		}
	}
	return v, nil
}

// Save persists the vault to disk. Returns an error if no vaultPath was configured.
// Only applies to the in-memory backend; pluggable stores manage their own durability.
func (v *Vault) Save() error {
	if v.vaultPath == "" {
		return fmt.Errorf("vault: no vault path configured")
	}
	data, err := v.Export()
	if err != nil {
		return err
	}
	return os.WriteFile(v.vaultPath, data, 0600)
}

// WithStore attaches a pluggable storage backend. When set, Store/Retrieve/
// List/Delete delegate to the store instead of the in-memory map; the file
// export path is bypassed. Returns the receiver for chaining.
func (v *Vault) WithStore(s SecretStore) *Vault {
	v.store = s
	return v
}

// Store encrypts and stores a secret.
func (v *Vault) Store(ctx context.Context, name, secretType, reference, plaintext string) (string, error) {
	id := generateSecretID()
	ciphertext, err := v.encrypt([]byte(plaintext))
	if err != nil {
		return "", fmt.Errorf("vault: encrypt failed: %w", err)
	}

	entry := SecretEntry{
		ID:         id,
		Name:       name,
		Type:       secretType,
		Reference:  reference,
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Version:    1,
	}

	if v.store != nil {
		actualID, err := v.store.Put(ctx, entry)
		if err != nil {
			return "", fmt.Errorf("vault: store.Put failed: %w", err)
		}
		log.Printf("[VAULT] Stored secret via backend: %s (%s) for %s (id=%s)", name, secretType, reference, actualID)
		return actualID, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	v.secrets[id] = entry
	log.Printf("[VAULT] Stored secret: %s (%s) for %s", name, secretType, reference)
	return id, nil
}

// Retrieve decrypts and returns a secret.
func (v *Vault) Retrieve(ctx context.Context, id string) (string, error) {
	if v.store != nil {
		entry, err := v.store.Get(ctx, id)
		if err != nil {
			return "", fmt.Errorf("vault: %w", err)
		}
		ct, err := base64.StdEncoding.DecodeString(entry.Ciphertext)
		if err != nil {
			return "", fmt.Errorf("vault: decode failed: %w", err)
		}
		pt, err := v.decrypt(ct)
		if err != nil {
			return "", fmt.Errorf("vault: decrypt failed: %w", err)
		}
		return string(pt), nil
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	entry, ok := v.secrets[id]
	if !ok {
		return "", fmt.Errorf("vault: secret not found: %s", id)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(entry.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("vault: decode failed: %w", err)
	}
	plaintext, err := v.decrypt(ciphertext)
	if err != nil {
		return "", fmt.Errorf("vault: decrypt failed: %w", err)
	}
	return string(plaintext), nil
}

// List returns all stored secret entries (without plaintext).
func (v *Vault) List(ctx context.Context) []SecretEntry {
	if v.store != nil {
		entries, err := v.store.List(ctx, currentTenantID(ctx))
		if err != nil {
			log.Printf("[VAULT] backend List error: %v", err)
			return nil
		}
		for i := range entries {
			entries[i].Ciphertext = "[encrypted]"
		}
		return entries
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	entries := make([]SecretEntry, 0, len(v.secrets))
	for _, entry := range v.secrets {
		entry.Ciphertext = "[encrypted]"
		entries = append(entries, entry)
	}
	return entries
}

// Delete removes a secret.
func (v *Vault) Delete(ctx context.Context, id string) error {
	if v.store != nil {
		if err := v.store.Delete(ctx, id); err != nil {
			return fmt.Errorf("vault: %w", err)
		}
		return nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.secrets[id]; !ok {
		return fmt.Errorf("vault: secret not found: %s", id)
	}
	delete(v.secrets, id)
	return nil
}

// currentTenantID extracts the tenant_id from context. The audit/RLS layer
// sets it on request context via `app.current_tenant`. For non-request
// callers (background jobs), returns the default tenant.
func currentTenantID(ctx context.Context) string {
	if v, ok := ctx.Value(TenantCtxKey{}).(string); ok {
		return v
	}
	// Fallback to default tenant for background workers or if middleware didn't set it
	return "00000000-0000-0000-0000-000000000001"
}

type TenantCtxKey struct{}

type TxContextKey struct{}

// TxFromContext returns the transaction from context if present.
func TxFromContext(ctx context.Context) interface{} {
	return ctx.Value(TxContextKey{})
}

// ─── Encryption ──────────────────────────────────────────────

func (v *Vault) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(v.masterKey)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return aesGCM.Seal(nonce, nonce, plaintext, nil), nil
}

func (v *Vault) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(v.masterKey)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return aesGCM.Open(nil, nonce, ciphertext, nil)
}

func deriveKey(masterKey string) []byte {
	h := sha256.Sum256([]byte(masterKey))
	return h[:]
}

// ─── JSON Serialization ─────────────────────────────────────

func (v *Vault) Export() ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return json.Marshal(v.secrets)
}

func (v *Vault) Import(data []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return json.Unmarshal(data, &v.secrets)
}

// generateSecretID returns a UUID v4 for the secret ID.
// The vault_secrets table expects a UUID for the primary key column.
func generateSecretID() string {
	return uuid.New().String()
}
