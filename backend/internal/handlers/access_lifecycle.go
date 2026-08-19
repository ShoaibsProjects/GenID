package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/observeid/genid/internal/risk"
	"github.com/observeid/genid/internal/services"
	"github.com/observeid/genid/internal/workflow"
	"go.temporal.io/sdk/client"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (h *Handler) FirecallAccess(w http.ResponseWriter, r *http.Request) {
	var req workflow.FirecallInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid firecall request: "+err.Error())
		return
	}
	if req.IdentityID == "" || req.ResourceID == "" {
		respondError(w, http.StatusBadRequest, "identity_id and resource_id are required")
		return
	}
	if req.DurationMins <= 0 {
		req.DurationMins = 60
	}
	if req.DurationMins > 240 {
		req.DurationMins = 240
	}
	if req.TenantID == "" {
		req.TenantID = "00000000-0000-0000-0000-000000000001"
	}

	// 1) Persist request row first (so the UI can show status immediately)
	idemKey := r.Header.Get("Idempotency-Key")
	payload, _ := json.Marshal(map[string]any{
		"resource_id":   req.ResourceID,
		"resource_type": req.ResourceType,
		"reason":        req.Reason,
		"justification": req.Justification,
		"duration_mins": req.DurationMins,
		"incident_id":   req.IncidentID,
	})
	expiresAt := time.Now().Add(time.Duration(req.DurationMins+7*24*60) * time.Minute) // access + 7d review window
	wfReq := &workflow.Request{
		TenantID:       req.TenantID,
		Type:           "access.request.firecall",
		Status:         "pending",
		RequesterID:    req.RequestedBy,
		TargetID:       req.IdentityID,
		Payload:        payload,
		IdempotencyKey: idemKey,
		ExpiresAt:      &expiresAt,
	}
	created, err := h.WorkflowStore().CreateRequest(r.Context(), wfReq)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create firecall request: "+err.Error())
		return
	}
	if !created {
		// Idempotency-key replay: return the existing request, do NOT start a new workflow.
		existing, gerr := h.WorkflowStore().GetRequest(r.Context(), wfReq.ID)
		if gerr == nil {
			respondJSON(w, http.StatusOK, map[string]any{
				"request_id":           existing.ID,
				"status":               existing.Status,
				"type":                 existing.Type,
				"idempotent_replay":    true,
				"temporal_workflow_id": existing.TemporalWorkflowID,
				"created_at":           existing.CreatedAt.Format(time.RFC3339),
			})
		} else {
			respondJSON(w, http.StatusOK, map[string]any{
				"request_id":        wfReq.ID,
				"status":            wfReq.Status,
				"type":              wfReq.Type,
				"idempotent_replay": true,
			})
		}
		return
	}
	_ = h.WorkflowStore().AppendAudit(r.Context(), wfReq.ID, "workflow.requested", "user:"+req.RequestedBy, map[string]any{
		"type":          "access.request.firecall",
		"target_id":     req.IdentityID,
		"resource_id":   req.ResourceID,
		"duration_mins": req.DurationMins,
		"justification": req.Justification,
		"incident_id":   req.IncidentID,
	})

	// 2) Start Temporal workflow
	wfID := "firecall-" + wfReq.ID
	workflowOptions := client.StartWorkflowOptions{
		ID:        wfID,
		TaskQueue: "genid-identity-task-queue",
	}
	run, err := h.TemporalClient().ExecuteWorkflow(r.Context(), workflowOptions, workflow.FirecallAccessWorkflow, req)
	if err != nil {
		_ = h.WorkflowStore().FailRequest(r.Context(), wfReq.ID, "temporal start failed: "+err.Error())
		respondError(w, http.StatusInternalServerError, "Failed to start firecall workflow: "+err.Error())
		return
	}
	if err := h.WorkflowStore().SetTemporalIDs(r.Context(), wfReq.ID, run.GetID(), run.GetRunID()); err != nil {
		log.Printf("[FIRECALL] set temporal ids failed: %v", err)
	}
	if err := h.WorkflowStore().UpdateRequestStatus(r.Context(), wfReq.ID, "approved"); err != nil { // firecall = auto-approved
		log.Printf("[FIRECALL] update status failed: %v", err)
	}
	_ = h.WorkflowStore().AppendAudit(r.Context(), wfReq.ID, "workflow.approved", "system", map[string]any{
		"reason": "firecall auto-approved; post-review required within 7d",
	})

	log.Printf("[FIRECALL] granted identity=%s resource=%s duration=%dm incident=%s request=%s",
		req.IdentityID, req.ResourceID, req.DurationMins, req.IncidentID, wfReq.ID)

	respondJSON(w, http.StatusAccepted, map[string]any{
		"request_id":           wfReq.ID,
		"temporal_workflow_id": run.GetID(),
		"temporal_run_id":      run.GetRunID(),
		"status":               "approved",
		"type":                 "access.request.firecall",
		"duration_mins":        req.DurationMins,
		"expires_at":           expiresAt.Format(time.RFC3339),
		"post_review_due":      time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339),
		"warning":              "BREAK-GLASS — post-event security review required",
	})
}

