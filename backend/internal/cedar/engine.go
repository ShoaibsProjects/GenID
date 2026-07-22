package cedar

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	cedar "github.com/cedar-policy/cedar-go"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/observeid/identity-platform/pkg/telemetry"
)

// CedarEngine is the core Cedar authorization engine.
// It loads policies from PostgreSQL, builds in-memory PolicySets per tenant,
// and evaluates authorization requests using the official cedar-go library.
type CedarEngine struct {
	mu       sync.RWMutex
	pgPool   *pgxpool.Pool
	policies map[string]*cedar.PolicySet // tenant_id -> PolicySet
	loadedAt map[string]time.Time        // tenant_id -> last load time
}

// NewCedarEngine creates a new Cedar authorization engine.
func NewCedarEngine(pgPool *pgxpool.Pool) *CedarEngine {
	return &CedarEngine{
		pgPool:   pgPool,
		policies: make(map[string]*cedar.PolicySet),
		loadedAt: make(map[string]time.Time),
	}
}

// AuthRequest represents an authorization request to the Cedar engine.
type AuthRequest struct {
	PrincipalID      string
	PrincipalType    string // "User", "Agent", "NonHumanIdentity"
	Action           string // "ReadAccess", "AdminAccess", etc.
	ResourceID       string
	ResourceType     string
	TenantID         string
	Context          map[string]any
	Department       string
	Clearance        int64
	IsActive         bool
	Region           string
	IsContractor     bool
	EmploymentType   string
	AssuranceLevel   string
	DataClassification string
	Criticality      string
	OwnerDepartment  string
	IsRevoked        bool
	DeploymentEnv    string
	DelegationDepth  int64
	MFAPresent       bool
	SourceIP         string
}

// AuthDecision represents the result of a Cedar authorization evaluation.
type AuthDecision struct {
	Allowed          bool
	Decision         string   // "permit", "forbid", "not_applicable"
	MatchedPolicies  []string
	Errors           []string
}

// LoadPolicies loads all active Cedar policies for a tenant from PostgreSQL
// and builds an in-memory PolicySet.
func (e *CedarEngine) LoadPolicies(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	// Try loading cedar_text first, fall back to policy_source for migration
	rows, err := e.pgPool.Query(ctx, `
		SELECT policy_id, COALESCE(cedar_text, policy_source), effect
		FROM cedar_policies
		WHERE tenant_id = $1 AND is_active = true
		ORDER BY version DESC
	`, tenantID)
	if err != nil {
		return fmt.Errorf("cedar: load policies: %w", err)
	}
	defer rows.Close()

	ps := cedar.NewPolicySet()
	var loaded int

	for rows.Next() {
		var policyID, policyText, effect string
		if err := rows.Scan(&policyID, &policyText, &effect); err != nil {
			log.Printf("[CEDAR] skip policy scan error: %v", err)
			continue
		}

		// If cedar_text is empty, convert the old pattern format to Cedar text
		if policyText == "" || (!strings.Contains(policyText, "permit(") && !strings.Contains(policyText, "forbid(")) {
			policyText = convertPatternToCedar(effect, policyText)
		}

		if policyText == "" {
			continue
		}

		var policy cedar.Policy
		if err := policy.UnmarshalCedar([]byte(policyText)); err != nil {
			log.Printf("[CEDAR] skip invalid policy %s: %v", policyID, err)
			continue
		}

		ps.Add(cedar.PolicyID(policyID), &policy)
		loaded++
	}

	e.mu.Lock()
	e.policies[tenantID] = ps
	e.loadedAt[tenantID] = time.Now()
	e.mu.Unlock()

	log.Printf("[CEDAR] loaded %d policies for tenant %s", loaded, tenantID)
	return nil
}

// IsAuthorized evaluates an authorization request against the loaded policies.
func (e *CedarEngine) IsAuthorized(ctx context.Context, req AuthRequest) (AuthDecision, error) {
	start := time.Now()
	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	// Ensure policies are loaded
	e.mu.RLock()
	ps, exists := e.policies[tenantID]
	e.mu.RUnlock()

	if !exists || ps == nil {
		if err := e.LoadPolicies(ctx, tenantID); err != nil {
			return AuthDecision{Decision: "not_applicable"}, fmt.Errorf("cedar: load policies: %w", err)
		}
		e.mu.RLock()
		ps = e.policies[tenantID]
		e.mu.RUnlock()
	}

	if ps == nil {
		return AuthDecision{Decision: "not_applicable"}, nil
	}

	// Build Cedar request and entities
	cedarReq, entities := e.buildCedarRequest(req)

	// Evaluate
	decision, diag := cedar.Authorize(ps, entities, cedarReq)

	result := AuthDecision{
		Allowed: decision == cedar.Allow,
	}

	switch decision {
	case cedar.Allow:
		result.Decision = "permit"
	case cedar.Deny:
		result.Decision = "forbid"
	}

	for _, reason := range diag.Reasons {
		result.MatchedPolicies = append(result.MatchedPolicies, string(reason.PolicyID))
	}
	for _, err := range diag.Errors {
		result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", err.PolicyID, err.Message))
	}

	RecordEvaluation(req, result, time.Since(start))

	return result, nil
}

