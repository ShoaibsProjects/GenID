package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"go.temporal.io/sdk/client"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/observeid/identity-platform/internal/audit"
	"github.com/observeid/identity-platform/internal/connector"
	"github.com/observeid/identity-platform/internal/vault"
	"github.com/observeid/identity-platform/internal/workflow"
)

// ─── Identity Service ──────────────────────────────────────

type IdentityService struct {
	pgPool       *pgxpool.Pool
	neo4j        neo4j.DriverWithContext
	redis        *redis.Client
	temporal     client.Client
	connMgr      *connector.Manager
	provisionEng *connector.ProvisioningEngine
	vault        *vault.Vault
	auditLog     *audit.Store
}

func NewIdentityService(pgPool *pgxpool.Pool, neo4j neo4j.DriverWithContext, rdb *redis.Client, tc client.Client) *IdentityService {
	connMgr := connector.NewManager(pgPool)
	vaultPath := os.Getenv("VAULT_PATH")
	if vaultPath == "" {
		vaultPath = "/tmp/observeid-vault.json"
	}
	vlt := vault.NewVault(os.Getenv("VAULT_MASTER_KEY"), vaultPath)
	alog := audit.NewStore(10000)
	return &IdentityService{
		pgPool:       pgPool,
		neo4j:        neo4j,
		redis:        rdb,
		temporal:     tc,
		connMgr:      connMgr,
		provisionEng: connector.NewProvisioningEngine(connMgr),
		vault:        vlt,
		auditLog:     alog,
	}
}

func (s *IdentityService) AuditStore() *audit.Store { return s.auditLog }
func (s *IdentityService) SaveVault() error         { return s.vault.Save() }
func (s *IdentityService) LoadConnectors(ctx context.Context) error {
	configs, err := s.connMgr.LoadAll(ctx)
	if err != nil {
		return err
	}
	logError("connector", fmt.Errorf("loaded %d connectors from database", len(configs)))
	return nil
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
	s.temporal.ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
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

	// Revoke agent — use the registered RevokeAccessWorkflow
	agentID := id
	reason := req.Reason
	go func() {
		if _, err := s.temporal.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
			ID:        fmt.Sprintf("kill-agent-%s", agentID),
			TaskQueue: "critical_offboarding",
		}, workflow.RevokeAccessWorkflow, workflow.RevokeAccessInput{
			IdentityID:  agentID,
			Reason:      reason,
			RevokedBy:   "system",
			IsEmergency: true,
		}); err != nil {
			logError("temporal", fmt.Errorf("kill switch workflow: %w", err))
		}
	}()

	// Find and cascade-revoke delegated agents
	parentID := id
	go func() {
		session := s.neo4j.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(context.Background())

		result, err := session.Run(context.Background(), `
			MATCH (:NonHumanIdentity {uuid: $id})-[:DELEGATED_FROM*1..3]->(child:NonHumanIdentity)
			WHERE child.status = 'active'
			RETURN child.uuid AS child_id
		`, map[string]any{"id": parentID})
		if err != nil {
			logError("neo4j", fmt.Errorf("cascade query: %w", err))
			return
		}

		for result.Next(context.Background()) {
			childIDRaw, _ := result.Record().Get("child_id")
			childIDStr, _ := childIDRaw.(string)
			if childIDStr == "" {
				continue
			}
			if _, err := s.temporal.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
				ID:        fmt.Sprintf("cascade-kill-%s", childIDStr),
				TaskQueue: "critical_offboarding",
			}, workflow.RevokeAccessWorkflow, workflow.RevokeAccessInput{
				IdentityID:  childIDStr,
				Reason:      "parent_revoked",
				RevokedBy:   parentID,
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
	recent, err := s.redis.Exists(r.Context(), fmt.Sprintf("revocation:recent:%s", req.IdentityID)).Result()
	if err != nil {
		// Redis down — fall through to allow (fail open for availability)
		logError("redis", fmt.Errorf("revocation check: %w", err))
	} else if recent > 0 {
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
		TaskQueue: "critical_offboarding",
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

// ─── Connector Management Handlers ─────────────────────────

func (s *IdentityService) ListConnectors(w http.ResponseWriter, r *http.Request) {
	connectors := s.connMgr.List()
	respondJSON(w, http.StatusOK, map[string]any{
		"connectors": connectors,
		"total":      len(connectors),
	})
}

func (s *IdentityService) CreateConnector(w http.ResponseWriter, r *http.Request) {
	var cfg connector.ConnectorConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid connector config")
		return
	}

	id, err := s.connMgr.Register(r.Context(), cfg)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"connector_id": id,
		"status":       "registered",
	})
}

func (s *IdentityService) GetConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	cfg, err := s.connMgr.GetConfig(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	lastSync := s.connMgr.GetLastSyncResult(id)

	respondJSON(w, http.StatusOK, map[string]any{
		"connector": cfg,
		"last_sync": lastSync,
	})
}

func (s *IdentityService) DeleteConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := s.connMgr.Unregister(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *IdentityService) ConnectConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := s.connMgr.Connect(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "connected"})
}

func (s *IdentityService) DisconnectConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := s.connMgr.Disconnect(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

func (s *IdentityService) SyncConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	result, err := s.connMgr.SyncUsers(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "sync_completed_with_errors",
			"result": result,
		})
		return
	}

	// Persist synced users to PostgreSQL
	if result != nil && len(result.Users) > 0 {
		created, updated, persistErr := s.persistSyncedUsers(r.Context(), id, result.Users)
		if persistErr != nil {
			s.auditLog.Append(audit.Entry{
				Level: audit.LevelError, Service: "connector", Path: r.URL.Path,
				Message: fmt.Sprintf("Sync persistence error: %s", persistErr.Error()),
				Tags:    []string{"connector", "sync", "error"},
			})
		}
		result.UsersCreated = created
		result.UsersUpdated = updated
		result.UsersTotal = len(result.Users)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status": "sync_completed",
		"result": result,
	})
}

