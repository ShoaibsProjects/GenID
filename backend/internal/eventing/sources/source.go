// Package sources implements the GenID ingestion edge: per-source adapters
// that normalize external provider events (Entra ID, Okta, Jira, Azure
// Service Bus, ...) into canonical GenID identity events published onto the
// internal NATS JetStream fabric.
//
// Design rule (ADR-001): external systems never touch the core. A source
// adapter is a thin mapping layer — it extracts an event type, an identity
// id, and a severity from a provider payload and translates provider event
// names into GenID canonical names (e.g. "SignInFailure" → "auth.failed_login").
package sources

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

// FieldMapping describes where to find values inside a provider payload.
// Paths are dot-separated keys into the JSON object, e.g. "user.id",
// "data.identity.upn". An EventType path may also be a "literal:<value>"
// string for sources that send a fixed event shape per endpoint.
type FieldMapping struct {
	EventType  string `json:"event_type"`
	IdentityID string `json:"identity_id"`
	Severity   string `json:"severity,omitempty"`
}

// SourceConfig is the full configuration for one external event source.
type SourceConfig struct {
	// Name is the URL slug: POST /api/v1/events/ingest/{name}.
	Name string `json:"name"`
	// DisplayName is recorded as the event "source" for risk attribution.
	DisplayName string `json:"display_name"`
	// SecretEnv names the environment variable holding the HMAC-SHA256
	// shared secret. Empty means signature verification is disabled
	// (acceptable only for local demos).
	SecretEnv string `json:"secret_env,omitempty"`
	// Mapping locates fields inside the provider payload.
	Mapping FieldMapping `json:"mapping"`
	// EventTypeMap translates provider event names to canonical GenID
	// event types (the keys of the risk weight table in the processor).
	EventTypeMap map[string]string `json:"event_type_map"`
	// DefaultEventType is used when the payload's type is unmapped.
	DefaultEventType string `json:"default_event_type,omitempty"`
}

// NormalizedEvent is the canonical internal shape handed to the bus.
type NormalizedEvent struct {
	EventType  string
	IdentityID string
	Source     string
	Severity   string
	Raw        map[string]any
}

// Normalize maps a raw provider payload into a NormalizedEvent.
// Returns an error when the identity cannot be resolved — an event with no
// identity is noise, not signal, and must be rejected at the edge.
func (c *SourceConfig) Normalize(payload map[string]any) (NormalizedEvent, error) {
	evt := NormalizedEvent{
		Source:   c.DisplayName,
		Severity: "medium",
		Raw:      payload,
	}

	rawType, _ := extractString(payload, c.Mapping.EventType)
	if mapped, ok := c.EventTypeMap[rawType]; ok {
		evt.EventType = mapped
	} else if c.DefaultEventType != "" {
		evt.EventType = c.DefaultEventType
	} else if rawType != "" {
		evt.EventType = rawType
	}
	if evt.EventType == "" {
		return evt, fmt.Errorf("source %q: could not determine event type (path %q, no default)", c.Name, c.Mapping.EventType)
	}

	id, _ := extractString(payload, c.Mapping.IdentityID)
	if id == "" {
		return evt, fmt.Errorf("source %q: identity id not found at path %q", c.Name, c.Mapping.IdentityID)
	}
	evt.IdentityID = id

	if c.Mapping.Severity != "" {
		if sev, ok := extractString(payload, c.Mapping.Severity); ok && sev != "" {
			evt.Severity = strings.ToLower(sev)
		}
	}
	return evt, nil
}

// VerifySignature checks an HMAC-SHA256 hex digest of body against the
// signature header value ("sha256=<hex>"). Constant-time comparison.
// Returns true when no secret is configured (verification disabled).
func (c *SourceConfig) VerifySignature(secret, signatureHeader string, body []byte) bool {
	if secret == "" {
		return true
	}
	sig := strings.TrimPrefix(signatureHeader, "sha256=")
	want, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return subtle.ConstantTimeCompare(mac.Sum(nil), want) == 1
}

// extractString walks a dot-separated path through nested JSON objects.
// A "literal:<value>" path returns the value without touching the payload.
func extractString(payload map[string]any, path string) (string, bool) {
	if strings.HasPrefix(path, "literal:") {
		return strings.TrimPrefix(path, "literal:"), true
	}
	if path == "" {
		return "", false
	}
	var cur any = payload
	for _, key := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		cur, ok = m[key]
		if !ok {
			return "", false
		}
	}
	switch v := cur.(type) {
	case string:
		return v, true
	case float64:
		return fmt.Sprintf("%v", v), true
	case bool:
		return fmt.Sprintf("%v", v), true
	default:
		return "", false
	}
}
