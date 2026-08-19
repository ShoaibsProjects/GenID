package cedar

import (
	"os"
	"strings"
	"testing"

	cedar "github.com/cedar-policy/cedar-go"
)

// loadDefaultPolicies parses internal/cedar/templates/default_policy.cedar
// into a PolicySet, exactly as migration 008 seeds the DB.
func loadDefaultPolicies(t *testing.T) *cedar.PolicySet {
	t.Helper()
	raw, err := os.ReadFile("templates/default_policy.cedar")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}

	ps := cedar.NewPolicySet()
	// Strip comment lines first (the header comment mentions "@advice("),
	// then split on annotation markers so each block stays intact.
	var sb strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	blocks := strings.Split(sb.String(), "@advice(")
	if len(blocks) < 2 {
		t.Fatalf("no policies found in template")
	}
	for i, block := range blocks[1:] {
		text := "@advice(" + block
		var p cedar.Policy
		if err := p.UnmarshalCedar([]byte(text)); err != nil {
			t.Fatalf("policy %d failed to parse: %v\n%s", i+1, err, text)
		}
		ps.Add(cedar.PolicyID("pol_"+string(rune('0'+i))), &p)
	}
	return ps
}

// TestConditionalAccessMatrix is the mandatory 6-case matrix (spec 4.5)
// evaluated end-to-end through evaluateSet with the flagship policies.
func TestConditionalAccessMatrix(t *testing.T) {
	ps := loadDefaultPolicies(t)
	e := NewCedarEngine(nil)

	cases := []struct {
		name       string
		role       string
		zone       string
		trust      string
		tod        string
		riskScore  int
		riskBand   string
		wantDec    string
		wantDur    string
		wantAdvice string
	}{
		{name: "Office-Managed-Business-Low", role: "it-admin", zone: "corporate", trust: "managed", tod: "business_hours", riskScore: 200, riskBand: "low",
			wantDec: "Allow", wantDur: "2h", wantAdvice: "auto_approve_2h"},
		{name: "Office-Unmanaged-Business-Low", role: "it-admin", zone: "corporate", trust: "unmanaged", tod: "business_hours", riskScore: 200, riskBand: "low",
			wantDec: "StepUp", wantAdvice: "step_up_approval"},
		{name: "Remote-Managed-Business-Low", role: "it-admin", zone: "public", trust: "managed", tod: "business_hours", riskScore: 200, riskBand: "low",
			wantDec: "StepUp", wantAdvice: "step_up_approval"},
		{name: "Office-Managed-AfterHours-Low", role: "oncall", zone: "corporate", trust: "managed", tod: "after_hours", riskScore: 200, riskBand: "low",
			wantDec: "Allow", wantDur: "30m", wantAdvice: "approve_30m_jit"},
		{name: "Office-Managed-Business-Critical", role: "it-admin", zone: "corporate", trust: "managed", tod: "business_hours", riskScore: 850, riskBand: "critical",
			wantDec: "Deny", wantAdvice: "deny_due_to_risk"},
		{name: "VPN-Managed-Business-Low", role: "it-admin", zone: "vpn", trust: "managed", tod: "business_hours", riskScore: 200, riskBand: "low",
			wantDec: "StepUp", wantAdvice: "step_up_default"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := PolicyCheckParams{
				TenantID:    "00000000-0000-0000-0000-000000000001",
				PrincipalID: "demo-user-001",
				ResourceID:  "demo-resource-001",
				Action:      "grant",
				Context: map[string]interface{}{
					"role":         tc.role,
					"network_zone": tc.zone,
					"time_of_day":  tc.tod,
					"device_trust": tc.trust,
					"risk_score":   tc.riskScore,
					"risk_band":    tc.riskBand,
				},
			}
			res := e.evaluateSet(ps, params)
			if res.Decision != tc.wantDec {
				t.Errorf("decision = %q, want %q (advice %q, reason %q)", res.Decision, tc.wantDec, res.Advice, res.Reason)
			}
			if tc.wantDur != "" && res.Duration != tc.wantDur {
				t.Errorf("duration = %q, want %q", res.Duration, tc.wantDur)
			}
			if res.Advice != tc.wantAdvice {
				t.Errorf("advice = %q, want %q", res.Advice, tc.wantAdvice)
			}
		})
	}
}

// TestConditionalAccessPriority pins tie-break rules: critical-risk deny
// dominates a step-up forbid when both match.
func TestConditionalAccessPriority(t *testing.T) {
	ps := loadDefaultPolicies(t)
	e := NewCedarEngine(nil)

	// public network + critical risk → deny_due_to_risk wins.
	res := e.evaluateSet(ps, PolicyCheckParams{
		TenantID: "00000000-0000-0000-0000-000000000001", PrincipalID: "u1", ResourceID: "r1",
		Action: "grant",
		Context: map[string]interface{}{
			"role": "it-admin", "network_zone": "public", "time_of_day": "business_hours",
			"device_trust": "managed", "risk_score": 900, "risk_band": "critical",
		},
	})
	if res.Decision != "Deny" || res.Advice != "deny_due_to_risk" {
		t.Errorf("public+critical = %s/%s, want Deny/deny_due_to_risk", res.Decision, res.Advice)
	}

	// After-hours oncall but critical risk → deny_due_to_risk wins over
	// the approve_30m_jit permit (forbid always beats permit in Cedar).
	res = e.evaluateSet(ps, PolicyCheckParams{
		TenantID: "00000000-0000-0000-0000-000000000001", PrincipalID: "u1", ResourceID: "r1",
		Action: "grant",
		Context: map[string]interface{}{
			"role": "oncall", "network_zone": "corporate", "time_of_day": "after_hours",
			"device_trust": "managed", "risk_score": 900, "risk_band": "critical",
		},
	})
	if res.Decision != "Deny" || res.Advice != "deny_due_to_risk" {
		t.Errorf("critical oncall = %s/%s, want Deny/deny_due_to_risk", res.Decision, res.Advice)
	}

	// Unmanaged device during business hours as it-admin → step-up wins
	// over the auto-approve permit (forbid beats permit).
	res = e.evaluateSet(ps, PolicyCheckParams{
		TenantID: "00000000-0000-0000-0000-000000000001", PrincipalID: "u1", ResourceID: "r1",
		Action: "grant",
		Context: map[string]interface{}{
			"role": "it-admin", "network_zone": "corporate", "time_of_day": "business_hours",
			"device_trust": "unmanaged", "risk_score": 200, "risk_band": "low",
		},
	})
	if res.Decision != "StepUp" || res.Advice != "step_up_approval" {
		t.Errorf("unmanaged it-admin = %s/%s, want StepUp/step_up_approval", res.Decision, res.Advice)
	}
}
