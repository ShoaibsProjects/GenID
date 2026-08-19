package eventing

import (
	"encoding/json"
	"time"
)

type IngestedEvent struct {
	EventType  string          `json:"event_type" validate:"required"`
	IdentityID string          `json:"identity_id,omitempty"`
	Source     string          `json:"source"`
	Severity   string          `json:"severity"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

func (e *IngestedEvent) Validate() error {
	if e.EventType == "" {
		return ErrMissingEventType
	}
	if e.Source == "" {
		e.Source = "unknown"
	}
	if e.Severity == "" {
		e.Severity = "medium"
	}
	return nil
}

type RiskSignal struct {
	EventType  string
	Source     string
	Severity   string
	IdentityID string
	Timestamp  time.Time
	ScoreDelta float64
	Metadata   json.RawMessage
}
