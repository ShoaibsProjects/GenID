package sources

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/observeid/genid/internal/eventbus"
)

// Handler serves the per-source ingestion edge:
//
//	POST /api/v1/events/ingest/{source}
//	GET  /api/v1/events/sources          (discovery)
//
// Each request is HMAC-verified (when a secret is configured), normalized to
// a canonical GenID event, and published onto the NATS JetStream fabric.
type Handler struct {
	registry *Registry
	bus      *eventbus.NatsBus
}

// NewHandler builds the edge handler. bus may be nil; publishes are then
// dropped (logged) so the API stays honest in degraded mode.
func NewHandler(registry *Registry, bus *eventbus.NatsBus) *Handler {
	return &Handler{registry: registry, bus: bus}
}

// RegisterRoutes mounts the edge endpoints on the API router.
func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/events/sources", h.handleList).Methods("GET")
	r.HandleFunc("/events/ingest/{source}", h.handleIngest).Methods("POST")
}

func (h *Handler) handleList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sources": h.registry.Names()})
}

func (h *Handler) handleIngest(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["source"]
	cfg := h.registry.Get(name)
	if cfg == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "unknown event source — see GET /api/v1/events/sources",
		})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB cap
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unreadable body"})
		return
	}
	defer r.Body.Close()

	secret := ""
	if cfg.SecretEnv != "" {
		secret = os.Getenv(cfg.SecretEnv)
	}
	if !cfg.VerifySignature(secret, r.Header.Get("X-GenID-Signature"), body) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
		return
	}

	norm, err := cfg.Normalize(payload)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}

	raw, _ := json.Marshal(payload)
	evt := eventbus.Event{
		ID:          uuid.New().String(),
		EventType:   norm.EventType,
		AggregateID: norm.IdentityID,
		TenantID:    "00000000-0000-0000-0000-000000000001",
		Payload:     raw,
		Timestamp:   time.Now().UTC(),
	}
	// Stamp source/severity into the payload so the risk processor can
	// attribute and weight the signal.
	enriched := map[string]any{
		"identity_id": norm.IdentityID,
		"source":      norm.Source,
		"severity":    norm.Severity,
		"raw":         json.RawMessage(raw),
	}
	evt.Payload, _ = json.Marshal(enriched)

	if err := h.bus.Publish(r.Context(), evt); err != nil {
		log.Printf("[SOURCES] publish failed for %s: %v", name, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "event bus unavailable"})
		return
	}

	log.Printf("[SOURCES] %s → %s for identity=%s (severity=%s)",
		name, norm.EventType, norm.IdentityID, norm.Severity)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":     "accepted",
		"eventId":    evt.ID,
		"eventType":  norm.EventType,
		"identityId": norm.IdentityID,
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
