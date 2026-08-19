package handlers

import (
	"encoding/json"
	"github.com/observeid/genid/internal/connector"
	"net/http"
)

func (h *Handler) ExecuteLCM(w http.ResponseWriter, r *http.Request) {
	var req connector.LCMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid LCM request")
		return
	}

	results := h.ProvisioningEngine().ExecuteLCM(r.Context(), req)

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

func (h *Handler) GetLCMHistory(w http.ResponseWriter, r *http.Request) {
	history := h.ProvisioningEngine().GetHistory()
	respondJSON(w, http.StatusOK, map[string]any{
		"history": history,
		"total":   len(history),
	})
}

// ─── Identity CRUD (PostgreSQL + Neo4j) Handlers ─────────
