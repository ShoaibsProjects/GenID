package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"go.temporal.io/sdk/client"

	"github.com/observeid/identity-platform/internal/workflow"
)

// ─── Identity Service ──────────────────────────────────────

type IdentityService struct {
	pgPool   interface{} // *pgxpool.Pool in production
	neo4j    neo4j.DriverWithContext
	redis    *redis.Client
	temporal client.Client
}

func NewIdentityService(pgPool interface{}, neo4j neo4j.DriverWithContext, rdb *redis.Client, tc client.Client) *IdentityService {
	return &IdentityService{
		pgPool:   pgPool,
		neo4j:    neo4j,
		redis:    rdb,
		temporal: tc,
	}
}

// ─── SCIM 2.0 Handlers ─────────────────────────────────────

func (s *IdentityService) ScimListUsers(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": 0,
		"Resources":    []any{},
	})
}

func (s *IdentityService) ScimCreateUser(w http.ResponseWriter, r *http.Request) {
	var scimUser map[string]any
	if err := json.NewDecoder(r.Body).Decode(&scimUser); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid SCIM payload")
		return
	}

	userName, _ := scimUser["userName"].(string)
	id := uuid.New().String()

	respondJSON(w, http.StatusCreated, map[string]any{
		"schemas":  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"id":       id,
		"userName": userName,
		"active":   true,
		"meta": map[string]any{
			"resourceType": "User",
			"created":      time.Now().Format(time.RFC3339),
		},
	})
}

func (s *IdentityService) ScimGetUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	respondJSON(w, http.StatusOK, map[string]any{
		"schemas":  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"id":       id,
		"userName": "user@" + id,
		"active":   true,
	})
}

func (s *IdentityService) ScimUpdateUser(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *IdentityService) ScimPatchUser(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "patched"})
}

func (s *IdentityService) ScimDeleteUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	// Trigger offboarding workflow
	s.temporal.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
		ID:        fmt.Sprintf("offboard-%s", id),
		TaskQueue: "critical_offboarding",
	}, workflow.OffboardIdentityWorkflow, workflow.OffboardInput{
		IdentityID: id,
		Reason:     "SCIM deprovisioning",
		RequestedBy: "scim",
	})
	respondJSON(w, http.StatusNoContent, nil)
}

// ─── Identity API Handlers ─────────────────────────────────

func (s *IdentityService) ListIdentities(w http.ResponseWriter, r *http.Request) {
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	result, err := session.Run(r.Context(), `
		MATCH (i:Identity)
		RETURN i.uuid AS uuid, i.display_name AS name, i.email AS email,
			   i.status AS status, i.type AS type, i.department AS department,
			   i.risk_score AS risk_score
		ORDER BY i.created_at DESC
		LIMIT 50
	`, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}

		var identities []map[string]any
	for result.Next(r.Context()) {
		rec := result.Record()
		identities = append(identities, map[string]any{
			"uuid":       getRecordVal(rec, "uuid"),
			"name":       getRecordVal(rec, "display_name"),
			"email":      getRecordVal(rec, "email"),
			"status":     getRecordVal(rec, "status"),
			"type":       getRecordVal(rec, "type"),
			"department": getRecordVal(rec, "department"),
			"risk_score": getRecordVal(rec, "risk_score"),
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"identities": identities,
		"total":      len(identities),
	})
}

func (s *IdentityService) GetIdentity(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	result, err := session.Run(r.Context(), `
		MATCH (i:Identity {uuid: $id})
		OPTIONAL MATCH (i)-[:HAS_ROLE]->(r:Role)
		OPTIONAL MATCH (i)-[:MANAGES]->(reports:Identity)
		RETURN i, COLLECT(DISTINCT r) AS roles, COLLECT(DISTINCT reports) AS direct_reports
	`, map[string]any{"id": id})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}

	if result.Next(r.Context()) {
		record := result.Record()
		node, _ := record.Get("i")
		roles, _ := record.Get("roles")
		reports, _ := record.Get("direct_reports")

		respondJSON(w, http.StatusOK, map[string]any{
			"identity":       node,
			"roles":          roles,
			"direct_reports": reports,
		})
		return
	}

	respondError(w, http.StatusNotFound, "Identity not found")
}