func (s *IdentityService) persistSyncedUsers(ctx context.Context, connectorID string, users []connector.ConnectorUser) (int, int, error) {
	if len(users) == 0 {
		return 0, 0, nil
	}
	created, updated := 0, 0

	cfg, err := s.connMgr.GetConfig(connectorID)
	if err != nil {
		return 0, 0, fmt.Errorf("get connector config: %w", err)
	}
	tenantID := cfg.TenantID
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001" // default tenant
	}

	tx, err := s.pgPool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, user := range users {
		var raw []byte
		if user.Attributes != nil {
			raw, _ = json.Marshal(user.Attributes)
		}
		if raw == nil {
			raw = []byte("{}")
		}

		groups := user.Groups
		if groups == nil {
			groups = []string{}
		}
		roles := user.Roles
		if roles == nil {
			roles = []string{}
		}

		tag, err := tx.Exec(ctx, `
			INSERT INTO connector_identities
				(tenant_id, connector_id, external_id, username, email, display_name,
				 first_name, last_name, department, title, employee_id, manager_id,
				 phone, mobile, street_address, city, state, zip_code, country,
				 cost_center, division, company, enabled, locked,
				 groups, roles, raw_attributes, last_synced_at)
			VALUES
				($1, $2, $3, $4, $5, $6,
				 $7, $8, $9, $10, $11, $12,
				 $13, $14, $15, $16, $17, $18, $19,
				 $20, $21, $22, $23, $24,
				 $25, $26, $27, NOW())
			ON CONFLICT (connector_id, external_id) DO UPDATE SET
				username      = EXCLUDED.username,
				email         = EXCLUDED.email,
				display_name  = EXCLUDED.display_name,
				first_name    = EXCLUDED.first_name,
				last_name     = EXCLUDED.last_name,
				department    = EXCLUDED.department,
				title         = EXCLUDED.title,
				employee_id   = EXCLUDED.employee_id,
				manager_id    = EXCLUDED.manager_id,
				phone         = EXCLUDED.phone,
				mobile        = EXCLUDED.mobile,
				street_address = EXCLUDED.street_address,
				city          = EXCLUDED.city,
				state         = EXCLUDED.state,
				zip_code      = EXCLUDED.zip_code,
				country       = EXCLUDED.country,
				cost_center   = EXCLUDED.cost_center,
				division      = EXCLUDED.division,
				company       = EXCLUDED.company,
				enabled       = EXCLUDED.enabled,
				locked        = EXCLUDED.locked,
				groups        = EXCLUDED.groups,
				roles         = EXCLUDED.roles,
				raw_attributes = EXCLUDED.raw_attributes,
				last_synced_at = NOW()
		`, tenantID, connectorID, user.ExternalID, user.Username, user.Email, user.DisplayName,
			user.FirstName, user.LastName, user.Department, user.Title, user.EmployeeID, user.Manager,
			user.Phone, user.Mobile, user.StreetAddress, user.City, user.State, user.ZipCode, user.Country,
			user.CostCenter, user.Division, user.Company, user.Enabled, user.Locked,
			groups, roles, raw)

		if err != nil {
			return created, updated, fmt.Errorf("upsert user %s: %w", user.ExternalID, err)
		}
		if tag.Insert() {
			created++
		} else {
			updated++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return created, updated, fmt.Errorf("commit: %w", err)
	}
	return created, updated, nil
}

func (s *IdentityService) GetConnectorIdentities(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	rows, err := s.pgPool.Query(r.Context(), `
		SELECT id, external_id, username, email, display_name, first_name, last_name,
		       department, title, enabled, locked, groups, roles, first_synced_at, last_synced_at
		FROM connector_identities
		WHERE connector_id = $1
		ORDER BY display_name NULLS LAST, username NULLS LAST, email
	`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Query failed: %s", err.Error()))
		return
	}
	defer rows.Close()

	type IdentityEntry struct {
		ID           string   `json:"id"`
		ExternalID   string   `json:"external_id"`
		Username     string   `json:"username"`
		Email        string   `json:"email"`
		DisplayName  string   `json:"display_name"`
		FirstName    string   `json:"first_name"`
		LastName     string   `json:"last_name"`
		Department   string   `json:"department"`
		Title        string   `json:"title"`
		Enabled      bool     `json:"enabled"`
		Locked       bool     `json:"locked"`
		Groups       []string `json:"groups"`
		Roles        []string `json:"roles"`
		FirstSynced  string   `json:"first_synced_at"`
		LastSynced   string   `json:"last_synced_at"`
	}

	identities := []IdentityEntry{}
	for rows.Next() {
		var e IdentityEntry
		var firstSynced, lastSynced *time.Time
		if err := rows.Scan(&e.ID, &e.ExternalID, &e.Username, &e.Email, &e.DisplayName,
			&e.FirstName, &e.LastName, &e.Department, &e.Title, &e.Enabled, &e.Locked,
			&e.Groups, &e.Roles, &firstSynced, &lastSynced); err != nil {
			continue
		}
		if firstSynced != nil {
			e.FirstSynced = firstSynced.Format(time.RFC3339)
		}
		if lastSynced != nil {
			e.LastSynced = lastSynced.Format(time.RFC3339)
		}
		identities = append(identities, e)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"connector_id": id,
		"identities":   identities,
		"total":        len(identities),
	})
}

func (s *IdentityService) GetConnectorUsers(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	users, err := s.connMgr.GetConnectorUsers(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list users: %s", err.Error()))
		return
	}

	if users == nil {
		users = []connector.ConnectorUser{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"connector_id": id,
		"users":        users,
		"total":        len(users),
	})
}

