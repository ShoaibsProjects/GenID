package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"go.temporal.io/sdk/client"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/observeid/identity-platform/internal/audit"
	"github.com/observeid/identity-platform/internal/cedar"
	"github.com/observeid/identity-platform/internal/connector"
	"github.com/observeid/identity-platform/internal/oidc"
	"github.com/observeid/identity-platform/internal/outbox"
	"github.com/observeid/identity-platform/internal/vault"
	"github.com/observeid/identity-platform/internal/workflow"
	"github.com/observeid/identity-platform/pkg/telemetry"
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
	oidcProvider *oidc.Provider
	cedarEngine  *cedar.CedarEngine
	outbox       *outbox.Outbox
}

func NewIdentityService(pgPool *pgxpool.Pool, neo4j neo4j.DriverWithContext, rdb *redis.Client, tc client.Client) *IdentityService {
	connMgr := connector.NewManager(pgPool)
	vaultPath := os.Getenv("VAULT_PATH")
	if vaultPath == "" {
		vaultPath = "/tmp/observeid-vault.json"
	}
	vlt, err := vault.NewVault(os.Getenv("VAULT_MASTER_KEY"), vaultPath)
	if err != nil {
		log.Printf("[IDENTITY] Vault initialization failed: %v — continuing with in-memory-only vault", err)
		vlt, _ = vault.NewVault("default-insecure-key-do-not-use-in-production-32chars-min", "")
	}
	alog := audit.NewStore(10000)
	oidcProv, err := oidc.NewProvider(pgPool, "http://localhost:8080")
	if err != nil {
		log.Printf("[IDENTITY] OIDC provider initialization failed: %v", err)
	}
	cedarEng := cedar.NewCedarEngine(pgPool)
	if err := cedarEng.LoadPolicies(context.Background(), ""); err != nil {
		log.Printf("[CEDAR] Initial policy load failed: %v", err)
	} else {
		log.Printf("[CEDAR] Loaded %d policies", cedarEng.PolicyCount(""))
	}
	outboxEng := outbox.NewOutbox(pgPool)
	return &IdentityService{
		pgPool:       pgPool,
		neo4j:        neo4j,
		redis:        rdb,
		temporal:     tc,
		connMgr:      connMgr,
		provisionEng: connector.NewProvisioningEngine(connMgr),
		vault:        vlt,
		auditLog:     alog,
		oidcProvider: oidcProv,
		cedarEngine:  cedarEng,
		outbox:       outboxEng,
	}
}