// InvalidateTenant forces a reload of policies for a tenant on next evaluation.
func (e *CedarEngine) InvalidateTenant(tenantID string) {
	e.mu.Lock()
	delete(e.policies, tenantID)
	delete(e.loadedAt, tenantID)
	e.mu.Unlock()
}

// ReloadAll reloads policies for all known tenants.
func (e *CedarEngine) ReloadAll(ctx context.Context) {
	e.mu.RLock()
	tenants := make([]string, 0, len(e.policies))
	for t := range e.policies {
		tenants = append(tenants, t)
	}
	e.mu.RUnlock()

	for _, t := range tenants {
		if err := e.LoadPolicies(ctx, t); err != nil {
			log.Printf("[CEDAR] reload failed for tenant %s: %v", t, err)
		}
	}
}

// PolicyCount returns the number of loaded policies for a tenant.
func (e *CedarEngine) PolicyCount(tenantID string) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if ps := e.policies[tenantID]; ps != nil {
		count := 0
		for range ps.All() {
			count++
		}
		return count
	}
	return 0
}

// ─── Entity Construction ──────────────────────────────────────

func (e *CedarEngine) buildCedarRequest(req AuthRequest) (cedar.Request, cedar.EntityMap) {
	entities := make(cedar.EntityMap)

	// Build principal entity
	principalUID := cedar.NewEntityUID(cedar.EntityType(req.PrincipalType), cedar.String(req.PrincipalID))
	principalAttrs := make(cedar.RecordMap)

	if req.Department != "" {
		principalAttrs["department"] = cedar.String(req.Department)
	}
	if req.Region != "" {
		principalAttrs["region"] = cedar.String(req.Region)
	}
	principalAttrs["clearance"] = cedar.Long(req.Clearance)
	if req.IsActive {
		principalAttrs["is_active"] = cedar.True
	} else {
		principalAttrs["is_active"] = cedar.False
	}
	if req.IsContractor {
		principalAttrs["is_contractor"] = cedar.True
	} else {
		principalAttrs["is_contractor"] = cedar.False
	}
	if req.EmploymentType != "" {
		principalAttrs["employment_type"] = cedar.String(req.EmploymentType)
	}
	if req.AssuranceLevel != "" {
		principalAttrs["assurance_level"] = cedar.String(req.AssuranceLevel)
	}
	if req.TenantID != "" {
		principalAttrs["tenant_id"] = cedar.String(req.TenantID)
	}

	// Agent-specific attributes
	if req.PrincipalType == "Agent" || req.PrincipalType == "NonHumanIdentity" {
		if req.DeploymentEnv != "" {
			principalAttrs["deployment_env"] = cedar.String(req.DeploymentEnv)
		}
		if req.IsRevoked {
			principalAttrs["is_revoked"] = cedar.True
		} else {
			principalAttrs["is_revoked"] = cedar.False
		}
	}

	entities[principalUID] = cedar.Entity{
		UID:        principalUID,
		Attributes: cedar.NewRecord(principalAttrs),
		Parents:    cedar.NewEntityUIDSet(),
	}

	// Build resource entity
	resourceUID := cedar.NewEntityUID(cedar.EntityType(req.ResourceType), cedar.String(req.ResourceID))
	resourceAttrs := make(cedar.RecordMap)

	if req.DataClassification != "" {
		resourceAttrs["data_classification"] = cedar.String(req.DataClassification)
	}
	if req.Criticality != "" {
		resourceAttrs["criticality"] = cedar.String(req.Criticality)
	}
	if req.OwnerDepartment != "" {
		resourceAttrs["owner_department"] = cedar.String(req.OwnerDepartment)
	}
	if req.Region != "" {
		resourceAttrs["region"] = cedar.String(req.Region)
	}
	if req.TenantID != "" {
		resourceAttrs["tenant_id"] = cedar.String(req.TenantID)
	}
	resourceAttrs["name"] = cedar.String(req.ResourceID)
	resourceAttrs["type"] = cedar.String(req.ResourceType)

	entities[resourceUID] = cedar.Entity{
		UID:        resourceUID,
		Attributes: cedar.NewRecord(resourceAttrs),
		Parents:    cedar.NewEntityUIDSet(),
	}

	// Build context
	contextAttrs := make(cedar.RecordMap)
	if req.MFAPresent {
		contextAttrs["mfa_present"] = cedar.True
	} else {
		contextAttrs["mfa_present"] = cedar.False
	}
	if req.SourceIP != "" {
		contextAttrs["source_ip"] = cedar.String(req.SourceIP)
	}
	if req.DelegationDepth > 0 {
		contextAttrs["delegation_depth"] = cedar.Long(req.DelegationDepth)
	}

	// Merge any extra context values
	for k, v := range req.Context {
		contextAttrs[cedar.String(k)] = toCedarValue(v)
	}

	cedarReq := cedar.Request{
		Principal: principalUID,
		Action:    cedar.NewEntityUID("Action", cedar.String(req.Action)),
		Resource:  resourceUID,
		Context:   cedar.NewRecord(contextAttrs),
	}

	return cedarReq, entities
}