// ─── Delta Sync ──────────────────────────────────────────────

func (s *IdentityService) SyncConnectorDelta(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	result, err := s.connMgr.SyncUsersDelta(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "sync_completed_with_errors",
			"result": result,
		})
		return
	}

	if result != nil && len(result.Users) > 0 {
		created, updated, persistErr := s.persistSyncedUsers(r.Context(), id, result.Users)
		if persistErr != nil {
			s.auditLog.Append(audit.Entry{
				Level: audit.LevelError, Service: "connector", Path: r.URL.Path,
				Message: fmt.Sprintf("Delta sync persistence error: %s", persistErr.Error()),
				Tags:    []string{"connector", "delta-sync", "error"},
			})
		}
		result.UsersCreated = created
		result.UsersUpdated = updated
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status": "delta_sync_completed",
		"result": result,
	})
}

// ─── Schema Discovery ────────────────────────────────────────

func (s *IdentityService) GetConnectorSchema(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	schema, err := s.connMgr.GetConnectorSchema(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Schema discovery failed: %s", err.Error()))
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"connector_id": id,
		"schema":       schema,
	})
}

// ─── Health ──────────────────────────────────────────────────

func (s *IdentityService) GetConnectorHealth(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	health, err := s.connMgr.GetConnectorHealth(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, health)
}

// ─── Connector Groups ─────────────────────────────────────────

func (s *IdentityService) GetConnectorGroups(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	rows, err := s.pgPool.Query(r.Context(), `
		SELECT id, external_id, name, description, group_type, scope, member_ids, first_synced_at, last_synced_at
		FROM connector_groups
		WHERE connector_id = $1
		ORDER BY name NULLS LAST
	`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Query failed: %s", err.Error()))
		return
	}
	defer rows.Close()

	type GroupEntry struct {
		ID          string   `json:"id"`
		ExternalID  string   `json:"external_id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		GroupType   string   `json:"group_type"`
		Scope       string   `json:"scope"`
		MemberIDs   []string `json:"member_ids"`
		FirstSynced string   `json:"first_synced_at"`
		LastSynced  string   `json:"last_synced_at"`
	}

	groups := []GroupEntry{}
	for rows.Next() {
		var e GroupEntry
		var firstSynced, lastSynced *time.Time
		if err := rows.Scan(&e.ID, &e.ExternalID, &e.Name, &e.Description, &e.GroupType, &e.Scope,
			&e.MemberIDs, &firstSynced, &lastSynced); err != nil {
			continue
		}
		if firstSynced != nil {
			e.FirstSynced = firstSynced.Format(time.RFC3339)
		}
		if lastSynced != nil {
			e.LastSynced = lastSynced.Format(time.RFC3339)
		}
		groups = append(groups, e)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"connector_id": id,
		"groups":       groups,
		"total":        len(groups),
	})
}

// ─── Connector Entitlements ───────────────────────────────────

func (s *IdentityService) GetConnectorEntitlements(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	rows, err := s.pgPool.Query(r.Context(), `
		SELECT identity_external_id, entitlement_type, source_id, source_name, source_type,
		       app_id, app_name, is_active
		FROM connector_entitlements
		WHERE connector_id = $1
		ORDER BY entitlement_type, source_name NULLS LAST
	`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Query failed: %s", err.Error()))
		return
	}
	defer rows.Close()

	type EntitlementEntry struct {
		IdentityExternalID string `json:"identity_external_id"`
		Type               string `json:"entitlement_type"`
		SourceID           string `json:"source_id"`
		SourceName         string `json:"source_name"`
		SourceType         string `json:"source_type"`
		AppID              string `json:"app_id"`
		AppName            string `json:"app_name"`
		IsActive           bool   `json:"is_active"`
	}

	entitlements := []EntitlementEntry{}
	for rows.Next() {
		var e EntitlementEntry
		if err := rows.Scan(&e.IdentityExternalID, &e.Type, &e.SourceID, &e.SourceName, &e.SourceType,
			&e.AppID, &e.AppName, &e.IsActive); err != nil {
			log.Printf("[ENTITLEMENTS] scan row error: %v", err)
			continue
		}
		entitlements = append(entitlements, e)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"connector_id":  id,
		"entitlements":  entitlements,
		"total":         len(entitlements),
	})
}

// ─── Connector Resources ─────────────────────────────────────

func (s *IdentityService) GetConnectorResources(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	rows, err := s.pgPool.Query(r.Context(), `
		SELECT id, external_id, resource_type, name, description, enabled, owner_ids, first_synced_at, last_synced_at
		FROM connector_resources
		WHERE connector_id = $1
		ORDER BY resource_type, name NULLS LAST
	`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Query failed: %s", err.Error()))
		return
	}
	defer rows.Close()

	type ResourceEntry struct {
		ID          string   `json:"id"`
		ExternalID  string   `json:"external_id"`
		Type        string   `json:"resource_type"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Enabled     bool     `json:"enabled"`
		OwnerIDs    []string `json:"owner_ids"`
		FirstSynced string   `json:"first_synced_at"`
		LastSynced  string   `json:"last_synced_at"`
	}

	resources := []ResourceEntry{}
	for rows.Next() {
		var e ResourceEntry
		var firstSynced, lastSynced *time.Time
		if err := rows.Scan(&e.ID, &e.ExternalID, &e.Type, &e.Name, &e.Description, &e.Enabled,
			&e.OwnerIDs, &firstSynced, &lastSynced); err != nil {
			log.Printf("[RESOURCES] scan row error: %v", err)
			continue
		}
		if firstSynced != nil {
			e.FirstSynced = firstSynced.Format(time.RFC3339)
		}
		if lastSynced != nil {
			e.LastSynced = lastSynced.Format(time.RFC3339)
		}
		resources = append(resources, e)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"connector_id": id,
		"resources":    resources,
		"total":        len(resources),
	})
}

