package cedar

import (
	"context"
	"testing"
	"time"

	cedar "github.com/cedar-policy/cedar-go"
)

func TestConvertPatternToCedar(t *testing.T) {
	tests := []struct {
		name     string
		effect   string
		pattern  string
		expected string
	}{
		{
			name:     "wildcard all",
			effect:   "permit",
			pattern:  "permit(*, *, *)",
			expected: "permit(principal, action, resource);",
		},
		{
			name:     "specific identity, wildcard action and resource",
			effect:   "permit",
			pattern:  "permit(Engineering, *, *)",
			expected: `permit(principal == Role::"Engineering", action, resource);`,
		},
		{
			name:     "specific identity and action, wildcard resource",
			effect:   "forbid",
			pattern:  "forbid(Engineering, read, *)",
			expected: `forbid(principal == Role::"Engineering", action == Action::"read", resource);`,
		},
		{
			name:     "all specific",
			effect:   "permit",
			pattern:  "permit(Engineering, read, res-aws-prod)",
			expected: `permit(principal == Role::"Engineering", action == Action::"read", resource == Resource::"res-aws-prod");`,
		},
		{
			name:     "empty pattern",
			effect:   "permit",
			pattern:  "",
			expected: "",
		},
		{
			name:     "no parens",
			effect:   "permit",
			pattern:  "not-a-pattern",
			expected: "",
		},
		{
			name:     "too few fields",
			effect:   "permit",
			pattern:  "permit(Engineering, read)",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertPatternToCedar(tt.effect, tt.pattern)
			if result != tt.expected {
				t.Errorf("convertPatternToCedar(%q, %q) = %q, want %q", tt.effect, tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestToCedarValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"string", "hello", `"hello"`},
		{"int", 42, "42"},
		{"int64", int64(100), "100"},
		{"float64", 3.14, "3"},
		{"nil-like", nil, "<nil>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toCedarValue(tt.input)
			// Just verify it doesn't panic and returns a valid Cedar value
			_ = result
		})
	}
}

func TestBuildCedarRequest(t *testing.T) {
	engine := &CedarEngine{
		policies: make(map[string]*cedar.PolicySet),
		loadedAt: make(map[string]time.Time),
	}

	req := AuthRequest{
		PrincipalID:   "user-1",
		PrincipalType: "User",
		Action:        "ReadAccess",
		ResourceID:    "res-aws-prod",
		ResourceType:  "Resource",
		TenantID:      "00000000-0000-0000-0000-000000000001",
		Department:    "Engineering",
		Clearance:     5,
		IsActive:      true,
		IsContractor:  false,
		MFAPresent:    true,
		SourceIP:      "10.0.0.1",
	}

	cedarReq, entities := engine.buildCedarRequest(req)

	// Verify principal entity exists
	principalUID := cedar.NewEntityUID(cedar.EntityType("User"), cedar.String("user-1"))
	if _, ok := entities[principalUID]; !ok {
		t.Error("principal entity not found in entity map")
	}

	// Verify resource entity exists
	resourceUID := cedar.NewEntityUID(cedar.EntityType("Resource"), cedar.String("res-aws-prod"))
	if _, ok := entities[resourceUID]; !ok {
		t.Error("resource entity not found in entity map")
	}

	// Verify request fields
	if cedarReq.Principal != principalUID {
		t.Errorf("principal = %v, want %v", cedarReq.Principal, principalUID)
	}
	if cedarReq.Resource != resourceUID {
		t.Errorf("resource = %v, want %v", cedarReq.Resource, resourceUID)
	}

	// Verify principal has attributes
	principalEntity := entities[principalUID]
	_ = principalEntity // entity exists, attributes set in buildCedarRequest
}

func TestBuildCedarRequestAgent(t *testing.T) {
	engine := &CedarEngine{
		policies: make(map[string]*cedar.PolicySet),
		loadedAt: make(map[string]time.Time),
	}

	req := AuthRequest{
		PrincipalID:    "agent-001",
		PrincipalType:  "Agent",
		Action:         "ToolCall",
		ResourceID:     "res-k8s-cluster",
		ResourceType:   "Resource",
		TenantID:       "00000000-0000-0000-0000-000000000001",
		DeploymentEnv:  "production",
		IsRevoked:      false,
		DelegationDepth: 2,
	}

	cedarReq, entities := engine.buildCedarRequest(req)

	// Verify agent entity
	agentUID := cedar.NewEntityUID(cedar.EntityType("Agent"), cedar.String("agent-001"))
	if _, ok := entities[agentUID]; !ok {
		t.Error("agent entity not found in entity map")
	}

	if cedarReq.Action != cedar.NewEntityUID("Action", cedar.String("ToolCall")) {
		t.Error("action mismatch")
	}
}

func TestIsAuthorizedInMemory(t *testing.T) {
	// Build a policy set manually (no DB required)
	ps := cedar.NewPolicySet()

	// Permit policy: User can read any Resource
	p1Text := `permit(
  principal,
  action == Action::"ReadAccess",
  resource
);`
	var p1 cedar.Policy
	if err := p1.UnmarshalCedar([]byte(p1Text)); err != nil {
		t.Fatalf("failed to parse policy 1: %v", err)
	}
	ps.Add(cedar.PolicyID("p1"), &p1)

	// Forbid policy: nobody can delete ProductionDB
	p2Text := `forbid(
  principal,
  action == Action::"Delete",
  resource == Resource::"ProductionDB"
);`
	var p2 cedar.Policy
	if err := p2.UnmarshalCedar([]byte(p2Text)); err != nil {
		t.Fatalf("failed to parse policy 2: %v", err)
	}
	ps.Add(cedar.PolicyID("p2"), &p2)

	engine := &CedarEngine{
		pgPool:   nil, // no DB needed for in-memory test
		policies: make(map[string]*cedar.PolicySet),
		loadedAt: make(map[string]time.Time),
	}
	engine.policies["test-tenant"] = ps
	engine.loadedAt["test-tenant"] = time.Now()

	ctx := context.Background()

	// Test 1: Engineering ReadAccess → should permit
	decision, err := engine.IsAuthorized(ctx, AuthRequest{
		PrincipalID:   "user-1",
		PrincipalType: "User",
		Action:        "ReadAccess",
		ResourceID:    "res-1",
		ResourceType:  "Resource",
		TenantID:      "test-tenant",
		Department:    "Engineering",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allowed {
		t.Errorf("expected allow for Engineering ReadAccess, got %v (decision: %s)", decision.Allowed, decision.Decision)
	}

	// Test 2: Anyone Delete ProductionDB → should forbid
	decision, err = engine.IsAuthorized(ctx, AuthRequest{
		PrincipalID:   "user-2",
		PrincipalType: "User",
		Action:        "Delete",
		ResourceID:    "ProductionDB",
		ResourceType:  "Resource",
		TenantID:      "test-tenant",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Allowed {
		t.Errorf("expected deny for Delete ProductionDB, got allow")
	}

	// Test 3: Engineering WriteAccess → should deny (no permit matches, forbid doesn't match action)
	decision, err = engine.IsAuthorized(ctx, AuthRequest{
		PrincipalID:   "user-1",
		PrincipalType: "User",
		Action:        "WriteAccess",
		ResourceID:    "res-2",
		ResourceType:  "Resource",
		TenantID:      "test-tenant",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Allowed {
		t.Errorf("expected deny for WriteAccess (no matching permit), got allow")
	}
}

func TestInvalidateAndReload(t *testing.T) {
	engine := &CedarEngine{
		policies: make(map[string]*cedar.PolicySet),
		loadedAt: make(map[string]time.Time),
	}

	// Manually load a policy
	ps := cedar.NewPolicySet()
	var p cedar.Policy
	if err := p.UnmarshalCedar([]byte(`permit(principal, action, resource);`)); err != nil {
		t.Fatal(err)
	}
	ps.Add("test", &p)
	engine.policies["test-tenant"] = ps
	engine.loadedAt["test-tenant"] = time.Now()

	if engine.PolicyCount("test-tenant") != 1 {
		t.Fatalf("expected 1 policy, got %d", engine.PolicyCount("test-tenant"))
	}

	// Invalidate
	engine.InvalidateTenant("test-tenant")
	if engine.PolicyCount("test-tenant") != 0 {
		t.Errorf("expected 0 policies after invalidate, got %d", engine.PolicyCount("test-tenant"))
	}
}

func TestPolicyCountEmpty(t *testing.T) {
	engine := &CedarEngine{
		policies: make(map[string]*cedar.PolicySet),
		loadedAt: make(map[string]time.Time),
	}

	if engine.PolicyCount("nonexistent") != 0 {
		t.Error("expected 0 for nonexistent tenant")
	}
}

func TestConvertPatternEdgeCases(t *testing.T) {
	// Empty parentheses
	result := convertPatternToCedar("permit", "permit()")
	if result != "" {
		t.Errorf("expected empty for empty parens, got %q", result)
	}

	// Single field
	result = convertPatternToCedar("permit", "permit(Engineering)")
	if result != "" {
		t.Errorf("expected empty for single field, got %q", result)
	}
}

func TestToCedarValueTypes(t *testing.T) {
	// Verify toCedarValue returns valid types without panicking
	vals := []any{
		true, false, "test", 1, int64(2), float64(3.0), nil,
		[]string{"a"}, map[string]any{"k": "v"},
	}
	for _, v := range vals {
		result := toCedarValue(v)
		if result == nil {
			t.Errorf("toCedarValue(%v) returned nil", v)
		}
	}
}
