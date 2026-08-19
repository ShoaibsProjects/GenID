package handlers

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/observeid/genid/internal/risk"
	"github.com/observeid/genid/internal/stores"
	"net/http"
	"time"
)

func (h *Handler) ListIdentities(w http.ResponseWriter, r *http.Request) {
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
	if err := h.DB(r.Context()).QueryRow(r.Context(), countSQL, args...).Scan(&total); err != nil {
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

	rows, err := h.DB(r.Context()).Query(r.Context(), dataSQL, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	type identityItem struct {
		ID             string   `json:"id"`
		TenantID       string   `json:"tenant_id"`
		Type           string   `json:"type"`
		Status         string   `json:"status"`
		Email          string   `json:"email"`
		DisplayName    string   `json:"display_name"`
		Department     *string  `json:"department"`
		EmployeeID     *string  `json:"employee_id"`
		ManagerID      *string  `json:"manager_id"`
		Source         string   `json:"source"`
		RiskScore      float64  `json:"risk_score"`
		RiskFactors    []string `json:"risk_factors"`
		AssuranceLevel string   `json:"assurance_level"`
		Attributes     string   `json:"attributes"`
		CreatedAt      string   `json:"created_at"`
		UpdatedAt      string   `json:"updated_at"`
		LastAccessedAt *string  `json:"last_accessed_at"`
		LastReviewedAt *string  `json:"last_reviewed_at"`
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

func (h *Handler) GetIdentity(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
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
			"id":           stores.GetRecordVal(rec, "uuid"),
			"display_name": stores.GetRecordVal(rec, "display_name"),
			"email":        stores.GetRecordVal(rec, "email"),
			"status":       stores.GetRecordVal(rec, "status"),
			"type":         stores.GetRecordVal(rec, "type"),
			"department":   stores.GetRecordVal(rec, "department"),
			"title":        stores.GetRecordVal(rec, "title"),
			"employee_id":  stores.GetRecordVal(rec, "employee_id"),
			"source":       stores.GetRecordVal(rec, "source"),
			"risk_score":   stores.GetRecordVal(rec, "risk_score"),
			"created_at":   stores.GetRecordVal(rec, "created_at"),
			"updated_at":   stores.GetRecordVal(rec, "updated_at"),
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

func (h *Handler) RecalculateIdentityRisk(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	score, factors, err := risk.CalculateIdentityRisk(r.Context(), h.Neo4j(), h.RawPool(), tenantID, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "risk calculation failed: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"identity_id":  id,
		"risk_score":   score,
		"risk_factors": factors,
	})
}

func (h *Handler) GetIdentityEntitlements(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
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

func (h *Handler) GetBlastRadius(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())

	result, err := session.Run(r.Context(), `
		MATCH (i {uuid: $id}) WHERE i:Identity OR i:NonHumanIdentity
		OPTIONAL MATCH pathRole = (i)-[:HAS_ROLE]->(:Role)-[:GRANTS]->(e:Entitlement)-[:ACCESSES]->(r:Resource)
		OPTIONAL MATCH pathDirectEnt = (i)-[:HAS_ENTITLEMENT]->(e2:Entitlement)-[:ACCESSES]->(r2:Resource)
		OPTIONAL MATCH pathDirect = (i)-[:HAS_DIRECT_ACCESS]->(r3:Resource)
		OPTIONAL MATCH pathTemp = (i)-[:HAS_TEMPORARY_ACCESS]->(r4:Resource)
		WITH i,
		     COLLECT(DISTINCT CASE WHEN pathRole IS NOT NULL THEN {
				path: pathRole,
				resource: r,
				entitlement: e,
				depth: length(pathRole),
				source: 'role'
			} END) AS rolePaths,
		     COLLECT(DISTINCT CASE WHEN pathDirectEnt IS NOT NULL THEN {
				path: pathDirectEnt,
				resource: r2,
				entitlement: e2,
				depth: length(pathDirectEnt),
				source: 'direct_entitlement'
			} END) AS directEntPaths,
		     COLLECT(DISTINCT CASE WHEN pathDirect IS NOT NULL THEN {
				path: pathDirect,
				resource: r3,
				entitlement: null,
				depth: length(pathDirect),
				source: 'direct_access'
			} END) AS directPaths,
		     COLLECT(DISTINCT CASE WHEN pathTemp IS NOT NULL THEN {
				path: pathTemp,
				resource: r4,
				entitlement: null,
				depth: length(pathTemp),
				source: 'temporary_access'
			} END) AS tempPaths
		WITH [p IN rolePaths + directEntPaths + directPaths + tempPaths WHERE p IS NOT NULL] AS paths
		UNWIND paths AS p
		RETURN p.resource.name AS resource_name,
			   p.resource.criticality AS criticality,
			   COALESCE(p.entitlement.permission_level, 'direct') AS permission_level,
			   p.depth AS path_depth,
			   [n IN NODES(p.path) | labels(n)[0]] AS path_types,
			   p.source AS source
		ORDER BY p.resource.criticality DESC, p.depth ASC
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
		source, _ := record.Get("source")

		resources = append(resources, map[string]any{
			"resource":         name,
			"criticality":      crit,
			"permission_level": perm,
			"path_depth":       depth,
			"path_types":       types,
			"source":           source,
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"identity_id":  id,
		"blast_radius": resources,
		"graph":        h.Store().BlastRadiusGraph(r.Context(), id),
	})
}

// blastRadiusGraph returns the graph-native view of an identity's blast
// radius: every node (Identity/NonHumanIdentity, Role, Entitlement, Resource)
// and every relationship (HAS_ROLE, DIRECTLY_OWNS, DELEGATED_FROM, ACCESSES)
// on the paths, deduplicated. Powers the force-graph visualization.
