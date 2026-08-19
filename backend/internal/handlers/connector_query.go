package handlers

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/observeid/genid/internal/audit"
	"github.com/observeid/genid/internal/connector"
	"log"
	"net/http"
	"time"
)

func (h *Handler) GetConnectorIdentities(w http.ResponseWriter, r *http.Request) {
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
	if err := h.DB(r.Context()).QueryRow(r.Context(), countSQL, args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	// Query
	rows, err := h.DB(r.Context()).Query(r.Context(), fmt.Sprintf(`
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
		ID          string   `json:"id"`
		ExternalID  string   `json:"external_id"`
		Username    string   `json:"username"`
		Email       string   `json:"email"`
		DisplayName string   `json:"display_name"`
		FirstName   string   `json:"first_name"`
		LastName    string   `json:"last_name"`
		Department  string   `json:"department"`
		Title       string   `json:"title"`
		EmployeeID  string   `json:"employee_id,omitempty"`
		Enabled     bool     `json:"enabled"`
		Locked      bool     `json:"locked"`
		Groups      []string `json:"groups"`
		Roles       []string `json:"roles"`
		FirstSynced string   `json:"first_synced_at"`
		LastSynced  string   `json:"last_synced_at"`
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

func (h *Handler) GetConnectorUsers(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	users, err := h.ConnectorManager().GetConnectorUsers(r.Context(), id)
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

func (h *Handler) GetConnectorStats(w http.ResponseWriter, r *http.Request) {
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
	_ = h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connectors`).Scan(&stats.TotalConnectors)
	_ = h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connectors WHERE status = $1`, "connected").Scan(&stats.ConnectedCount)
	_ = h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connectors WHERE status = $1`, "disconnected").Scan(&stats.DisconnectedCount)
	_ = h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connectors WHERE status = $1`, "error").Scan(&stats.ErrorCount)
	_ = h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connectors WHERE status = $1`, "syncing").Scan(&stats.SyncingCount)

	// Count synced entities
	_ = h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_identities`).Scan(&stats.TotalIdentities)
	_ = h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_groups`).Scan(&stats.TotalGroups)
	_ = h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_entitlements`).Scan(&stats.TotalEntitlements)
	_ = h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_resources`).Scan(&stats.TotalResources)

	respondJSON(w, http.StatusOK, stats)
}

// ─── Delta Sync ──────────────────────────────────────────────

func (h *Handler) SyncConnectorDelta(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	result, err := h.ConnectorManager().SyncUsersDelta(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "sync_completed_with_errors",
			"result": result,
		})
		return
	}

	if result != nil && len(result.Users) > 0 {
		tenantID, tenantErr := h.ConnectorTenantID(id)
		var created, updated int
		var persistErr error
		if tenantErr != nil {
			persistErr = tenantErr
		} else {
			created, updated, persistErr = h.Store().PersistSyncedUsers(r.Context(), tenantID, id, result.Users)
		}
		if persistErr != nil {
			h.AuditStore().Append(audit.Entry{
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

func (h *Handler) GetConnectorSchema(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	schema, err := h.ConnectorManager().GetConnectorSchema(r.Context(), id)
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

func (h *Handler) GetConnectorHealth(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	health, err := h.ConnectorManager().GetConnectorHealth(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, health)
}

// ─── Connector Groups ─────────────────────────────────────────

func (h *Handler) GetConnectorGroups(w http.ResponseWriter, r *http.Request) {
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
	if err := h.DB(r.Context()).QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM connector_groups %s", where), args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	rows, err := h.DB(r.Context()).Query(r.Context(), fmt.Sprintf(`
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

func (h *Handler) GetConnectorEntitlements(w http.ResponseWriter, r *http.Request) {
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
	if err := h.DB(r.Context()).QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM connector_entitlements %s", where), args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	rows, err := h.DB(r.Context()).Query(r.Context(), fmt.Sprintf(`
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
		"connector_id": id,
		"entitlements": entitlements,
		"total":        total,
	})
}

// ─── Connector Resources ─────────────────────────────────────

func (h *Handler) GetConnectorResources(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	limit, offset := paginationParams(r, 100, 0)

	args := []any{id}
	idx := 2
	where := "WHERE connector_id = $1"

	var total int
	if err := h.DB(r.Context()).QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM connector_resources %s", where), args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	rows, err := h.DB(r.Context()).Query(r.Context(), fmt.Sprintf(`
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

// ─── Connector Permissions (Catalog) ──────────────────────────

func (h *Handler) GetConnectorPermissions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	q := r.URL.Query()
	limit, offset := paginationParams(r, 100, 0)
	search := q.Get("search")
	adminOnly := q.Get("admin_only") == "true"

	args := []any{id}
	idx := 2
	where := "WHERE connector_id = $1"

	if search != "" {
		where += fmt.Sprintf(" AND (name ILIKE $%d OR app_name ILIKE $%d)", idx, idx)
		args = append(args, "%"+search+"%")
		idx++
	}
	if adminOnly {
		where += fmt.Sprintf(" AND is_admin = $%d", idx)
		args = append(args, true)
		idx++
	}

	var total int
	if err := h.DB(r.Context()).QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM connector_permissions %s", where), args...).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "Count failed")
		return
	}

	rows, err := h.DB(r.Context()).Query(r.Context(), fmt.Sprintf(`
		SELECT permission_id, name, permission_type, app_id, app_name, description, is_admin
		FROM connector_permissions
		%s
		ORDER BY app_name NULLS LAST, name NULLS LAST
		LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), append(args, limit, offset)...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Query failed: %s", err.Error()))
		return
	}
	defer rows.Close()

	type PermissionEntry struct {
		PermissionID   string `json:"permission_id"`
		Name           string `json:"name"`
		Type           string `json:"permission_type"`
		AppID          string `json:"app_id"`
		AppName        string `json:"app_name"`
		Description    string `json:"description"`
		IsAdmin        bool   `json:"is_admin"`
	}

	permissions := []PermissionEntry{}
	for rows.Next() {
		var e PermissionEntry
		if err := rows.Scan(&e.PermissionID, &e.Name, &e.Type, &e.AppID, &e.AppName, &e.Description, &e.IsAdmin); err != nil {
			log.Printf("[PERMISSIONS] scan row error: %v", err)
			continue
		}
		permissions = append(permissions, e)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"connector_id": id,
		"permissions":  permissions,
		"total":        total,
	})
}

// ─── Sync Handlers ─────────────────────────────────────────