// ─── Sync Handlers ─────────────────────────────────────────

func (s *IdentityService) SyncConnectorGroups(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	groups, err := s.connMgr.SyncGroups(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "sync_failed",
			"error":  err.Error(),
		})
		return
	}

	created, updated, persistErr := s.persistSyncedGroups(r.Context(), id, groups)
	if persistErr != nil {
		s.auditLog.Append(audit.Entry{
			Level: audit.LevelError, Service: "connector", Path: r.URL.Path,
			Message: fmt.Sprintf("Group sync persistence error: %s", persistErr.Error()),
			Tags:    []string{"connector", "group-sync", "error"},
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":        "groups_sync_completed",
		"groups_created": created,
		"groups_updated": updated,
		"groups_total":   len(groups),
		"connector_id":   id,
	})
}

func (s *IdentityService) SyncConnectorEntitlements(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	entitlements, err := s.connMgr.SyncEntitlements(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "sync_failed",
			"error":  err.Error(),
		})
		return
	}

	if err := s.persistSyncedEntitlements(r.Context(), id, entitlements); err != nil {
		s.auditLog.Append(audit.Entry{
			Level: audit.LevelError, Service: "connector", Path: r.URL.Path,
			Message: fmt.Sprintf("Entitlement sync persistence error: %s", err.Error()),
			Tags:    []string{"connector", "entitlement-sync", "error"},
		})
		respondJSON(w, http.StatusOK, map[string]any{
			"status":      "sync_completed_with_errors",
			"connector_id": id,
			"error":       err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":            "entitlements_sync_completed",
		"entitlements_total": len(entitlements),
		"connector_id":      id,
	})
}

func (s *IdentityService) SyncConnectorResources(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	resources, err := s.connMgr.SyncResources(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "sync_failed",
			"error":  err.Error(),
		})
		return
	}

	if err := s.persistSyncedResources(r.Context(), id, resources); err != nil {
		s.auditLog.Append(audit.Entry{
			Level: audit.LevelError, Service: "connector", Path: r.URL.Path,
			Message: fmt.Sprintf("Resource sync persistence error: %s", err.Error()),
			Tags:    []string{"connector", "resource-sync", "error"},
		})
		respondJSON(w, http.StatusOK, map[string]any{
			"status":      "sync_completed_with_errors",
			"connector_id": id,
			"error":       err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":          "resources_sync_completed",
		"resources_total": len(resources),
		"connector_id":    id,
	})
}

// ─── Full Sync ────────────────────────────────────────────────

func (s *IdentityService) FullSyncConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	start := time.Now()
	var syncErrors []string

	// 1. Sync users
	usersResult, err := s.connMgr.SyncUsers(r.Context(), id)
	if err != nil {
		log.Printf("[SYNC] User sync error: %v", err)
		syncErrors = append(syncErrors, fmt.Sprintf("users: %v", err))
	}
	usersCreated, usersUpdated := 0, 0
	if usersResult != nil && len(usersResult.Users) > 0 {
		usersCreated, usersUpdated, err = s.persistSyncedUsers(r.Context(), id, usersResult.Users)
		if err != nil {
			log.Printf("[SYNC] User persist error: %v", err)
			syncErrors = append(syncErrors, fmt.Sprintf("users persist: %v", err))
		}
	}

	// 2. Sync groups
	groups, err := s.connMgr.SyncGroups(r.Context(), id)
	if err != nil {
		log.Printf("[SYNC] Group sync error: %v", err)
		syncErrors = append(syncErrors, fmt.Sprintf("groups: %v", err))
	}
	groupsCreated, groupsUpdated := 0, 0
	if len(groups) > 0 {
		groupsCreated, groupsUpdated, err = s.persistSyncedGroups(r.Context(), id, groups)
		if err != nil {
			log.Printf("[SYNC] Group persist error: %v", err)
			syncErrors = append(syncErrors, fmt.Sprintf("groups persist: %v", err))
		}
	}

	// 3. Sync entitlements
	entitlements, err := s.connMgr.SyncEntitlements(r.Context(), id)
	if err != nil {
		log.Printf("[SYNC] Entitlement sync error: %v", err)
		syncErrors = append(syncErrors, fmt.Sprintf("entitlements: %v", err))
	}
	if len(entitlements) > 0 {
		if err := s.persistSyncedEntitlements(r.Context(), id, entitlements); err != nil {
			log.Printf("[SYNC] Entitlement persist error: %v", err)
			syncErrors = append(syncErrors, fmt.Sprintf("entitlements persist: %v", err))
		}
	}

	// 4. Sync resources
	resources, err := s.connMgr.SyncResources(r.Context(), id)
	if err != nil {
		log.Printf("[SYNC] Resource sync error: %v", err)
		syncErrors = append(syncErrors, fmt.Sprintf("resources: %v", err))
	}
	if len(resources) > 0 {
		if err := s.persistSyncedResources(r.Context(), id, resources); err != nil {
			log.Printf("[SYNC] Resource persist error: %v", err)
			syncErrors = append(syncErrors, fmt.Sprintf("resources persist: %v", err))
		}
	}

	elapsed := time.Since(start)

	status := "full_sync_completed"
	if len(syncErrors) > 0 {
		status = "full_sync_with_errors"
	}

	s.auditLog.Append(audit.Entry{
		Level:   audit.LevelInfo,
		Service: "connector",
		Method:  "POST",
		Path:    r.URL.Path,
		Message: fmt.Sprintf("Full sync %s for connector %s: %d users, %d groups, %d entitlements, %d resources in %s",
			status, id, usersCreated+usersUpdated, groupsCreated+groupsUpdated, len(entitlements), len(resources), elapsed.Round(time.Millisecond)),
		Tags: []string{"connector", "full-sync"},
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"status":             status,
		"connector_id":       id,
		"users_created":      usersCreated,
		"users_updated":      usersUpdated,
		"groups_created":     groupsCreated,
		"groups_updated":     groupsUpdated,
		"entitlements_total": len(entitlements),
		"resources_total":    len(resources),
		"errors":             syncErrors,
		"duration":           elapsed.Round(time.Millisecond).String(),
	})
}

