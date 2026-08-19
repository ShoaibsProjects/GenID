package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/observeid/genid/internal/cedar"
	"github.com/observeid/genid/internal/risk"
	"github.com/observeid/genid/internal/services"
	"github.com/observeid/genid/internal/workflow"
	"github.com/observeid/genid/pkg/telemetry"
	"go.temporal.io/sdk/client"
	"log"
	"net/http"
	"time"
)

func (h *Handler) CheckAccess(w http.ResponseWriter, r *http.Request) {
	tenantID := "default"
	start := time.Now()

	var req struct {
		IdentityID string `json:"identity_id"`
		ResourceID string `json:"resource_id"`
		Action     string `json:"action"`
		TenantID   string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// Check sticky revocation cache
	recent, err := h.Redis().Exists(r.Context(), fmt.Sprintf("revocation:recent:%s", req.IdentityID)).Result()
	if err != nil {
		// Redis down — fall through (fail open for availability)
		logError("redis", fmt.Errorf("revocation check: %w", err))
	} else if recent > 0 {
		respondJSON(w, http.StatusOK, map[string]any{
			"allowed":   false,
			"reason":    "recent_revocation",
			"evaluated": "redis_cache",
		})
		return
	}

	// Check Redis policy decision cache
	cacheKey := fmt.Sprintf("policy:decision:%s:%s:%s", req.IdentityID, req.ResourceID, req.Action)
	if cached, err := h.Redis().Get(r.Context(), cacheKey).Bytes(); err == nil && len(cached) > 0 {
		var decision map[string]any
		if json.Unmarshal(cached, &decision) == nil {
			respondJSON(w, http.StatusOK, map[string]any{
				"allowed":    decision["allowed"],
				"reason":     decision["reason"],
				"evaluated":  "cedar_cached",
				"latency_ms": 1,
			})
			return
		}
	}

	start = time.Now()

	// Query Neo4j for entitlement path through the unified governance graph:
	// Identity-[:HAS_ROLE]->Role-[:GRANTS]->Entitlement-[:ACCESSES]->Resource
	// plus direct/temporary access edges.
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	query := `
		MATCH (i {uuid: $identityId}) WHERE i:Identity OR i:NonHumanIdentity
		OPTIONAL MATCH pathRole = (i)-[:HAS_ROLE]->(:Role)-[:GRANTS]->(:Entitlement)-[:ACCESSES]->(res:Resource {uuid: $resourceId})
		OPTIONAL MATCH pathDirectEnt = (i)-[:HAS_ENTITLEMENT]->(:Entitlement)-[:ACCESSES]->(res:Resource {uuid: $resourceId})
		OPTIONAL MATCH pathDirect = (i)-[:HAS_DIRECT_ACCESS]->(res:Resource {uuid: $resourceId})
		OPTIONAL MATCH pathTemp = (i)-[:HAS_TEMPORARY_ACCESS]->(res:Resource {uuid: $resourceId})
		RETURN
			i.status AS identityStatus,
			CASE WHEN pathRole IS NOT NULL OR pathDirectEnt IS NOT NULL OR pathDirect IS NOT NULL OR pathTemp IS NOT NULL
				THEN true ELSE false END AS hasPath,
			CASE
				WHEN pathRole IS NOT NULL THEN length(pathRole)
				WHEN pathDirectEnt IS NOT NULL THEN length(pathDirectEnt)
				WHEN pathDirect IS NOT NULL THEN length(pathDirect)
				WHEN pathTemp IS NOT NULL THEN length(pathTemp)
				ELSE 0
			END AS pathLength
	`

	result, err := session.Run(r.Context(), query, map[string]any{
		"identityId": req.IdentityID,
		"resourceId": req.ResourceID,
	})
	if err != nil {
		logError("neo4j", fmt.Errorf("access check query: %w", err))
		respondError(w, http.StatusInternalServerError, "Access evaluation failed")
		return
	}

	var identityStatus string
	hasPath := false

	if result.Next(r.Context()) {
		rec := result.Record()
		if status, _ := rec.Get("identityStatus"); status != nil {
			identityStatus, _ = status.(string)
		}
		if path, _ := rec.Get("hasPath"); path != nil {
			hasPath, _ = path.(bool)
		}
	}

	// If identity is revoked or suspended, deny
	if identityStatus == "revoked" || identityStatus == "suspended" {
		respondJSON(w, http.StatusOK, map[string]any{
			"allowed":    false,
			"reason":     fmt.Sprintf("identity_%s", identityStatus),
			"evaluated":  "neo4j",
			"latency_ms": time.Since(start).Milliseconds(),
		})
		return
	}

	// If no entitlement path found, deny
	if !hasPath {
		respondJSON(w, http.StatusOK, map[string]any{
			"allowed":    false,
			"reason":     "no_entitlement_path",
			"evaluated":  "neo4j",
			"latency_ms": time.Since(start).Milliseconds(),
		})
		return
	}

	// Check Cedar policies via real Cedar engine
	var identityType, identityDept string
	var isActive bool
	_ = h.DB(r.Context()).QueryRow(r.Context(),
		`SELECT COALESCE(type::text, 'User'), COALESCE(department, ''),
		        (status = 'active')
		 FROM identities WHERE id = $1`, req.IdentityID,
	).Scan(&identityType, &identityDept, &isActive)

	var resourceType, resourceClass string
	_ = h.DB(r.Context()).QueryRow(r.Context(),
		`SELECT COALESCE(resource_type, 'Resource'), COALESCE(criticality, '')
		 FROM resources WHERE id = $1`, req.ResourceID,
	).Scan(&resourceType, &resourceClass)

	cedarDecision, cedarErr := h.CedarEngine().IsAuthorized(r.Context(), cedar.AuthRequest{
		PrincipalID:   req.IdentityID,
		PrincipalType: identityType,
		Action:        req.Action,
		ResourceID:    req.ResourceID,
		ResourceType:  resourceType,
		TenantID:      req.TenantID,
		Department:    identityDept,
		IsActive:      isActive,
		Criticality:   resourceClass,
		MFAPresent:    true,
	})
	if cedarErr != nil {
		logError("cedar", fmt.Errorf("cedar evaluation: %w", cedarErr))
	}

	var allowed bool
	var reason string
	if cedarErr == nil && cedarDecision.Decision != "not_applicable" {
		allowed = cedarDecision.Allowed
		reason = fmt.Sprintf("cedar_%s", cedarDecision.Decision)
	} else {
		allowed = hasPath
		reason = "default_allow_by_path"
	}

	latency := time.Since(start).Milliseconds()

	// Cache decision for fast subsequent checks
	cacheVal, _ := json.Marshal(map[string]any{
		"allowed": allowed,
		"reason":  reason,
	})
	h.Redis().Set(r.Context(), cacheKey, cacheVal, 30*time.Second)

	// Record metrics
	metricDecision := "deny"
	if allowed {
		metricDecision = "permit"
	}
	telemetry.AccessCheckTotal.WithLabelValues(metricDecision, tenantID).Inc()
	telemetry.AccessCheckLatency.WithLabelValues(tenantID).Observe(float64(latency))
	if !allowed {
		telemetry.CedarDenyRate.WithLabelValues("human", req.Action, "resource").Inc()
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"allowed":    allowed,
		"reason":     reason,
		"evaluated":  "neo4j+cedar",
		"latency_ms": latency,
	})
}

