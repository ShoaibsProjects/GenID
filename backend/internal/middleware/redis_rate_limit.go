package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisRateLimiter is a fixed-window per-tenant+IP limiter backed by Redis
// (INCR + EXPIRE), so limits survive restarts and are shared across replicas.
// The tenant comes from the JWT context when present, falling back to IP-only.
type RedisRateLimiter struct {
	rdb    *redis.Client
	limit  int
	window time.Duration
}

// NewRedisRateLimiter creates a limiter with `limit` requests per `window`
// per (tenant, IP) pair.
func NewRedisRateLimiter(rdb *redis.Client, limit int, window time.Duration) *RedisRateLimiter {
	return &RedisRateLimiter{rdb: rdb, limit: limit, window: window}
}

func (rl *RedisRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = fwd
		}
		tenant := TenantIDFromContext(r.Context())
		key := "rl:" + tenant + ":" + ip

		ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
		defer cancel()

		count, err := rl.rdb.Incr(ctx, key).Result()
		if err != nil {
			// Redis unavailable: fail open to avoid taking the API down.
			next.ServeHTTP(w, r)
			return
		}
		if count == 1 {
			rl.rdb.Expire(ctx, key, rl.window)
		}
		if count > int64(rl.limit) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", strconv.Itoa(int(rl.window.Seconds())))
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate_limit_exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