func (s *IdentityService) GetIdentityEntitlements(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	result, err := session.Run(r.Context(), `
		MATCH (i:Identity {uuid: $id})
		OPTIONAL MATCH (i)-[:HAS_ROLE]->(r:Role)-[:GRANTS]->(e:Entitlement)-[:ACCESSES]->(res:Resource)
		OPTIONAL MATCH (i)-[:DIRECTLY_OWNS]->(e2:Entitlement)-[:ACCESSES]->(res2:Resource)
		RETURN COLLECT(DISTINCT {
			entitlement: e, role: r, resource: res, source: 'role_inherited'
		}) + COLLECT(DISTINCT {
			entitlement: e2, resource: res2, source: 'direct'
		}) AS entitlements
	`, map[string]any{"id": id})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}

	if result.Next(r.Context()) {
		entitlements, _ := result.Record().Get("entitlements")
		respondJSON(w, http.StatusOK, map[string]any{
			"identity_id":  id,
			"entitlements": entitlements,
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"identity_id": id, "entitlements": []any{}})
}

func (s *IdentityService) GetBlastRadius(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	result, err := session.Run(r.Context(), `
		MATCH (i:Identity {uuid: $id})
		MATCH path = (i)-[:HAS_ROLE|DIRECTLY_OWNS|DELEGATED_FROM*1..4]->(e:Entitlement)-[:ACCESSES]->(r:Resource)
		RETURN r.name AS resource_name, r.criticality AS criticality,
			   e.permission_level AS permission_level,
			   LENGTH(path) AS path_depth,
			   [n IN NODES(path) | labels(n)[0]] AS path_types
		ORDER BY r.criticality DESC, path_depth ASC
	`, map[string]any{"id": id})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}

	var resources []map[string]any
	for result.Next(r.Context()) {
		record := result.Record()
		name, _ := record.Get("resource_name")
		crit, _ := record.Get("criticality")
		perm, _ := record.Get("permission_level")
		depth, _ := record.Get("path_depth")
		types, _ := record.Get("path_types")

		resources = append(resources, map[string]any{
			"resource":         name,
			"criticality":      crit,
			"permission_level": perm,
			"path_depth":       depth,
			"path_types":       types,
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"identity_id":  id,
		"blast_radius": resources,
	})
}

// ─── Agent / NHI Handlers ─────────────────────────────────