func (h *Handler) GrantAccess(w http.ResponseWriter, r *http.Request) {
	var req workflow.GrantAccessInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if req.TenantID == "" {
		req.TenantID = "00000000-0000-0000-0000-000000000001"
	}
	if req.RequestedBy == "" {
		req.RequestedBy = req.TenantID
	}
	if req.DeviceTrust == "" {
		// X-Device-Trust header feeds conditional-access enrichment.
		req.DeviceTrust = r.Header.Get("X-Device-Trust")
	}

	// Apply ZSP (Zero Standing Privilege) rules if tenant has it enabled.
	if err := h.applyZSPConfig(&req); err != nil {
		logError("zsp-config", err)
		respondError(w, http.StatusInternalServerError, "ZSP config error")
		return
	}

	// 1) Persist the request row so the UI shows it immediately.
	idemKey := r.Header.Get("Idempotency-Key")
	payload, _ := json.Marshal(map[string]any{
		"resource_id":       req.ResourceID,
		"resource_type":     req.ResourceType,
		"role_id":           req.RoleID,
		"reason":            req.Reason,
		"duration_hours":    req.DurationHours,
		"requires_approval": req.RequiresApproval,
	})
	wfReq := &workflow.Request{
		TenantID:       req.TenantID,
		Type:           "access.request.grant",
		Status:         "pending",
		RequesterID:    req.RequestedBy,
		TargetID:       req.IdentityID,
		Payload:        payload,
		IdempotencyKey: idemKey,
	}
	created, err := h.WorkflowStore().CreateRequest(r.Context(), wfReq)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create grant request: "+err.Error())
		return
	}
	if !created {
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
		"type":           "access.request.grant",
		"target_id":      req.IdentityID,
		"resource_id":    req.ResourceID,
		"duration_hours": req.DurationHours,
		"reason":         req.Reason,
	})
	req.RequestID = wfReq.ID

	// 2) Derive risk band for approval routing (best-effort).
	if req.RiskBand == "" {
		if score, _, err := risk.CalculateIdentityRisk(r.Context(), h.Neo4j(), h.RawPool(), req.TenantID, req.IdentityID); err == nil {
			req.RiskBand = services.RiskBandFromScore(score)
		}
	}

	workflowID := fmt.Sprintf("grant-access-%s-%s", req.IdentityID, uuid.New().String()[:8])
	we, err := h.TemporalClient().ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "critical-offboarding",
	}, workflow.GrantAccessWorkflow, req)
	if err != nil {
		_ = h.WorkflowStore().FailRequest(r.Context(), wfReq.ID, "temporal start failed: "+err.Error())
		respondError(w, http.StatusInternalServerError, "Failed to start grant workflow")
		return
	}
	if err := h.WorkflowStore().SetTemporalIDs(r.Context(), wfReq.ID, we.GetID(), we.GetRunID()); err != nil {
		log.Printf("[GRANT] set temporal ids failed: %v", err)
	}

	telemetry.WorkflowExecutions.WithLabelValues("grant_access", "started", "default").Inc()

	respondJSON(w, http.StatusAccepted, map[string]any{
		"request_id":           wfReq.ID,
		"status":               wfReq.Status,
		"type":                 "access.request.grant",
		"temporal_workflow_id": we.GetID(),
		"risk_band":            req.RiskBand,
	})
}

