package workflow

import "time"

// ─── Approval Routing Engine ─────────────────────────────────
// Pure, testable decision logic: given a request type, resource
// type, and risk band, produce the ordered approval chain.
//
// Modes:
//   - "sequential" — one step active at a time; next step opens
//     only after the previous one is decided.
//   - "parallel"   — all steps in the group open at once; every
//     one must approve for the request to proceed.

type ApprovalStep struct {
	Level        int    `json:"level"`
	ApproverRole string `json:"approver_role"` // e.g. resource_owner, security_admin, ciso
	Mode         string `json:"mode"`
	DueInHours   int    `json:"due_in_hours"`
}

type RoutingRules struct {
	// Overrides: resource type → how many bands to escalate.
	Escalation map[string]int
	// Overrides: risk band → chain. Highest priority.
	BandOverride map[string][]ApprovalStep
}

var DefaultRoutingRules = RoutingRules{
	Escalation: map[string]int{
		"database":    1,
		"infra":       1,
		"secrets":     2,
		"production":  1,
		"payment":     1,
		"pii":         1,
		"role":        1, // self-service role requests
	},
	BandOverride: map[string][]ApprovalStep{},
}

// EffectiveBand returns the risk band after resource-type
// escalation is applied. Bands: minimal, low, elevated, high, critical.
func EffectiveBand(rules RoutingRules, riskBand, resourceType string) string {
	bandRank := map[string]int{
		"minimal":  0,
		"low":      1,
		"elevated": 2,
		"high":     3,
		"critical": 4,
	}
	rank, ok := bandRank[riskBand]
	if !ok {
		rank = 1 // unknown → treat as low
	}
	rank += rules.Escalation[resourceType]
	if rank > 4 {
		rank = 4
	}
	if rank < 0 {
		rank = 0
	}
	for b, r := range bandRank {
		if r == rank {
			return b
		}
	}
	return "low"
}

// EvaluateRouting returns the approval chain for a request.
// A nil/empty result means the request is auto-approved (no gate).
func EvaluateRouting(rules RoutingRules, requestType, riskBand, resourceType string) []ApprovalStep {
	if rules.BandOverride == nil {
		rules.BandOverride = map[string][]ApprovalStep{}
	}
	band := EffectiveBand(rules, riskBand, resourceType)
	if over, ok := rules.BandOverride[band]; ok {
		return over
	}

	owner := ApprovalStep{Level: 1, ApproverRole: "resource_owner", Mode: "sequential", DueInHours: 24}
	sec := ApprovalStep{Level: 2, ApproverRole: "security_admin", Mode: "sequential", DueInHours: 8}
	ciso := ApprovalStep{Level: 3, ApproverRole: "ciso", Mode: "sequential", DueInHours: 4}

	switch band {
	case "minimal":
		// Auto-approve: no gate for minimal risk.
		return nil
	case "low":
		return []ApprovalStep{owner}
	case "elevated":
		return []ApprovalStep{
			owner,
			{Level: 2, ApproverRole: "security_admin", Mode: "sequential", DueInHours: 8},
		}
	case "high":
		return []ApprovalStep{
			owner,
			sec,
		}
	case "critical":
		return []ApprovalStep{
			owner,
			sec,
			ciso,
		}
	default:
		return []ApprovalStep{owner}
	}
}

// Parallelize converts a sequential chain into a parallel gate of
// the same roles (used when the request explicitly asks for it).
func Parallelize(steps []ApprovalStep) []ApprovalStep {
	out := make([]ApprovalStep, len(steps))
	for i, s := range steps {
		out[i] = s
		out[i].Mode = "parallel"
	}
	return out
}

// DueAt computes the decision deadline for a step.
func DueAt(now time.Time, step ApprovalStep) time.Time {
	h := step.DueInHours
	if h <= 0 {
		h = 24
	}
	return now.Add(time.Duration(h) * time.Hour)
}