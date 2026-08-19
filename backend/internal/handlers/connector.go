package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/observeid/genid/internal/audit"
	"github.com/observeid/genid/internal/connector"
	"github.com/observeid/genid/internal/services"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (h *Handler) ListConnectors(w http.ResponseWriter, r *http.Request) {
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
	if err := h.DB(r.Context()).QueryRow(r.Context(), countSQL, args...).Scan(&total); err != nil {
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

	rows, err := h.DB(r.Context()).Query(r.Context(), dataSQL, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	// Fetch health + sync stats for each connector from the manager
	type connectorSummary struct {
		ID         string                  `json:"id"`
		TenantID   string                  `json:"tenant_id"`
		Name       string                  `json:"name"`
		Type       string                  `json:"type"`
		Status     string                  `json:"status"`
		LastSyncAt *string                 `json:"last_sync_at"`
		LastError  *string                 `json:"last_error"`
		CreatedAt  string                  `json:"created_at"`
		UpdatedAt  string                  `json:"updated_at"`
		Health     *connector.HealthReport `json:"health,omitempty"`
		SyncStats  *struct {
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
		if health, err := h.ConnectorManager().GetConnectorHealth(id); err == nil && health != nil {
			c.Health = health
		}

		// Query sync stats from connector child tables
		var userCount, groupCount, entCount, resCount int
		if err := h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_identities WHERE connector_id = $1`, id).Scan(&userCount); err == nil && userCount > 0 {
			c.SyncStats = &struct {
				Users        int `json:"users"`
				Groups       int `json:"groups"`
				Entitlements int `json:"entitlements"`
				Resources    int `json:"resources"`
			}{Users: userCount}
		}
		if c.SyncStats != nil {
			h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_groups WHERE connector_id = $1`, id).Scan(&groupCount)
			h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_entitlements WHERE connector_id = $1`, id).Scan(&entCount)
			h.DB(r.Context()).QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_resources WHERE connector_id = $1`, id).Scan(&resCount)
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

func (h *Handler) CreateConnector(w http.ResponseWriter, r *http.Request) {
	var cfg connector.ConnectorConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid connector config")
		return
	}

	// Always generate a proper UUID for the connector ID so that
	// RegisterSecure can use it as a valid vault reference.
	cfg.ID = uuid.New().String()

	// Use RegisterSecure for production - stores secrets in vault
	id, err := h.ConnectorManager().RegisterSecure(r.Context(), cfg)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"connector_id": id,
		"status":       "registered",
		"vault_secured": true,
	})
}

func (h *Handler) GetConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	cfg, err := h.ConnectorManager().GetConfig(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	lastSync := h.ConnectorManager().GetLastSyncResult(id)

	respondJSON(w, http.StatusOK, map[string]any{
		"connector": services.SanitizeConfig(cfg),
		"last_sync": lastSync,
	})
}

func (h *Handler) DeleteConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.ConnectorManager().Unregister(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) ConnectConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.ConnectorManager().Connect(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "connected"})
}

func (h *Handler) DisconnectConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.ConnectorManager().Disconnect(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

func (h *Handler) SyncConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	result, err := h.ConnectorManager().SyncUsers(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "sync_completed_with_errors",
			"result": result,
		})
		return
	}

	// Persist synced users to PostgreSQL
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

// SyncConnectorHR runs an HR-source connector sync and propagates the
// resulting users into the canonical GenID identity model: creates, updates,
// and terminations — each via the atomic PG+outbox path so Neo4j sync
// (and the event processor's downstream handlers) is consistent.
//
// Behaviour:
//   - For each connector user NOT already in `identities` (matched on
//     employee_id, falling back to email, scoped to source='hris'), INSERT
//     a new identity + emit identity.created.
//   - For each connector user whose dept/title/manager has changed since
//     last sync, UPDATE the identity + emit identity.updated.
//   - For each GenID identity whose source='hris' within the same tenant,
//     and whose employee_id (or email) is NOT present in the current CSV
//     roster, mark status='terminated' and emit identity.deleted — kicking
//     off offboarding via the event processor.
//
// Idempotent: re-running with no CSV changes produces zero new outbox events.

func (h *Handler) SyncConnectorHR(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	cfg, err := h.ConnectorManager().GetConfig(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "connector not found")
		return
	}
	tenantID := cfg.TenantID
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	result, err := h.ConnectorManager().SyncUsers(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "sync_completed_with_errors",
			"result": result,
		})
		return
	}
	if result == nil {
		result = &connector.SyncResult{ConnectorID: id}
	}

	createdInCache, updatedInCache, _ := h.Store().PersistSyncedUsers(r.Context(), tenantID, id, result.Users)
	result.UsersCreated = createdInCache
	result.UsersUpdated = updatedInCache
	result.UsersTotal = len(result.Users)

	jml, jmlErr := h.ApplyJMLFromConnectorUsers(r.Context(), tenantID, id, result.Users)
	if jmlErr != nil {
		h.AuditStore().Append(audit.Entry{
			Level: audit.LevelError, Service: "connector-hr", Path: r.URL.Path,
			Message: fmt.Sprintf("HR JML propagation error: %s", jmlErr.Error()),
			Tags:    []string{"connector", "hr", "jml", "error"},
		})
		log.Printf("[HR-SYNC] JML error: %v", jmlErr)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status": "sync_hr_completed",
		"result": result,
		"jml":    jml,
	})
}

// JMLResult summarises the propagation of a connector sync into the
// canonical identity model.
type JMLResult struct {
	IdentitiesCreated    int      `json:"identities_created"`
	IdentitiesUpdated    int      `json:"identities_updated"`
	IdentitiesTerminated int      `json:"identities_terminated"`
	CreatedIDs           []string `json:"created_ids,omitempty"`
	UpdatedIDs           []string `json:"updated_ids,omitempty"`
	TerminatedIDs        []string `json:"terminated_ids,omitempty"`
}

// applyJMLFromConnectorUsers diffs a freshly-synced connector roster against
// the canonical identities table and emits the right outbox events for each
// kind of change. Safe to run repeatedly: unchanged rows produce no events.
//
// Identity resolution order: employee_id first (within tenant), falling back
// to email.
//
// Termination: any existing identity with source='hris' within the same
// tenant, whose employee_id (or email) is NOT in the new roster and whose
// status is still active/inactive/on_leave, is marked status='terminated'
// and emits identity.deleted. Pending/suspended/terminated rows are skipped.
