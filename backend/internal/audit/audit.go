package audit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Level string

const (
	LevelDebug   Level = "debug"
	LevelInfo    Level = "info"
	LevelWarn    Level = "warn"
	LevelError   Level = "error"
	LevelFatal   Level = "fatal"
)

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

func (s *Store) List(limit, offset int, level Level, path string) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.entries)
	if total == 0 {
		return nil
	}

	var filtered []Entry
	for _, e := range s.entries {
		if level != "" && e.Level != level {
			continue
		}
		if path != "" && !strings.Contains(e.Path, path) {
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

func LoggingMiddleware(store *Store) func(http.Handler) http.Handler {
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

			store.Append(Entry{
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
			})
		})
	}
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
