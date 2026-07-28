package middleware

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Context keys for JWT claims injected into request context.
type contextKey string

const (
	ContextKeyTenantID contextKey = "tenant_id"
	ContextKeyUserID   contextKey = "user_id"
	ContextKeyRoles    contextKey = "roles"
	ContextKeyClaims   contextKey = "jwt_claims"
)

// JWTClaims holds the extracted claims from a validated JWT.
type JWTClaims struct {
	TenantID string   `json:"tenant_id"`
	UserID   string   `json:"sub"`
	Roles    []string `json:"roles"`
	Raw      jwt.MapClaims
}

// JWTAuth validates JWTs using JWKS fetched from the OIDC provider.
type JWTAuth struct {
	jwksURL    string
	jwksCache  *jwksKeySet
	cacheMu    sync.RWMutex
	cacheTTL   time.Duration
	lastFetch  time.Time
	skipPaths  map[string]bool
	apiKeys    map[string]string // internal Temporal worker keys
}

// jwksKeySet caches the JWKS response.
type jwksKeySet struct {
	Keys []jwkEntry `json:"keys"`
}

type jwkEntry struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// NewJWTAuth creates a new JWT authentication middleware.
// jwksURL is the full URL to the JWKS endpoint (e.g., http://localhost:8080/.well-known/jwks.json).
// apiKeys are internal keys for Temporal workers (bypass JWT, inject system tenant).
func NewJWTAuth(jwksURL string, apiKeys map[string]string, skipPaths ...string) *JWTAuth {
	skip := make(map[string]bool, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = true
	}
	return &JWTAuth{
		jwksURL:   jwksURL,
		cacheTTL:  5 * time.Minute,
		skipPaths: skip,
		apiKeys:   apiKeys,
	}
}

// Middleware validates JWT Bearer tokens and injects claims into context.
// Falls back to X-API-Key for internal Temporal workers.
func (j *JWTAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health/readiness endpoints
		if j.skipPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		// Skip auth for Next.js static asset paths
		if strings.HasPrefix(r.URL.Path, "/_next/") || strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")

		// Path 1: JWT Bearer token (external users/agents)
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := j.validateJWT(r.Context(), tokenStr)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(w, `{"error":"invalid_token","detail":"%s"}`, err.Error())
				return
			}
			ctx := context.WithValue(r.Context(), ContextKeyTenantID, claims.TenantID)
			ctx = context.WithValue(ctx, ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyRoles, claims.Roles)
			ctx = context.WithValue(ctx, ContextKeyClaims, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Path 2: X-API-Key (internal Temporal workers only)
		if apiKey := r.Header.Get("X-API-Key"); apiKey != "" && j.apiKeys != nil {
			if name, ok := j.apiKeys[apiKey]; ok {
				// Inject system tenant context for internal workers
				ctx := context.WithValue(r.Context(), ContextKeyTenantID, "00000000-0000-0000-0000-000000000001")
				ctx = context.WithValue(ctx, ContextKeyUserID, "system:"+name)
				ctx = context.WithValue(ctx, ContextKeyRoles, []string{"system", "worker"})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// No valid authentication found
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"missing_token","detail":"Authorization: Bearer <token> or X-API-Key required"}`))
	})
}

// validateJWT validates a JWT token using JWKS keys.
func (j *JWTAuth) validateJWT(ctx context.Context, tokenStr string) (*JWTClaims, error) {
	// Parse the token header to get the key ID
	token, _, err := jwt.NewParser().ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("malformed token")
	}

	kid, _ := token.Header["kid"].(string)
	if kid == "" {
		return nil, fmt.Errorf("missing kid in token header")
	}

	// Get the public key from JWKS
	pubKey, err := j.getRSAPublicKey(ctx, kid)
	if err != nil {
		return nil, fmt.Errorf("key not found: %w", err)
	}

	// Validate the token with the public key
	parsed, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pubKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Extract standard claims
	sub, _ := mapClaims["sub"].(string)
	tenantID, _ := mapClaims["tenant_id"].(string)
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001" // default tenant
	}

	var roles []string
	if r, ok := mapClaims["roles"].([]any); ok {
		for _, v := range r {
			if s, ok := v.(string); ok {
				roles = append(roles, s)
			}
		}
	}
	if len(roles) == 0 {
		roles = []string{"user"}
	}

	return &JWTClaims{
		TenantID: tenantID,
		UserID:   sub,
		Roles:    roles,
		Raw:      mapClaims,
	}, nil
}

// getRSAPublicKey retrieves an RSA public key from the JWKS cache.
func (j *JWTAuth) getRSAPublicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	keys, err := j.fetchJWKS(ctx)
	if err != nil {
		return nil, err
	}

	for _, key := range keys.Keys {
		if key.Kid == kid && key.Kty == "RSA" {
			return j.parseRSAPublicKey(key)
		}
	}
	return nil, fmt.Errorf("key %s not found in JWKS", kid)
}

// fetchJWKS fetches and caches the JWKS key set.
func (j *JWTAuth) fetchJWKS(ctx context.Context) (*jwksKeySet, error) {
	j.cacheMu.RLock()
	if j.jwksCache != nil && time.Since(j.lastFetch) < j.cacheTTL {
		cache := j.jwksCache
		j.cacheMu.RUnlock()
		return cache, nil
	}
	j.cacheMu.RUnlock()

	j.cacheMu.Lock()
	defer j.cacheMu.Unlock()

	// Double-check after acquiring write lock
	if j.jwksCache != nil && time.Since(j.lastFetch) < j.cacheTTL {
		return j.jwksCache, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", j.jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var keySet jwksKeySet
	if err := json.NewDecoder(resp.Body).Decode(&keySet); err != nil {
		return nil, fmt.Errorf("decode JWKS: %w", err)
	}

	j.jwksCache = &keySet
	j.lastFetch = time.Now()

	return &keySet, nil
}

// parseRSAPublicKey parses a JWK entry into an RSA public key.
func (j *JWTAuth) parseRSAPublicKey(entry jwkEntry) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(entry.N)
	if err != nil {
		return nil, fmt.Errorf("decode N: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(entry.E)
	if err != nil {
		return nil, fmt.Errorf("decode E: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}

	return &rsa.PublicKey{N: n, E: e}, nil
}

// Helper functions to extract claims from context.

// TenantIDFromContext extracts the tenant ID from context.
func TenantIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyTenantID).(string); ok {
		return v
	}
	return "00000000-0000-0000-0000-000000000001"
}

// UserIDFromContext extracts the user/agent ID from context.
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyUserID).(string); ok {
		return v
	}
	return ""
}

// RolesFromContext extracts the roles from context.
func RolesFromContext(ctx context.Context) []string {
	if v, ok := ctx.Value(ContextKeyRoles).([]string); ok {
		return v
	}
	return nil
}

// ClaimsFromContext extracts the full JWT claims from context.
func ClaimsFromContext(ctx context.Context) *JWTClaims {
	if v, ok := ctx.Value(ContextKeyClaims).(*JWTClaims); ok {
		return v
	}
	return nil
}
