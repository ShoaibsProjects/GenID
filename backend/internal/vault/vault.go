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
)

// ─── Credential Vault ────────────────────────────────────────
// Encrypted secret storage for connector credentials and sensitive config.
// Uses AES-256-GCM with a master key derived from an environment variable.

type Vault struct {
	mu        sync.RWMutex
	masterKey  []byte
	secrets    map[string]SecretEntry
	vaultPath  string // file path for persistent storage
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
func NewVault(masterKey string, vaultPath string) *Vault {
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
	return v
}

// Save persists the vault to disk. Returns an error if no vaultPath was configured.
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

// Store encrypts and stores a secret.
func (v *Vault) Store(ctx context.Context, name, secretType, reference, plaintext string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	id := fmt.Sprintf("sec-%d", time.Now().UnixNano())
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

	v.secrets[id] = entry
	log.Printf("[VAULT] Stored secret: %s (%s) for %s", name, secretType, reference)
	return id, nil
}

// Retrieve decrypts and returns a secret.
func (v *Vault) Retrieve(ctx context.Context, id string) (string, error) {
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
	v.mu.RLock()
	defer v.mu.RUnlock()

	entries := make([]SecretEntry, 0, len(v.secrets))
	for _, entry := range v.secrets {
		// Strip ciphertext for listing
		entry.Ciphertext = "[encrypted]"
		entries = append(entries, entry)
	}
	return entries
}

// Delete removes a secret.
func (v *Vault) Delete(ctx context.Context, id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if _, ok := v.secrets[id]; !ok {
		return fmt.Errorf("vault: secret not found: %s", id)
	}
	delete(v.secrets, id)
	return nil
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
	if masterKey == "" {
		log.Fatal("[VAULT] VAULT_MASTER_KEY is not set. Set a 64-char hex key in .env or environment. Exiting.")
	}
	if len(masterKey) < 32 {
		log.Fatalf("[VAULT] VAULT_MASTER_KEY is too short (%d chars). Minimum 32 characters required. Exiting.", len(masterKey))
	}
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
