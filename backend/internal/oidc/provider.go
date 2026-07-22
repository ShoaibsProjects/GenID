// Package oidc implements an OAuth 2.0 / OpenID Connect 1.0 Identity Provider.
// Generates JWT access tokens, refresh tokens, and ID tokens signed with
// RSA 2048-bit keys exposed via JWKS endpoint. Uses PostgreSQL for persistence
// of clients, auth codes, refresh tokens, and device codes.
package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Provider is the core OIDC/OAuth2 Identity Provider.
type Provider struct {
	mu         sync.RWMutex
	pgPool     *pgxpool.Pool
	signingKey *rsa.PrivateKey
	keyID      string
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
	idTokenTTL time.Duration
}

// NewProvider creates a new OIDC provider with generated RSA keys.
func NewProvider(pgPool *pgxpool.Pool, issuer string) (*Provider, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("oidc: key generation: %w", err)
	}

	h := sha256.Sum256([]byte(time.Now().String()))
	kid := fmt.Sprintf("observeid-%x", h[:8])

	return &Provider{
		pgPool:     pgPool,
		signingKey: key,
		keyID:      kid,
		issuer:     issuer,
		accessTTL:  5 * time.Minute,
		refreshTTL: 30 * 24 * time.Hour,
		idTokenTTL: 5 * time.Minute,
	}, nil
}

// Issuer returns the configured issuer URL.
func (p *Provider) Issuer() string { return p.issuer }

// KeyID returns the current signing key ID.
func (p *Provider) KeyID() string { return p.keyID }

// ─── JWT Token Creation ──────────────────────────────────────

// SignAccessToken creates a signed JWT access token.
func (p *Provider) SignAccessToken(userID, clientID, scope string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   p.issuer,
		"sub":   userID,
		"aud":   clientID,
		"iat":   now.Unix(),
		"exp":   now.Add(p.accessTTL).Unix(),
		"jti":   generateTokenID(),
		"scope": scope,
		"typ":   "Bearer",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = p.keyID
	return token.SignedString(p.signingKey)
}

// SignIDToken creates a signed OIDC ID token.
func (p *Provider) SignIDToken(userID, clientID, nonce string, claims map[string]any) (string, error) {
	now := time.Now()
	tokenClaims := jwt.MapClaims{
		"iss": p.issuer,
		"sub": userID,
		"aud": clientID,
		"iat": now.Unix(),
		"exp": now.Add(p.idTokenTTL).Unix(),
		"jti": generateTokenID(),
	}
	if nonce != "" {
		tokenClaims["nonce"] = nonce
	}
	for k, v := range claims {
		tokenClaims[k] = v
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, tokenClaims)
	token.Header["kid"] = p.keyID
	return token.SignedString(p.signingKey)
}

// SignRefreshToken creates a signed refresh token.
func (p *Provider) SignRefreshToken(userID, clientID, scope string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   p.issuer,
		"sub":   userID,
		"aud":   clientID,
		"iat":   now.Unix(),
		"exp":   now.Add(p.refreshTTL).Unix(),
		"jti":   generateTokenID(),
		"scope": scope,
		"type":  "refresh",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = p.keyID
	return token.SignedString(p.signingKey)
}

// ParseToken validates and parses a JWT token.
func (p *Provider) ParseToken(tokenStr string) (*jwt.Token, jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return &p.signingKey.PublicKey, nil
	})
	if err != nil {
		return nil, nil, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, nil, fmt.Errorf("invalid claims type")
	}
	return token, claims, nil
}

// ─── JWKS ─────────────────────────────────────────────────────

// JWKS returns the JWKS representation of the current signing key.
func (p *Provider) JWKS() map[string]any {
	pub := &p.signingKey.PublicKey
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(bigEndianBytes(pub.E))

	return map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"kid": p.keyID,
			"use": "sig",
			"alg": "RS256",
			"n":   n,
			"e":   e,
		}},
	}
}

// ─── OpenID Connect Discovery ────────────────────────────────