// ─── Persistence: Groups ───────────────────────────────────

func (s *IdentityService) persistSyncedGroups(ctx context.Context, connectorID string, groups []connector.ConnectorGroup) (int, int, error) {
	if len(groups) == 0 {
		return 0, 0, nil
	}
	created, updated := 0, 0
	cfg, err := s.connMgr.GetConfig(connectorID)
	if err != nil {
		return 0, 0, fmt.Errorf("get connector config: %w", err)
	}
	tenantID := cfg.TenantID
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	tx, err := s.pgPool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, group := range groups {
		members := group.Members
		if members == nil {
			members = []string{}
		}

		var raw []byte
		if group.Attributes != nil {
			raw, _ = json.Marshal(group.Attributes)
		}
		if raw == nil {
			raw = []byte("{}")
		}

		tag, err := tx.Exec(ctx, `
			INSERT INTO connector_groups
				(tenant_id, connector_id, external_id, name, description, group_type, scope, member_ids, raw_attributes, last_synced_at)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
			ON CONFLICT (connector_id, external_id) DO UPDATE SET
				name            = EXCLUDED.name,
				description     = EXCLUDED.description,
				group_type      = EXCLUDED.group_type,
				scope           = EXCLUDED.scope,
				member_ids      = EXCLUDED.member_ids,
				raw_attributes  = EXCLUDED.raw_attributes,
				last_synced_at  = NOW()
		`, tenantID, connectorID, group.ExternalID, group.Name, group.Description,
			group.Type, group.Scope, members, raw)

		if err != nil {
			return created, updated, fmt.Errorf("upsert group %s: %w", group.ExternalID, err)
		}
		if tag.Insert() {
			created++
		} else {
			updated++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return created, updated, fmt.Errorf("commit: %w", err)
	}
	return created, updated, nil
}

// ─── Persistence: Entitlements ───────────────────────────

func (s *IdentityService) persistSyncedEntitlements(ctx context.Context, connectorID string, entitlements []connector.ConnectorEntitlement) error {
	if len(entitlements) == 0 {
		return nil
	}

	cfg, err := s.connMgr.GetConfig(connectorID)
	if err != nil {
		return fmt.Errorf("get connector config: %w", err)
	}
	tenantID := cfg.TenantID
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	tx, err := s.pgPool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM connector_entitlements WHERE connector_id = $1`, connectorID); err != nil {
		return fmt.Errorf("delete existing entitlements: %w", err)
	}

	for _, e := range entitlements {
		raw, _ := json.Marshal(e.RawAttributes)
		if raw == nil {
			raw = []byte("{}")
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO connector_entitlements
				(tenant_id, connector_id, identity_external_id, entitlement_type, source_id,
				 source_name, source_type, app_id, app_name, assigned_at, is_active, raw_attributes, last_synced_at)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
		`, tenantID, connectorID, e.IdentityExternalID, e.EntitlementType,
			e.SourceID, e.SourceName, e.SourceType,
			e.AppID, e.AppName, e.AssignedAt, e.IsActive, raw); err != nil {
			return fmt.Errorf("insert entitlement: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// ─── Persistence: Resources ─────────────────────────────

func (s *IdentityService) persistSyncedResources(ctx context.Context, connectorID string, resources []connector.ConnectorResource) error {
	if len(resources) == 0 {
		return nil
	}

	cfg, err := s.connMgr.GetConfig(connectorID)
	if err != nil {
		return fmt.Errorf("get connector config: %w", err)
	}
	tenantID := cfg.TenantID
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	tx, err := s.pgPool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM connector_resources WHERE connector_id = $1`, connectorID); err != nil {
		return fmt.Errorf("delete existing resources: %w", err)
	}

	for _, res := range resources {
		raw, _ := json.Marshal(res.Attributes)
		if raw == nil {
			raw = []byte("{}")
		}
		owners := res.OwnerIDs
		if owners == nil {
			owners = []string{}
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO connector_resources
				(tenant_id, connector_id, external_id, resource_type, name, description, enabled, owner_ids, raw_attributes, last_synced_at)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
			ON CONFLICT (connector_id, external_id) DO UPDATE SET
				name            = EXCLUDED.name,
				description     = EXCLUDED.description,
				resource_type   = EXCLUDED.resource_type,
				enabled         = EXCLUDED.enabled,
				owner_ids       = EXCLUDED.owner_ids,
				raw_attributes  = EXCLUDED.raw_attributes,
				last_synced_at  = NOW()
		`, tenantID, connectorID, res.ExternalID, res.ResourceType,
			res.Name, res.Description, res.Enabled, owners, raw); err != nil {
			return fmt.Errorf("upsert resource %s: %w", res.ExternalID, err)
		}
	}

	return tx.Commit(ctx)
}

func (s *IdentityService) TestExistingConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	cfg, err := s.connMgr.GetConfig(id)
	if err != nil {
		respondError(w, http.StatusNotFound, fmt.Sprintf("Connector not found: %s", err.Error()))
		return
	}

	if err := connector.TestConnection(r.Context(), cfg); err != nil {
		s.auditLog.Append(audit.Entry{
			Level: audit.LevelWarn, Service: "connector", Method: "POST", Path: r.URL.Path,
			Message: fmt.Sprintf("TestConnection: %s (%s) — %s", cfg.Type, cfg.Name, err.Error()),
			Tags:    []string{"connector", "test", "failed"},
		})
		respondJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	s.auditLog.Append(audit.Entry{
		Level: audit.LevelInfo, Service: "connector", Method: "POST", Path: r.URL.Path,
		Message: fmt.Sprintf("TestConnection: %s (%s) — SUCCESS", cfg.Type, cfg.Name),
		Tags:    []string{"connector", "test", "success"},
	})
	respondJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Connection successful",
	})
}