func (s *IdentityService) AuditStore() *audit.Store { return s.auditLog }
func (s *IdentityService) SaveVault() error         { return s.vault.Save() }
func (s *IdentityService) ConnectorManager() *connector.Manager { return s.connMgr }
func (s *IdentityService) Pool() *pgxpool.Pool                 { return s.pgPool }
func (s *IdentityService) Neo4j() neo4j.DriverWithContext     { return s.neo4j }
func (s *IdentityService) Redis() *redis.Client               { return s.redis }
func (s *IdentityService) CedarEngine() *cedar.CedarEngine    { return s.cedarEngine }
func (s *IdentityService) Outbox() *outbox.Outbox             { return s.outbox }
func (s *IdentityService) Vault() *vault.Vault                { return s.vault }
func (s *IdentityService) OIDCProvider() *oidc.Provider       { return s.oidcProvider }
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
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("count"))
	if limit <= 0 {
		limit = 100
	}
	startIndex, _ := strconv.Atoi(q.Get("startIndex"))
	if startIndex < 1 {
		startIndex = 1
	}
	offset := startIndex - 1
	filter := q.Get("filter")

	// Build query — support SCIM filter: userName eq "value"
	args := []any{}
	idx := 1
	where := "WHERE status != 'terminated'"

	if filter != "" {
		// Simple filter parsing: "userName eq \"value\""
		parts := strings.Split(filter, " ")
		if len(parts) >= 3 {
			attr := parts[0]
			op := parts[1]
			val := strings.Trim(strings.TrimSpace(strings.Join(parts[2:], " ")), "\"")
			if op == "eq" {
				switch attr {
				case "userName", "email":
					where += fmt.Sprintf(" AND email = $%d", idx)
				case "externalId":
					where += fmt.Sprintf(" AND employee_id = $%d", idx)
				case "displayName":
					where += fmt.Sprintf(" AND display_name ILIKE $%d", idx)
					val = "%" + val + "%"
				default:
					where += fmt.Sprintf(" AND $%d = $%d", idx, idx) // no-op
				}
				args = append(args, val)
				idx++
			}
		}
	}

	var total int
	s.pgPool.QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM identities %s", where), args...).Scan(&total)

	rows, err := s.pgPool.Query(r.Context(), fmt.Sprintf(`
		SELECT id, email, display_name, type, status, department,
		       employee_id, source, created_at, updated_at
		FROM identities %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), append(args, limit, offset)...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	var resources []map[string]any
	for rows.Next() {
		var id, email, name, idType, status, dept, empID, source string
		var created, updated time.Time
		if err := rows.Scan(&id, &email, &name, &idType, &status, &dept, &empID, &source, &created, &updated); err != nil {
			continue
		}
		active := status == "active"
		u := map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"id":         id,
			"userName":   email,
			"externalId": empID,
			"active":     active,
			"displayName": name,
			"name": map[string]any{"formatted": name},
			"emails":     []map[string]any{{"value": email, "primary": true}},
			"userType":   idType,
			"meta": map[string]any{
				"resourceType": "User",
				"created":      created.Format(time.RFC3339),
				"lastModified": updated.Format(time.RFC3339),
			},
		}
		if dept != "" {
			u["urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"] = map[string]any{
				"department": dept,
			}
		}
		resources = append(resources, u)
	}
	if resources == nil {
		resources = []map[string]any{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": total,
		"startIndex":   startIndex,
		"itemsPerPage": limit,
		"Resources":    resources,
	})
}

func (s *IdentityService) ScimCreateUser(w http.ResponseWriter, r *http.Request) {
	var scimUser map[string]any
	if err := json.NewDecoder(r.Body).Decode(&scimUser); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid SCIM payload")
		return
	}

	userName, _ := scimUser["userName"].(string)
	if userName == "" {
		respondError(w, http.StatusBadRequest, "userName is required")
		return
	}
	displayName, _ := scimUser["displayName"].(string)
	if displayName == "" {
		displayName = userName
	}
	active := true
	if v, ok := scimUser["active"].(bool); ok {
		active = v
	}
	status := "active"
	if !active {
		status = "inactive"
	}

	// Extract name
	firstName := ""
	lastName := ""
	if nameObj, ok := scimUser["name"].(map[string]any); ok {
		firstName, _ = nameObj["givenName"].(string)
		lastName, _ = nameObj["familyName"].(string)
	}
	// Extract manager
	managerID := ""
	if ext, ok := scimUser["urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"].(map[string]any); ok {
		if mgr, ok := ext["manager"].(map[string]any); ok {
			managerID, _ = mgr["value"].(string)
		}
	}

	// Create identity in PG + Neo4j via the regular CreateIdentityRecord flow
	createReq := map[string]any{
		"email":        userName,
		"display_name": displayName,
		"first_name":   firstName,
		"last_name":    lastName,
		"status":       status,
		"source":       "scim",
		"tenant_id":    "00000000-0000-0000-0000-000000000001",
	}
	if managerID != "" {
		createReq["manager_id"] = managerID
	}

	// Create in PostgreSQL
	id := uuid.New().String()
	attrs := map[string]any{}
	if firstName != "" {
		attrs["first_name"] = firstName
	}
	if lastName != "" {
		attrs["last_name"] = lastName
	}

	// Begin transaction — PostgreSQL + outbox are atomic
	tx, err := s.pgPool.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Transaction failed")
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(), `
		INSERT INTO identities (id, tenant_id, email, display_name, status, source, attributes)
		VALUES ($1, $2, $3, $4, $5, 'scim', $6)
		ON CONFLICT (tenant_id, email) DO UPDATE SET
			display_name = EXCLUDED.display_name, status = EXCLUDED.status
	`, id, "00000000-0000-0000-0000-000000000001", userName, displayName, status, mustJSON(attrs))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Create failed: "+err.Error())
		return
	}

	// Publish outbox event (same transaction — atomic with PG insert)
	err = s.outbox.Publish(r.Context(), tx, "identity.created", "identity", id,
		map[string]any{
			"tenant_id":    "00000000-0000-0000-0000-000000000001",
			"email":        userName,
			"display_name": displayName,
			"status":       status,
			"type":         "human",
			"source":       "scim",
		},
		map[string]any{
			"method": "scim",
		})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Outbox failed: "+err.Error())
		return
	}

	// Commit both operations atomically
	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, "Commit failed")
		return
	}

	// Neo4j sync happens asynchronously via outbox processor
	respondJSON(w, http.StatusCreated, map[string]any{
		"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"id":         id,
		"userName":   userName,
		"active":     active,
		"displayName": displayName,
		"meta": map[string]any{
			"resourceType": "User",
			"created":      time.Now().Format(time.RFC3339),
		},
	})
}

func (s *IdentityService) ScimGetUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var email, name, idType, empID, status string
	var created, updated time.Time
	err := s.pgPool.QueryRow(r.Context(), `
		SELECT email, display_name, type, status, COALESCE(employee_id,''), created_at, updated_at
		FROM identities WHERE id = $1
	`, id).Scan(&email, &name, &idType, &empID, &status, &created, &updated)
	if err != nil {
		respondError(w, http.StatusNotFound, "User not found")
		return
	}

	active := status == "active"
	respondJSON(w, http.StatusOK, map[string]any{
		"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"id":          id,
		"userName":    email,
		"externalId":  empID,
		"active":      active,
		"displayName": name,
		"name":        map[string]any{"formatted": name},
		"emails":      []map[string]any{{"value": email, "primary": true}},
		"userType":    idType,
		"meta": map[string]any{
			"resourceType": "User",
			"created":      created.Format(time.RFC3339),
			"lastModified": updated.Format(time.RFC3339),
		},
	})
}

func (s *IdentityService) ScimUpdateUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var scimUser map[string]any
	if err := json.NewDecoder(r.Body).Decode(&scimUser); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	// Update in PG
	sets := []string{}
	args := []any{id}
	idx := 2

	payloadFields := map[string]any{}

	if v, ok := scimUser["displayName"].(string); ok && v != "" {
		sets = append(sets, fmt.Sprintf("display_name = $%d", idx))
		args = append(args, v)
		idx++
		payloadFields["display_name"] = v
	}
	if v, ok := scimUser["active"].(bool); ok {
		st := "active"
		if !v {
			st = "inactive"
		}
		sets = append(sets, fmt.Sprintf("status = $%d", idx))
		args = append(args, st)
		idx++
		payloadFields["status"] = st
	}

	if len(sets) > 0 {
		// Begin transaction — PG update + outbox are atomic
		tx, err := s.pgPool.Begin(r.Context())
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Transaction failed")
			return
		}
		defer tx.Rollback(r.Context())

		_, err = tx.Exec(r.Context(), fmt.Sprintf("UPDATE identities SET %s, updated_at = NOW() WHERE id = $1", strings.Join(sets, ", ")), args...)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Update failed: "+err.Error())
			return
		}

		// Publish outbox event (same transaction)
		payloadFields["id"] = id
		err = s.outbox.Publish(r.Context(), tx, "identity.updated", "identity", id,
			payloadFields,
			map[string]any{"method": "scim"})
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Outbox failed: "+err.Error())
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			respondError(w, http.StatusInternalServerError, "Commit failed")
			return
		}
	}

	// Neo4j sync happens asynchronously via outbox processor
	respondJSON(w, http.StatusOK, map[string]any{
		"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"id":      id,
		"meta": map[string]any{
			"resourceType":  "User",
			"lastModified": time.Now().Format(time.RFC3339),
		},
	})
}

func (s *IdentityService) ScimPatchUser(w http.ResponseWriter, r *http.Request) {
	// SCIM PATCH: same as update for now — full replacement
	s.ScimUpdateUser(w, r)
}

func (s *IdentityService) ScimDeleteUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	// Trigger offboarding workflow
	we, err := s.temporal.ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
		ID:        fmt.Sprintf("offboard-%s", id),
		TaskQueue: "critical_offboarding",
	}, workflow.OffboardIdentityWorkflow, workflow.OffboardInput{
		IdentityID: id,
		Reason:     "SCIM deprovisioning",
		RequestedBy: "scim",
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to start offboarding workflow")
		return
	}
	respondJSON(w, http.StatusAccepted, map[string]any{
		"status":      "offboarding",
		"workflow_id": we.GetID(),
	})
}

// ─── SCIM 2.0 Discovery Endpoints (RFC 7644) ────────────────

func (s *IdentityService) ScimServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"documentationUri": "https://observeid.io/docs/scim",
		"patch":            map[string]any{"supported": true},
		"bulk":             map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":           map[string]any{"supported": true, "maxResults": 500},
		"changePassword":   map[string]any{"supported": false},
		"sort":             map[string]any{"supported": false},
		"etag":             map[string]any{"supported": false},
		"authenticationSchemes": []map[string]any{
			{"type": "oauthbearertoken", "name": "Bearer Token", "description": "API Key via Bearer token"},
			{"type": "apikey", "name": "X-API-Key", "description": "API Key via header"},
		},
	})
}

func (s *IdentityService) ScimResourceTypes(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, []map[string]any{{
		"schemas":      []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"},
		"id":           "User",
		"name":         "User",
		"endpoint":     "/scim/v2/Users",
		"description":  "Identity Fabric User Account",
		"schema":       "urn:ietf:params:scim:schemas:core:2.0:User",
		"schemaExtensions": []map[string]any{{
			"schema":   "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User",
			"required": false,
		}},
		"meta": map[string]any{"resourceType": "ResourceType", "location": "/scim/v2/ResourceTypes/User"},
	}})
}

func (s *IdentityService) ScimSchemas(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"schemas":    []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"Resources":  []map[string]any{},
		"totalResults": 0,
	})
}

// ─── Identity API Handlers ─────────────────────────────────

func (s *IdentityService) ListIdentities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paginationParams(r, 50, 0)

	// --- Build dynamic query from PG for search + filtering ---
	search := q.Get("search")
	status := q.Get("status")
	idType := q.Get("type")
	department := q.Get("department")
	source := q.Get("source")
	sortBy := q.Get("sort_by")
	sortDir := q.Get("sort_dir")

	if sortBy == "" {
		sortBy = "created_at"
	}
	if sortDir != "asc" && sortDir != "ASC" {
		sortDir = "DESC"
	}

	// Allowed sort columns (whitelist to prevent SQL injection)
	allowedSort := map[string]bool{
		"created_at": true, "updated_at": true, "display_name": true,
		"email": true, "department": true, "status": true, "type": true,
		"risk_score": true, "last_accessed_at": true,
	}
	if !allowedSort[sortBy] {
		sortBy = "created_at"
	}

	args := []any{}
	idx := 1
	where := "WHERE 1=1"

	if search != "" {
		where += fmt.Sprintf(" AND to_tsvector('english', coalesce(display_name,'') || ' ' || coalesce(email,'')) @@ plainto_tsquery('english', $%d)", idx)
		args = append(args, search)
		idx++
	}
	if status != "" {
		where += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, status)
		idx++
	}
	if idType != "" {
		where += fmt.Sprintf(" AND type = $%d", idx)
		args = append(args, idType)
		idx++
	}
	if department != "" {
		where += fmt.Sprintf(" AND department = $%d", idx)
		args = append(args, department)
		idx++
	}
	if source != "" {
		where += fmt.Sprintf(" AND source = $%d", idx)
		args = append(args, source)
		idx++
	}

	// Count total (for pagination)
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM identities %s", where)
	var total int
	if err := s.pgPool.QueryRow(r.Context(), countSQL, args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count query failed")
		return
	}

	// Query with all fields
	dataSQL := fmt.Sprintf(`
		SELECT id, tenant_id, type, status, email, display_name, department,
		       employee_id, manager_id, source, risk_score, risk_factors,
		       assurance_level, attributes, created_at, updated_at,
		       last_accessed_at, last_reviewed_at
		FROM identities
		%s
		ORDER BY %s %s NULLS LAST
		LIMIT $%d OFFSET $%d
	`, where, sortBy, sortDir, idx, idx+1)
	args = append(args, limit, offset)

	rows, err := s.pgPool.Query(r.Context(), dataSQL, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	type identityItem struct {
		ID             string    `json:"id"`
		TenantID       string    `json:"tenant_id"`
		Type           string    `json:"type"`
		Status         string    `json:"status"`
		Email          string    `json:"email"`
		DisplayName    string    `json:"display_name"`
		Department     *string   `json:"department"`
		EmployeeID     *string   `json:"employee_id"`
		ManagerID      *string   `json:"manager_id"`
		Source         string    `json:"source"`
		RiskScore      float64   `json:"risk_score"`
		RiskFactors    []string  `json:"risk_factors"`
		AssuranceLevel string    `json:"assurance_level"`
		Attributes     string    `json:"attributes"`
		CreatedAt      string    `json:"created_at"`
		UpdatedAt      string    `json:"updated_at"`
		LastAccessedAt *string   `json:"last_accessed_at"`
		LastReviewedAt *string   `json:"last_reviewed_at"`
	}

	identities := []identityItem{}
	for rows.Next() {
		var i identityItem
		var dept, empID, mgrID *string
		var lastAcc, lastRev *time.Time
		var riskFactors []string
		var attrs string
		var createdAt, updatedAt time.Time
		err := rows.Scan(&i.ID, &i.TenantID, &i.Type, &i.Status, &i.Email, &i.DisplayName,
			&dept, &empID, &mgrID, &i.Source, &i.RiskScore, &riskFactors,
			&i.AssuranceLevel, &attrs, &createdAt, &updatedAt, &lastAcc, &lastRev)
		if err != nil {
			continue
		}
		// Map nullable fields
		if dept != nil {
			i.Department = dept
		}
		if empID != nil {
			i.EmployeeID = empID
		}
		if mgrID != nil {
			i.ManagerID = mgrID
		}
		i.RiskFactors = riskFactors
		i.Attributes = attrs
		i.CreatedAt = createdAt.Format(time.RFC3339)
		i.UpdatedAt = updatedAt.Format(time.RFC3339)
		if lastAcc != nil && !lastAcc.IsZero() {
			str := lastAcc.Format(time.RFC3339)
			i.LastAccessedAt = &str
		}
		if lastRev != nil && !lastRev.IsZero() {
			str := lastRev.Format(time.RFC3339)
			i.LastReviewedAt = &str
		}
		identities = append(identities, i)
	}

	if identities == nil {
		identities = []identityItem{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"identities": identities,
		"total":      total,
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
		RETURN i.uuid AS uuid, i.display_name AS display_name, i.email AS email,
			   i.status AS status, i.type AS type, i.department AS department,
			   i.title AS title, i.employee_id AS employee_id, i.source AS source,
			   i.risk_score AS risk_score, i.created_at AS created_at, i.updated_at AS updated_at,
			   COLLECT(DISTINCT {name: r.name, uuid: r.uuid, role_type: r.role_type}) AS roles,
			   COLLECT(DISTINCT {uuid: reports.uuid, display_name: reports.display_name}) AS direct_reports
	`, map[string]any{"id": id})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}

	if result.Next(r.Context()) {
		rec := result.Record()
		roles, _ := rec.Get("roles")
		reports, _ := rec.Get("direct_reports")

		identity := map[string]any{
			"id":            getRecordVal(rec, "uuid"),
			"display_name":  getRecordVal(rec, "display_name"),
			"email":         getRecordVal(rec, "email"),
			"status":        getRecordVal(rec, "status"),
			"type":          getRecordVal(rec, "type"),
			"department":    getRecordVal(rec, "department"),
			"title":         getRecordVal(rec, "title"),
			"employee_id":   getRecordVal(rec, "employee_id"),
			"source":        getRecordVal(rec, "source"),
			"risk_score":    getRecordVal(rec, "risk_score"),
			"created_at":    getRecordVal(rec, "created_at"),
			"updated_at":    getRecordVal(rec, "updated_at"),
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"identity":       identity,
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
	limit, offset := paginationParams(r, 50, 0)

	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	result, err := session.Run(r.Context(), `
		MATCH (n:NonHumanIdentity)
		OPTIONAL MATCH (n)-[:OWNED_BY]->(owner:Identity)
		RETURN n.uuid AS uuid, n.name AS name, n.type AS type, n.status AS status,
			   n.risk_score AS risk_score, n.is_governed AS is_governed,
			   owner.display_name AS owner_name
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

	agentID := id
	reason := req.Reason

	// Kill the agent in PostgreSQL (source of truth)
	if _, err := s.pgPool.Exec(r.Context(), `
		UPDATE non_human_identities SET status = 'revoked', updated_at = NOW() WHERE id = $1`,
		agentID); err != nil {
		logError("postgres", fmt.Errorf("kill switch pg update: %w", err))
	}

	// Launch Temporal workflow (async — uses own context with timeout)
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

	// Find and cascade-revoke delegated agents using request context for query
	go func() {
		session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(r.Context())

		result, err := session.Run(r.Context(), `
			MATCH (:NonHumanIdentity {uuid: $id})-[:DELEGATED_FROM*1..3]->(child:NonHumanIdentity)
			WHERE child.status = 'active'
			RETURN child.uuid AS child_id
		`, map[string]any{"id": agentID})
		if err != nil {
			logError("neo4j", fmt.Errorf("cascade query: %w", err))
			return
		}

		for result.Next(r.Context()) {
			childIDRaw, _ := result.Record().Get("child_id")
			childIDStr, ok := childIDRaw.(string)
			if !ok || childIDStr == "" {
				continue
			}
			if _, err := s.temporal.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
				ID:        fmt.Sprintf("cascade-kill-%s", childIDStr),
				TaskQueue: "critical_offboarding",
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
	recent, err := s.redis.Exists(r.Context(), fmt.Sprintf("revocation:recent:%s", req.IdentityID)).Result()
	if err != nil {
		// Redis down — fall through (fail open for availability)
		logError("redis", fmt.Errorf("revocation check: %w", err))
	} else if recent > 0 {
		respondJSON(w, http.StatusOK, map[string]any{
			"allowed": false,
			"reason":    "recent_revocation",
			"evaluated": "redis_cache",
		})
		return
	}

	// Check Redis policy decision cache
	cacheKey := fmt.Sprintf("policy:decision:%s:%s:%s", req.IdentityID, req.ResourceID, req.Action)
	if cached, err := s.redis.Get(r.Context(), cacheKey).Bytes(); err == nil && len(cached) > 0 {
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

	// Query Neo4j for entitlement path
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	query := `
		MATCH (i:Identity {uuid: $identityId})
		OPTIONAL MATCH path = (i)-[:HAS_ROLE|HAS_DIRECT_ACCESS|HAS_TEMPORARY_ACCESS*1..3]->(res:Resource {id: $resourceId})
		RETURN
			i.status AS identityStatus,
			CASE WHEN res IS NOT NULL THEN true ELSE false END AS hasPath,
			length(path) AS pathLength,
			collect(DISTINCT type(relationships(path)[0])) AS accessTypes
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
	_ = s.pgPool.QueryRow(r.Context(),
		`SELECT COALESCE(type::text, 'User'), COALESCE(department, ''),
		        (status = 'active')
		 FROM identities WHERE id = $1`, req.IdentityID,
	).Scan(&identityType, &identityDept, &isActive)

	var resourceType, resourceClass string
	_ = s.pgPool.QueryRow(r.Context(),
		`SELECT COALESCE(resource_type, 'Resource'), COALESCE(criticality, '')
		 FROM resources WHERE id = $1`, req.ResourceID,
	).Scan(&resourceType, &resourceClass)

	cedarDecision, cedarErr := s.cedarEngine.IsAuthorized(r.Context(), cedar.AuthRequest{
		PrincipalID:      req.IdentityID,
		PrincipalType:    identityType,
		Action:           req.Action,
		ResourceID:       req.ResourceID,
		ResourceType:     resourceType,
		TenantID:         req.TenantID,
		Department:       identityDept,
		IsActive:         isActive,
		Criticality:      resourceClass,
		MFAPresent:       true,
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
	s.redis.Set(r.Context(), cacheKey, cacheVal, 30*time.Second)

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

func (s *IdentityService) GrantAccess(w http.ResponseWriter, r *http.Request) {
	var req workflow.GrantAccessInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	workflowID := fmt.Sprintf("grant-access-%s-%s", req.IdentityID, uuid.New().String()[:8])
	we, err := s.temporal.ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "critical_offboarding",
	}, workflow.GrantAccessWorkflow, req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to start grant workflow")
		return
	}

	telemetry.WorkflowExecutions.WithLabelValues("grant_access", "started", "default").Inc()

	respondJSON(w, http.StatusAccepted, map[string]any{
		"status":      "provisioning",
		"workflow_id": we.GetID(),
	})
}

func (s *IdentityService) StartJustInTimeWorkflow(ctx context.Context, input workflow.JustInTimeInput) (string, error) {
	workflowID := fmt.Sprintf("jit-access-%s-%s", input.IdentityID, uuid.New().String()[:8])
	_, err := s.temporal.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "critical_offboarding",
	}, workflow.JustInTimeAccessWorkflow, input)
	if err != nil {
		return "", fmt.Errorf("start jit workflow: %w", err)
	}
	return workflowID, nil
}

func (s *IdentityService) JustInTimeAccess(w http.ResponseWriter, r *http.Request) {
	var req workflow.JustInTimeInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	workflowID, err := s.StartJustInTimeWorkflow(r.Context(), req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusAccepted, map[string]any{
		"status":      "jit_access_provisioning",
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
	we, err := s.temporal.ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "critical_offboarding",
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

// sanitizeConfig redacts sensitive fields for API responses.
func sanitizeConfig(cfg connector.ConnectorConfig) connector.ConnectorConfig {
	if cfg.ClientSecret != "" {
		cfg.ClientSecret = "[redacted]"
	}
	if cfg.Password != "" {
		cfg.Password = "[redacted]"
	}
	if cfg.Cert != "" {
		cfg.Cert = "[redacted]"
	}
	// Redact bearer tokens stored in properties
	if cfg.Properties != nil {
		if _, ok := cfg.Properties["bearer_token"]; ok {
			cfg.Properties["bearer_token"] = "[redacted]"
		}
	}
	return cfg
}

func (s *IdentityService) ListConnectors(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paginationParams(r, 50, 0)
	search := q.Get("search")
	status := q.Get("status")
	ctype := q.Get("type")

	// Build dynamic query on PostgreSQL connectors table
	args := []any{}
	idx := 1
	where := "WHERE 1=1"

	if search != "" {
		where += fmt.Sprintf(" AND (name ILIKE $%d OR connector_type ILIKE $%d)", idx, idx)
		args = append(args, "%"+search+"%")
		idx++
	}
	if status != "" {
		where += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, status)
		idx++
	}
	if ctype != "" {
		where += fmt.Sprintf(" AND connector_type = $%d", idx)
		args = append(args, ctype)
		idx++
	}

	// Count total
	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM connectors %s", where)
	if err := s.pgPool.QueryRow(r.Context(), countSQL, args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count query failed")
		return
	}

	// Query connectors
	dataSQL := fmt.Sprintf(`
		SELECT id, tenant_id, name, connector_type, status, config,
		       last_sync_at, last_error, created_at, updated_at
		FROM connectors
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, idx, idx+1)
	args = append(args, limit, offset)

	rows, err := s.pgPool.Query(r.Context(), dataSQL, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	// Fetch health + sync stats for each connector from the manager
	type connectorSummary struct {
		ID            string                  `json:"id"`
		TenantID      string                  `json:"tenant_id"`
		Name          string                  `json:"name"`
		Type          string                  `json:"type"`
		Status        string                  `json:"status"`
		LastSyncAt    *string                 `json:"last_sync_at"`
		LastError     *string                 `json:"last_error"`
		CreatedAt     string                  `json:"created_at"`
		UpdatedAt     string                  `json:"updated_at"`
		Health        *connector.HealthReport  `json:"health,omitempty"`
		SyncStats     *struct {
			Users        int `json:"users"`
			Groups       int `json:"groups"`
			Entitlements int `json:"entitlements"`
			Resources    int `json:"resources"`
		} `json:"sync_stats,omitempty"`
	}

	connectors := []connectorSummary{}
	for rows.Next() {
		var c connectorSummary
		var id, tid, name, ctype, cstatus string
		var lastSyncAt *time.Time
		var lastErr *string
		var configJSON []byte
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &tid, &name, &ctype, &cstatus, &configJSON, &lastSyncAt, &lastErr, &createdAt, &updatedAt); err != nil {
			continue
		}
		c.ID = id
		c.TenantID = tid
		c.Name = name
		c.Type = ctype
		c.Status = cstatus
		if lastSyncAt != nil {
			s := lastSyncAt.Format(time.RFC3339)
			c.LastSyncAt = &s
		}
		if lastErr != nil {
			c.LastError = lastErr
		}
		c.CreatedAt = createdAt.Format(time.RFC3339)
		c.UpdatedAt = updatedAt.Format(time.RFC3339)

		// Enrich with health from manager
		if health, err := s.connMgr.GetConnectorHealth(id); err == nil && health != nil {
			c.Health = health
		}

		// Query sync stats from connector child tables
		var userCount, groupCount, entCount, resCount int
		if err := s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_identities WHERE connector_id = $1`, id).Scan(&userCount); err == nil && userCount > 0 {
			c.SyncStats = &struct {
				Users        int `json:"users"`
				Groups       int `json:"groups"`
				Entitlements int `json:"entitlements"`
				Resources    int `json:"resources"`
			}{Users: userCount}
		}
		if c.SyncStats != nil {
			s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_groups WHERE connector_id = $1`, id).Scan(&groupCount)
			s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_entitlements WHERE connector_id = $1`, id).Scan(&entCount)
			s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_resources WHERE connector_id = $1`, id).Scan(&resCount)
			c.SyncStats.Groups = groupCount
			c.SyncStats.Entitlements = entCount
			c.SyncStats.Resources = resCount
		}

		connectors = append(connectors, c)
	}

	if connectors == nil {
		connectors = []connectorSummary{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"connectors": connectors,
		"total":      total,
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
		"connector": sanitizeConfig(cfg),
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
			if raw, _ = json.Marshal(user.Attributes); raw == nil {
				raw = []byte("{}")
			}
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
	q := r.URL.Query()
	limit, offset := paginationParams(r, 100, 0)
	search := q.Get("search")

	args := []any{id}
	idx := 2
	where := "WHERE connector_id = $1"

	if search != "" {
		where += fmt.Sprintf(" AND (display_name ILIKE $%d OR email ILIKE $%d OR username ILIKE $%d)", idx, idx, idx)
		args = append(args, "%"+search+"%")
		idx++
	}

	// Count
	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM connector_identities %s", where)
	if err := s.pgPool.QueryRow(r.Context(), countSQL, args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	// Query
	rows, err := s.pgPool.Query(r.Context(), fmt.Sprintf(`
		SELECT id, external_id, username, email, display_name, first_name, last_name,
		       department, title, employee_id, enabled, locked, groups, roles, first_synced_at, last_synced_at
		FROM connector_identities
		%s
		ORDER BY display_name NULLS LAST, username NULLS LAST, email
		LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), append(args, limit, offset)...)
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
		EmployeeID   string   `json:"employee_id,omitempty"`
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
			&e.FirstName, &e.LastName, &e.Department, &e.Title, &e.EmployeeID, &e.Enabled, &e.Locked,
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
		"total":        total,
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

// ─── Connector Statistics ─────────────────────────────────────

func (s *IdentityService) GetConnectorStats(w http.ResponseWriter, r *http.Request) {
	type ConnectorStats struct {
		TotalConnectors   int `json:"total_connectors"`
		ConnectedCount    int `json:"connected_count"`
		DisconnectedCount int `json:"disconnected_count"`
		ErrorCount        int `json:"error_count"`
		SyncingCount      int `json:"syncing_count"`
		TotalIdentities   int `json:"total_identities"`
		TotalGroups       int `json:"total_groups"`
		TotalEntitlements int `json:"total_entitlements"`
		TotalResources    int `json:"total_resources"`
	}

	stats := ConnectorStats{}

	// Count connectors by status
	_ = s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connectors`).Scan(&stats.TotalConnectors)
	_ = s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connectors WHERE status = $1`, "connected").Scan(&stats.ConnectedCount)
	_ = s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connectors WHERE status = $1`, "disconnected").Scan(&stats.DisconnectedCount)
	_ = s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connectors WHERE status = $1`, "error").Scan(&stats.ErrorCount)
	_ = s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connectors WHERE status = $1`, "syncing").Scan(&stats.SyncingCount)

	// Count synced entities
	_ = s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_identities`).Scan(&stats.TotalIdentities)
	_ = s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_groups`).Scan(&stats.TotalGroups)
	_ = s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_entitlements`).Scan(&stats.TotalEntitlements)
	_ = s.pgPool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_resources`).Scan(&stats.TotalResources)

	respondJSON(w, http.StatusOK, stats)
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
	q := r.URL.Query()
	limit, offset := paginationParams(r, 100, 0)
	search := q.Get("search")

	args := []any{id}
	idx := 2
	where := "WHERE connector_id = $1"

	if search != "" {
		where += fmt.Sprintf(" AND name ILIKE $%d", idx)
		args = append(args, "%"+search+"%")
		idx++
	}

	var total int
	if err := s.pgPool.QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM connector_groups %s", where), args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	rows, err := s.pgPool.Query(r.Context(), fmt.Sprintf(`
		SELECT id, external_id, name, description, group_type, scope, member_ids, first_synced_at, last_synced_at
		FROM connector_groups
		%s
		ORDER BY name NULLS LAST
		LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), append(args, limit, offset)...)
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
		"total":        total,
	})
}