// GetRequest returns one workflow request + its approvals + audit trail.

func (h *Handler) GetRequest(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}
	req, err := h.WorkflowStore().GetRequest(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Request not found")
		return
	}
	approvals, _ := h.WorkflowStore().ListApprovals(r.Context(), id)
	audit, _ := h.WorkflowStore().ListAudit(r.Context(), id)
	respondJSON(w, http.StatusOK, map[string]any{
		"request":   req,
		"approvals": approvals,
		"audit":     audit,
	})
}

// ListRequests returns recent workflow requests filtered by status / type.

func (h *Handler) ListRequests(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	reqType := q.Get("type")
	limit, _ := strconv.Atoi(q.Get("limit"))
	rows, err := h.WorkflowStore().ListRequests(r.Context(), status, reqType, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []*workflow.Request{}
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"requests": rows,
		"total":    len(rows),
	})
}

// DecideApproval records an approver decision on one approval row and
// forwards it to the running ApprovalGateWorkflow (child of the grant
// workflow) via the ApprovalDecision signal. The gate advances or
// denies the chain; the request row is updated by the workflow.

func (h *Handler) DecideApproval(w http.ResponseWriter, r *http.Request) {
	approvalID := mux.Vars(r)["approval_id"]
	if approvalID == "" {
		respondError(w, http.StatusBadRequest, "approval_id is required")
		return
	}
	var body struct {
		ApproverID string `json:"approver_id"`
		Approved   bool   `json:"approved"`
		Comment    string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	if body.ApproverID == "" {
		respondError(w, http.StatusBadRequest, "approver_id is required")
		return
	}

	// 1) Persist the decision on the approval row (immutable once decided).
	status := "denied"
	if body.Approved {
		status = "approved"
	}
	approval, err := h.WorkflowStore().DecideApproval(r.Context(), approvalID, body.ApproverID, status, body.Comment)
	if err != nil {
		respondError(w, http.StatusConflict, err.Error())
		return
	}
	_ = h.WorkflowStore().AppendAudit(r.Context(), approval.RequestID, "approval."+status,
		"user:"+body.ApproverID, map[string]any{
			"approval_id": approvalID,
			"level":       approval.Level,
			"comment":     body.Comment,
		})

	// 2) Signal the gate workflow so it advances (best-effort; the gate
	//    also reads decisions from the store on replay).
	workflowID := "approval-gate-" + approval.RequestID
	if err := h.TemporalClient().SignalWorkflow(r.Context(), workflowID, "", workflow.ApprovalSignalName, workflow.ApprovalDecision{
		ApprovalID: approvalID,
		ApproverID: body.ApproverID,
		Approved:   body.Approved,
		Comment:    body.Comment,
	}); err != nil {
		log.Printf("[APPROVAL] signal gate failed (decision persisted anyway): %v", err)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"approval_id": approvalID,
		"request_id":  approval.RequestID,
		"status":      approval.Status,
		"decided_at":  approval.DecidedAt,
	})
}