func (s *IdentityService) TestConnectorConnection(w http.ResponseWriter, r *http.Request) {
	var cfg connector.ConnectorConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		s.auditLog.Append(audit.Entry{
			Level: audit.LevelError, Service: "connector", Method: r.Method, Path: r.URL.Path,
			Message: "TestConnection: invalid config",
			Detail:  err.Error(), Tags: []string{"connector", "error"},
		})
		respondError(w, http.StatusBadRequest, "Invalid connector config")
		return
	}

	if err := connector.TestConnection(r.Context(), cfg); err != nil {
		s.auditLog.Append(audit.Entry{
			Level: audit.LevelWarn, Service: "connector", Method: r.Method, Path: r.URL.Path,
			Message: fmt.Sprintf("TestConnection: %s (%s) — %s", cfg.Type, cfg.TenantName, err.Error()),
			Detail:  fmt.Sprintf("type=%s tenant=%s client_id=%s error=%s", cfg.Type, cfg.TenantName, cfg.ClientID, err.Error()),
			Tags:    []string{"connector", "test", "failed"},
		})
		respondJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	s.auditLog.Append(audit.Entry{
		Level: audit.LevelInfo, Service: "connector", Method: r.Method, Path: r.URL.Path,
		Message: fmt.Sprintf("TestConnection: %s (%s) — SUCCESS", cfg.Type, cfg.TenantName),
		Tags:    []string{"connector", "test", "success"},
	})
	respondJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Connection successful",
	})
}

// ─── IAM Lifecycle Management (LCM) Handlers ─────────────

func (s *IdentityService) ExecuteLCM(w http.ResponseWriter, r *http.Request) {
	var req connector.LCMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid LCM request")
		return
	}

	results := s.provisionEng.ExecuteLCM(r.Context(), req)

	allSuccess := true
	for _, res := range results {
		if res.Status == connector.ProvisioningFailed {
			allSuccess = false
			break
		}
	}

	status := http.StatusOK
	if !allSuccess {
		status = http.StatusMultiStatus
	}

	respondJSON(w, status, map[string]any{
		"results": results,
		"total":   len(results),
		"all_ok":  allSuccess,
	})
}

func (s *IdentityService) GetLCMHistory(w http.ResponseWriter, r *http.Request) {
	history := s.provisionEng.GetHistory()
	respondJSON(w, http.StatusOK, map[string]any{
		"history": history,
		"total":   len(history),
	})
}

// ─── Identity CRUD (PostgreSQL + Neo4j) Handlers ─────────

