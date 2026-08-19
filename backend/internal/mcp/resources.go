package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/observeid/genid/internal/risk"
)

// RegisterResources wires the three resources + three URI templates onto
// the server. Templates are URI patterns; concrete reads are handled by
// ResourceTemplateHandlerFuncs (same ReadResourceRequest shape).
func RegisterResources(s *server.MCPServer, d *Deps) {
	s.AddResourceTemplate(mcp.NewResourceTemplate(
		"identity://{id}",
		"Identity",
		mcp.WithTemplateDescription("Identity record, entitlements graph and risk profile for {id} (UUID or email)."),
		mcp.WithTemplateMIMEType("application/json"),
	), func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		id := strings.TrimPrefix(req.Params.URI, "identity://")
		if id == "" {
			return nil, fmt.Errorf("identity://<id> requires an id")
		}
		tenantID := session(ctx, d).TenantID
		row, err := queryIdentity(ctx, d, id)
		if err != nil {
			return nil, fmt.Errorf("identity %q not found", id)
		}
		ents, _ := identityEntitlements(ctx, d, row.ID)
		score, factors, _ := risk.CalculateIdentityRisk(ctx, d.Neo4j, d.Pool, tenantID, row.ID)
		payload, _ := json.MarshalIndent(map[string]any{
			"identity":     row,
			"entitlements": ents,
			"risk":         map[string]any{"risk_score": score, "risk_band": band(score), "factors": factors},
		}, "", "  ")
		return []mcp.ResourceContents{mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(payload),
		}}, nil
	})

	s.AddResourceTemplate(mcp.NewResourceTemplate(
		"policy://{tenant_id}",
		"Policies",
		mcp.WithTemplateDescription("Active Cedar policy set for tenant {tenant_id}."),
		mcp.WithTemplateMIMEType("text/plain"),
	), func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		tenantID := strings.TrimPrefix(req.Params.URI, "policy://")
		if tenantID == "" {
			return nil, fmt.Errorf("policy://<tenant_id> requires a tenant id")
		}
		rows, err := d.Pool.Query(ctx, `
			SELECT policy_id, effect::text, cedar_text, COALESCE(advice, '')
			FROM cedar_policies WHERE tenant_id = $1 AND is_active = true
			ORDER BY priority, policy_id
		`, tenantID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var sb strings.Builder
		for rows.Next() {
			var policyID, effect, text, advice string
			if err := rows.Scan(&policyID, &effect, &text, &advice); err != nil {
				return nil, err
			}
			fmt.Fprintf(&sb, "// %s (%s) advice: %s\n%s\n\n", policyID, effect, advice, text)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return []mcp.ResourceContents{mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "text/plain",
			Text:     sb.String(),
		}}, nil
	})

	s.AddResourceTemplate(mcp.NewResourceTemplate(
		"risk://{tenant_id}",
		"Risk Posture",
		mcp.WithTemplateDescription("Aggregate risk posture for tenant {tenant_id}: human identities, NHIs and band distribution."),
		mcp.WithTemplateMIMEType("application/json"),
	), func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		tenantID := strings.TrimPrefix(req.Params.URI, "risk://")
		if tenantID == "" {
			return nil, fmt.Errorf("risk://<tenant_id> requires a tenant id")
		}
		var total int
		var avg float64
		if err := d.Pool.QueryRow(ctx, `
			SELECT COUNT(*), COALESCE(AVG(risk_score), 0) FROM identities WHERE tenant_id = $1
		`, tenantID).Scan(&total, &avg); err != nil {
			return nil, err
		}
		var low, med, high, crit int
		rows, err := d.Pool.Query(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE risk_score < 30),
				COUNT(*) FILTER (WHERE risk_score >= 30 AND risk_score < 60),
				COUNT(*) FILTER (WHERE risk_score >= 60 AND risk_score < 80),
				COUNT(*) FILTER (WHERE risk_score >= 80)
			FROM identities WHERE tenant_id = $1
		`, tenantID)
		if err == nil {
			if rows.Next() {
				_ = rows.Scan(&low, &med, &high, &crit)
			}
			rows.Close()
		}
		var nhiCount, nhiHigh int
		if err := d.Pool.QueryRow(ctx, `
			SELECT COUNT(*), COUNT(*) FILTER (WHERE risk_score >= 60)
			FROM non_human_identities WHERE tenant_id = $1 AND status = 'active'
		`, tenantID).Scan(&nhiCount, &nhiHigh); err != nil {
			nhiCount, nhiHigh = -1, -1
		}
		payload, _ := json.MarshalIndent(map[string]any{
			"tenant_id":  tenantID,
			"updated_at": time.Now().UTC().Format(time.RFC3339),
			"identities": map[string]any{"total": total, "avg_risk": avg, "bands": map[string]any{"low": low, "medium": med, "high": high, "critical": crit}},
			"non_human":  map[string]any{"active": nhiCount, "high_risk": nhiHigh},
		}, "", "  ")
		return []mcp.ResourceContents{mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(payload),
		}}, nil
	})
}

func band(score float64) string {
	switch {
	case score < 30:
		return "low"
	case score < 60:
		return "medium"
	case score < 80:
		return "high"
	default:
		return "critical"
	}
}