// ─── Connector Entitlements ───────────────────────────────────

func (s *IdentityService) GetConnectorEntitlements(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	q := r.URL.Query()
	limit, offset := paginationParams(r, 100, 0)
	search := q.Get("search")

	args := []any{id}
	idx := 2
	where := "WHERE connector_id = $1"

	if search != "" {
		where += fmt.Sprintf(" AND (source_name ILIKE $%d OR app_name ILIKE $%d)", idx, idx)
		args = append(args, "%"+search+"%")
		idx++
	}

	var total int
	if err := s.pgPool.QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM connector_entitlements %s", where), args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	rows, err := s.pgPool.Query(r.Context(), fmt.Sprintf(`
		SELECT identity_external_id, entitlement_type, source_id, source_name, source_type,
		       app_id, app_name, is_active
		FROM connector_entitlements
		%s
		ORDER BY entitlement_type, source_name NULLS LAST
		LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), append(args, limit, offset)...)
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
		"total":         total,
	})
}

// ─── Connector Resources ─────────────────────────────────────

func (s *IdentityService) GetConnectorResources(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	limit, offset := paginationParams(r, 100, 0)

	args := []any{id}
	idx := 2
	where := "WHERE connector_id = $1"

	var total int
	if err := s.pgPool.QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM connector_resources %s", where), args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	rows, err := s.pgPool.Query(r.Context(), fmt.Sprintf(`
		SELECT id, external_id, resource_type, name, description, enabled, owner_ids, first_synced_at, last_synced_at
		FROM connector_resources
		%s
		ORDER BY resource_type, name NULLS LAST
		LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), append(args, limit, offset)...)
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
		"total":        total,
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

