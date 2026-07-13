package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	visitors map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
	stopCh   chan struct{}
}

func NewRateLimiter(r rate.Limit, burst int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    burst,
		stopCh:   make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) Stop() {
	select {
	case <-rl.stopCh:
		// already closed
	default:
		close(rl.stopCh)
	}
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.RLock()
	limiter, ok := rl.visitors[ip]
	rl.mu.RUnlock()
	if ok {
		return limiter
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	limiter = rate.NewLimiter(rl.rate, rl.burst)
	rl.visitors[ip] = limiter
	return limiter
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			for ip, lim := range rl.visitors {
				if lim.Tokens() == float64(rl.burst) {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = fwd
		}
		if !rl.getLimiter(ip).Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate_limit_exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

