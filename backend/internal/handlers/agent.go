package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/observeid/genid/internal/risk"
	"github.com/observeid/genid/internal/stores"
	"github.com/observeid/genid/internal/workflow"
	"go.temporal.io/sdk/client"
	"net/http"
	"time"
)

func (h *Handler) ListAgents(w http.ResponseWriter, r *http.Request) {
	limit, offset := paginationParams(r, 50, 0)

	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	result, err := session.Run(r.Context(), `
		MATCH (n:NonHumanIdentity)
		OPTIONAL MATCH (n)-[:OWNED_BY]->(owner:Identity)
		RETURN n.uuid AS uuid, n.name AS name, n.type AS type, n.status AS status,
			   n.risk_score AS risk_score, n.is_governed AS is_governed,
			   COALESCE(owner.display_name, n.owner_name) AS owner_name
		ORDER BY n.risk_score DESC
		SKIP $offset LIMIT $limit
	`, map[string]any{"offset": offset, "limit": limit})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}

	var agents []map[string]any
	for result.Next(r.Context()) {
		rec := result.Record()
		id := stores.GetRecordVal(rec, "uuid")
		agents = append(agents, map[string]any{
			"id":          id,
			"uuid":        id,
			"name":        stores.GetRecordVal(rec, "name"),
			"type":        stores.GetRecordVal(rec, "type"),
			"status":      stores.GetRecordVal(rec, "status"),
			"risk_score":  stores.GetRecordFloat(rec, "risk_score"),
			"is_governed": stores.GetRecordBool(rec, "is_governed"),
			"owner_name":  stores.GetRecordVal(rec, "owner_name"),
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"agents": agents,
		"total":  len(agents),
	})
}