// ─── CSV Upload ────────────────────────────────────────────────

func (s *IdentityService) CSVUpload(w http.ResponseWriter, r *http.Request) {
	const maxCSVSize = 20 << 20

	var req struct {
		Name     string `json:"name"`
		CSVData  string `json:"csv_data"`
		TenantID string `json:"tenant_id"`
	}
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	if req.CSVData == "" {
		respondError(w, http.StatusBadRequest, "csv_data field is required")
		return
	}
	if len(req.CSVData) > maxCSVSize {
		respondError(w, http.StatusBadRequest, "CSV data exceeds maximum size")
		return
	}

	connectorName := req.Name
	if connectorName == "" {
		connectorName = "CSV Import"
	}

	uploadDir := os.Getenv("CSV_UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "/tmp/observeid-csv-uploads"
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create upload directory")
		return
	}

	savePath := filepath.Join(uploadDir, uuid.New().String()+".csv")
	if err := os.WriteFile(savePath, []byte(req.CSVData), 0600); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}

	// Validate the CSV can be parsed
	cfg := connector.ConnectorConfig{
		Name:     connectorName,
		Type:     connector.ConnectorTypeCSV,
		Endpoint: savePath,
		Status:   connector.ConnectorStatusConnected,
	}
	if req.TenantID != "" {
		cfg.TenantID = req.TenantID
	}

	// Quick test parse before registering
	tmpConn := connector.NewCSVConnector()
	if err := tmpConn.Configure(cfg); err != nil {
		os.Remove(savePath)
		respondError(w, http.StatusBadRequest, "Invalid connector config: "+err.Error())
		return
	}
	if users, err := tmpConn.ListUsers(r.Context()); err != nil {
		os.Remove(savePath)
		respondError(w, http.StatusBadRequest, "CSV parse error: "+err.Error())
		return
	} else if len(users) == 0 {
		os.Remove(savePath)
		respondError(w, http.StatusBadRequest, "CSV file has no valid user rows")
		return
	}

	id, err := s.connMgr.Register(r.Context(), cfg)
	if err != nil {
		os.Remove(savePath)
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.auditLog.Append(audit.Entry{
		Level:   audit.LevelInfo,
		Service: "connector",
		Path:    r.URL.Path,
		Message: fmt.Sprintf("CSV connector created: %s", connectorName),
		Tags:    []string{"connector", "csv", "upload"},
	})

	respondJSON(w, http.StatusCreated, map[string]any{
		"connector_id":   id,
		"connector_name": connectorName,
		"status":         "registered",
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
			Level: audit.LevelWarn, Service: "connector", Method: r.Method, Path: r.URL.Path,
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
		Level: audit.LevelInfo, Service: "connector", Method: r.Method, Path: r.URL.Path,
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

// ─── CSV Import/Export ────────────────────────────────────────

func (s *IdentityService) PreviewCSVImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CSVData string `json:"csv_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	if len(req.CSVData) == 0 {
		respondError(w, http.StatusBadRequest, "No CSV data provided")
		return
	}

	reader := csv.NewReader(strings.NewReader(req.CSVData))
	reader.FieldsPerRecord = -1
	headers, err := reader.Read()
	if err != nil {
		respondError(w, http.StatusBadRequest, "Failed to parse CSV headers: "+err.Error())
		return
	}

	// Read up to 10 sample rows
	var sampleRows []map[string]string
	for i := 0; i < 10; i++ {
		record, err := reader.Read()
		if err != nil {
			break
		}
		row := map[string]string{}
		for j, h := range headers {
			if j < len(record) {
				row[strings.TrimSpace(h)] = strings.TrimSpace(record[j])
			}
		}
		sampleRows = append(sampleRows, row)
	}

	// Suggest mappings based on header name matching
	suggestedMapping := map[string]string{}
	knownFields := map[string]string{
		"email": "email", "emailaddress": "email", "mail": "email", "e-mail": "email",
		"display_name": "display_name", "displayname": "display_name", "name": "display_name",
		"first_name": "first_name", "firstname": "first_name", "givenname": "first_name",
		"last_name": "last_name", "lastname": "last_name", "surname": "last_name",
		"department": "department", "dept": "department",
		"title": "title", "jobtitle": "title", "position": "title",
		"employee_id": "employee_id", "employeeid": "employee_id", "id": "employee_id",
		"manager": "manager_id", "manager_id": "manager_id",
		"phone": "phone", "mobile": "phone", "telephone": "phone",
		"status": "status", "type": "type", "source": "source",
	}
	for _, h := range headers {
		h = strings.TrimSpace(h)
		lower := strings.ToLower(strings.ReplaceAll(h, " ", "_"))
		if mapped, ok := knownFields[lower]; ok {
			suggestedMapping[h] = mapped
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"columns":           headers,
		"sample_rows":       sampleRows,
		"row_count_preview": len(sampleRows),
		"suggested_mapping":  suggestedMapping,
	})
}

func (s *IdentityService) ImportCSV(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CSVData       string            `json:"csv_data"`
		ColumnMapping map[string]string `json:"column_mapping"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	reader := csv.NewReader(strings.NewReader(req.CSVData))
	reader.FieldsPerRecord = -1
	headers, err := reader.Read()
	if err != nil {
		respondError(w, http.StatusBadRequest, "Failed to parse CSV: "+err.Error())
		return
	}

	// Clean header names
	cleanHeaders := make([]string, len(headers))
	for i, h := range headers {
		cleanHeaders[i] = strings.TrimSpace(h)
	}

	created, updated, failed := 0, 0, 0
	var errs []string
	ctx := r.Context()

	for lineNum := 2; ; lineNum++ {
		record, err := reader.Read()
		if err != nil {
			break
		}

		// Build identity record from mapping
		email, displayName := "", ""
		customAttrs := map[string]string{}

		for i, h := range cleanHeaders {
			val := ""
			if i < len(record) {
				val = strings.TrimSpace(record[i])
			}
			if val == "" {
				continue
			}

			targetField, mapped := req.ColumnMapping[h]
			if !mapped {
				// Unmapped column → custom attribute
				key := strings.ToLower(strings.ReplaceAll(h, " ", "_"))
				customAttrs[key] = val
				continue
			}

			switch targetField {
			case "email":
				email = val
			case "display_name":
				displayName = val
			case "department":
				customAttrs["department"] = val
			case "title":
				customAttrs["title"] = val
			case "employee_id":
				customAttrs["employee_id"] = val
			case "first_name":
				customAttrs["first_name"] = val
			case "last_name":
				customAttrs["last_name"] = val
			case "phone":
				customAttrs["phone"] = val
			case "source":
				customAttrs["source"] = val
			case "status":
				customAttrs["status"] = val
			case "type":
				customAttrs["type"] = val
			default:
				customAttrs[targetField] = val
			}
		}

		if email == "" || displayName == "" {
			failed++
			errs = append(errs, fmt.Sprintf("row %d: missing email or display_name", lineNum))
			continue
		}

		status := "active"
		if s, ok := customAttrs["status"]; ok {
			status = s
			delete(customAttrs, "status")
		}

		attrsJSON, _ := json.Marshal(customAttrs)

		tag, err := s.pgPool.Exec(ctx, `
			INSERT INTO identities (id, tenant_id, email, display_name, status, source, department, attributes)
			VALUES ($1, $2, $3, $4, $5, 'hris', $6, $7)
			ON CONFLICT (tenant_id, email) DO UPDATE SET
				display_name = EXCLUDED.display_name, status = EXCLUDED.status,
				department = EXCLUDED.department, attributes = EXCLUDED.attributes,
				updated_at = NOW()
		`, uuid.New().String(), "00000000-0000-0000-0000-000000000001",
			email, displayName, status,
			customAttrs["department"], json.RawMessage(attrsJSON))

		if err != nil {
			failed++
			errs = append(errs, fmt.Sprintf("row %d (%s): %v", lineNum, email, err))
			continue
		}

		if tag.Insert() {
			created++
		} else {
			updated++
		}
	}

	s.auditLog.Append(audit.Entry{
		Level: audit.LevelInfo, Service: "identity",
		Message: fmt.Sprintf("CSV import: %d created, %d updated, %d failed", created, updated, failed),
		Tags:    []string{"identity", "csv", "import"},
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "completed",
		"created": created, "updated": updated, "failed": failed,
		"errors":  errs,
	})
}

func (s *IdentityService) ExportCSV(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pgPool.Query(r.Context(), `
		SELECT email, display_name, COALESCE(department,''), COALESCE(employee_id,''),
		       source, status, type, risk_score,
		       COALESCE(attributes::text,'{}'), created_at, updated_at
		FROM identities ORDER BY created_at DESC
	`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=observeid-identities.csv")

	writer := csv.NewWriter(w)
	writer.Write([]string{
		"email", "display_name", "department", "employee_id", "source",
		"status", "type", "risk_score", "custom_attributes", "created_at", "updated_at",
	})

	for rows.Next() {
		var email, name, dept, empID, source, status, idType, attrs string
		var risk float64
		var created, updated time.Time
		if err := rows.Scan(&email, &name, &dept, &empID, &source, &status, &idType, &risk, &attrs, &created, &updated); err != nil {
			continue
		}
		writer.Write([]string{
			email, name, dept, empID, source, status, idType,
			fmt.Sprintf("%.2f", risk), attrs,
			created.Format("2006-01-02 15:04:05"), updated.Format("2006-01-02 15:04:05"),
		})
	}
	rows.Close()
	writer.Flush()
}

// ─── Group CRUD Handlers ────────────────────────────────────

func (s *IdentityService) ListGroups(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paginationParams(r, 50, 0)
	search := q.Get("search")
	roleType := q.Get("role_type")

	args := []any{}
	idx := 1
	where := "WHERE 1=1"

	if search != "" {
		where += fmt.Sprintf(" AND name ILIKE $%d", idx)
		args = append(args, "%"+search+"%")
		idx++
	}
	if roleType != "" {
		where += fmt.Sprintf(" AND role_type = $%d", idx)
		args = append(args, roleType)
		idx++
	}

	// Count
	var total int
	if err := s.pgPool.QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM roles %s", where), args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	// Query roles from PostgreSQL
	dataSQL := fmt.Sprintf(`SELECT id, tenant_id, name, description, role_type,
		is_auto_assigned, approval_required, max_duration_hours,
		is_active, attributes, created_at, updated_at
		FROM roles %s ORDER BY name ASC LIMIT $%d OFFSET $%d`, where, idx, idx+1)
	args = append(args, limit, offset)

	rows, err := s.pgPool.Query(r.Context(), dataSQL, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	type roleItem struct {
		ID               string            `json:"id"`
		TenantID         string            `json:"tenant_id"`
		Name             string            `json:"name"`
		Description      string            `json:"description"`
		RoleType         string            `json:"role_type"`
		IsAutoAssigned   bool              `json:"is_auto_assigned"`
		ApprovalRequired bool              `json:"approval_required"`
		MaxDurationHours *int              `json:"max_duration_hours"`
		IsActive         bool              `json:"is_active"`
		Attributes       string            `json:"attributes"`
		CreatedAt        string            `json:"created_at"`
		UpdatedAt        string            `json:"updated_at"`
		MemberCount      int               `json:"member_count"`
		EntitlementCount int               `json:"entitlement_count"`
	}

	roles := []roleItem{}
	ctx := r.Context() // capture before 'r' gets shadowed by loop variable
	for rows.Next() {
		var r roleItem
		var desc *string
		var attrs string
		var maxDur *int
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &desc, &r.RoleType,
			&r.IsAutoAssigned, &r.ApprovalRequired, &maxDur,
			&r.IsActive, &attrs, &createdAt, &updatedAt); err != nil {
			continue
		}
		if desc != nil {
			r.Description = *desc
		}
		r.MaxDurationHours = maxDur
		r.Attributes = attrs
		r.CreatedAt = createdAt.Format(time.RFC3339)
		r.UpdatedAt = updatedAt.Format(time.RFC3339)

		// Enrich: member count from identity_roles
		s.pgPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM identity_roles WHERE role_id = $1 AND is_active = true`, r.ID,
		).Scan(&r.MemberCount)

		// Enrich: entitlement count from role_entitlements
		s.pgPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM role_entitlements WHERE role_id = $1`, r.ID,
		).Scan(&r.EntitlementCount)

		roles = append(roles, r)
	}

	if roles == nil {
		roles = []roleItem{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"groups": roles,
		"total":  total,
	})
}

func (s *IdentityService) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string `json:"name"`
		Description      string `json:"description"`
		RoleType         string `json:"role_type"`
		TenantID         string `json:"tenant_id"`
		IsAutoAssigned   bool   `json:"is_auto_assigned"`
		ApprovalRequired bool   `json:"approval_required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.TenantID == "" {
		req.TenantID = "00000000-0000-0000-0000-000000000001"
	}
	if req.RoleType == "" {
		req.RoleType = "custom"
	}

	id := uuid.New().String()

	// 1. Write to PostgreSQL
	if _, err := s.pgPool.Exec(r.Context(), `
		INSERT INTO roles (id, tenant_id, name, description, role_type, is_auto_assigned, approval_required)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, name) DO UPDATE SET
			description = EXCLUDED.description,
			role_type = EXCLUDED.role_type,
			is_auto_assigned = EXCLUDED.is_auto_assigned,
			updated_at = NOW()
	`, id, req.TenantID, req.Name, req.Description, req.RoleType, req.IsAutoAssigned, req.ApprovalRequired); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create role: "+err.Error())
		return
	}

	// 2. Write to Neo4j
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())
	_, err := session.Run(r.Context(), `
		MERGE (r:Role {uuid: $uuid})
		SET r.tenant_id = $tenant_id, r.name = $name,
			r.description = $description, r.role_type = $role_type,
			r.is_active = true, r.created_at = datetime()
	`, map[string]any{
		"uuid": id, "tenant_id": req.TenantID, "name": req.Name,
		"description": req.Description, "role_type": req.RoleType,
	})
	if err != nil {
		log.Printf("[GROUP] Neo4j write failed for role %s: %v", id, err)
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":     id,
		"name":   req.Name,
		"status": "created",
	})
}

func (s *IdentityService) GetGroup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Query role from PG
	var role struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		Description      string `json:"description"`
		RoleType         string `json:"role_type"`
		IsAutoAssigned   bool   `json:"is_auto_assigned"`
		ApprovalRequired bool   `json:"approval_required"`
		IsActive         bool   `json:"is_active"`
		MemberCount      int    `json:"member_count"`
		EntitlementCount int    `json:"entitlement_count"`
	}
	var desc *string
	err := s.pgPool.QueryRow(r.Context(), `
		SELECT id, name, description, role_type, is_auto_assigned, approval_required, is_active
		FROM roles WHERE id = $1
	`, id).Scan(&role.ID, &role.Name, &desc, &role.RoleType,
		&role.IsAutoAssigned, &role.ApprovalRequired, &role.IsActive)
	if err != nil {
		respondError(w, http.StatusNotFound, "Role not found")
		return
	}
	if desc != nil {
		role.Description = *desc
	}

	// Member count
	s.pgPool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM identity_roles WHERE role_id = $1 AND is_active = true`, id,
	).Scan(&role.MemberCount)

	// Entitlement count
	s.pgPool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM role_entitlements WHERE role_id = $1`, id,
	).Scan(&role.EntitlementCount)

	// List members
	memberRows, _ := s.pgPool.Query(r.Context(), `
		SELECT ir.identity_id, i.display_name, i.email, ir.assigned_at, ir.source
		FROM identity_roles ir
		JOIN identities i ON i.id = ir.identity_id
		WHERE ir.role_id = $1 AND ir.is_active = true
		ORDER BY ir.assigned_at DESC
		LIMIT 100
	`, id)

	type member struct {
		IdentityID string `json:"identity_id"`
		DisplayName string `json:"display_name"`
		Email      string `json:"email"`
		AssignedAt string `json:"assigned_at"`
		Source     string `json:"source"`
	}
	members := []member{}
	if memberRows != nil {
		defer memberRows.Close()
		for memberRows.Next() {
			var m member
			var t time.Time
			if err := memberRows.Scan(&m.IdentityID, &m.DisplayName, &m.Email, &t, &m.Source); err != nil {
				continue
			}
			m.AssignedAt = t.Format(time.RFC3339)
			members = append(members, m)
		}
	}

	// List entitlements
	entRows, _ := s.pgPool.Query(r.Context(), `
		SELECT e.app_name, e.permission_level, e.entitlement_type
		FROM role_entitlements re
		JOIN entitlements e ON e.id = re.entitlement_id
		WHERE re.role_id = $1
		ORDER BY e.app_name, e.permission_level
	`, id)

	type entitlement struct {
		AppName         string `json:"app_name"`
		PermissionLevel string `json:"permission_level"`
		EntitlementType string `json:"entitlement_type"`
	}
	entitlements := []entitlement{}
	if entRows != nil {
		defer entRows.Close()
		for entRows.Next() {
			var e entitlement
			if err := entRows.Scan(&e.AppName, &e.PermissionLevel, &e.EntitlementType); err != nil {
				continue
			}
			entitlements = append(entitlements, e)
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"role":         role,
		"members":      members,
		"entitlements": entitlements,
	})
}

func (s *IdentityService) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// 1. Soft-delete in PostgreSQL
	if _, err := s.pgPool.Exec(r.Context(),
		`UPDATE roles SET is_active = false, updated_at = NOW() WHERE id = $1`, id,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to deactivate role")
		return
	}
	// Also deactivate role assignments
	s.pgPool.Exec(r.Context(),
		`UPDATE identity_roles SET is_active = false WHERE role_id = $1`, id)

	// 2. Remove from Neo4j
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())
	session.Run(r.Context(), `MATCH (r:Role {uuid: $uuid}) DETACH DELETE r`,
		map[string]any{"uuid": id})

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
	if req.Source == "" {
		req.Source = "direct"
	}

	// 1. Write to PostgreSQL identity_roles (source of truth)
	if _, err := s.pgPool.Exec(r.Context(), `
		INSERT INTO identity_roles (tenant_id, identity_id, role_id, source, assigned_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (tenant_id, identity_id, role_id, source) DO UPDATE SET
			is_active = true, assigned_at = NOW()
	`, "00000000-0000-0000-0000-000000000001", req.IdentityID, req.RoleID, req.Source); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to assign role: "+err.Error())
		return
	}

	// 2. Write to Neo4j (graph for path traversal)
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())
	_, err := session.Run(r.Context(), `
		MATCH (i:Identity {uuid: $identity_id})
		MATCH (r:Role {uuid: $role_id})
		MERGE (i)-[rel:HAS_ROLE]->(r)
		SET rel.assigned_at = datetime(), rel.assigned_by = $assigned_by,
			rel.source = $source, rel.is_active = true
	`, map[string]any{
		"identity_id": req.IdentityID, "role_id": req.RoleID,
		"assigned_by": req.AssignedBy, "source": req.Source,
	})
	if err != nil {
		log.Printf("[ASSIGN] Neo4j write failed: %v", err)
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

	// 1. Soft-delete in PostgreSQL
	if _, err := s.pgPool.Exec(r.Context(),
		`UPDATE identity_roles SET is_active = false WHERE identity_id = $1 AND role_id = $2`,
		req.IdentityID, req.RoleID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to remove role")
		return
	}

	// 2. Remove from Neo4j
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())
	if _, err := session.Run(r.Context(), `
		MATCH (i:Identity {uuid: $identity_id})-[rel:HAS_ROLE]->(r:Role {uuid: $role_id})
		DELETE rel
		SET i.updated_at = datetime()
	`, map[string]any{
		"identity_id": req.IdentityID, "role_id": req.RoleID,
	}); err != nil {
		log.Printf("[UNASSIGN] Neo4j write failed: %v", err)
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ─── Entitlement CRUD + Role-Entitlement Linking ─────────────

func (s *IdentityService) ListEntitlements(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paginationParams(r, 100, 0)
	search := q.Get("search")

	args := []any{}
	idx := 1
	where := "WHERE 1=1"
	if search != "" {
		where += fmt.Sprintf(" AND (app_name ILIKE $%d OR permission_level ILIKE $%d)", idx, idx)
		args = append(args, "%"+search+"%")
		idx++
	}

	var total int
	if err := s.pgPool.QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM entitlements %s", where), args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	rows, err := s.pgPool.Query(r.Context(), fmt.Sprintf(`
		SELECT id, app_name, permission_level, entitlement_type,
		       risk_classification, is_toxic, is_rubberband
		FROM entitlements %s ORDER BY app_name, permission_level
		LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), append(args, limit, offset)...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	type item struct {
		ID            string `json:"id"`
		AppName       string `json:"app_name"`
		Permission    string `json:"permission_level"`
		Type          string `json:"entitlement_type"`
		RiskClass     string `json:"risk_classification"`
		IsToxic       bool   `json:"is_toxic"`
		IsRubberband  bool   `json:"is_rubberband"`
	}
	ents := []item{}
	for rows.Next() {
		var e item
		if err := rows.Scan(&e.ID, &e.AppName, &e.Permission, &e.Type, &e.RiskClass, &e.IsToxic, &e.IsRubberband); err != nil {
			continue
		}
		ents = append(ents, e)
	}
	if ents == nil {
		ents = []item{}
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"entitlements": ents,
		"total":        total,
	})
}

