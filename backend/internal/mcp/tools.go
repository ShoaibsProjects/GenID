package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	cedar "github.com/observeid/genid/internal/cedar"
	"github.com/observeid/genid/internal/risk"
	"github.com/observeid/genid/internal/services"
	"github.com/observeid/genid/internal/workflow"
	"go.temporal.io/sdk/client"
)

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// identityRow is the subset of the identities table exposed to MCP callers.
type identityRow struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	Type        string    `json:"type"`
	DisplayName string    `json:"display_name,omitempty"`
	Department  string    `json:"department,omitempty"`
	Status      string    `json:"status"`
	Source      string    `json:"source"`
	RiskScore   float64   `json:"risk_score"`
	RiskFactors []string  `json:"risk_factors,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// queryIdentity resolves an identity by UUID or email (tenant-scoped).
func queryIdentity(ctx context.Context, d *Deps, idOrEmail string) (*identityRow, error) {
	var row identityRow
	var createdAt time.Time
	var riskFactors []string
	err := d.Pool.QueryRow(ctx, `
		SELECT id::text, email, type::text, COALESCE(display_name, ''), COALESCE(department, ''),
		       status::text, source::text, risk_score, risk_factors, created_at
		FROM identities
		WHERE tenant_id = $1 AND (id::text = $2 OR email = $2)
		ORDER BY (id::text = $2) DESC LIMIT 1
	`, d.TenantID, idOrEmail).Scan(&row.ID, &row.Email, &row.Type, &row.DisplayName, &row.Department,
		&row.Status, &row.Source, &row.RiskScore, &riskFactors, &createdAt)
	if err != nil {
		return nil, err
	}
	row.RiskFactors = riskFactors
	row.CreatedAt = createdAt
	return &row, nil
}

// identityEntitlements returns the Neo4j entitlements graph for an identity.
func identityEntitlements(ctx context.Context, d *Deps, id string) (any, error) {
	session := d.Neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $id})
		OPTIONAL MATCH (i)-[:HAS_ROLE]->(r:Role)-[:GRANTS]->(e:Entitlement)-[:ACCESSES]->(res:Resource)
		OPTIONAL MATCH (i)-[:DIRECTLY_OWNS]->(e2:Entitlement)-[:ACCESSES]->(res2:Resource)
		RETURN COLLECT(DISTINCT {
			entitlement: e, role: r, resource: res, source: 'role_inherited'
		}) + COLLECT(DISTINCT {
			entitlement: e2, resource: res2, source: 'direct'
		}) AS entitlements
	`, map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	if result.Next(ctx) {
		ent, _ := result.Record().Get("entitlements")
		return ent, nil
	}
	return []any{}, nil
}

// explainPath computes the shortest graph path identity→resource.
func explainPath(ctx context.Context, d *Deps, identityID, resourceID string) (any, error) {
	session := d.Neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.Run(ctx, `
		MATCH p = shortestPath((i:Identity {uuid: $iid})-[:HAS_ROLE|DIRECTLY_OWNS|GRANTS|ACCESSES*..6]->(r:Resource {id: $rid}))
		RETURN [n IN nodes(p) | {
			labels: labels(n),
			props: properties(n)
		}] AS path
	`, map[string]any{"iid": identityID, "rid": resourceID})
	if err != nil {
		return nil, err
	}
	if result.Next(ctx) {
		path, _ := result.Record().Get("path")
		return path, nil
	}
	return nil, nil
}

// resourceType resolves the resource's stored type, defaulting to Resource.
func resourceType(ctx context.Context, d *Deps, resourceID string) string {
	var rt string
	if err := d.Pool.QueryRow(ctx,
		`SELECT COALESCE(resource_type, 'Resource') FROM resources WHERE id = $1`, resourceID,
	).Scan(&rt); err != nil {
		rt = "Resource"
	}
	return rt
}

