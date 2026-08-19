package eventing

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/observeid/genid/internal/eventbus"
)

type IngestHandler struct {
	natsBus *eventbus.NatsBus
}

func NewIngestHandler(bus *eventbus.NatsBus) *IngestHandler {
	return &IngestHandler{natsBus: bus}
}

func (h *IngestHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/events/ingest", h.handleIngest).Methods("POST")
}

func (h *IngestHandler) handleIngest(w http.ResponseWriter, r *http.Request) {
	var event IngestedEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, `{"error": "invalid JSON"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := event.Validate(); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	evt := eventbus.Event{
		ID:          uuid.New().String(),
		EventType:   event.EventType,
		AggregateID: event.IdentityID,
		TenantID:    "00000000-0000-0000-0000-000000000001",
		Payload:     mustMarshal(event),
		Timestamp:   time.Now().UTC(),
	}

	if err := h.natsBus.Publish(r.Context(), evt); err != nil {
		http.Error(w, `{"error": "failed to publish event"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "accepted",
		"eventId": evt.ID,
	})
}

func mustMarshal(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
