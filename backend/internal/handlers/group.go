package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/observeid/genid/internal/middleware"
	"github.com/observeid/genid/internal/risk"
	"log"
	"net/http"
	"time"
)

func (h *Handler) ListGroups(w http.ResponseWriter, r *http.Request) {
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
	if err := h.DB(r.Context()).QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM roles %s", where), args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	// Query roles from PostgreSQL
	dataSQL := fmt.Sprintf(`SELECT id, tenant_id, name, description, role_type,
		is_auto_assigned, approval_required, max_duration_hours,
		is_active, attributes, created_at, updated_at
		FROM roles %s ORDER BY name ASC LIMIT $%d OFFSET $%d`, where, idx, idx+1)
	args = append(args, limit, offset)

	rows, err := h.DB(r.Context()).Query(r.Context(), dataSQL, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	type roleItem struct {
		ID               string `json:"id"`
		TenantID         string `json:"tenant_id"`
		Name             string `json:"name"`
		Description      string `json:"description"`
		RoleType         string `json:"role_type"`
		IsAutoAssigned   bool   `json:"is_auto_assigned"`
		ApprovalRequired bool   `json:"approval_required"`
		MaxDurationHours *int   `json:"max_duration_hours"`
		IsActive         bool   `json:"is_active"`
		Attributes       string `json:"attributes"`
		CreatedAt        string `json:"created_at"`
		UpdatedAt        string `json:"updated_at"`
		MemberCount      int    `json:"member_count"`
		EntitlementCount int    `json:"entitlement_count"`
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
		h.DB(ctx).QueryRow(ctx,
			`SELECT COUNT(*) FROM identity_roles WHERE role_id = $1 AND is_active = true`, r.ID,
		).Scan(&r.MemberCount)

		// Enrich: entitlement count from role_entitlements
		h.DB(ctx).QueryRow(ctx,
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

func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
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
		if hdrTenant := r.Header.Get("X-Tenant-ID"); hdrTenant != "" {
			req.TenantID = hdrTenant
		} else if txTenant := middleware.TenantIDFromContext(r.Context()); txTenant != "" {
			req.TenantID = txTenant
		} else {
			req.TenantID = "00000000-0000-0000-0000-000000000001"
		}
	}
	if req.RoleType == "" {
		req.RoleType = "custom"
	}

	id := uuid.New().String()

	// 1. Write to PostgreSQL
	if _, err := h.DB(r.Context()).Exec(r.Context(), `
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
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
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

func (h *Handler) GetGroup(w http.ResponseWriter, r *http.Request) {
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
	err := h.DB(r.Context()).QueryRow(r.Context(), `
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
	h.DB(r.Context()).QueryRow(r.Context(),
		`SELECT COUNT(*) FROM identity_roles WHERE role_id = $1 AND is_active = true`, id,
	).Scan(&role.MemberCount)

	// Entitlement count
	h.DB(r.Context()).QueryRow(r.Context(),
		`SELECT COUNT(*) FROM role_entitlements WHERE role_id = $1`, id,
	).Scan(&role.EntitlementCount)

	// List members
	memberRows, _ := h.DB(r.Context()).Query(r.Context(), `
		SELECT ir.identity_id, i.display_name, i.email, ir.assigned_at, ir.source
		FROM identity_roles ir
		JOIN identities i ON i.id = ir.identity_id
		WHERE ir.role_id = $1 AND ir.is_active = true
		ORDER BY ir.assigned_at DESC
		LIMIT 100
	`, id)

	type member struct {
		IdentityID  string `json:"identity_id"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		AssignedAt  string `json:"assigned_at"`
		Source      string `json:"source"`
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
	entRows, _ := h.DB(r.Context()).Query(r.Context(), `
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

func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// 1. Soft-delete in PostgreSQL
	if _, err := h.DB(r.Context()).Exec(r.Context(),
		`UPDATE roles SET is_active = false, updated_at = NOW() WHERE id = $1`, id,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to deactivate role")
		return
	}
	// Also deactivate role assignments
	h.DB(r.Context()).Exec(r.Context(),
		`UPDATE identity_roles SET is_active = false WHERE role_id = $1`, id)

	// 2. Remove from Neo4j
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())
	session.Run(r.Context(), `MATCH (r:Role {uuid: $uuid}) DETACH DELETE r`,
		map[string]any{"uuid": id})

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── Role Assignment ────────────────────────────────────────

func (h *Handler) AssignRole(w http.ResponseWriter, r *http.Request) {
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
	if _, err := h.DB(r.Context()).Exec(r.Context(), `
		INSERT INTO identity_roles (tenant_id, identity_id, role_id, source, assigned_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (tenant_id, identity_id, role_id, source) DO UPDATE SET
			is_active = true, assigned_at = NOW()
	`, "00000000-0000-0000-0000-000000000001", req.IdentityID, req.RoleID, req.Source); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to assign role: "+err.Error())
		return
	}

	// 2. Write to Neo4j (graph for path traversal)
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())
	_, err := session.Run(r.Context(), `
		MATCH (i {uuid: $identity_id}) WHERE i:Identity OR i:NonHumanIdentity
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

	// 3. Recalculate risk so inherited entitlements are reflected immediately.
	score, factors, _ := risk.CalculateIdentityRisk(r.Context(), h.Neo4j(), h.RawPool(), "00000000-0000-0000-0000-000000000001", req.IdentityID)

	respondJSON(w, http.StatusOK, map[string]any{
		"status":       "assigned",
		"risk_score":   score,
		"risk_factors": factors,
	})
}

func (h *Handler) RemoveRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IdentityID string `json:"identity_id"`
		RoleID     string `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// 1. Soft-delete in PostgreSQL
	if _, err := h.DB(r.Context()).Exec(r.Context(),
		`UPDATE identity_roles SET is_active = false WHERE identity_id = $1 AND role_id = $2`,
		req.IdentityID, req.RoleID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to remove role")
		return
	}

	// 2. Remove from Neo4j
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())
	if _, err := session.Run(r.Context(), `
		MATCH (i {uuid: $identity_id}) WHERE i:Identity OR i:NonHumanIdentity
		MATCH (i)-[rel:HAS_ROLE]->(r:Role {uuid: $role_id})
		DELETE rel
		SET i.updated_at = datetime()
	`, map[string]any{
		"identity_id": req.IdentityID, "role_id": req.RoleID,
	}); err != nil {
		log.Printf("[UNASSIGN] Neo4j write failed: %v", err)
	}

	// 3. Recalculate risk so removed entitlements stop contributing.
	score, factors, _ := risk.CalculateIdentityRisk(r.Context(), h.Neo4j(), h.RawPool(), "00000000-0000-0000-0000-000000000001", req.IdentityID)

	respondJSON(w, http.StatusOK, map[string]any{
		"status":       "removed",
		"risk_score":   score,
		"risk_factors": factors,
	})
}

// ─── Entitlement CRUD + Role-Entitlement Linking ─────────────