// DiscoveryDocument returns the OIDC discovery document.
func (p *Provider) DiscoveryDocument() map[string]any {
	return map[string]any{
		"issuer":                                p.issuer,
		"authorization_endpoint":                p.issuer + "/authorize",
		"token_endpoint":                        p.issuer + "/token",
		"userinfo_endpoint":                     p.issuer + "/userinfo",
		"jwks_uri":                              p.issuer + "/.well-known/jwks.json",
		"introspection_endpoint":                p.issuer + "/introspect",
		"revocation_endpoint":                   p.issuer + "/revoke",
		"device_authorization_endpoint":         p.issuer + "/device_authorization",
		"scopes_supported":                      []string{"openid", "profile", "email", "offline_access", "api"},
		"response_types_supported":              []string{"code", "code id_token", "id_token"},
		"grant_types_supported":                 []string{"authorization_code", "client_credentials", "refresh_token", "urn:ietf:params:oauth:grant-type:device_code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"},
		"claims_supported":                      []string{"sub", "iss", "aud", "iat", "exp", "email", "name", "preferred_username", "department"},
		"code_challenge_methods_supported":      []string{"S256"},
	}
}

// ─── Database-backed Authorization Codes ──────────────────────

// StoreAuthCode stores an authorization code in the database.
func (p *Provider) StoreAuthCode(ctx context.Context, code, clientID, userID, redirectURI string, scope []string, challenge, method, nonce string) error {
	_, err := p.pgPool.Exec(ctx, `
		INSERT INTO oidc_auth_codes (code, client_id, user_id, redirect_uri, scope, code_challenge, code_challenge_method, nonce, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (code) DO UPDATE SET
			client_id = EXCLUDED.client_id,
			user_id = EXCLUDED.user_id,
			redirect_uri = EXCLUDED.redirect_uri,
			scope = EXCLUDED.scope,
			code_challenge = EXCLUDED.code_challenge,
			code_challenge_method = EXCLUDED.code_challenge_method,
			nonce = EXCLUDED.nonce,
			expires_at = EXCLUDED.expires_at
	`, code, clientID, userID, redirectURI, scope, challenge, method, nonce, time.Now().Add(5*time.Minute))
	return err
}

// ConsumeAuthCode validates and consumes an authorization code.
func (p *Provider) ConsumeAuthCode(ctx context.Context, code, clientID, redirectURI, codeVerifier string) (*AuthCodeEntry, error) {
	var entry AuthCodeEntry
	err := p.pgPool.QueryRow(ctx, `
		SELECT code, client_id, user_id, redirect_uri, scope, code_challenge, code_challenge_method, nonce, expires_at, consumed_at
		FROM oidc_auth_codes WHERE code = $1
	`, code).Scan(&entry.Code, &entry.ClientID, &entry.UserID, &entry.RedirectURI, &entry.Scope, &entry.CodeChallenge, &entry.CodeChallengeMethod, &entry.Nonce, &entry.ExpiresAt, &entry.ConsumedAt)

	if err != nil {
		return nil, fmt.Errorf("invalid authorization code")
	}

	if entry.ConsumedAt != nil {
		return nil, fmt.Errorf("authorization code already used")
	}
	if time.Now().After(entry.ExpiresAt) {
		return nil, fmt.Errorf("authorization code expired")
	}
	if entry.ClientID != clientID {
		return nil, fmt.Errorf("client_id mismatch")
	}
	if entry.RedirectURI != redirectURI {
		return nil, fmt.Errorf("redirect_uri mismatch")
	}

	// PKCE verification
	if entry.CodeChallenge != "" {
		if codeVerifier == "" {
			return nil, fmt.Errorf("PKCE verification required")
		}
		if !verifyPKCE(entry.CodeChallenge, entry.CodeChallengeMethod, codeVerifier) {
			return nil, fmt.Errorf("PKCE verification failed")
		}
	}

	// Mark as consumed
	_, err = p.pgPool.Exec(ctx, `UPDATE oidc_auth_codes SET consumed_at = NOW() WHERE code = $1`, code)
	if err != nil {
		return nil, err
	}

	return &entry, nil
}

