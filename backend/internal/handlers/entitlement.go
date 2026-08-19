package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"log"
	"net/http"
	"time"
)

func (h *Handler) ListEntitlements(w http.ResponseWriter, r *http.Request) {
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
	if err := h.DB(r.Context()).QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM entitlements %s", where), args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	rows, err := h.DB(r.Context()).Query(r.Context(), fmt.Sprintf(`
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
		ID           string `json:"id"`
		AppName      string `json:"app_name"`
		Permission   string `json:"permission_level"`
		Type         string `json:"entitlement_type"`
		RiskClass    string `json:"risk_classification"`
		IsToxic      bool   `json:"is_toxic"`
		IsRubberband bool   `json:"is_rubberband"`
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

func (h *Handler) CreateEntitlement(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AppName            string `json:"app_name"`
		PermissionLevel    string `json:"permission_level"`
		EntitlementType    string `json:"entitlement_type"`
		RiskClassification string `json:"risk_classification"`
		IsToxic            bool   `json:"is_toxic"`
		IsRubberband       bool   `json:"is_rubberband"`
		ResourceID         string `json:"resource_id"`
		Condition          string `json:"condition"`  // ABAC: when this evaluates true, grant
		ExpiresAt          string `json:"expires_at"` // time-bound expiry
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
	if _, err := h.DB(r.Context()).Exec(r.Context(), `
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
		session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
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

func (h *Handler) LinkEntitlementToRole(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roleID := vars["id"]

	var req struct {
		EntitlementID string `json:"entitlement_id"`
		Condition     string `json:"condition"`  // ABAC: e.g. "identity.department == 'Engineering'"
		ExpiresAt     string `json:"expires_at"` // ISO 8601 expiry
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// 1. Write to PG role_entitlements with condition
	if _, err := h.DB(r.Context()).Exec(r.Context(), `
		INSERT INTO role_entitlements (tenant_id, role_id, entitlement_id, condition)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id, role_id, entitlement_id) DO UPDATE SET
			condition = EXCLUDED.condition
	`, "00000000-0000-0000-0000-000000000001", roleID, req.EntitlementID, req.Condition); err != nil {
		respondError(w, http.StatusInternalServerError, "Link failed: "+err.Error())
		return
	}

	// 2. Write to Neo4j — create GRANTS relationship with metadata
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
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

func (h *Handler) UnlinkEntitlementFromRole(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roleID := vars["id"]
	entitlementID := vars["entitlement_id"]

	// 1. Delete from PG
	if _, err := h.DB(r.Context()).Exec(r.Context(),
		`DELETE FROM role_entitlements WHERE role_id = $1 AND entitlement_id = $2`, roleID, entitlementID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "Unlink failed: "+err.Error())
		return
	}

	// 2. Delete from Neo4j
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())
	session.Run(r.Context(), `
		MATCH (role:Role {uuid: $role_id})-[rel:GRANTS]->(ent:Entitlement {id: $entitlement_id})
		DELETE rel
	`, map[string]any{"role_id": roleID, "entitlement_id": entitlementID})

	respondJSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
}

// ─── Vault / Credential Management Handlers ─────────────