// toCedarValue converts a Go value to a Cedar Value.
func toCedarValue(v any) cedar.Value {
	switch val := v.(type) {
	case bool:
		if val {
			return cedar.True
		}
		return cedar.False
	case string:
		return cedar.String(val)
	case int:
		return cedar.Long(int64(val))
	case int64:
		return cedar.Long(val)
	case float64:
		return cedar.Long(int64(val))
	default:
		return cedar.String(fmt.Sprintf("%v", val))
	}
}

// convertPatternToCedar converts the old custom pattern format to Cedar text.
// Old format: "permit(Engineering, read, res-aws-prod)"
// Cedar format: 'permit(principal == Role::"Engineering", action == Action::"ReadAccess", resource == Resource::"res-aws-prod");'
func convertPatternToCedar(effect, pattern string) string {
	// Extract the parenthesized content
	open := strings.Index(pattern, "(")
	close := strings.LastIndex(pattern, ")")
	if open == -1 || close == -1 || open >= close {
		return ""
	}

	raw := pattern[open+1 : close]
	parts := strings.SplitN(raw, ",", 3)
	if len(parts) < 3 {
		return ""
	}

	idPat := strings.TrimSpace(parts[0])
	actPat := strings.TrimSpace(parts[1])
	resPat := strings.TrimSpace(parts[2])

	// Build Cedar principal clause
	var principalClause string
	if idPat == "*" || idPat == "" {
		principalClause = "principal"
	} else {
		principalClause = fmt.Sprintf(`principal == Role::"%s"`, idPat)
	}

	// Build Cedar action clause
	var actionClause string
	if actPat == "*" || actPat == "" {
		actionClause = "action"
	} else {
		actionClause = fmt.Sprintf(`action == Action::"%s"`, actPat)
	}

	// Build Cedar resource clause
	var resourceClause string
	if resPat == "*" || resPat == "" {
		resourceClause = "resource"
	} else {
		resourceClause = fmt.Sprintf(`resource == Resource::"%s"`, resPat)
	}

	return fmt.Sprintf(`%s(%s, %s, %s);`, effect, principalClause, actionClause, resourceClause)
}

// mustJSON marshals a value to JSON, returning empty bytes on error.
func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// ─── Hot Reload ──────────────────────────────────────────────────

// StartHotReload starts a background goroutine that polls PostgreSQL for policy
// changes every interval and reloads any stale tenants.
func (e *CedarEngine) StartHotReload(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		log.Printf("[CEDAR] hot reload started (interval: %s)", interval)
		for {
			select {
			case <-ctx.Done():
				log.Printf("[CEDAR] hot reload stopped")
				return
			case <-ticker.C:
				e.reloadChangedTenants(ctx)
			}
		}
	}()
}

// reloadChangedTenants checks each tenant for updated_at changes and reloads stale ones.
func (e *CedarEngine) reloadChangedTenants(ctx context.Context) {
	// Discover all tenant IDs that have policies
	rows, err := e.pgPool.Query(ctx, `
		SELECT DISTINCT tenant_id FROM cedar_policies WHERE is_active = true
	`)
	if err != nil {
		log.Printf("[CEDAR] hot reload query failed: %v", err)
		return
	}
	defer rows.Close()

	var tenantIDs []string
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			continue
		}
		tenantIDs = append(tenantIDs, tid)
	}

	for _, tid := range tenantIDs {
		// Get latest updated_at for this tenant
		var latestUpdate time.Time
		err := e.pgPool.QueryRow(ctx, `
			SELECT COALESCE(MAX(updated_at), '1970-01-01') FROM cedar_policies
			WHERE tenant_id = $1 AND is_active = true
		`, tid).Scan(&latestUpdate)
		if err != nil {
			continue
		}

		e.mu.RLock()
		loaded, exists := e.loadedAt[tid]
		e.mu.RUnlock()

		// Reload if: not loaded yet, or updated_at is newer than last load
		if !exists || latestUpdate.After(loaded) {
			if err := e.LoadPolicies(ctx, tid); err != nil {
				log.Printf("[CEDAR] hot reload failed for tenant %s: %v", tid, err)
			} else {
				log.Printf("[CEDAR] hot reload succeeded for tenant %s", tid)
			}
		}
	}
}

// ─── Prometheus Metrics ──────────────────────────────────────────

// RecordEvaluation records a Cedar evaluation result for Prometheus metrics.
// Uses the metrics defined in pkg/telemetry.
func RecordEvaluation(req AuthRequest, decision AuthDecision, duration time.Duration) {
	telemetry.CedarEvaluationLatency.WithLabelValues(decision.Decision, req.TenantID).Observe(duration.Seconds())
	if !decision.Allowed {
		telemetry.CedarDenyRate.WithLabelValues(req.PrincipalType, req.Action, req.ResourceType).Inc()
	}
}