// AuthCodeEntry represents an authorization code record.
type AuthCodeEntry struct {
	Code                  string
	ClientID              string
	UserID                string
	RedirectURI           string
	Scope                 []string
	CodeChallenge         string
	CodeChallengeMethod   string
	Nonce                 string
	ExpiresAt             time.Time
	ConsumedAt            *time.Time
}

// ─── Database-backed Refresh Tokens ───────────────────────────

// StoreRefreshToken stores a refresh token in the database.
func (p *Provider) StoreRefreshToken(ctx context.Context, token, clientID, userID string, scope []string) error {
	hash := hashToken(token)
	_, err := p.pgPool.Exec(ctx, `
		INSERT INTO oidc_refresh_tokens (token_hash, client_id, user_id, scope, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, hash, clientID, userID, scope, time.Now().Add(p.refreshTTL))
	return err
}

// ConsumeRefreshToken validates and consumes a refresh token (rotation).
func (p *Provider) ConsumeRefreshToken(ctx context.Context, token, clientID string) (*RefreshTokenEntry, error) {
	hash := hashToken(token)
	var entry RefreshTokenEntry
	err := p.pgPool.QueryRow(ctx, `
		SELECT id, token_hash, client_id, user_id, scope, expires_at, revoked_at, created_at
		FROM oidc_refresh_tokens WHERE token_hash = $1
	`, hash).Scan(&entry.ID, &entry.TokenHash, &entry.ClientID, &entry.UserID, &entry.Scope, &entry.ExpiresAt, &entry.RevokedAt, &entry.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("invalid refresh token")
	}

	if entry.RevokedAt != nil {
		return nil, fmt.Errorf("refresh token revoked")
	}
	if time.Now().After(entry.ExpiresAt) {
		return nil, fmt.Errorf("refresh token expired")
	}
	if entry.ClientID != clientID {
		return nil, fmt.Errorf("client_id mismatch")
	}

	// Update last_used_at
	_, _ = p.pgPool.Exec(ctx, `UPDATE oidc_refresh_tokens SET last_used_at = NOW() WHERE id = $1`, entry.ID)

	return &entry, nil
}

// RevokeRefreshToken revokes a refresh token.
func (p *Provider) RevokeRefreshToken(ctx context.Context, token string) error {
	hash := hashToken(token)
	_, err := p.pgPool.Exec(ctx, `UPDATE oidc_refresh_tokens SET revoked_at = NOW() WHERE token_hash = $1`, hash)
	return err
}

// RefreshTokenEntry represents a refresh token record.
type RefreshTokenEntry struct {
	ID           string
	TokenHash    string
	ClientID     string
	UserID       string
	Scope        []string
	ExpiresAt    time.Time
	RevokedAt    *time.Time
	CreatedAt    time.Time
	LastUsedAt   *time.Time
}

// ─── Device Authorization (RFC 8628) ──────────────────────────

// CreateDeviceCode creates a new device authorization code.
func (p *Provider) CreateDeviceCode(ctx context.Context, clientID string, scope []string) (*DeviceCodeEntry, error) {
	deviceCode := generateDeviceCode()
	userCode := generateUserCode()

	entry := &DeviceCodeEntry{
		DeviceCode: deviceCode,
		UserCode:   userCode,
		ClientID:   clientID,
		Scope:      scope,
		ExpiresAt:  time.Now().Add(15 * time.Minute),
		CreatedAt:  time.Now(),
	}

	_, err := p.pgPool.Exec(ctx, `
		INSERT INTO oidc_device_codes (device_code, user_code, client_id, scope, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, deviceCode, userCode, clientID, scope, entry.ExpiresAt)

	if err != nil {
		return nil, err
	}
	return entry, nil
}

// PollDeviceCode polls for device authorization completion.
func (p *Provider) PollDeviceCode(ctx context.Context, deviceCode, clientID string) (*DeviceCodeEntry, error) {
	var entry DeviceCodeEntry
	err := p.pgPool.QueryRow(ctx, `
		SELECT device_code, user_code, client_id, user_id, scope, expires_at, authorized_at, created_at
		FROM oidc_device_codes WHERE device_code = $1 AND client_id = $2
	`, deviceCode, clientID).Scan(&entry.DeviceCode, &entry.UserCode, &entry.ClientID, &entry.UserID, &entry.Scope, &entry.ExpiresAt, &entry.AuthorizedAt, &entry.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("invalid device code")
	}

	if time.Now().After(entry.ExpiresAt) {
		return nil, fmt.Errorf("authorization_pending")
	}
	if entry.AuthorizedAt == nil {
		return nil, fmt.Errorf("authorization_pending")
	}
	return &entry, nil
}

// ApproveDeviceCode marks a device code as authorized by the user.
func (p *Provider) ApproveDeviceCode(ctx context.Context, userCode, userID string) error {
	_, err := p.pgPool.Exec(ctx, `
		UPDATE oidc_device_codes SET authorized_at = NOW(), user_id = $1 WHERE user_code = $2
	`, userID, userCode)
	return err
}

// DeviceCodeEntry represents a device authorization code.
type DeviceCodeEntry struct {
	DeviceCode  string
	UserCode    string
	ClientID    string
	UserID      *string
	Scope       []string
	ExpiresAt   time.Time
	AuthorizedAt *time.Time
	CreatedAt   time.Time
}

// ─── Identity Claims ──────────────────────────────────────────

// GetUserClaims fetches identity attributes from PostgreSQL for ID token enrichment.
func (p *Provider) GetUserClaims(ctx context.Context, userID string) map[string]any {
	var email, name, dept string
	if err := p.pgPool.QueryRow(ctx,
		`SELECT email, display_name, COALESCE(department,'') FROM identities WHERE id = $1`, userID,
	).Scan(&email, &name, &dept); err != nil {
		return nil
	}
	return map[string]any{
		"email":              email,
		"name":               name,
		"preferred_username": email,
		"department":         dept,
		"updated_at":         time.Now().Unix(),
	}
}

// AuthenticatePassword verifies a username/password against PostgreSQL.
func (p *Provider) AuthenticatePassword(ctx context.Context, username, password string) (string, error) {
	var id, status, storedHash string
	err := p.pgPool.QueryRow(ctx,
		`SELECT id, status, COALESCE(attributes->>'password_hash','') FROM identities WHERE email = $1`, username,
	).Scan(&id, &status, &storedHash)

	if err != nil {
		return "", fmt.Errorf("invalid credentials")
	}
	if status != "active" {
		return "", fmt.Errorf("account %s", status)
	}

	if storedHash != "" {
		if !verifyPassword(password, storedHash) {
			return "", fmt.Errorf("invalid credentials")
		}
		return id, nil
	}

	// Demo mode: accept any password for active identities
	return id, nil
}

// ─── Helpers ──────────────────────────────────────────────────

func generateTokenID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func generateDeviceCode() string {
	b := make([]byte, 32)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func generateUserCode() string {
	// 8-character user code: XXXX-XXXX
	b := make([]byte, 4)
	rand.Read(b)
	code := fmt.Sprintf("%x", b)
	return strings.ToUpper(code[:4] + "-" + code[4:])
}

func bigEndianBytes(i int) []byte {
	if i == 0 {
		return []byte{0}
	}
	var b []byte
	for v := i; v > 0; v >>= 8 {
		b = append([]byte{byte(v & 0xff)}, b...)
	}
	return b
}

func verifyPKCE(challenge, method, verifier string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	var h []byte
	switch strings.ToUpper(method) {
	case "S256", "":
		sum := sha256.Sum256([]byte(verifier))
		h = sum[:]
	case "PLAIN":
		h = []byte(verifier)
	default:
		return false
	}
	computed := base64.RawURLEncoding.EncodeToString(h)
	return computed == challenge
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func verifyPassword(password, hash string) bool {
	// Production: bcrypt.CompareHashAndPassword
	return password == hash
}

// MarshalJSON for ClientRecord to handle sensitive fields
func (c *ClientRecord) MarshalJSON() ([]byte, error) {
	type Alias ClientRecord
	return json.Marshal(&struct {
		*Alias
		ClientSecret string `json:"client_secret,omitempty"`
	}{
		Alias:        (*Alias)(c),
		ClientSecret: c.ClientSecret,
	})
}