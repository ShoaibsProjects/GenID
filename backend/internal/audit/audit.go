package audit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/observeid/identity-platform/internal/middleware"
)

type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
	LevelFatal Level = "fatal"
)

// Filter holds all supported query-time filters for the audit log.
type Filter struct {
	Level    Level
	Method   string
	Path     string
	Status   int
	SourceIP string
	Since    time.Time
	Until    time.Time
}

type Entry struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     Level     `json:"level"`
	Service   string    `json:"service"`
	Method    string    `json:"method,omitempty"`
	Path      string    `json:"path,omitempty"`
	Status    int       `json:"status,omitempty"`
	Latency   string    `json:"latency,omitempty"`
	Message   string    `json:"message"`
	Detail    string    `json:"detail,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
	UserID    string    `json:"user_id,omitempty"`
	SourceIP  string    `json:"source_ip,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
	// Hash is the SHA-256 chain hash assigned when this entry was
	// persisted to the tamper-evident ledger (audit_log).
	Hash      string    `json:"hash,omitempty"`
}

type Store struct {
	mu      sync.RWMutex
	entries []Entry
	cap     int
	nextID  int
}

func NewStore(capacity int) *Store {
	if capacity <= 0 {
		capacity = 10000
	}
	return &Store{
		entries: make([]Entry, 0, capacity),
		cap:     capacity,
	}
}

func (s *Store) Append(e Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	e.ID = fmt.Sprintf("LOG-%d", s.nextID)
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	s.entries = append(s.entries, e)
	if len(s.entries) > s.cap {
		s.entries = s.entries[len(s.entries)-s.cap:]
	}
}

func (s *Store) List(limit, offset int, f Filter) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.entries)
	if total == 0 {
		return nil
	}

	var filtered []Entry
	for _, e := range s.entries {
		if f.Level != "" && e.Level != f.Level {
			continue
		}
		if f.Method != "" && e.Method != f.Method {
			continue
		}
		if f.Path != "" && !strings.Contains(e.Path, f.Path) {
			continue
		}
		if f.Status > 0 && e.Status != f.Status {
			continue
		}
		if f.SourceIP != "" && !strings.Contains(e.SourceIP, f.SourceIP) {
			continue
		}
		if !f.Since.IsZero() && e.Timestamp.Before(f.Since) {
			continue
		}
		if !f.Until.IsZero() && e.Timestamp.After(f.Until) {
			continue
		}
		filtered = append(filtered, e)
	}

	if offset >= len(filtered) {
		return nil
	}
	filtered = filtered[offset:]
	if limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}

	// Return in reverse chronological order (most recent first)
	result := make([]Entry, len(filtered))
	for i, e := range filtered {
		result[len(result)-1-i] = e
	}
	return result
}

func (s *Store) Count(level Level) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if level == "" {
		return len(s.entries)
	}
	count := 0
	for _, e := range s.entries {
		if e.Level == level {
			count++
		}
	}
	return count
}

func (s *Store) Get(id string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.entries {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

type responseWriter struct {
	http.ResponseWriter
	status int
	body   strings.Builder
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.body.Write(b)
	return rw.ResponseWriter.Write(b)
}

// LoggingMiddleware records every HTTP request into the in-memory ring
// buffer (fast UI reads) and, when chain is non-nil, persists it to the
// tamper-evident ledger. Chain errors never block the request path.
func LoggingMiddleware(store *Store, chain *Chain) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rw, r)

			latency := time.Since(start)
			level := LevelInfo
			if rw.status >= 500 {
				level = LevelError
			} else if rw.status >= 400 {
				level = LevelWarn
			}

			msg := fmt.Sprintf("%s %s -> %d", r.Method, r.URL.Path, rw.status)

			detail := ""
			if rw.status >= 400 {
				detail = rw.body.String()
				if len(detail) > 2000 {
					detail = detail[:2000]
				}
			}

			tags := []string{"http"}
			if rw.status >= 400 {
				tags = append(tags, "error")
			}

			entry := Entry{
				Level:     level,
				Service:   "observeid-api",
				Method:    r.Method,
				Path:      r.URL.Path,
				Status:    rw.status,
				Latency:   latency.Round(time.Millisecond).String(),
				Message:   msg,
				Detail:    detail,
				SourceIP:  r.RemoteAddr,
				Tags:      tags,
			}

			// Tamper-evident ledger write (post-response, ~1ms local).
			// The assigned chain hash rides along on the in-memory entry
			// so the UI can display it.
			if chain != nil {
				details, _ := json.Marshal(map[string]any{
					"method": r.Method, "path": r.URL.Path,
					"status": rw.status, "latency": entry.Latency,
				})
				_, hash, err := chain.Append(r.Context(), ChainEntry{
					EventType: "http_request",
					ActorID:   actorFromRequest(r),
					ActorType: "http",
					Action:    r.Method + " " + r.URL.Path,
					Resource:  r.URL.Path,
					Details:   details,
					IPAddress: r.RemoteAddr,
				})
				if err != nil {
					fmt.Printf("[WARN] audit chain append failed: %v\n", err)
				} else {
					entry.Hash = hash
				}
			}

			store.Append(entry)
		})
	}
}

// actorFromRequest extracts the authenticated user id from context if present.
func actorFromRequest(r *http.Request) string {
	if id := middleware.UserIDFromContext(r.Context()); id != "" {
		return id
	}
	return "anonymous"
}

type StoreStats struct {
	Total       int            `json:"total"`
	ByLevel     map[Level]int  `json:"by_level"`
	Capacity    int            `json:"capacity"`
	UsagePct    float64        `json:"usage_pct"`
}

func (s *Store) Stats() StoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := StoreStats{
		Total:    len(s.entries),
		Capacity: s.cap,
		ByLevel:  make(map[Level]int),
	}
	for _, e := range s.entries {
		stats.ByLevel[e.Level]++
	}
	if s.cap > 0 {
		stats.UsagePct = float64(len(s.entries)) / float64(s.cap) * 100
	}
	return stats
}
