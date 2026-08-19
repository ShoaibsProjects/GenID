package mcp

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/observeid/genid/internal/audit"
	"github.com/observeid/genid/internal/middleware"
)

// Session is the authenticated identity backing an MCP connection. For
// stdio the API key comes from the MCP_API_KEY env var; for SSE it is
// presented on every request.
type Session struct {
	KeyID    string
	Name     string
	TenantID string
	Scopes   []string
}

// ErrInsufficientScope is returned when a key lacks the required scope.
var ErrInsufficientScope = errors.New("api key lacks required scope")

// Authenticate verifies a genid_<id>_<secret> API key and enforces the
// scope needed for the operation. Read-only tools pass needWrite=false;
// mutating tools (request_access) pass needWrite=true.
func Authenticate(ctx context.Context, store middleware.APIKeyStore, key string, needWrite bool) (*Session, error) {
	rec, err := middleware.VerifyAPIKey(ctx, store, key)
	if err != nil {
		return nil, err
	}
	if needWrite && !hasScope(rec.Scopes, ScopeMCPWrite) {
		return nil, ErrInsufficientScope
	}
	if !needWrite && !hasScope(rec.Scopes, ScopeMCPRead) && !hasScope(rec.Scopes, ScopeMCPWrite) {
		return nil, ErrInsufficientScope
	}
	return &Session{
		KeyID:    rec.ID,
		Name:     rec.Name,
		TenantID: rec.TenantID,
		Scopes:   rec.Scopes,
	}, nil
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

// AuditCall appends a tamper-evident audit_log row for an MCP tool call.
// Failures are logged, never fatal: auditing must not break the tool.
func AuditCall(ctx context.Context, chain *audit.Chain, sess *Session, action, resource string, details any) {
	raw, err := jsonMarshal(details)
	if err != nil {
		raw = []byte("{}")
	}
	if _, _, err := chain.Append(ctx, audit.ChainEntry{
		TenantID:  sess.TenantID,
		EventType: "mcp.tool_call",
		ActorID:   sess.KeyID,
		ActorType: "api_key",
		Action:    action,
		Resource:  resource,
		Details:   raw,
	}); err != nil {
		log.Printf("[mcp] audit append failed: %v", err)
	}
}

// apiKeyFromRequest extracts the key from an Authorization Bearer header,
// an X-API-Key header, or an api_key query parameter (SSE EventSource
// cannot set headers, so the query form is accepted for /sse).
func apiKeyFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if k := r.Header.Get("X-API-Key"); k != "" {
		return k
	}
	return r.URL.Query().Get("api_key")
}

// APIKeyMiddleware guards an SSE MCP endpoint. It injects the session into
// the request context; handlers downstream can read it via SessionFromContext.
func APIKeyMiddleware(store middleware.APIKeyStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := apiKeyFromRequest(r)
		if key == "" {
			http.Error(w, `{"error":"api key required"}`, http.StatusUnauthorized)
			return
		}
		sess, err := Authenticate(r.Context(), store, key, false)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), sessionKey{}, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type sessionKey struct{}

// SessionFromContext returns the session attached by APIKeyMiddleware.
func SessionFromContext(ctx context.Context) *Session {
	if s, ok := ctx.Value(sessionKey{}).(*Session); ok {
		return s
	}
	return nil
}

// session resolves the caller's session: per-request (SSE) or the
// stdio bootstrap session. Never nil.
func session(ctx context.Context, d *Deps) *Session {
	if s := SessionFromContext(ctx); s != nil {
		return s
	}
	return d.DefaultSession
}

// ValidateStdioAPIKey fails fast at stdio startup when MCP_API_KEY is
// unset — a stdio transport has no request headers to authenticate.
func ValidateStdioAPIKey(deps *Deps) error {
	if deps.MCPAPIKey == "" {
		return errors.New("MCP_API_KEY must be set for stdio transport")
	}
	sess, err := Authenticate(context.Background(), deps.APIKeys, deps.MCPAPIKey, false)
	if err != nil {
		return err
	}
	deps.TenantID = sess.TenantID
	deps.APIKeyName = sess.Name
	deps.DefaultSession = sess
	log.Printf("[mcp] stdio session: key=%s tenant=%s scopes=%v", sess.Name, sess.TenantID, sess.Scopes)
	return nil
}

var _ = time.Now
