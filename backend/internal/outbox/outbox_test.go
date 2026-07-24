package outbox

import (
	"encoding/json"
	"testing"
	"time"
)

func TestProcessorConfig_Defaults(t *testing.T) {
	config := DefaultConfig()

	if config.PollInterval != 500*time.Millisecond {
		t.Errorf("expected PollInterval 500ms, got %v", config.PollInterval)
	}
	if config.BatchSize != 100 {
		t.Errorf("expected BatchSize 100, got %d", config.BatchSize)
	}
	if config.MaxRetries != 5 {
		t.Errorf("expected MaxRetries 5, got %d", config.MaxRetries)
	}
	if config.Neo4jTimeout != 30*time.Second {
		t.Errorf("expected Neo4jTimeout 30s, got %v", config.Neo4jTimeout)
	}
}

func TestNewOutbox(t *testing.T) {
	o := NewOutbox(nil)
	if o == nil {
		t.Fatal("expected non-nil Outbox")
	}
	if o.pgPool != nil {
		t.Error("expected nil pgPool when passed nil")
	}
}

func TestEvent_MarshalUnmarshal(t *testing.T) {
	event := Event{
		ID:            "test-id-123",
		EventType:     "identity.created",
		AggregateType: "identity",
		AggregateID:   "user-abc",
		Payload:       json.RawMessage(`{"email":"test@example.com","status":"active"}`),
		Metadata:      json.RawMessage(`{"method":"scim"}`),
		CreatedAt:     time.Now(),
		RetryCount:    0,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if decoded.ID != event.ID {
		t.Errorf("expected ID %s, got %s", event.ID, decoded.ID)
	}
	if decoded.EventType != event.EventType {
		t.Errorf("expected EventType %s, got %s", event.EventType, decoded.EventType)
	}
	if string(decoded.Payload) != string(event.Payload) {
		t.Errorf("expected Payload %s, got %s", event.Payload, decoded.Payload)
	}
}

func TestGetStr(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		key      string
		expected string
	}{
		{
			name:     "existing string",
			input:    map[string]any{"email": "test@example.com"},
			key:      "email",
			expected: "test@example.com",
		},
		{
			name:     "missing key",
			input:    map[string]any{"email": "test@example.com"},
			key:      "phone",
			expected: "",
		},
		{
			name:     "non-string value",
			input:    map[string]any{"count": 42},
			key:      "count",
			expected: "",
		},
		{
			name:     "nil map",
			input:    nil,
			key:      "anything",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStr(tt.input, tt.key)
			if result != tt.expected {
				t.Errorf("getStr(%v, %q) = %q, want %q", tt.input, tt.key, result, tt.expected)
			}
		})
	}
}

func TestPublish_NilTx(t *testing.T) {
	o := NewOutbox(nil)
	err := o.Publish(nil, nil, "identity.created", "identity", "user-1",
		map[string]any{"email": "test@example.com"}, nil)
	if err == nil {
		t.Error("expected error for nil transaction")
	}
	if err.Error() != "outbox: transaction is nil" {
		t.Errorf("expected 'outbox: transaction is nil', got %q", err.Error())
	}
}

func TestPublish_InvalidPayload(t *testing.T) {
	o := NewOutbox(nil)
	// This will fail because we can't actually execute without a real tx
	// but we can at least verify it doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()
	_ = o.Publish(nil, nil, "identity.created", "identity", "user-1", make(chan int), nil)
}