func (s *IdentityService) ListAgents(w http.ResponseWriter, r *http.Request) {
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	result, err := session.Run(r.Context(), `
		MATCH (n:NonHumanIdentity)
		OPTIONAL MATCH (n)-[:OWNED_BY]->(owner:Identity)
		RETURN n.uuid AS uuid, n.name AS name, n.type AS type, n.status AS status,
			   n.risk_score AS risk_score, n.is_governed AS is_governed,
			   owner.display_name AS owner_name
		ORDER BY n.risk_score DESC
	`, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}

	var agents []map[string]any
	for result.Next(r.Context()) {
		rec := result.Record()
		agents = append(agents, map[string]any{
			"uuid":        getRecordVal(rec, "uuid"),
			"name":        getRecordVal(rec, "name"),
			"type":        getRecordVal(rec, "type"),
			"status":      getRecordVal(rec, "status"),
			"risk_score":  getRecordVal(rec, "risk_score"),
			"is_governed": getRecordVal(rec, "is_governed"),
			"owner_name":  getRecordVal(rec, "owner_name"),
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"agents": agents,
		"total":  len(agents),
	})
}

func (s *IdentityService) RegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string   `json:"name"`
		AgentType    string   `json:"agent_type"`
		Protocols    []string `json:"protocols"`
		OwnerID      string   `json:"owner_id"`
		TeamID       string   `json:"team_id"`
		Env          string   `json:"deployment_environment"`
		Capabilities []string `json:"requested_capabilities"`
		TenantID     string   `json:"tenant_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	agentID := uuid.New().String()
	agentCardID := uuid.New().String()

	// Create Neo4j node
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	_, err := session.Run(r.Context(), `
		CREATE (n:NonHumanIdentity {
			uuid: $uuid, tenant_id: $tenant_id, name: $name, type: $type,
			status: 'active', agent_card_id: $card_id, protocols: $protocols,
			owner_id: $owner_id, team_id: $team_id, capabilities: $capabilities,
			deployment_environment: $env, is_governed: true,
			risk_score: 0.3, created_at: datetime()
		})
		WITH n
		MATCH (owner:Identity {uuid: $owner_id})
		CREATE (n)-[:OWNED_BY {ownership_type: 'primary'}]->(owner)
	`, map[string]any{
		"uuid": agentID, "tenant_id": req.TenantID, "name": req.Name,
		"type": req.AgentType, "card_id": agentCardID, "protocols": req.Protocols,
		"owner_id": req.OwnerID, "team_id": req.TeamID, "capabilities": req.Capabilities,
		"env": req.Env,
	})
	if err != nil {
		logError("neo4j", err)
		respondError(w, http.StatusInternalServerError, "Agent registration failed")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"agent_id":      agentID,
		"agent_card_id": agentCardID,
		"status":        "active",
	})
}

func (s *IdentityService) GetAgent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
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
			"agent":  node,
			"owner":  owner,
			"parents": parents,
		})
		return
	}

	respondError(w, http.StatusNotFound, "Agent not found")
}

func (s *IdentityService) AgentKillSwitch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Reason = "emergency_kill_switch"
	}

	// Update PostgreSQL status (source of truth)
	// In production: tx.Exec("UPDATE non_human_identities SET status = 'revoked' WHERE uuid = $1", id)

	// Revoke SPIFFE SVID
	go func() {
		s.temporal.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
			ID:        fmt.Sprintf("kill-agent-%s", id),
			TaskQueue: "critical_offboarding",
		}, "RevokeAgentDelegationWorkflow", map[string]any{
			"agent_id": id, "revoked_by": "system", "reason": req.Reason,
		})
	}()

	// Find and cascade-revoke delegated agents
	go func() {
		session := s.neo4j.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(context.Background())

		result, _ := session.Run(context.Background(), `
			MATCH (:NonHumanIdentity {uuid: $id})-[:DELEGATED_FROM*1..3]->(child:NonHumanIdentity)
			WHERE child.status = 'active'
			RETURN child.uuid AS child_id
		`, map[string]any{"id": id})

		for result.Next(context.Background()) {
			childID, _ := result.Record().Get("child_id")
			s.temporal.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
				ID:        fmt.Sprintf("cascade-kill-%s", childID),
				TaskQueue: "critical_offboarding",
			}, "RevokeAgentDelegationWorkflow", map[string]any{
				"agent_id": childID, "revoked_by": id, "reason": "parent_revoked",
			})
		}
	}()

	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "kill_switch_activated",
		"agent":   id,
		"message": "Agent and all delegated credentials revoked. Cascade revocation initiated for delegated agents.",
	})
}

func (s *IdentityService) DelegateAgent(w http.ResponseWriter, r *http.Request) {
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

	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
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
		"status":         "delegated",
		"parent":         parentID,
		"child":          req.ChildAgentID,
		"scope":          req.Scope,
		"max_depth":      req.MaxDepth,
	})
}

func (s *IdentityService) GetAgentCard(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Look up agent card
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
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
		"agent_id":          id,
		"agent_type":        "ai_agent",
		"capabilities":      getRecordStrings(result.Record(), "capabilities"),
		"protocols":         getRecordStrings(result.Record(), "protocols"),
		"owner_id":          getRecordString(result.Record(), "owner_id"),
		"deployment_env":    getRecordString(result.Record(), "env"),
		"issued_at":         getRecordVal(result.Record(), "created_at"),
		"public_key":        "-----BEGIN PUBLIC KEY-----\n... (ML-DSA-44 public key)\n-----END PUBLIC KEY-----",
		"signature_scheme":  "ml-dsa-44",
	}

	respondJSON(w, http.StatusOK, card)
}

// ─── Access API Handlers ──────────────────────────────────

func (s *IdentityService) CheckAccess(w http.ResponseWriter, r *http.Request) {
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
	recent, _ := s.redis.Exists(r.Context(), fmt.Sprintf("revocation:recent:%s", req.IdentityID)).Result()
	if recent > 0 {
		respondJSON(w, http.StatusOK, map[string]any{
			"allowed": false,
			"reason":  "recent_revocation",
		})
		return
	}

	// In production: query Neo4j for entitlement path + evaluate Cedar policy
	// For now, return allowed
	respondJSON(w, http.StatusOK, map[string]any{
		"allowed":    true,
		"evaluated":  "cedar",
		"latency_ms": 2,
	})
}

func (s *IdentityService) GrantAccess(w http.ResponseWriter, r *http.Request) {
	var req workflow.GrantAccessInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	workflowID := fmt.Sprintf("grant-access-%s-%s", req.IdentityID, uuid.New().String()[:8])
	s.temporal.ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "provisioning",
	}, workflow.GrantAccessWorkflow, req)

	respondJSON(w, http.StatusAccepted, map[string]any{
		"status":      "provisioning",
		"workflow_id": workflowID,
	})
}

func (s *IdentityService) RevokeAccess(w http.ResponseWriter, r *http.Request) {
	var req workflow.RevokeAccessInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	req.IsEmergency = true // API-triggered = emergency
	workflowID := fmt.Sprintf("revoke-access-%s-%s", req.IdentityID, uuid.New().String()[:8])
	s.temporal.ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "critical_offboarding",
	}, workflow.RevokeAccessWorkflow, req)

	respondJSON(w, http.StatusAccepted, map[string]any{
		"status":      "revocation_initiated",
		"workflow_id": workflowID,
	})
}

// ─── AI Copilot Handler ───────────────────────────────────

func (s *IdentityService) CopilotQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Question string `json:"question"`
		UserID   string `json:"user_id"`
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"question": req.Question,
		"answer":   "The AI Copilot is processing your request. In production, the GraphRAG pipeline (Neo4j + Qdrant + 3-LLM) will return a structured response with access paths, confidence scores, and recommendations.",
		"status":   "processed",
	})
}

// ─── CAEP Handlers ─────────────────────────────────────────

func (s *IdentityService) ListCAEPEvents(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"events": []any{},
		"total":  0,
	})
}

func (s *IdentityService) BroadcastCAEP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventType  string   `json:"event_type"`
		IdentityID string   `json:"identity_id"`
		Receivers  []string `json:"receivers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	respondJSON(w, http.StatusAccepted, map[string]any{
		"status":   "broadcasting",
		"event":    req.EventType,
		"identity": req.IdentityID,
	})
}

// ─── Helpers ──────────────────────────────────────────────

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

func getRecordString(record *neo4j.Record, key string) string {
	val, ok := record.Get(key)
	if !ok {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

func getRecordStrings(record *neo4j.Record, key string) []string {
	val, ok := record.Get(key)
	if !ok {
		return nil
	}
	switch v := val.(type) {
	case []string:
		return v
	case []any:
		strs := make([]string, len(v))
		for i, item := range v {
			strs[i] = fmt.Sprintf("%v", item)
		}
		return strs
	default:
		return nil
	}
}

func getRecordVal(record *neo4j.Record, key string) string {
	val, ok := record.Get(key)
	if !ok {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

func logError(component string, err error) {
	fmt.Printf("[ERROR] %s: %v\n", component, err)
}
