package risk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEntitlementRiskScore(t *testing.T) {
	tests := []struct {
		class string
		want  float64
	}{
		{"critical", 100},
		{"high", 70},
		{"medium", 40},
		{"low", 10},
		{"", 20},
		{"UNKNOWN", 20},
	}
	for _, tt := range tests {
		t.Run(tt.class, func(t *testing.T) {
			assert.Equal(t, tt.want, entitlementRiskScore(tt.class))
		})
	}
}

func TestResourceCriticalityScore(t *testing.T) {
	tests := []struct {
		crit string
		want float64
	}{
		{"p0", 100},
		{"critical", 100},
		{"p1", 70},
		{"high", 70},
		{"p2", 40},
		{"p3", 10},
		{"", 20},
	}
	for _, tt := range tests {
		t.Run(tt.crit, func(t *testing.T) {
			assert.Equal(t, tt.want, resourceCriticalityScore(tt.crit))
		})
	}
}

func TestComputeRiskScoreV1(t *testing.T) {
	tests := []struct {
		name               string
		avgEntitlementRisk float64
		resourceCount      int
		maxDepth           int
		standingPriv       int
		jitPriv            int
		wantRisk           float64
		wantFactors        []string
	}{
		{
			name:               "no access paths",
			avgEntitlementRisk: 0,
			wantRisk:           0,
			wantFactors:        []string{"no_access_paths"},
		},
		{
			name:               "admin-like critical access",
			avgEntitlementRisk: 85,
			resourceCount:      4,
			maxDepth:           3,
			standingPriv:       4,
			jitPriv:            0,
			wantRisk:           73.5,
			wantFactors:        []string{"avg_entitlement_risk=85", "reachable_resources=4", "max_path_depth=3", "standing_privileges=4"},
		},
		{
			name:               "engineer low/medium access",
			avgEntitlementRisk: 30,
			resourceCount:      2,
			maxDepth:           3,
			standingPriv:       2,
			jitPriv:            0,
			wantRisk:           38,
			wantFactors:        []string{"avg_entitlement_risk=30", "reachable_resources=2", "max_path_depth=3", "standing_privileges=2"},
		},
		{
			name:               "capped at 100",
			avgEntitlementRisk: 100,
			resourceCount:      20,
			maxDepth:           5,
			standingPriv:       20,
			jitPriv:            0,
			wantRisk:           100,
			wantFactors:        []string{"avg_entitlement_risk=100", "reachable_resources=20", "max_path_depth=5", "standing_privileges=20"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk, factors := ComputeRiskScoreV1(tt.avgEntitlementRisk, tt.resourceCount, tt.maxDepth, tt.standingPriv, tt.jitPriv)
			assert.Equal(t, tt.wantRisk, risk)
			assert.Equal(t, tt.wantFactors, factors)
		})
	}
}