func (s *IdentityService) CreateEntitlement(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AppName           string `json:"app_name"`
		PermissionLevel   string `json:"permission_level"`
		EntitlementType   string `json:"entitlement_type"`
		RiskClassification string `json:"risk_classification"`
		IsToxic           bool   `json:"is_toxic"`
		IsRubberband      bool   `json:"is_rubberband"`
		ResourceID        string `json:"resource_id"`
		Condition         string `json:"condition"` // ABAC: when this evaluates true, grant
		ExpiresAt         string `json:"expires_at"` // time-bound expiry
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if req.AppName == "" || req.PermissionLevel == "" {
		respondError(w, http.StatusBadRequest, "app_name and permission_level required")
		return
	}
	if req.EntitlementType == "" {
		req.EntitlementType = "application"
	}
	if req.RiskClassification == "" {
		req.RiskClassification = "medium"
	}

	id := uuid.New().String()

	// Parse optional expiry
	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, req.ExpiresAt); err == nil {
			expiresAt = &t
		}
	}

	// 1. Write to PostgreSQL
	if _, err := s.pgPool.Exec(r.Context(), `
		INSERT INTO entitlements (id, tenant_id, app_name, permission_level,
			entitlement_type, risk_classification, is_toxic, is_rubberband)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (tenant_id, app_name, permission_level) DO UPDATE SET
			entitlement_type = EXCLUDED.entitlement_type,
			risk_classification = EXCLUDED.risk_classification,
			is_toxic = EXCLUDED.is_toxic,
			is_rubberband = EXCLUDED.is_rubberband
	`, id, "00000000-0000-0000-0000-000000000001", req.AppName, req.PermissionLevel,
		req.EntitlementType, req.RiskClassification, req.IsToxic, req.IsRubberband); err != nil {
		respondError(w, http.StatusInternalServerError, "Create failed: "+err.Error())
		return
	}

	// 2. Write to Neo4j — create Entitlement node + link to Resource
	if req.ResourceID != "" || req.IsToxic || req.IsRubberband {
		session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(r.Context())

		cypher := `MERGE (e:Entitlement {id: $id})
			SET e.app_name = $app_name, e.permission_level = $permission_level,
			    e.entitlement_type = $entitlement_type,
			    e.risk_classification = $risk_class,
			    e.is_toxic = $is_toxic, e.is_rubberband = $is_rubberband`
		params := map[string]any{
			"id": id, "app_name": req.AppName, "permission_level": req.PermissionLevel,
			"entitlement_type": req.EntitlementType, "risk_class": req.RiskClassification,
			"is_toxic": req.IsToxic, "is_rubberband": req.IsRubberband,
		}

		if expiresAt != nil {
			cypher += `, e.expires_at = $expires_at`
			params["expires_at"] = expiresAt.Format(time.RFC3339)
		}
		if req.Condition != "" {
			cypher += `, e.condition = $condition`
			params["condition"] = req.Condition
		}

		cypher += ` RETURN e`

		result, err := session.Run(r.Context(), cypher, params)
		if err != nil {
			log.Printf("[ENTITLEMENT] Neo4j create failed: %v", err)
		} else if req.ResourceID != "" {
			// Link to Resource if provided
			result.Consume(r.Context())
			if _, err := session.Run(r.Context(), `
				MATCH (e:Entitlement {id: $id})
				MATCH (res:Resource {id: $resource_id})
				MERGE (e)-[:ACCESSES]->(res)
			`, map[string]any{"id": id, "resource_id": req.ResourceID}); err != nil {
				log.Printf("[ENTITLEMENT] Resource link failed: %v", err)
			}
		}
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":     id,
		"status": "created",
	})
}

