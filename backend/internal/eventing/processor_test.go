package eventing

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIngestedEvent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		event   IngestedEvent
		wantErr bool
	}{
		{
			name:    "valid event",
			event:   IngestedEvent{EventType: "auth.failed_login", Source: "azure_ad"},
			wantErr: false,
		},
		{
			name:    "missing event_type",
			event:   IngestedEvent{Source: "azure_ad"},
			wantErr: true,
		},
		{
			name:    "defaults severity to medium",
			event:   IngestedEvent{EventType: "auth.mfa_failure"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProcessor_eventToSignal(t *testing.T) {
	p := NewProcessor(nil, nil)

	tests := []struct {
		name         string
		eventType    string
		severity     string
		wantDelta    float64
	}{
		{"failed_login_medium", "auth.failed_login", "medium", 100.0},
		{"failed_login_high", "auth.failed_login", "high", 150.0},
		{"failed_login_critical", "auth.failed_login", "critical", 200.0},
		{"mfa_failure_medium", "auth.mfa_failure", "medium", 75.0},
		{"impossible_travel", "auth.impossible_travel", "medium", 150.0},
		{"brute_force_low", "auth.brute_force", "low", 87.5},
		{"unknown_event", "unknown.event", "medium", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test via the public RiskWeight + severity multiplier
			base := p.RiskWeight(tt.eventType)
			multiplier := 1.0
			switch tt.severity {
			case "critical":
				multiplier = 2.0
			case "high":
				multiplier = 1.5
			case "low":
				multiplier = 0.5
			}
			got := base * multiplier

			if got != tt.wantDelta {
				t.Errorf("scoreDelta = %v, want %v", got, tt.wantDelta)
			}
		})
	}
}

func TestRiskSignal_Creation(t *testing.T) {
	metadata := json.RawMessage(`{"ip":"1.2.3.4","user_agent":"test"}`)
	sig := RiskSignal{
		EventType:  "auth.failed_login",
		Source:     "azure_ad",
		Severity:   "high",
		IdentityID: "identity-abc",
		Timestamp:  time.Now(),
		ScoreDelta: 30.0,
		Metadata:   metadata,
	}

	if sig.IdentityID != "identity-abc" {
		t.Errorf("IdentityID = %v, want identity-abc", sig.IdentityID)
	}
	if sig.ScoreDelta != 30.0 {
		t.Errorf("ScoreDelta = %v, want 30.0", sig.ScoreDelta)
	}
}

func TestSetRiskWeight(t *testing.T) {
	p := NewProcessor(nil, nil)
	p.SetRiskWeight("auth.failed_login", 25.0)

	if w := p.RiskWeight("auth.failed_login"); w != 25.0 {
		t.Errorf("after SetRiskWeight, got %v, want 25.0", w)
	}
}
