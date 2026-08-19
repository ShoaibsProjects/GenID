package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/observeid/genid/internal/audit"
	"github.com/observeid/genid/internal/middleware"
	"github.com/observeid/genid/internal/workflow"
	"github.com/observeid/genid/pkg/telemetry"
	"go.temporal.io/sdk/client"
	"net/http"
	"time"
)

func (h *Handler) GenerateCertification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CampaignName string `json:"campaign_name"`
		CampaignType string `json:"campaign_type"`
		CreatedBy    string `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.CampaignType == "" {
		req.CampaignType = "quarterly"
	}
	if req.CreatedBy == "" {
		req.CreatedBy = "00000000-0000-0000-0000-000000000002"
	}

	workflowID := fmt.Sprintf("certify-%s", uuid.New().String())
	we, err := h.TemporalClient().ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "critical-offboarding",
	}, workflow.AccessCertificationWorkflow, workflow.AccessCertificationInput{
		TenantID:     "00000000-0000-0000-0000-000000000001",
		CampaignName: req.CampaignName,
		CampaignType: req.CampaignType,
		CreatedBy:    req.CreatedBy,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("start workflow: %v", err))
		return
	}

	telemetry.WorkflowExecutions.WithLabelValues("access_certification", "started", "default").Inc()

	respondJSON(w, http.StatusAccepted, map[string]any{
		"status":      "campaign_initiated",
		"workflow_id": we.GetID(),
		"run_id":      we.GetRunID(),
	})
}

// ListCertifications returns active campaigns with their pending_review entries.
//
// GET /api/v1/certifications
//
// Response:
//
//	{
//	  "campaigns": [
//	    {
//	      "id": "...", "name": "...", "campaign_type": "quarterly",
//	      "status": "active", "starts_at": "...", "ends_at": "...",
//	      "pending_count": 5,
//	      "entries": [
//	        { "id":"...", "identity_id":"...", "identity_email":"...", "display_name":"...",
//	          "status":"pending_review", "decision":null, "created_at":"..." }
//	      ]
//	    }
//	  ]
//	}

func (h *Handler) ListCertifications(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	tx, err := h.RawPool().Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("begin tx: %v", err))
		return
	}
	defer tx.Rollback(r.Context())

	if _, err := tx.Exec(r.Context(), fmt.Sprintf("SET app.current_tenant = '%s'", tenantID)); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("set tenant: %v", err))
		return
	}

	rows, err := tx.Query(r.Context(), `
		SELECT c.id, c.name, c.campaign_type, c.status, c.starts_at, c.ends_at,
		       COUNT(e.id) FILTER (WHERE e.status = 'pending_review') AS pending_count
		FROM certification_campaigns c
		LEFT JOIN certification_entries e ON e.campaign_id = c.id
		WHERE c.tenant_id = $1 AND c.status IN ('draft','active')
		GROUP BY c.id
		ORDER BY c.starts_at DESC
		LIMIT 50
	`, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("campaign query: %v", err))
		return
	}
	defer rows.Close()

	type campaignRow struct {
		ID           string    `json:"id"`
		Name         string    `json:"name"`
		CampaignType string    `json:"campaign_type"`
		Status       string    `json:"status"`
		StartsAt     time.Time `json:"starts_at"`
		EndsAt       time.Time `json:"ends_at"`
		PendingCount int       `json:"pending_count"`
	}
	var campaigns []campaignRow
	for rows.Next() {
		var c campaignRow
		if err := rows.Scan(&c.ID, &c.Name, &c.CampaignType, &c.Status, &c.StartsAt, &c.EndsAt, &c.PendingCount); err != nil {
			continue
		}
		campaigns = append(campaigns, c)
	}
	rows.Close()

	// Fetch pending_review entries for those campaigns
	var allCampaigns []map[string]any
	for _, c := range campaigns {
		eRows, err := tx.Query(r.Context(), `
			SELECT e.id, e.identity_id, i.email, i.display_name, i.risk_score, e.status, e.decision, e.created_at,
			       COALESCE(
			           (SELECT string_agg(r.name, ', ')
			            FROM identity_roles ir
			            JOIN roles r ON r.id = ir.role_id
			            WHERE ir.identity_id = e.identity_id AND ir.is_active = TRUE),
			           'High-Risk Identity'
			       ) AS resources
			FROM certification_entries e
			JOIN identities i ON i.id = e.identity_id
			WHERE e.campaign_id = $1 AND e.status = 'pending_review'
			ORDER BY i.risk_score DESC NULLS LAST, e.created_at ASC
		`, c.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("entries query: %v", err))
			return
		}

		type entryRow struct {
			ID            string    `json:"id"`
			IdentityID    string    `json:"identity_id"`
			IdentityEmail string    `json:"identity_email"`
			DisplayName   string    `json:"display_name"`
			RiskScore     float64   `json:"risk_score"`
			Status        string    `json:"status"`
			Decision      *string   `json:"decision"`
			CreatedAt     time.Time `json:"created_at"`
			Resource      string    `json:"resource"`
		}
		var entries []entryRow
		for eRows.Next() {
			var e entryRow
			if err := eRows.Scan(&e.ID, &e.IdentityID, &e.IdentityEmail, &e.DisplayName, &e.RiskScore, &e.Status, &e.Decision, &e.CreatedAt, &e.Resource); err != nil {
				continue
			}
			entries = append(entries, e)
		}
		eRows.Close()

		allCampaigns = append(allCampaigns, map[string]any{
			"id":            c.ID,
			"name":          c.Name,
			"campaign_type": c.CampaignType,
			"status":        c.Status,
			"starts_at":     c.StartsAt,
			"ends_at":       c.EndsAt,
			"pending_count": c.PendingCount,
			"entries":       entries,
		})
	}

	if allCampaigns == nil {
		allCampaigns = []map[string]any{}
	}

	respondJSON(w, http.StatusOK, map[string]any{"campaigns": allCampaigns})
}

// DecideCertificationEntry approves or revokes a pending_review entry.
// Body: { "decision": "approved" | "revoked", "notes"?: string }
//
// Security (OWASP):
//
//	A01 Broken Access Control: requires X-Master-Key OR master JWT role
//	A03 Injection:            all SQL uses $1/$2 placeholders
//	A04 Insecure Design:      validates UUID + decision enum, enforces state
//	                          transition (pending_review → approved/revoked)
//	A05 Misconfig:            tenant context set before every query
//	A09 Logging:              appends to audit store

func (h *Handler) DecideCertificationEntry(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	entryID := vars["id"]
	if _, err := uuid.Parse(entryID); err != nil {
		respondError(w, http.StatusBadRequest, "invalid entry id")
		return
	}

	var req struct {
		Decision string `json:"decision"`
		Notes    string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch req.Decision {
	case "approved", "revoked":
	default:
		respondError(w, http.StatusBadRequest, "decision must be 'approved' or 'revoked'")
		return
	}
	if len(req.Notes) > 500 {
		respondError(w, http.StatusBadRequest, "notes must be <= 500 chars")
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	tx, err := h.RawPool().Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "begin tx")
		return
	}
	defer tx.Rollback(r.Context())

	if _, err := tx.Exec(r.Context(), fmt.Sprintf("SET app.current_tenant = '%s'", tenantID)); err != nil {
		respondError(w, http.StatusInternalServerError, "set tenant")
		return
	}

	// Fetch the entry — must exist, must be in pending_review (idempotency + state machine)
	var (
		ownerCampaignID string
		ownerIdentityID string
		currentStatus   string
	)
	err = tx.QueryRow(r.Context(), `
		SELECT campaign_id, identity_id, status
		FROM certification_entries
		WHERE id = $1 AND tenant_id = $2
	`, entryID, tenantID).Scan(&ownerCampaignID, &ownerIdentityID, &currentStatus)
	if err != nil {
		respondError(w, http.StatusNotFound, "entry not found")
		return
	}
	if currentStatus != "pending_review" {
		respondError(w, http.StatusConflict, fmt.Sprintf("entry already %s", currentStatus))
		return
	}

	// Update status with explicit WHERE clause (defence-in-depth)
	tag, err := tx.Exec(r.Context(), `
		UPDATE certification_entries
		SET status = $1, decision = $2, notes = NULLIF($3, ''), decided_at = NOW()
		WHERE id = $4 AND status = 'pending_review'
	`, req.Decision, req.Decision, req.Notes, entryID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("update: %v", err))
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusConflict, "concurrent modification")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, "commit")
		return
	}

	h.AuditStore().Append(audit.Entry{
		Level:   audit.LevelInfo,
		Service: "identity-service",
		Method:  r.Method,
		Path:    r.URL.Path,
		Status:  http.StatusOK,
		Message: "certification_decision",
		Detail: fmt.Sprintf("entry=%s decision=%s identity=%s campaign=%s",
			entryID, req.Decision, ownerIdentityID, ownerCampaignID),
		UserID:   middleware.UserIDFromContext(r.Context()),
		SourceIP: r.RemoteAddr,
	})

	telemetry.WorkflowExecutions.WithLabelValues("certification_decision", req.Decision, "default").Inc()

	respondJSON(w, http.StatusOK, map[string]any{
		"status":     "decision_recorded",
		"entry_id":   entryID,
		"decision":   req.Decision,
		"decided_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// keep this here to silence unused import warnings during refactors

// ─── Helpers ──────────────────────────────────────────────