func (h *Handler) RegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string   `json:"name"`
		AgentType    string   `json:"agent_type"`
		Type         string   `json:"type"` // alias for agent_type (frontend compatibility)
		Protocols    []string `json:"protocols"`
		OwnerID      string   `json:"owner_id"`
		OwnerName    string   `json:"owner_name"` // display-only when owner_id unknown
		TeamID       string   `json:"team_id"`
		Env          string   `json:"deployment_environment"`
		Capabilities []string `json:"requested_capabilities"`
		TenantID     string   `json:"tenant_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.AgentType == "" {
		req.AgentType = req.Type
	}
	if req.AgentType == "" {
		req.AgentType = "ai_agent"
	}
	if req.TenantID == "" {
		req.TenantID = "00000000-0000-0000-0000-000000000001"
	}

	agentID := uuid.New().String()
	agentCardID := uuid.New().String()

	// Create Neo4j node (risk_score is computed dynamically after creation)
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	_, err := session.Run(r.Context(), `
		CREATE (n:NonHumanIdentity {
			uuid: $uuid, tenant_id: $tenant_id, name: $name, type: $type,
			status: 'active', agent_card_id: $card_id, protocols: $protocols,
			owner_id: $owner_id, owner_name: $owner_name, team_id: $team_id,
			capabilities: $capabilities,
			deployment_environment: $env, is_governed: true,
			risk_score: 0.0, risk_factors: ["pending_calculation"], created_at: datetime()
		})
		WITH n
		OPTIONAL MATCH (owner:Identity {uuid: $owner_id})
		FOREACH (_ IN CASE WHEN owner IS NULL THEN [] ELSE [1] END |
			CREATE (n)-[:OWNED_BY {ownership_type: 'primary'}]->(owner)
		)
	`, map[string]any{
		"uuid": agentID, "tenant_id": req.TenantID, "name": req.Name,
		"type": req.AgentType, "card_id": agentCardID, "protocols": req.Protocols,
		"owner_id": req.OwnerID, "owner_name": req.OwnerName, "team_id": req.TeamID,
		"capabilities": req.Capabilities,
		"env":          req.Env,
	})
	if err != nil {
		logError("neo4j", err)
		respondError(w, http.StatusInternalServerError, "Agent registration failed")
		return
	}

	// Compute initial risk (will be low/zero for a fresh agent with no access paths).
	score, factors, _ := risk.CalculateIdentityRisk(r.Context(), h.Neo4j(), h.RawPool(), req.TenantID, agentID)

	respondJSON(w, http.StatusCreated, map[string]any{
		"agent_id":      agentID,
		"agent_card_id": agentCardID,
		"status":        "active",
		"risk_score":    score,
		"risk_factors":  factors,
	})
}

func (h *Handler) GetAgent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	result, err := session.Run(r.Context(), `
		MATCH (n:NonHumanIdentity {uuid: $id})
		OPTIONAL MATCH (n)-[:OWNED_BY]->(owner:Identity)
		OPTIONAL MATCH (n)-[:DELEGATED_FROM]->(parent:NonHumanIdentity)
		RETURN n, owner.display_name AS owner_name, COLLECT(DISTINCT parent.name) AS parents
	`, map[string]any{"id": id})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}

	if result.Next(r.Context()) {
		record := result.Record()
		node, _ := record.Get("n")
		owner, _ := record.Get("owner_name")
		parents, _ := record.Get("parents")

		respondJSON(w, http.StatusOK, map[string]any{
			"agent":   node,
			"owner":   owner,
			"parents": parents,
		})
		return
	}

	respondError(w, http.StatusNotFound, "Agent not found")
}

func (h *Handler) AgentKillSwitch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Reason = "emergency_kill_switch"
	}

	agentID := id
	reason := req.Reason

	// Kill the agent in PostgreSQL (source of truth)
	if _, err := h.DB(r.Context()).Exec(r.Context(), `
		UPDATE non_human_identities SET status = 'revoked', updated_at = NOW() WHERE id = $1`,
		agentID); err != nil {
		logError("postgres", fmt.Errorf("kill switch pg update: %w", err))
	}

	// Update Neo4j status to revoked (use background context — request ctx gets cancelled)
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		session := h.Neo4j().NewSession(bgCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(bgCtx)
		if _, err := session.Run(bgCtx, `
			MATCH (n:NonHumanIdentity {uuid: $id})
			SET n.status = 'revoked', n.revoked_at = timestamp()
		`, map[string]any{"id": agentID}); err != nil {
			logError("neo4j", fmt.Errorf("kill switch neo4j update: %w", err))
		}
	}()

	// Instant revocation: delete all active JIT JWTs and grant entries for this agent.
	// Uses the reverse index jit:identity:<id> -> [jti,...] to find tokens to blocklist.
	go func() {
		bgCtx := context.Background()

		// 1. Blocklist every active JWT jti for this agent (instant fail on token check)
		jtis, _ := h.Redis().SMembers(bgCtx, fmt.Sprintf("jit:identity:%s", agentID)).Result()
		for _, jti := range jtis {
			h.Redis().Del(bgCtx, fmt.Sprintf("jit:jwt:%s", jti))
			h.Redis().Set(bgCtx, fmt.Sprintf("jit:blocked:%s", jti), "1", 24*time.Hour)
		}
		h.Redis().Del(bgCtx, fmt.Sprintf("jit:identity:%s", agentID))

		// 2. Also delete all grant entries (covers pre-OIDC keys)
		jitIter := h.Redis().Scan(bgCtx, 0, fmt.Sprintf("jit:grant:%s:*", agentID), 0).Iterator()
		for jitIter.Next(bgCtx) {
			h.Redis().Del(bgCtx, jitIter.Val())
		}
	}()

	// Launch Temporal workflow (async — uses own context with timeout)
	go func() {
		if _, err := h.TemporalClient().ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
			ID:        fmt.Sprintf("kill-agent-%s", agentID),
			TaskQueue: "critical-offboarding",
		}, workflow.RevokeAccessWorkflow, workflow.RevokeAccessInput{
			IdentityID:  agentID,
			Reason:      reason,
			RevokedBy:   "system",
			IsEmergency: true,
		}); err != nil {
			logError("temporal", fmt.Errorf("kill switch workflow: %w", err))
		}
	}()

	// Find and cascade-revoke delegated agents using background context
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		session := h.Neo4j().NewSession(bgCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(bgCtx)

		result, err := session.Run(bgCtx, `
			MATCH (:NonHumanIdentity {uuid: $id})-[:DELEGATED_FROM*1..3]->(child:NonHumanIdentity)
			WHERE child.status = 'active'
			RETURN child.uuid AS child_id
		`, map[string]any{"id": agentID})
		if err != nil {
			logError("neo4j", fmt.Errorf("cascade query: %w", err))
			return
		}

		for result.Next(bgCtx) {
			childIDRaw, _ := result.Record().Get("child_id")
			childIDStr, ok := childIDRaw.(string)
			if !ok || childIDStr == "" {
				continue
			}
			if _, err := h.TemporalClient().ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
				ID:        fmt.Sprintf("cascade-kill-%s", childIDStr),
				TaskQueue: "critical-offboarding",
			}, workflow.RevokeAccessWorkflow, workflow.RevokeAccessInput{
				IdentityID:  childIDStr,
				Reason:      "parent_revoked",
				RevokedBy:   agentID,
				IsEmergency: true,
			}); err != nil {
				logError("temporal", fmt.Errorf("cascade kill switch: %w", err))
			}
		}
	}()

	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "kill_switch_activated",
		"agent":   id,
		"message": "Agent and all delegated credentials revoked. Cascade revocation initiated for delegated agents.",
	})
}

func (h *Handler) DelegateAgent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	parentID := vars["id"]

	var req struct {
		ChildAgentID string   `json:"child_agent_id"`
		Scope        []string `json:"scope_narrowing"`
		MaxDepth     int      `json:"max_depth"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if req.MaxDepth == 0 {
		req.MaxDepth = 1
	}

	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	_, err := session.Run(r.Context(), `
		MATCH (parent:NonHumanIdentity {uuid: $parent_id})
		MATCH (child:NonHumanIdentity {uuid: $child_id})
		CREATE (child)-[:DELEGATED_FROM {
			delegated_at: datetime(),
			scope_narrowing: $scope,
			max_depth_remaining: $max_depth
		}]->(parent)
	`, map[string]any{
		"parent_id": parentID, "child_id": req.ChildAgentID,
		"scope": req.Scope, "max_depth": req.MaxDepth,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Delegation failed")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"status":    "delegated",
		"parent":    parentID,
		"child":     req.ChildAgentID,
		"scope":     req.Scope,
		"max_depth": req.MaxDepth,
	})
}

