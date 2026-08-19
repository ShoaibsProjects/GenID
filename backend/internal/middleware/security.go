package middleware

import (
	"net/http"
	"os"
)

// SecurityHeadersMiddleware applies hardened response headers on every
// request. CSP is only emitted when CSP_ENABLED=true so existing frontend
// deployments that rely on inline styles/scripts keep working unchanged.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	cspEnabled := os.Getenv("CSP_ENABLED") == "true"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if cspEnabled {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		}
		next.ServeHTTP(w, r)
	})
}
