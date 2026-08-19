package cedar

import (
	"context"
	"fmt"

	cedar "github.com/cedar-policy/cedar-go"
)

// PolicyCheckParams is the conditional-access evaluation request.
// Context carries the enriched request context (role, network_zone,
// time_of_day, device_trust, risk_score, risk_band, ...).
type PolicyCheckParams struct {
	TenantID    string                 `json:"tenant_id"`
	PrincipalID string                 `json:"principal_id"`
	ResourceID  string                 `json:"resource_id"`
	Action      string                 `json:"action"`
	Context     map[string]interface{} `json:"context"` // enriched context
}

// PolicyResult is the conditional-access evaluation outcome. Decision is
// one of "Allow", "Deny", "StepUp". Duration ("2h", "30m", "permanent")
// and Advice ("auto_approve_2h", "step_up_approval", ...) come from the
// matched policy's @advice annotation.
type PolicyResult struct {
	Decision string `json:"decision"`           // "Allow", "Deny", "StepUp"
	Duration string `json:"duration,omitempty"` // "2h", "30m", "permanent"
	Reason   string `json:"reason"`
	PolicyID string `json:"policy_id"`
	Advice   string `json:"advice"` // "auto_approve_2h", "step_up", etc.
}

// EvaluateConditionalAccess loads the tenant's policies and evaluates the
// request with the enriched context attached.
func (e *CedarEngine) EvaluateConditionalAccess(ctx context.Context, params PolicyCheckParams) (PolicyResult, error) {
	tenantID := params.TenantID
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	if err := e.LoadPolicies(ctx, tenantID); err != nil {
		return PolicyResult{Decision: "Deny"}, fmt.Errorf("cedar: load policies: %w", err)
	}

	e.mu.RLock()
	ps := e.policies[tenantID]
	e.mu.RUnlock()
	if ps == nil {
		return PolicyResult{Decision: "StepUp", Reason: "no policies loaded", Advice: "step_up_default"}, nil
	}

	return e.evaluateSet(ps, params), nil
}

// evaluateSet evaluates against an in-memory PolicySet. Pure: no I/O, so
// tests can drive the full 6-case matrix without a database.
//
// Decision mapping (conditional-access semantics):
//   - any forbid matched → "Deny", unless its @advice is step_up_approval
//     → "StepUp" (public network / unmanaged device escalates instead of
//     blocking). Multiple forbids: deny_due_to_risk wins over step-up.
//   - permit matched (no forbid) → "Allow" + duration from advice.
//   - nothing matched → "StepUp" (no explicit permit → escalate to
//     approval; never auto-grant on a miss).
func (e *CedarEngine) evaluateSet(ps *cedar.PolicySet, params PolicyCheckParams) PolicyResult {
	if ps == nil {
		return PolicyResult{Decision: "StepUp", Reason: "no policies", Advice: "step_up_default"}
	}

	action := params.Action
	if action == "" {
		action = "grant"
	}
	principalType, resourceType := "User", "Resource"
	if params.PrincipalID == "" {
		principalType = "User"
		params.PrincipalID = "anonymous"
	}
	if params.ResourceID == "" {
		params.ResourceID = "resource"
	}

	req := AuthRequest{
		PrincipalID:   params.PrincipalID,
		PrincipalType: principalType,
		Action:        action,
		ResourceID:    params.ResourceID,
		ResourceType:  resourceType,
		TenantID:      params.TenantID,
		Context:       params.Context,
	}
	cedarReq, entities := e.buildCedarRequest(req)

	decision, diag := cedar.Authorize(ps, entities, cedarReq)
	_ = decision

	// Collect matched policies with their @advice annotations.
	type matched struct {
		id     string
		effect string
		advice string
	}
	var matchedPolicies []matched
	for _, reason := range diag.Reasons {
		p := ps.Get(reason.PolicyID)
		if p == nil {
			continue
		}
		advice := ""
		if a, ok := p.Annotations()["advice"]; ok {
			advice = string(a)
		}
		m := matched{id: string(reason.PolicyID), advice: advice}
		if p.Effect() == cedar.Forbid {
			m.effect = "forbid"
		} else {
			m.effect = "permit"
		}
		matchedPolicies = append(matchedPolicies, m)
	}

	var forbids, permits []matched
	for _, m := range matchedPolicies {
		if m.effect == "forbid" {
			forbids = append(forbids, m)
		} else {
			permits = append(permits, m)
		}
	}

	// Any forbid → Deny/StepUp family. deny_due_to_risk dominates.
	if len(forbids) > 0 {
		advice := forbids[0].advice
		for _, f := range forbids {
			if f.advice == "deny_due_to_risk" {
				return PolicyResult{
					Decision: "Deny", Advice: "deny_due_to_risk",
					Reason: "critical risk band", PolicyID: f.id,
				}
			}
			if f.advice == "step_up_approval" {
				advice = "step_up_approval"
			}
		}
		if advice == "step_up_approval" {
			return PolicyResult{
				Decision: "StepUp", Advice: "step_up_approval",
				Reason: "public network or unmanaged device", PolicyID: forbids[0].id,
			}
		}
		return PolicyResult{
			Decision: "Deny", Advice: advice,
			Reason: fmt.Sprintf("forbidden by %d policies", len(forbids)), PolicyID: forbids[0].id,
		}
	}

	// Permit matched → Allow with the longest duration (2h auto-approve
	// beats 30m JIT when both apply).
	if len(permits) > 0 {
		duration, advice := "permanent", ""
		bestID := permits[0].id
		for _, p := range permits {
			if p.advice == "auto_approve_2h" && duration != "2h" {
				duration, advice, bestID = "2h", "auto_approve_2h", p.id
			} else if p.advice == "approve_30m_jit" && duration != "2h" && duration != "30m" {
				duration, advice, bestID = "30m", "approve_30m_jit", p.id
			}
		}
		return PolicyResult{
			Decision: "Allow", Duration: duration, Advice: advice,
			Reason: fmt.Sprintf("permitted by %d policies", len(permits)), PolicyID: bestID,
		}
	}

	return PolicyResult{Decision: "StepUp", Reason: "no policy matched", Advice: "step_up_default"}
}