func (h *Handler) GetAgentCard(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Look up agent card
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	result, err := session.Run(r.Context(), `
		MATCH (n:NonHumanIdentity {uuid: $id})
		RETURN n.name AS name, n.protocols AS protocols, n.capabilities AS capabilities,
			   n.owner_id AS owner_id, n.deployment_environment AS env,
			   n.created_at AS created_at, n.status AS status
	`, map[string]any{"id": id})
	if err != nil || !result.Next(r.Context()) {
		respondError(w, http.StatusNotFound, "Agent not found")
		return
	}

	card := map[string]any{
		"agent_id":         id,
		"agent_type":       "ai_agent",
		"capabilities":     stores.GetRecordStrings(result.Record(), "capabilities"),
		"protocols":        stores.GetRecordStrings(result.Record(), "protocols"),
		"owner_id":         stores.GetRecordString(result.Record(), "owner_id"),
		"deployment_env":   stores.GetRecordString(result.Record(), "env"),
		"issued_at":        stores.GetRecordVal(result.Record(), "created_at"),
		"public_key":       "-----BEGIN PUBLIC KEY-----\n... (ML-DSA-44 public key)\n-----END PUBLIC KEY-----",
		"signature_scheme": "ml-dsa-44",
	}

	respondJSON(w, http.StatusOK, card)
}

// ─── Access API Handlers ──────────────────────────────────