// ListPendingApprovals returns all pending approval rows for the
// approval inbox. Optional ?approver_id= filter.

func (h *Handler) ListPendingApprovals(w http.ResponseWriter, r *http.Request) {
	approverID := r.URL.Query().Get("approver_id")
	rows, err := h.WorkflowStore().ListPendingApprovals(r.Context(), approverID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []*workflow.Approval{}
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"approvals": rows,
		"total":     len(rows),
	})
}

// DelegateApproval reassigns a pending approval to another approver.

func (h *Handler) DelegateApproval(w http.ResponseWriter, r *http.Request) {
	approvalID := mux.Vars(r)["approval_id"]
	if approvalID == "" {
		respondError(w, http.StatusBadRequest, "approval_id is required")
		return
	}
	var body struct {
		ToApproverID string `json:"to_approver_id"`
		ToEmail      string `json:"to_email"`
		ToRole       string `json:"to_role"`
		DelegatedBy  string `json:"delegated_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	approval, err := h.WorkflowStore().DelegateApproval(r.Context(), approvalID, body.ToApproverID, body.ToEmail, body.ToRole)
	if err != nil {
		respondError(w, http.StatusConflict, err.Error())
		return
	}
	_ = h.WorkflowStore().AppendAudit(r.Context(), approval.RequestID, "approval.delegated",
		"user:"+body.DelegatedBy, map[string]any{
			"approval_id": approvalID,
			"to_approver": body.ToApproverID,
			"to_email":    body.ToEmail,
			"to_role":     body.ToRole,
		})
	respondJSON(w, http.StatusOK, map[string]any{
		"approval_id":    approval.ID,
		"request_id":     approval.RequestID,
		"status":         approval.Status,
		"approver_id":    approval.ApproverID,
		"approver_email": approval.ApproverEmail,
		"approver_role":  approval.ApproverRole,
	})
}

// ListRoleCatalog returns requestable roles (approval_required +
// active, not auto-assigned) for the self-service catalog.

func (h *Handler) ListRoleCatalog(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB(r.Context()).Query(r.Context(), `
		SELECT id, tenant_id, name, description, role_type,
		       is_auto_assigned, approval_required, max_duration_hours,
		       is_active, attributes, created_at, updated_at
		FROM roles
		WHERE is_active = TRUE AND approval_required = TRUE AND is_auto_assigned = FALSE
		ORDER BY name ASC
	`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Catalog query failed")
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, tenantID, name, roleType string
		var description, attributes any
		var autoAssigned, approvalRequired, isActive bool
		var maxDuration *int
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &tenantID, &name, &description, &roleType,
			&autoAssigned, &approvalRequired, &maxDuration, &isActive, &attributes,
			&createdAt, &updatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "Catalog scan failed")
			return
		}
		out = append(out, map[string]any{
			"id":                 id,
			"tenant_id":          tenantID,
			"name":               name,
			"description":        description,
			"role_type":          roleType,
			"approval_required":  approvalRequired,
			"max_duration_hours": maxDuration,
		})
	}
	respondJSON(w, http.StatusOK, map[string]any{"roles": out, "total": len(out)})
}

// RequestRoleAccess creates a self-service role request routed through
// the approval engine (GrantAccessWorkflow with RoleID + gate).

func (h *Handler) RequestRoleAccess(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RoleID        string `json:"role_id"`
		IdentityID    string `json:"identity_id"`
		RequestedBy   string `json:"requested_by"`
		Reason        string `json:"reason"`
		DurationHours int    `json:"duration_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	if body.RoleID == "" || body.IdentityID == "" {
		respondError(w, http.StatusBadRequest, "role_id and identity_id are required")
		return
	}
	if body.RequestedBy == "" {
		body.RequestedBy = body.IdentityID
	}

	// Role must be requestable.
	var approved, active bool
	if err := h.DB(r.Context()).QueryRow(r.Context(), `
		SELECT approval_required, is_active FROM roles WHERE id = $1
	`, body.RoleID).Scan(&approved, &active); err != nil {
		respondError(w, http.StatusNotFound, "role not found")
		return
	}
	if !active || !approved {
		respondError(w, http.StatusBadRequest, "role is not requestable")
		return
	}

	// Reuse the grant flow: RoleID set, requires approval, no resource.
	req := workflow.GrantAccessInput{
		TenantID:         "00000000-0000-0000-0000-000000000001",
		IdentityID:       body.IdentityID,
		RoleID:           body.RoleID,
		ResourceType:     "role",
		Reason:           body.Reason,
		RequestedBy:      body.RequestedBy,
		RequiresApproval: true,
		DurationHours:    body.DurationHours,
	}
	if score, _, err := risk.CalculateIdentityRisk(r.Context(), h.Neo4j(), h.RawPool(), req.TenantID, req.IdentityID); err == nil {
		req.RiskBand = services.RiskBandFromScore(score)
	}

	idemKey := r.Header.Get("Idempotency-Key")
	payload, _ := json.Marshal(map[string]any{
		"role_id":        body.RoleID,
		"reason":         body.Reason,
		"duration_hours": body.DurationHours,
	})
	wfReq := &workflow.Request{
		TenantID:       req.TenantID,
		Type:           "access.request.role",
		Status:         "pending",
		RequesterID:    req.RequestedBy,
		TargetID:       req.IdentityID,
		Payload:        payload,
		IdempotencyKey: idemKey,
	}
	created, err := h.WorkflowStore().CreateRequest(r.Context(), wfReq)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create role request: "+err.Error())
		return
	}
	if !created {
		respondJSON(w, http.StatusOK, map[string]any{
			"request_id":        wfReq.ID,
			"status":            wfReq.Status,
			"type":              "access.request.role",
			"idempotent_replay": true,
		})
		return
	}
	_ = h.WorkflowStore().AppendAudit(r.Context(), wfReq.ID, "workflow.requested", "user:"+req.RequestedBy, map[string]any{
		"type":      "access.request.role",
		"role_id":   body.RoleID,
		"target_id": req.IdentityID,
	})
	req.RequestID = wfReq.ID

	workflowID := fmt.Sprintf("request-role-%s-%s", body.IdentityID, uuid.New().String()[:8])
	we, err := h.TemporalClient().ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "critical-offboarding",
	}, workflow.GrantAccessWorkflow, req)
	if err != nil {
		_ = h.WorkflowStore().FailRequest(r.Context(), wfReq.ID, "temporal start failed: "+err.Error())
		respondError(w, http.StatusInternalServerError, "Failed to start role request workflow")
		return
	}
	if err := h.WorkflowStore().SetTemporalIDs(r.Context(), wfReq.ID, we.GetID(), we.GetRunID()); err != nil {
		log.Printf("[ROLE-REQ] set temporal ids failed: %v", err)
	}
	respondJSON(w, http.StatusAccepted, map[string]any{
		"request_id":           wfReq.ID,
		"status":               wfReq.Status,
		"type":                 "access.request.role",
		"temporal_workflow_id": we.GetID(),
		"risk_band":            req.RiskBand,
	})
}