// parseDuration converts "2h" | "1d" | "permanent" into hours.
func parseDuration(s string) (hours int, permanent bool, err error) {
	switch strings.TrimSpace(s) {
	case "", "2h":
		return 2, false, nil
	case "1d", "24h":
		return 24, false, nil
	case "permanent":
		return 0, true, nil
	default:
		if n, perr := strconv.Atoi(strings.TrimSuffix(s, "h")); perr == nil && n > 0 {
			return n, false, nil
		}
		return 0, false, fmt.Errorf("invalid duration %q (want 2h, 1d, permanent, or N hours)", s)
	}
}

func toolErr(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{mcp.TextContent{Type: "text", Text: msg}}}
}

func toolJSON(v any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return toolErr("marshal error: " + err.Error())
	}
	return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}
}

func stringArg(req mcp.CallToolRequest, name string) string {
	s, _ := req.GetArguments()[name].(string)
	return s
}

func boolArg(req mcp.CallToolRequest, name string) bool {
	b, _ := req.GetArguments()[name].(bool)
	return b
}

// cedarCheck evaluates the identity's standing access against the tenant's
// Cedar policies (the same engine the CheckAccess endpoint uses).
func cedarCheck(ctx context.Context, d *Deps, row *identityRow, resourceType, resourceID string) (*cedar.AuthDecision, error) {
	if err := d.Cedar.LoadPolicies(ctx, d.TenantID); err != nil {
		return nil, err
	}
	principalType := "User"
	switch row.Type {
	case "ai_agent", "service_account", "robot", "iot_device", "rpa_bot":
		principalType = "NonHumanIdentity"
	}
	dec, err := d.Cedar.IsAuthorized(ctx, cedar.AuthRequest{
		PrincipalID:   row.ID,
		PrincipalType: principalType,
		Action:        "ReadAccess",
		ResourceID:    resourceID,
		ResourceType:  resourceType,
		TenantID:      d.TenantID,
		Department:    row.Department,
		IsActive:      row.Status == "active",
	})
	if err != nil {
		return nil, err
	}
	return &dec, nil
}

// handleQueryIdentity implements query_identity.
func handleQueryIdentity(ctx context.Context, d *Deps, sess *Session, req mcp.CallToolRequest) *mcp.CallToolResult {
	identityID := stringArg(req, "identity_id")
	if identityID == "" {
		return toolErr("identity_id is required")
	}
	row, err := queryIdentity(ctx, d, identityID)
	if err != nil {
		return toolErr("identity not found: " + err.Error())
	}
	out := map[string]any{
		"identity": row,
	}
	if boolArg(req, "include_entitlements") {
		ents, err := identityEntitlements(ctx, d, row.ID)
		if err != nil {
			return toolErr("entitlements query failed: " + err.Error())
		}
		out["entitlements"] = ents
	}
	if boolArg(req, "include_risk") {
		score, factors, err := risk.CalculateIdentityRisk(ctx, d.Neo4j, d.Pool, sess.TenantID, row.ID)
		if err != nil {
			return toolErr("risk query failed: " + err.Error())
		}
		out["risk"] = map[string]any{
			"risk_score": score,
			"risk_band":  services.RiskBandFromScore(score),
			"factors":    factors,
		}
	}
	AuditCall(ctx, d.Audit, sess, "query_identity", "identity://"+row.ID, map[string]any{"identity_id": row.ID})
	return toolJSON(out)
}

