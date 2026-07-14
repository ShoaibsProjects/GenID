package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// ─── Rate Limiter ─────────────────────────────────────────

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1000), 100)
	handler := rl.Middleware(http.HandlerFunc(okHandler))

	for i := 0; i < 50; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "request %d should be allowed", i)
	}
}

func TestRateLimiter_BlocksWhenExceeded(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1)
	handler := rl.Middleware(http.HandlerFunc(okHandler))

	// First request should be allowed (burst=1)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second request should be rate-limited
	req = httptest.NewRequest("GET", "/test", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Body.String(), "rate_limit_exceeded")
}

func TestRateLimiter_PerIP(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1)
	handler := rl.Middleware(http.HandlerFunc(okHandler))

	// IP 1: first request passes
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.1:1234"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// IP 1: second request blocked
	req1b := httptest.NewRequest("GET", "/test", nil)
	req1b.RemoteAddr = "192.168.1.1:1234"
	rec1b := httptest.NewRecorder()
	handler.ServeHTTP(rec1b, req1b)
	assert.Equal(t, http.StatusTooManyRequests, rec1b.Code)

	// IP 2: first request passes (different limiter)
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "10.0.0.1:5678"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
}

func TestRateLimiter_UsesXForwardedFor(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1)
	handler := rl.Middleware(http.HandlerFunc(okHandler))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "proxynode:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Same X-Forwarded-For — blocked
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "proxynode:8080"
	req2.Header.Set("X-Forwarded-For", "203.0.113.1")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

func TestRateLimiter_StopsAndCleansUp(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(100), 10)
	rl.Stop()
	// Should not panic
	rl.Stop()
}

// ─── API Key Auth ──────────────────────────────────────────

func TestAPIKeyAuth_DisabledWhenNoKeys(t *testing.T) {
	auth := NewAPIKeyAuth(nil)
	assert.False(t, auth.Enabled())

	handler := auth.Middleware(http.HandlerFunc(okHandler))
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAPIKeyAuth_AllowsValidKey(t *testing.T) {
	auth := NewAPIKeyAuth(map[string]string{"valid-key-123": "test-client"})
	assert.True(t, auth.Enabled())

	handler := auth.Middleware(http.HandlerFunc(okHandler))
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "valid-key-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAPIKeyAuth_RejectsMissingKey(t *testing.T) {
	auth := NewAPIKeyAuth(map[string]string{"valid-key": "client"})
	handler := auth.Middleware(http.HandlerFunc(okHandler))
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "missing_api_key")
}

func TestAPIKeyAuth_RejectsInvalidKey(t *testing.T) {
	auth := NewAPIKeyAuth(map[string]string{"valid-key": "client"})
	handler := auth.Middleware(http.HandlerFunc(okHandler))
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid_api_key")
}

func TestAPIKeyAuth_AcceptsBearerToken(t *testing.T) {
	auth := NewAPIKeyAuth(map[string]string{"bearer-key": "client"})
	handler := auth.Middleware(http.HandlerFunc(okHandler))
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer bearer-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAPIKeyAuth_SkipsConfiguredPaths(t *testing.T) {
	auth := NewAPIKeyAuth(map[string]string{"key": "client"}, "/public", "/health")
	handler := auth.Middleware(http.HandlerFunc(okHandler))

	// Public path — no auth needed
	req := httptest.NewRequest("GET", "/public", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Protected path — auth required
	req = httptest.NewRequest("GET", "/api/v1/identities", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAPIKeyAuth_SetKeys(t *testing.T) {
	auth := NewAPIKeyAuth(nil)
	assert.False(t, auth.Enabled())

	auth.SetKeys(map[string]string{"new-key": "client"})
	assert.True(t, auth.Enabled())

	handler := auth.Middleware(http.HandlerFunc(okHandler))
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "new-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ─── Request Validation ───────────────────────────────────

func TestRequestValidation_AllowsJSON(t *testing.T) {
	v := NewRequestValidation()
	handler := v.Middleware(http.HandlerFunc(okHandler))

	body := strings.NewReader(`{"key":"value"}`)
	req := httptest.NewRequest("POST", "/api", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequestValidation_RejectsMissingContentType(t *testing.T) {
	v := NewRequestValidation()
	handler := v.Middleware(http.HandlerFunc(okHandler))

	body := strings.NewReader(`{"key":"value"}`)
	req := httptest.NewRequest("POST", "/api", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "missing_content_type")
}

func TestRequestValidation_RejectsWrongContentType(t *testing.T) {
	v := NewRequestValidation()
	handler := v.Middleware(http.HandlerFunc(okHandler))

	body := strings.NewReader(`<xml></xml>`)
	req := httptest.NewRequest("POST", "/api", body)
	req.Header.Set("Content-Type", "application/xml")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	assert.Contains(t, rec.Body.String(), "unsupported_media_type")
}

func TestRequestValidation_AllowsGET(t *testing.T) {
	v := NewRequestValidation()
	handler := v.Middleware(http.HandlerFunc(okHandler))

	req := httptest.NewRequest("GET", "/api", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequestValidation_LimitsBodySize(t *testing.T) {
	v := &RequestValidation{MaxBodyBytes: 100}
	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 200)
		_, err := r.Body.Read(buf)
		if err != nil {
			http.Error(w, "body read error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader(strings.Repeat("a", 200))
	req := httptest.NewRequest("POST", "/api", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	// Body exceeds MaxBytesReader limit, so the handler returns 500
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ─── QUERY Method Validation (RFC 10008) ─────────────────

func TestRequestValidation_AllowsQUERYWithJSON(t *testing.T) {
	v := NewRequestValidation()
	handler := v.Middleware(http.HandlerFunc(okHandler))

	body := strings.NewReader(`{"question":"what access does user 1 have?"}`)
	req := httptest.NewRequest("QUERY", "/api", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequestValidation_RejectsQUERYMissingContentType(t *testing.T) {
	v := NewRequestValidation()
	handler := v.Middleware(http.HandlerFunc(okHandler))

	body := strings.NewReader(`{"question":"test"}`)
	req := httptest.NewRequest("QUERY", "/api", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "missing_content_type")
}

func TestRequestValidation_RejectsQUERYWrongContentType(t *testing.T) {
	v := NewRequestValidation()
	handler := v.Middleware(http.HandlerFunc(okHandler))

	body := strings.NewReader(`not json`)
	req := httptest.NewRequest("QUERY", "/api", body)
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	assert.Contains(t, rec.Body.String(), "unsupported_media_type")
}
