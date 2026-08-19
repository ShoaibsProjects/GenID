package activities

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"

	"github.com/observeid/genid/internal/cedar"
	"github.com/observeid/genid/internal/enrichment"
)

// ─── Conditional Access Activities (Phase 1 MVP) ──────────────
//
// EnrichContext + CheckConditionalAccessPolicy implement conditional access flow:
//   signals → enrichment → Cedar policy → Allow/StepUp/Deny routing.
// Both are methods on ActivityService so the existing
// w.RegisterActivity(act) registration picks them up automatically.

// EnrichContextParams carries the raw request signals into the activity.
// DeviceTrust is the X-Device-Trust header captured by the API handler;
// EvaluateAt (RFC3339) pins the evaluation clock for deterministic
// demos/tests (nil = now).
type EnrichContextParams struct {
	TenantID    string                    `json:"tenant_id"`
	Signals     enrichment.ContextSignals `json:"signals"`
	DeviceTrust string                    `json:"device_trust,omitempty"`
	RiskScore   int                       `json:"risk_score"`
	EvaluateAt  *time.Time                `json:"evaluate_at,omitempty"`
}

// EnrichContext runs the enrichment orchestrator and returns the full
// EnrichedContext (zone, trust, time-of-day, risk band) for policy eval.
func (s *ActivityService) EnrichContext(ctx context.Context, params EnrichContextParams) (enrichment.EnrichedContext, error) {
	svc := enrichment.NewEnrichmentService(s.pgPool, s.redis)
	ec, err := svc.EnrichAt(ctx, params.TenantID, params.Signals, params.DeviceTrust, params.RiskScore, params.EvaluateAt)
	if err != nil {
		return enrichment.EnrichedContext{}, err
	}
	activity.RecordHeartbeat(ctx, "context_enriched", ec.NetworkZone)
	return ec, nil
}

// ConditionalPolicyParams is the enriched-context policy request.
type ConditionalPolicyParams struct {
	TenantID     string                 `json:"tenant_id"`
	PrincipalID  string                 `json:"principal_id"`
	ResourceID   string                 `json:"resource_id"`
	ResourceType string                 `json:"resource_type"`
	Action       string                 `json:"action"`
	Context      map[string]interface{} `json:"context"`
}

// CheckConditionalAccessPolicy evaluates the enriched context against the
// tenant's Cedar policies and returns the routing decision (Allow with
// duration, StepUp, or Deny).
func (s *ActivityService) CheckConditionalAccessPolicy(ctx context.Context, params ConditionalPolicyParams) (cedar.PolicyResult, error) {
	res, err := s.cedarEng.EvaluateConditionalAccess(ctx, cedar.PolicyCheckParams{
		TenantID:    params.TenantID,
		PrincipalID: params.PrincipalID,
		ResourceID:  params.ResourceID,
		Action:      params.Action,
		Context:     params.Context,
	})
	if err != nil {
		return cedar.PolicyResult{}, err
	}
	activity.RecordHeartbeat(ctx, "conditional_policy_checked", res.Decision)
	return res, nil
}