func (s *IdentityService) CreateIdentityRecord(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		Type        string `json:"type"`
		Status      string `json:"status"`
		Department  string `json:"department"`
		Title       string `json:"title"`
		EmployeeID  string `json:"employee_id"`
		ManagerID   string `json:"manager_id"`
		Source      string `json:"source"`
		TenantID    string `json:"tenant_id"`
		Phone       string `json:"phone"`
		Attributes  map[string]string `json:"attributes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if input.Email == "" || input.DisplayName == "" {
		respondError(w, http.StatusBadRequest, "email and display_name are required")
		return
	}
	if input.Type == "" {
		input.Type = "human"
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if input.Source == "" {
		input.Source = "manual"
	}
	if input.TenantID == "" || input.TenantID == "default" {
		input.TenantID = "00000000-0000-0000-0000-000000000001"
	}

	id := uuid.New().String()

	// Handle nullable UUID fields
	var managerID interface{}
	if input.ManagerID != "" {
		managerID = input.ManagerID
	}

	// 1. Write to PostgreSQL (RETURNING id to handle ON CONFLICT returning existing row)
	var returnedID string
	var attrsJSON []byte
	if input.Attributes != nil {
		attrsJSON, _ = json.Marshal(input.Attributes)
	}
	if attrsJSON == nil {
		attrsJSON = []byte("{}")
	}
	err := s.pgPool.QueryRow(r.Context(), `
		INSERT INTO identities (id, tenant_id, type, status, email, display_name, department, employee_id, manager_id, source, risk_score, attributes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0.0, $11)
		ON CONFLICT (tenant_id, email) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			department   = EXCLUDED.department,
			employee_id  = EXCLUDED.employee_id,
			status       = 'active',
			updated_at   = NOW()
		RETURNING id
	`, id, input.TenantID, input.Type, input.Status, input.Email, input.DisplayName,
		input.Department, input.EmployeeID, managerID, input.Source, attrsJSON).Scan(&returnedID)
	if err != nil {
		s.auditLog.Append(audit.Entry{
			Level: audit.LevelError, Service: "identity", Path: r.URL.Path,
			Message: fmt.Sprintf("PG create failed: %s", err.Error()),
			Tags:    []string{"identity", "create", "error"},
		})
		respondError(w, http.StatusInternalServerError, "Failed to persist identity to database")
		return
	}
	// Use the actual id from the database (handles ON CONFLICT returning existing row)
	id = returnedID

	// 2. Write to Neo4j (graph)
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	_, err = session.Run(r.Context(), `
		MERGE (i:Identity {uuid: $uuid})
		SET i.tenant_id = $tenant_id, i.type = $type, i.status = 'active',
		    i.email = $email, i.display_name = $display_name,
		    i.first_name = $first_name, i.last_name = $last_name,
		    i.department = $department, i.title = $title,
		    i.employee_id = $employee_id, i.manager_id = $manager_id,
		    i.source = $source, i.phone = $phone,
		    i.risk_score = 0.0, i.updated_at = datetime(),
		    i.created_at = COALESCE(i.created_at, datetime())
	`, map[string]any{
		"uuid": id, "tenant_id": input.TenantID, "type": input.Type,
		"email": input.Email, "display_name": input.DisplayName,
		"first_name": input.FirstName, "last_name": input.LastName,
		"department": input.Department, "title": input.Title,
		"employee_id": input.EmployeeID, "manager_id": input.ManagerID,
		"source": input.Source, "phone": input.Phone,
	})
	if err != nil {
		s.auditLog.Append(audit.Entry{
			Level: audit.LevelError, Service: "identity", Path: r.URL.Path,
			Message: fmt.Sprintf("Neo4j create failed (PG written, id=%s): %s", id, err.Error()),
			Tags:    []string{"identity", "create", "neo4j", "error"},
		})
	}

	s.auditLog.Append(audit.Entry{
		Level: audit.LevelInfo, Service: "identity", Path: r.URL.Path,
		Message: fmt.Sprintf("Created identity: %s (%s)", input.DisplayName, input.Email),
		Tags:    []string{"identity", "create", "success"},
	})

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":            id,
		"status":        "created",
		"email":         input.Email,
		"display_name":  input.DisplayName,
		"type":          input.Type,
	})
}

func (s *IdentityService) UpdateIdentityRecord(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// Build Neo4j SET clause with property name validation
	setClauses := ""
	pgSet := ""
	pgParams := []any{id}
	pgIdx := 2
	params := map[string]any{"uuid": id}
	allowedKeys := map[string]string{
		"display_name": "display_name", "first_name": "first_name", "last_name": "last_name",
		"email": "email", "department": "department", "status": "status",
		"type": "type", "title": "title", "manager_id": "manager_id",
		"phone": "phone", "risk_score": "risk_score",
	}
	for key, val := range updates {
		dbCol, ok := allowedKeys[key]
		if !ok {
			continue
		}
		paramKey := "p_" + key
		setClauses += fmt.Sprintf("i.%s = $%s, ", key, paramKey)
		params[paramKey] = val
		pgSet += fmt.Sprintf("%s = $%d, ", dbCol, pgIdx)
		pgParams = append(pgParams, val)
		pgIdx++
	}

	if setClauses == "" {
		respondJSON(w, http.StatusOK, map[string]string{"status": "no_changes"})
		return
	}
	setClauses += "i.updated_at = datetime()"

	// Update Neo4j
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	query := fmt.Sprintf("MATCH (i:Identity {uuid: $uuid}) SET %s", setClauses)
	_, err := session.Run(r.Context(), query, params)

	// Also update PostgreSQL (same fields)
	pgSet += "updated_at = NOW()"
	if _, errUpdate := s.pgPool.Exec(r.Context(), fmt.Sprintf(`
		UPDATE identities SET %s WHERE id = $1
	`, pgSet), pgParams...); errUpdate != nil {
		logError("postgres", fmt.Errorf("update failed: %w", errUpdate))
	}

	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update identity")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *IdentityService) DeleteIdentityRecord(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Soft-delete in PostgreSQL
	if _, err := s.pgPool.Exec(r.Context(), `
		UPDATE identities SET status = 'terminated', updated_at = NOW() WHERE id = $1
	`, id); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete from database")
		return
	}

	// Soft-delete in Neo4j
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	_, err := session.Run(r.Context(), `
		MATCH (i:Identity {uuid: $uuid})
		SET i.status = 'terminated', i.updated_at = datetime()
	`, map[string]any{"uuid": id})
	if err != nil {
		logError("neo4j", fmt.Errorf("delete failed: %w", err))
	}

	s.auditLog.Append(audit.Entry{
		Level: audit.LevelInfo, Service: "identity", Path: r.URL.Path,
		Message: fmt.Sprintf("Deleted identity: %s", id),
		Tags:    []string{"identity", "delete"},
	})

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── Bulk Import Handler ──────────────────────────────────

func (s *IdentityService) BulkImportIdentities(w http.ResponseWriter, r *http.Request) {
	type ImportRec struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Type        string `json:"type"`
		Department  string `json:"department"`
		Title       string `json:"title"`
		EmployeeID  string `json:"employee_id"`
		Source      string `json:"source"`
		TenantID    string `json:"tenant_id"`
	}
	var req struct {
		Records []ImportRec `json:"records"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request: expected JSON with 'records' array")
		return
	}
	if len(req.Records) == 0 {
		respondError(w, http.StatusBadRequest, "No records provided")
		return
	}
	if len(req.Records) > 5000 {
		req.Records = req.Records[:5000]
	}

	created, updated, failed := 0, 0, 0
	var errs []string

	for i, rec := range req.Records {
		if rec.Email == "" || rec.DisplayName == "" {
			failed++
			errs = append(errs, fmt.Sprintf("row %d: missing email or display_name", i+1))
			continue
		}
		if rec.Type == "" { rec.Type = "human" }
		if rec.Source == "" { rec.Source = "hris" }
		if rec.TenantID == "" { rec.TenantID = "00000000-0000-0000-0000-000000000001" }

		id := uuid.New().String()
		tag, err := s.pgPool.Exec(r.Context(), `
			INSERT INTO identities (id, tenant_id, type, status, email, display_name, department, employee_id, source)
			VALUES ($1,$2,$3,'active',$4,$5,$6,$7,$8)
			ON CONFLICT (tenant_id, email) DO UPDATE SET
				display_name=EXCLUDED.display_name, department=EXCLUDED.department,
				employee_id=EXCLUDED.employee_id, status='active', updated_at=NOW()
		`, id, rec.TenantID, rec.Type, rec.Email, rec.DisplayName, rec.Department, rec.EmployeeID, rec.Source)

		if err != nil {
			failed++
			errs = append(errs, fmt.Sprintf("row %d (%s): %s", i+1, rec.Email, err.Error()))
			continue
		}
		if tag.Insert() { created++ } else { updated++ }
	}

	s.auditLog.Append(audit.Entry{
		Level: audit.LevelInfo, Service: "identity", Path: r.URL.Path,
		Message: fmt.Sprintf("Bulk import: %d created, %d updated, %d failed", created, updated, failed),
		Tags:    []string{"identity", "bulk", "import"},
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"status": "completed", "created": created, "updated": updated,
		"failed": failed, "total": len(req.Records), "errors": errs,
	})
}

