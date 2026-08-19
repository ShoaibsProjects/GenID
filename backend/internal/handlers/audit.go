package handlers

import (
	"context"
	"github.com/gorilla/mux"
	"github.com/observeid/genid/internal/audit"
	"net/http"
	"strconv"
	"time"
)

func (h *Handler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	f := audit.Filter{
		Level:    audit.Level(q.Get("level")),
		Method:   q.Get("method"),
		Path:     q.Get("path"),
		SourceIP: q.Get("source_ip"),
	}
	if s := q.Get("status"); s != "" {
		f.Status, _ = strconv.Atoi(s)
	}
	if s := q.Get("since"); s != "" {
		f.Since, _ = time.Parse(time.RFC3339, s)
	}
	if s := q.Get("until"); s != "" {
		f.Until, _ = time.Parse(time.RFC3339, s)
	}

	entries := h.AuditStore().List(limit, offset, f)
	if entries == nil {
		entries = []audit.Entry{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"total":   len(entries),
		"stats":   h.AuditStore().Stats(),
	})
}

func (h *Handler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	entry, ok := h.AuditStore().Get(id)
	if !ok {
		respondError(w, http.StatusNotFound, "Log entry not found")
		return
	}
	respondJSON(w, http.StatusOK, entry)
}

func (h *Handler) GetAuditLogStats(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, h.AuditStore().Stats())
}

// VerifyAuditChain replays the entire tamper-evident ledger and recomputes
// every SHA-256 chain hash. Returns {"status":"intact"} or
// {"status":"tampered","broken_at":"<row_id>"}.
//
// GET /api/v1/audit/verify

func (h *Handler) VerifyAuditChain(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := h.AuditChain().Verify(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "chain verification failed: "+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// ─── Access Certification ─────────────────────────────────

// GenerateCertification triggers the Temporal AccessCertificationWorkflow which
// scans PostgreSQL for identities with critical access, creates a campaign row,
// and inserts one certification_entries row (status='pending_review') per identity.
//
// POST /api/v1/certifications/generate
// Body: { campaign_name?: string, campaign_type?: "quarterly"|"triggered"|"emergency" }
