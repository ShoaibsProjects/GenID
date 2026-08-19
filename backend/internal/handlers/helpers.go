package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

func paginationParams(r *http.Request, defaultLimit, defaultOffset int) (int, int) {
	limit := defaultLimit
	offset := defaultOffset
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}

func logError(component string, err error) {
	fmt.Printf("[ERROR] %s: %v\n", component, err)
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}

// ─── Identity Automation: Firecall + Request Lifecycle ───────────

// FirecallAccess handles a break-glass request. Auto-grants, time-bounded
// (default 1h, hard cap 4h), and mandates a post-event security review.
