package middleware

import (
	"net/http"
	"strings"
	"sync"
)

type APIKeyAuth struct {
	keys      map[string]string
	mu        sync.RWMutex
	enabled   bool
	skipPaths map[string]bool
}

func NewAPIKeyAuth(keys map[string]string, skipPaths ...string) *APIKeyAuth {
	skip := make(map[string]bool, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = true
	}
	return &APIKeyAuth{
		keys:      keys,
		enabled:   len(keys) > 0,
		skipPaths: skip,
	}
}

func (a *APIKeyAuth) Enabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.enabled
}

func (a *APIKeyAuth) SetKeys(keys map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.keys = keys
	a.enabled = len(keys) > 0
}

func (a *APIKeyAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.RLock()
		enabled := a.enabled
		keys := a.keys
		skip := a.skipPaths
		a.mu.RUnlock()

		if !enabled || skip[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		key := r.Header.Get("X-API-Key")
		if key == "" {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				key = strings.TrimPrefix(auth, "Bearer ")
			}
		}
		if key == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"missing_api_key"}`))
			return
		}
		a.mu.RLock()
		_, ok := keys[key]
		a.mu.RUnlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"invalid_api_key"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