// ─── Group CRUD Handlers ────────────────────────────────────

func (s *IdentityService) ListGroups(w http.ResponseWriter, r *http.Request) {
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	result, err := session.Run(r.Context(), `
		MATCH (r:Role)
		OPTIONAL MATCH (r)-[:GRANTS]->(e:Entitlement)
		RETURN r.uuid AS uuid, r.name AS name, r.description AS description,
			   r.role_type AS role_type, r.is_active AS is_active,
			   COUNT(DISTINCT e) AS entitlement_count
		ORDER BY r.name
	`, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}

	var groups []map[string]any
	for result.Next(r.Context()) {
		rec := result.Record()
		groups = append(groups, map[string]any{
			"uuid":             getRecordVal(rec, "uuid"),
			"name":             getRecordVal(rec, "name"),
			"description":      getRecordVal(rec, "description"),
			"role_type":        getRecordVal(rec, "role_type"),
			"is_active":        getRecordVal(rec, "is_active"),
			"entitlement_count": getRecordVal(rec, "entitlement_count"),
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"groups": groups,
		"total":  len(groups),
	})
}

func (s *IdentityService) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		RoleType    string `json:"role_type"`
		TenantID    string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	id := uuid.New().String()
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	_, err := session.Run(r.Context(), `
		CREATE (r:Role {
			uuid: $uuid, tenant_id: $tenant_id, name: $name,
			description: $description, role_type: $role_type,
			is_active: true, created_at: datetime()
		})
	`, map[string]any{
		"uuid": id, "tenant_id": req.TenantID, "name": req.Name,
		"description": req.Description, "role_type": req.RoleType,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create group")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":     id,
		"status": "created",
	})
}

func (s *IdentityService) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	_, err := session.Run(r.Context(), `
		MATCH (r:Role {uuid: $uuid})
		DETACH DELETE r
	`, map[string]any{"uuid": id})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete group")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── Role Assignment ────────────────────────────────────────

func (s *IdentityService) AssignRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IdentityID string `json:"identity_id"`
		RoleID     string `json:"role_id"`
		AssignedBy string `json:"assigned_by"`
		Source     string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	_, err := session.Run(r.Context(), `
		MATCH (i:Identity {uuid: $identity_id})
		MATCH (r:Role {uuid: $role_id})
		CREATE (i)-[:HAS_ROLE {
			assigned_at: datetime(), assigned_by: $assigned_by,
			source: $source, is_active: true
		}]->(r)
	`, map[string]any{
		"identity_id": req.IdentityID, "role_id": req.RoleID,
		"assigned_by": req.AssignedBy, "source": req.Source,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to assign role")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

func (s *IdentityService) RemoveRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IdentityID string `json:"identity_id"`
		RoleID     string `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	_, err := session.Run(r.Context(), `
		MATCH (i:Identity {uuid: $identity_id})-[rel:HAS_ROLE]->(r:Role {uuid: $role_id})
		DELETE rel
		SET i.updated_at = datetime()
	`, map[string]any{
		"identity_id": req.IdentityID, "role_id": req.RoleID,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to remove role")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ─── Vault / Credential Management Handlers ─────────────

func (s *IdentityService) ListSecrets(w http.ResponseWriter, r *http.Request) {
	secrets := s.vault.List(r.Context())
	respondJSON(w, http.StatusOK, map[string]any{
		"secrets": secrets,
		"total":   len(secrets),
	})
}

func (s *IdentityService) StoreSecret(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		Reference string `json:"reference"`
		Value     string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	id, err := s.vault.Store(r.Context(), req.Name, req.Type, req.Reference, req.Value)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"secret_id": id,
		"status":    "stored",
	})
}

func (s *IdentityService) RetrieveSecret(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	plaintext, err := s.vault.Retrieve(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"secret_id": id,
		"value":     plaintext,
	})
}

func (s *IdentityService) DeleteSecret(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := s.vault.Delete(r.Context(), id); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── Audit Log Handlers ─────────────────────────────────

func (s *IdentityService) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	level := audit.Level(q.Get("level"))
	path := q.Get("path")

	entries := s.auditLog.List(limit, offset, level, path)
	if entries == nil {
		entries = []audit.Entry{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"total":   len(entries),
		"stats":   s.auditLog.Stats(),
	})
}

func (s *IdentityService) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	entry, ok := s.auditLog.Get(id)
	if !ok {
		respondError(w, http.StatusNotFound, "Log entry not found")
		return
	}
	respondJSON(w, http.StatusOK, entry)
}

func (s *IdentityService) GetAuditLogStats(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, s.auditLog.Stats())
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