// handleRequestAccess implements request_access: it starts the same
// GrantAccessWorkflow the API uses, so conditional access + approval
// routing apply to MCP-initiated requests too.
func handleRequestAccess(ctx context.Context, d *Deps, sess *Session, req mcp.CallToolRequest) *mcp.CallToolResult {
	identityID := stringArg(req, "identity_id")
	resourceID := stringArg(req, "resource_id")
	reason := stringArg(req, "reason")
	if identityID == "" || resourceID == "" || reason == "" {
		return toolErr("identity_id, resource_id and reason are required")
	}
	hours, permanent, err := parseDuration(stringArg(req, "duration"))
	if err != nil {
		return toolErr(err.Error())
	}

	input := workflow.GrantAccessInput{
		TenantID:         sess.TenantID,
		IdentityID:       identityID,
		ResourceID:       resourceID,
		ResourceType:     resourceType(ctx, d, resourceID),
		Reason:           reason,
		RequestedBy:      sess.KeyID,
		DurationHours:    hours,
		RequiresApproval: permanent,
	}
	payload, _ := json.Marshal(map[string]any{
		"resource_id":       resourceID,
		"resource_type":     input.ResourceType,
		"reason":            reason,
		"duration_hours":    hours,
		"requires_approval": permanent,
	})
	wfReq := &workflow.Request{
		TenantID:    sess.TenantID,
		Type:        "access.request.grant",
		Status:      "pending",
		RequesterID: sess.KeyID,
		TargetID:    identityID,
		Payload:     payload,
	}
	created, err := d.Workflows.CreateRequest(ctx, wfReq)
	if err != nil {
		return toolErr("create request: " + err.Error())
	}
	if !created {
		return toolErr("idempotent replay: request already exists")
	}
	_ = d.Workflows.AppendAudit(ctx, wfReq.ID, "workflow.requested", "key:"+sess.KeyID, map[string]any{
		"type": "access.request.grant", "target_id": identityID, "resource_id": resourceID,
		"duration_hours": hours, "reason": reason, "via": "mcp",
	})

	workflowID := fmt.Sprintf("grant-access-%s-%s", identityID, shortID())
	we, err := d.Temporal.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: d.TaskQueue,
	}, workflow.GrantAccessWorkflow, input)
	if err != nil {
		_ = d.Workflows.FailRequest(ctx, wfReq.ID, "temporal start failed: "+err.Error())
		return toolErr("start workflow: " + err.Error())
	}
	_ = d.Workflows.SetTemporalIDs(ctx, wfReq.ID, we.GetID(), we.GetRunID())

	AuditCall(ctx, d.Audit, sess, "request_access", "resource://"+resourceID, map[string]any{
		"request_id": wfReq.ID, "identity_id": identityID, "resource_id": resourceID,
		"duration_hours": hours, "permanent": permanent,
	})
	return toolJSON(map[string]any{
		"request_id":  wfReq.ID,
		"status":      "pending",
		"policy":      "conditional access routing (async via Temporal)",
		"workflow_id": we.GetID(),
		"permanent":   permanent,
	})
}

// handleCheckRisk implements check_risk.
func handleCheckRisk(ctx context.Context, d *Deps, sess *Session, req mcp.CallToolRequest) *mcp.CallToolResult {
	identityID := stringArg(req, "identity_id")
	if identityID == "" {
		return toolErr("identity_id is required")
	}
	row, err := queryIdentity(ctx, d, identityID)
	if err != nil {
		return toolErr("identity not found: " + err.Error())
	}
	score, factors, err := risk.CalculateIdentityRisk(ctx, d.Neo4j, d.Pool, sess.TenantID, row.ID)
	if err != nil {
		return toolErr("risk query failed: " + err.Error())
	}
	lastEvent := "none"
	if len(factors) > 0 {
		lastEvent = factors[0]
	}
	AuditCall(ctx, d.Audit, sess, "check_risk", "identity://"+row.ID, map[string]any{"identity_id": row.ID})
	return toolJSON(map[string]any{
		"identity_id": row.ID,
		"risk_score":  score,
		"risk_band":   services.RiskBandFromScore(score),
		"last_event":  lastEvent,
		"last_source": row.Source,
		"factors":     factors,
	})
}

