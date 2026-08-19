package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// JITPassport is a short-lived credential minted for a specific access
// grant. It is single-scope, non-transferable, expires by clock, and is
// revoked or consumed when the grant completes.
type JITPassport struct {
	ID               string         `json:"id"`
	TenantID         string         `json:"tenant_id"`
	NHIID            string         `json:"nhi_id"`
	IssuerID         string         `json:"issuer_id,omitempty"`
	TokenHash        string         `json:"-"`
	Scope            string         `json:"scope"`
	ResourceID       string         `json:"resource_id,omitempty"`
	GrantID          string         `json:"grant_id,omitempty"`
	Status           string         `json:"status"`
	IssuedAt         time.Time      `json:"issued_at"`
	ExpiresAt        time.Time      `json:"expires_at"`
	RevokedAt        *time.Time     `json:"revoked_at,omitempty"`
	ConsumedAt       *time.Time     `json:"consumed_at,omitempty"`
	CreatedBy        string         `json:"created_by,omitempty"`
	ParentPassportID string         `json:"parent_passport_id,omitempty"`
	Attributes       map[string]any `json:"attributes,omitempty"`
}

// Passport statuses.
const (
	PassportActive   = "active"
	PassportExpired  = "expired"
	PassportRevoked  = "revoked"
	PassportConsumed = "consumed"
)

// MintToken returns a cryptographically random passport token. Only the
// SHA-256 hash is ever persisted; the raw token is handed to the caller
// exactly once.
func MintToken() (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token = "jit_" + hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:]), nil
}

// HashToken hashes a presented token for lookup.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
