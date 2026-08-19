package handlers

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"net/http"
)

func (h *Handler) ListSecrets(w http.ResponseWriter, r *http.Request) {
	secrets := h.Vault().List(r.Context())
	respondJSON(w, http.StatusOK, map[string]any{
		"secrets": secrets,
		"total":   len(secrets),
	})
}

func (h *Handler) StoreSecret(w http.ResponseWriter, r *http.Request) {
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

	id, err := h.Vault().Store(r.Context(), req.Name, req.Type, req.Reference, req.Value)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"secret_id": id,
		"status":    "stored",
	})
}

func (h *Handler) RetrieveSecret(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	plaintext, err := h.Vault().Retrieve(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"secret_id": id,
		"value":     plaintext,
	})
}

func (h *Handler) DeleteSecret(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.Vault().Delete(r.Context(), id); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── Audit Log Handlers ─────────────────────────────────