// h.RiskBandFromScore maps a 0-1000 risk score to the band names used
// by the approval routing engine.

func (h *Handler) JustInTimeAccess(w http.ResponseWriter, r *http.Request) {
	var req workflow.JustInTimeInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if req.TenantID == "" {
		req.TenantID = "00000000-0000-0000-0000-000000000001"
	}
	if req.RequestedBy == "" {
		req.RequestedBy = req.TenantID
	}

	// Persist the JIT request row (with expiry) so it appears in /requests.
	idemKey := r.Header.Get("Idempotency-Key")
	expiresAt := time.Now().UTC().Add(time.Duration(req.DurationMins) * time.Minute)
	payload, _ := json.Marshal(map[string]any{
		"resource_id":   req.ResourceID,
		"resource_type": req.ResourceType,
		"action":        req.Action,
		"reason":        req.Reason,
		"duration_mins": req.DurationMins,
	})
	wfReq := &workflow.Request{
		TenantID:       req.TenantID,
		Type:           "access.request.jit",
		Status:         "pending",
		RequesterID:    req.RequestedBy,
		TargetID:       req.IdentityID,
		Payload:        payload,
		ExpiresAt:      &expiresAt,
		IdempotencyKey: idemKey,
	}
	created, err := h.WorkflowStore().CreateRequest(r.Context(), wfReq)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create JIT request: "+err.Error())
		return
	}
	if !created {
		respondJSON(w, http.StatusAccepted, map[string]any{
			"request_id":        wfReq.ID,
			"status":            wfReq.Status,
			"type":              "access.request.jit",
			"idempotent_replay": true,
		})
		return
	}
	_ = h.WorkflowStore().AppendAudit(r.Context(), wfReq.ID, "workflow.requested", "user:"+req.RequestedBy, map[string]any{
		"type":          "access.request.jit",
		"target_id":     req.IdentityID,
		"resource_id":   req.ResourceID,
		"duration_mins": req.DurationMins,
	})
	req.RequestID = wfReq.ID

	workflowID, err := h.StartJustInTimeWorkflow(r.Context(), req)
	if err != nil {
		_ = h.WorkflowStore().FailRequest(r.Context(), wfReq.ID, "temporal start failed: "+err.Error())
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.WorkflowStore().SetTemporalIDs(r.Context(), wfReq.ID, workflowID, ""); err != nil {
		log.Printf("[JIT] set temporal ids failed: %v", err)
	}
	respondJSON(w, http.StatusAccepted, map[string]any{
		"request_id":  wfReq.ID,
		"status":      wfReq.Status,
		"type":        "access.request.jit",
		"expires_at":  expiresAt.UTC().Format(time.RFC3339),
		"workflow_id": workflowID,
	})
}