// handleExplainAccess implements explain_access.
func handleExplainAccess(ctx context.Context, d *Deps, sess *Session, req mcp.CallToolRequest) *mcp.CallToolResult {
	identityID := stringArg(req, "identity_id")
	resourceID := stringArg(req, "resource_id")
	if identityID == "" || resourceID == "" {
		return toolErr("identity_id and resource_id are required")
	}
	row, err := queryIdentity(ctx, d, identityID)
	if err != nil {
		return toolErr("identity not found: " + err.Error())
	}
	path, err := explainPath(ctx, d, row.ID, resourceID)
	if err != nil {
		return toolErr("graph path query failed: " + err.Error())
	}

	rt := resourceType(ctx, d, resourceID)
	decision, cerr := cedarCheck(ctx, d, row, rt, resourceID)
	score, factors, _ := risk.CalculateIdentityRisk(ctx, d.Neo4j, d.Pool, sess.TenantID, row.ID)

	AuditCall(ctx, d.Audit, sess, "explain_access", "resource://"+resourceID, map[string]any{
		"identity_id": row.ID, "resource_id": resourceID, "path_found": path != nil,
	})
	policyOut := map[string]any{"path_found": path != nil}
	if decision != nil {
		policyOut = map[string]any{
			"decision":         decision.Decision,
			"allowed":          decision.Allowed,
			"matched_policies": decision.MatchedPolicies,
		}
	}
	return toolJSON(map[string]any{
		"identity_id": row.ID,
		"resource_id": resourceID,
		"path":        path,
		"policy":      policyOut,
		"risk": map[string]any{
			"risk_score": score,
			"risk_band":  services.RiskBandFromScore(score),
			"factors":    factors,
		},
		"cedar_error": func() string {
			if cerr != nil {
				return cerr.Error()
			}
			return ""
		}(),
	})
}

// handleListApprovals implements list_approvals.
func handleListApprovals(ctx context.Context, d *Deps, sess *Session, req mcp.CallToolRequest) *mcp.CallToolResult {
	identityID := stringArg(req, "identity_id")
	status := stringArg(req, "status")
	rows, err := d.Pool.Query(ctx, `
		SELECT a.id::text, a.request_id::text, a.level, COALESCE(a.approver_id::text, ''),
		       COALESCE(a.approver_email, ''), COALESCE(a.approver_role, ''),
		       a.status, COALESCE(a.comment, ''), a.due_at, a.created_at,
		       COALESCE(w.target_id::text, ''), COALESCE(w.requester_id::text, '')
		FROM workflow_approvals a
		LEFT JOIN workflow_requests w ON w.id = a.request_id
		WHERE w.tenant_id = $1
		  AND ($2 = '' OR w.target_id::text = $2 OR w.requester_id::text = $2)
		  AND ($3 = '' OR a.status = $3)
		ORDER BY a.created_at DESC
		LIMIT 100
	`, sess.TenantID, identityID, status)
	if err != nil {
		return toolErr("approvals query failed: " + err.Error())
	}
	defer rows.Close()
	var approvals []map[string]any
	for rows.Next() {
		var id, requestID, approverID, approverEmail, approverRole, st, comment, targetID, requesterID string
		var level int
		var dueAt, createdAt *time.Time
		if err := rows.Scan(&id, &requestID, &level, &approverID, &approverEmail, &approverRole,
			&st, &comment, &dueAt, &createdAt, &targetID, &requesterID); err != nil {
			return toolErr("approvals scan failed: " + err.Error())
		}
		approvals = append(approvals, map[string]any{
			"approval_id": id, "request_id": requestID, "level": level,
			"approver_id": approverID, "approver_email": approverEmail, "approver_role": approverRole,
			"status": st, "comment": comment, "due_at": dueAt, "created_at": createdAt,
			"target_id": targetID, "requester_id": requesterID,
		})
	}
	AuditCall(ctx, d.Audit, sess, "list_approvals", "approvals", map[string]any{
		"identity_id": identityID, "status": status, "count": len(approvals),
	})
	return toolJSON(map[string]any{"approvals": approvals, "total": len(approvals)})
}

func shortID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())[:8]
}
