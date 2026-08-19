package sources

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestExtractStringDotPath(t *testing.T) {
	payload := map[string]any{
		"actor": map[string]any{"alternateId": "alice@corp.com"},
		"count": float64(3),
		"ok":    true,
	}
	if v, ok := extractString(payload, "actor.alternateId"); !ok || v != "alice@corp.com" {
		t.Fatalf("dot path: got %q ok=%v", v, ok)
	}
	if v, ok := extractString(payload, "count"); !ok || v != "3" {
		t.Fatalf("numeric: got %q ok=%v", v, ok)
	}
	if v, ok := extractString(payload, "ok"); !ok || v != "true" {
		t.Fatalf("bool: got %q ok=%v", v, ok)
	}
	if _, ok := extractString(payload, "actor.missing"); ok {
		t.Fatal("missing key should return ok=false")
	}
	if _, ok := extractString(payload, "actor.alternateId.deeper"); ok {
		t.Fatal("path through scalar should return ok=false")
	}
	if _, ok := extractString(payload, ""); ok {
		t.Fatal("empty path should return ok=false")
	}
}

func TestExtractStringLiteral(t *testing.T) {
	v, ok := extractString(map[string]any{}, "literal:auth.failed_login")
	if !ok || v != "auth.failed_login" {
		t.Fatalf("literal: got %q ok=%v", v, ok)
	}
}

func TestNormalizeEntraFailedLogin(t *testing.T) {
	reg := DefaultRegistry()
	cfg := reg.Get("entra")
	if cfg == nil {
		t.Fatal("entra source missing from default registry")
	}

	norm, err := cfg.Normalize(map[string]any{
		"eventType":         "SignInFailure",
		"userPrincipalName": "alice@corp.com",
		"riskLevel":         "High",
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if norm.EventType != "auth.failed_login" {
		t.Fatalf("event type: got %q want auth.failed_login", norm.EventType)
	}
	if norm.IdentityID != "alice@corp.com" {
		t.Fatalf("identity: got %q", norm.IdentityID)
	}
	if norm.Severity != "high" {
		t.Fatalf("severity: got %q want high", norm.Severity)
	}
	if norm.Source != "microsoft-entra" {
		t.Fatalf("source: got %q", norm.Source)
	}
}

func TestNormalizeDefaultEventType(t *testing.T) {
	cfg := DefaultRegistry().Get("entra")
	norm, err := cfg.Normalize(map[string]any{
		"eventType":         "SomethingUnmapped",
		"userPrincipalName": "bob@corp.com",
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if norm.EventType != "auth.failed_login" {
		t.Fatalf("default mapping: got %q", norm.EventType)
	}
}

func TestNormalizeRequiresIdentity(t *testing.T) {
	cfg := DefaultRegistry().Get("entra")
	_, err := cfg.Normalize(map[string]any{"eventType": "SignInFailure"})
	if err == nil {
		t.Fatal("expected error when identity id is missing")
	}
}

func TestNormalizeRequiresEventType(t *testing.T) {
	cfg := &SourceConfig{
		Name:    "bare",
		Mapping: FieldMapping{EventType: "nope", IdentityID: "id"},
	}
	_, err := cfg.Normalize(map[string]any{"id": "x"})
	if err == nil {
		t.Fatal("expected error when no event type and no default")
	}
}

func TestVerifySignature(t *testing.T) {
	cfg := &SourceConfig{Name: "x"}
	body := []byte(`{"eventType":"SignInFailure"}`)
	secret := "test-secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	good := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !cfg.VerifySignature(secret, good, body) {
		t.Fatal("valid signature rejected")
	}
	if cfg.VerifySignature(secret, "sha256=deadbeef", body) {
		t.Fatal("invalid signature accepted")
	}
	if cfg.VerifySignature(secret, "", body) {
		t.Fatal("empty signature accepted")
	}
	if cfg.VerifySignature("", "", body) {
		// no secret configured → verification disabled
	} else {
		t.Fatal("empty secret should disable verification")
	}
	if cfg.VerifySignature("different-secret", good, body) {
		t.Fatal("wrong secret accepted")
	}
}

func TestRegistryLoadFile(t *testing.T) {
	reg := DefaultRegistry()
	before := len(reg.Names())

	f := t.TempDir() + "/sources.json"
	content := `[{"name":"custom","mapping":{"event_type":"type","identity_id":"sub"},"event_type_map":{"X":"auth.failed_login"},"default_event_type":"auth.failed_login"}]`
	if err := writeFile(f, content); err != nil {
		t.Fatal(err)
	}
	if err := reg.LoadFile(f); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(reg.Names()) != before+1 {
		t.Fatalf("expected %d sources, got %d", before+1, len(reg.Names()))
	}
	if reg.Get("custom") == nil {
		t.Fatal("custom source not registered")
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