// RevokeJITSession sends a signal to the Temporal workflow to revoke
// a JIT or Firecall session early. Workflow auto-revokes on timer
// expiry otherwise.

func (h *Handler) RevokeJITSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IdentityID string `json:"identity_id"`
		ResourceID string `json:"resource_id"`
		Reason     string `json:"reason"`
		RevokedBy  string `json:"revoked_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	// Pattern-delete Redis JIT grants + remove Neo4j HAS_TEMPORARY_ACCESS relationship
	if err := h.RevokeTemporaryAccess(r.Context(), body.IdentityID, body.ResourceID, body.Reason, body.RevokedBy); err != nil {
		respondError(w, http.StatusInternalServerError, "Revoke failed: "+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"status": "revoked", "ts": time.Now().UTC()})
}

// getTenantZSPConfig fetches the ZSP configuration for a tenant.
func (h *Handler) getTenantZSPConfig(ctx context.Context, tenantID string) (enabled bool, maxJIT time.Duration, overrideApproval bool, err error) {
	err = h.DB(ctx).QueryRow(ctx, `
		SELECT zsp_enabled, zsp_max_jit_duration, zsp_override_requires_approval
		FROM tenants WHERE id = $1
	`, tenantID).Scan(&enabled, &maxJIT, &overrideApproval)
	if err != nil {
		return false, 0, false, err
	}
	return enabled, maxJIT, overrideApproval, nil
}

// applyZSPConfig applies Zero Standing Privilege rules to a grant request.
func (h *Handler) applyZSPConfig(req *workflow.GrantAccessInput) error {
	enabled, maxJIT, _, err := h.getTenantZSPConfig(context.Background(), req.TenantID)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	// Force approval required.
	req.RequiresApproval = true

	// Cap duration to max JIT duration.
	maxHours := int(maxJIT.Hours())
	if maxHours <= 0 {
		maxHours = 2 // fallback
	}
	if req.DurationHours == 0 || req.DurationHours > maxHours {
		req.DurationHours = maxHours
	}

	// If override_requires_approval is set and caller tries to bypass
	// (e.g., by setting RequiresApproval=false or DurationHours > max),
	// we've already forced approval and capped duration.
	// The workflow's conditional access will route to approval anyway.
	return nil
}

// GetZSPConfig implements GET /api/v1/tenants/{id}/zsp.
func (h *Handler) GetZSPConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tenantID := vars["id"]

	enabled, maxJIT, overrideApproval, err := h.getTenantZSPConfig(r.Context(), tenantID)
	if err != nil {
		respondError(w, http.StatusNotFound, "tenant not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"tenant_id":                      tenantID,
		"zsp_enabled":                    enabled,
		"zsp_max_jit_duration_hours":     int(maxJIT.Hours()),
		"zsp_override_requires_approval": overrideApproval,
	})
}

// UpdateZSPConfig implements PATCH /api/v1/tenants/{id}/zsp.
func (h *Handler) UpdateZSPConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tenantID := vars["id"]

	var req struct {
		Enabled                  *bool `json:"zsp_enabled"`
		MaxJITDurationHours      *int  `json:"zsp_max_jit_duration_hours"`
		OverrideRequiresApproval *bool `json:"zsp_override_requires_approval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	setParts := []string{}
	args := []any{tenantID}
	argIdx := 2

	if req.Enabled != nil {
		setParts = append(setParts, fmt.Sprintf("zsp_enabled = $%d", argIdx))
		args = append(args, *req.Enabled)
		argIdx++
	}
	if req.MaxJITDurationHours != nil {
		if *req.MaxJITDurationHours < 1 || *req.MaxJITDurationHours > 168 {
			respondError(w, http.StatusBadRequest, "zsp_max_jit_duration_hours must be 1..168")
			return
		}
		setParts = append(setParts, fmt.Sprintf("zsp_max_jit_duration = $%d * interval '1 hour'", argIdx))
		args = append(args, *req.MaxJITDurationHours)
		argIdx++
	}
	if req.OverrideRequiresApproval != nil {
		setParts = append(setParts, fmt.Sprintf("zsp_override_requires_approval = $%d", argIdx))
		args = append(args, *req.OverrideRequiresApproval)
		argIdx++
	}
	if len(setParts) == 0 {
		respondError(w, http.StatusBadRequest, "No fields to update")
		return
	}
	setParts = append(setParts, "updated_at = NOW()")

	query := fmt.Sprintf("UPDATE tenants SET %s WHERE id = $1", strings.Join(setParts, ", "))
	_, err := h.DB(r.Context()).Exec(r.Context(), query, args...)
	if err != nil {
		logError("zsp-update", err)
		respondError(w, http.StatusInternalServerError, "Failed to update ZSP config")
		return
	}

	// Return updated config.
	h.GetZSPConfig(w, r)
}