func (h *Handler) RevokeAccess(w http.ResponseWriter, r *http.Request) {
	var req workflow.RevokeAccessInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	req.IsEmergency = true // API-triggered = emergency
	workflowID := fmt.Sprintf("revoke-access-%s-%s", req.IdentityID, uuid.New().String()[:8])
	we, err := h.TemporalClient().ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "critical-offboarding",
	}, workflow.RevokeAccessWorkflow, req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to start revocation workflow")
		return
	}

	respondJSON(w, http.StatusAccepted, map[string]any{
		"status":      "revocation_initiated",
		"workflow_id": we.GetID(),
	})
}

func (h *Handler) ListActiveJITSessions(w http.ResponseWriter, r *http.Request) {
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	nowMs := time.Now().UnixMilli()
	result, err := session.Run(r.Context(), `
		MATCH (i)-[r:HAS_TEMPORARY_ACCESS]->(res:Resource)
		WHERE (i:Identity OR i:NonHumanIdentity) AND r.expires_at > $nowMs
		RETURN i.uuid AS identity_id, COALESCE(i.display_name, i.name, i.email, i.uuid) AS identity_name,
		       res.uuid AS resource_id, COALESCE(res.name, res.uuid) AS resource_name,
		       r.expires_at AS expires_at
	`, map[string]any{"nowMs": nowMs})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to query JIT sessions: "+err.Error())
		return
	}

	type JITSession struct {
		IdentityID   string `json:"identity_id"`
		IdentityName string `json:"identity_name"`
		ResourceID   string `json:"resource_id"`
		ResourceName string `json:"resource_name"`
		ExpiresAt    string `json:"expires_at"`
	}

	var sessions []JITSession
	for result.Next(r.Context()) {
		rec := result.Record()
		identityID, _ := rec.Get("identity_id")
		idName, _ := rec.Get("identity_name")
		resourceID, _ := rec.Get("resource_id")
		resourceName, _ := rec.Get("resource_name")
		expiresAt, _ := rec.Get("expires_at")

		expiresStr := ""
		if v, ok := expiresAt.(int64); ok && v > 0 {
			expiresStr = time.UnixMilli(v).UTC().Format(time.RFC3339)
		}

		nameStr := ""
		if idName != nil {
			nameStr = fmt.Sprint(idName)
		}

		resStr := ""
		if resourceName != nil {
			resStr = fmt.Sprint(resourceName)
		}

		sessions = append(sessions, JITSession{
			IdentityID:   fmt.Sprint(identityID),
			IdentityName: nameStr,
			ResourceID:   fmt.Sprint(resourceID),
			ResourceName: resStr,
			ExpiresAt:    expiresStr,
		})
	}

	if sessions == nil {
		sessions = []JITSession{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
	})
}

// ─── AI Copilot Handler ───────────────────────────────────
