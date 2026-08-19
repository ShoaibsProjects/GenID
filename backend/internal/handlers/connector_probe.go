package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/observeid/genid/internal/audit"
	"github.com/observeid/genid/internal/connector"
	"net/http"
)

func (h *Handler) TestExistingConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	cfg, err := h.ConnectorManager().GetConfig(id)
	if err != nil {
		respondError(w, http.StatusNotFound, fmt.Sprintf("Connector not found: %s", err.Error()))
		return
	}

	if err := h.ConnectorManager().TestConnection(r.Context(), cfg); err != nil {
		h.AuditStore().Append(audit.Entry{
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

	h.AuditStore().Append(audit.Entry{
		Level: audit.LevelInfo, Service: "connector", Method: r.Method, Path: r.URL.Path,
		Message: fmt.Sprintf("TestConnection: %s (%s) — SUCCESS", cfg.Type, cfg.Name),
		Tags:    []string{"connector", "test", "success"},
	})
	respondJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Connection successful",
	})
}

func (h *Handler) TestConnectorConnection(w http.ResponseWriter, r *http.Request) {
	var cfg connector.ConnectorConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		h.AuditStore().Append(audit.Entry{
			Level: audit.LevelError, Service: "connector", Method: r.Method, Path: r.URL.Path,
			Message: "TestConnection: invalid config",
			Detail:  err.Error(), Tags: []string{"connector", "error"},
		})
		respondError(w, http.StatusBadRequest, "Invalid connector config")
		return
	}

	if err := h.ConnectorManager().TestConnection(r.Context(), cfg); err != nil {
		h.AuditStore().Append(audit.Entry{
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

	h.AuditStore().Append(audit.Entry{
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
