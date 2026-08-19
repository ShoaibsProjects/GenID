package workflow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEvaluateRouting_MinimalAutoApproves(t *testing.T) {
	steps := EvaluateRouting(DefaultRoutingRules, "access.request", "minimal", "report")
	assert.Nil(t, steps, "minimal risk should need no approval")
}

func TestEvaluateRouting_LowSingleOwner(t *testing.T) {
	steps := EvaluateRouting(DefaultRoutingRules, "access.request", "low", "report")
	requireSteps(t, steps, []string{"resource_owner"})
	assert.Equal(t, "sequential", steps[0].Mode)
}

func TestEvaluateRouting_ElevatedTwoLevels(t *testing.T) {
	steps := EvaluateRouting(DefaultRoutingRules, "access.request", "elevated", "report")
	requireSteps(t, steps, []string{"resource_owner", "security_admin"})
	assert.Equal(t, 8, steps[1].DueInHours, "security_admin should have shorter deadline")
}

func TestEvaluateRouting_HighTwoLevels(t *testing.T) {
	steps := EvaluateRouting(DefaultRoutingRules, "access.request", "high", "report")
	requireSteps(t, steps, []string{"resource_owner", "security_admin"})
}

func TestEvaluateRouting_CriticalThreeLevels(t *testing.T) {
	steps := EvaluateRouting(DefaultRoutingRules, "access.request", "critical", "report")
	requireSteps(t, steps, []string{"resource_owner", "security_admin", "ciso"})
}

func TestEvaluateRouting_ResourceEscalation(t *testing.T) {
	// Low risk + database → elevated → two approvers.
	steps := EvaluateRouting(DefaultRoutingRules, "access.request", "low", "database")
	requireSteps(t, steps, []string{"resource_owner", "security_admin"})

	// Elevated risk + secrets (2 bands) → critical → three approvers.
	steps = EvaluateRouting(DefaultRoutingRules, "access.request", "elevated", "secrets")
	requireSteps(t, steps, []string{"resource_owner", "security_admin", "ciso"})
}

func TestEvaluateRouting_BandOverrideWins(t *testing.T) {
	rules := RoutingRules{
		BandOverride: map[string][]ApprovalStep{
			"critical": {{Level: 1, ApproverRole: "ciso", Mode: "sequential", DueInHours: 2}},
		},
	}
	steps := EvaluateRouting(rules, "access.request", "critical", "report")
	requireSteps(t, steps, []string{"ciso"})
}

func TestEffectiveBand_UnknownDefaultsLow(t *testing.T) {
	assert.Equal(t, "low", EffectiveBand(DefaultRoutingRules, "unknown", "report"))
}

func TestEffectiveBand_CapsAtCritical(t *testing.T) {
	assert.Equal(t, "critical", EffectiveBand(DefaultRoutingRules, "critical", "secrets"))
}

func TestParallelize(t *testing.T) {
	steps := EvaluateRouting(DefaultRoutingRules, "access.request", "high", "report")
	par := Parallelize(steps)
	for _, s := range par {
		assert.Equal(t, "parallel", s.Mode)
	}
	assert.Len(t, par, len(steps))
}

func TestDueAt(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	due := DueAt(now, ApprovalStep{DueInHours: 8})
	assert.Equal(t, now.Add(8*time.Hour), due)
}

func requireSteps(t *testing.T, steps []ApprovalStep, roles []string) {
	t.Helper()
	if !assert.Len(t, steps, len(roles)) {
		return
	}
	for i, r := range roles {
		assert.Equal(t, r, steps[i].ApproverRole, "level %d role", i+1)
	}
}