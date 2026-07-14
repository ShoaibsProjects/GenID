package middleware

import (
	"mime"
	"net/http"
)

const DefaultMaxBodyBytes = 10 << 20 // 10 MB

type RequestValidation struct {
	MaxBodyBytes int64
}

func NewRequestValidation() *RequestValidation {
	return &RequestValidation{
		MaxBodyBytes: DefaultMaxBodyBytes,
	}
}

func (v *RequestValidation) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, v.MaxBodyBytes)
		}
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" || r.Method == "QUERY" {
			ct := r.Header.Get("Content-Type")
			if ct == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"missing_content_type"}`))
				return
			}
			mediaType, _, err := mime.ParseMediaType(ct)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"invalid_content_type"}`))
				return
			}
			if mediaType != "application/json" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnsupportedMediaType)
				w.Write([]byte(`{"error":"unsupported_media_type"}`))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