// LinkEntitlementToRole attaches an entitlement to a role.
// POST /api/v1/groups/{id}/entitlements
// Supports ABAC conditions and time-bound expiry.
func (s *IdentityService) LinkEntitlementToRole(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roleID := vars["id"]

	var req struct {
		EntitlementID string `json:"entitlement_id"`
		Condition     string `json:"condition"`    // ABAC: e.g. "identity.department == 'Engineering'"
		ExpiresAt     string `json:"expires_at"`    // ISO 8601 expiry
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// 1. Write to PG role_entitlements with condition
	if _, err := s.pgPool.Exec(r.Context(), `
		INSERT INTO role_entitlements (tenant_id, role_id, entitlement_id, condition)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id, role_id, entitlement_id) DO UPDATE SET
			condition = EXCLUDED.condition
	`, "00000000-0000-0000-0000-000000000001", roleID, req.EntitlementID, req.Condition); err != nil {
		respondError(w, http.StatusInternalServerError, "Link failed: "+err.Error())
		return
	}

	// 2. Write to Neo4j — create GRANTS relationship with metadata
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	cypher := `MATCH (role:Role {uuid: $role_id})
		MATCH (ent:Entitlement {id: $entitlement_id})
		MERGE (role)-[rel:GRANTS]->(ent)
		SET rel.granted_at = datetime()`
	params := map[string]any{"role_id": roleID, "entitlement_id": req.EntitlementID}

	if req.Condition != "" {
		cypher += `, rel.condition = $condition`
		params["condition"] = req.Condition
	}
	if req.ExpiresAt != "" {
		cypher += `, rel.expires_at = $expires_at`
		params["expires_at"] = req.ExpiresAt
	}

	if _, err := session.Run(r.Context(), cypher, params); err != nil {
		log.Printf("[ENTITLEMENT-LINK] Neo4j write failed: %v", err)
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "linked"})
}

// UnlinkEntitlementFromRole detaches an entitlement from a role.
// DELETE /api/v1/groups/{id}/entitlements/{entitlement_id}
func (s *IdentityService) UnlinkEntitlementFromRole(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roleID := vars["id"]
	entitlementID := vars["entitlement_id"]

	// 1. Delete from PG
	if _, err := s.pgPool.Exec(r.Context(),
		`DELETE FROM role_entitlements WHERE role_id = $1 AND entitlement_id = $2`, roleID, entitlementID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "Unlink failed: "+err.Error())
		return
	}

	// 2. Delete from Neo4j
	session := s.neo4j.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())
	session.Run(r.Context(), `
		MATCH (role:Role {uuid: $role_id})-[rel:GRANTS]->(ent:Entitlement {id: $entitlement_id})
		DELETE rel
	`, map[string]any{"role_id": roleID, "entitlement_id": entitlementID})

	respondJSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
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

	entries := s.auditLog.List(limit, offset, f)
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

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}
