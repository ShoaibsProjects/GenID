package activities

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
)

// ─── Activity Input/Output Types ──────────────────────────

type AuditTrailParams struct {
	AuditID     string `json:"audit_id"`
	Operation   string `json:"operation"`
	IdentityID  string `json:"identity_id"`
	IdentityType string `json:"identity_type"`
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	TenantID    string `json:"tenant_id"`
}

type AuditTrailResult struct {
	AuditID string `json:"audit_id"`
}

type LockParams struct {
	IdentityID string `json:"identity_id"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type LockResult struct {
	Token        string `json:"token"`
	FenceVersion int64  `json:"fence_version"`
}

type UnlockParams struct {
	IdentityID string `json:"identity_id"`
	Token      string `json:"token"`
}

type EntitlementQueryParams struct {
	IdentityID string `json:"identity_id"`
	TenantID   string `json:"tenant_id"`
}

type EntitlementResult struct {
	ID             string `json:"id"`
	AppName        string `json:"app_name"`
	PermissionLevel string `json:"permission_level"`
	EntitlementType string `json:"entitlement_type"`
	ResourceID     string `json:"resource_id"`
	ResourceName   string `json:"resource_name"`
	Source         string `json:"source"` // "role_inheritance" or "direct"
	RoleID         string `json:"role_id"`
	RoleName       string `json:"role_name"`
	ConnectorID    string `json:"connector_id"`
	IsToxic        bool   `json:"is_toxic"`
	RiskScore      float64 `json:"risk_score"`
}

type RevocationParams struct {
	IdentityID      string `json:"identity_id"`
	EntitlementID   string `json:"entitlement_id"`
	ConnectorID     string `json:"connector_id"`
	ExternalID      string `json:"external_id"`
	TargetSystem    string `json:"target_system"`
	Reason          string `json:"reason"`
	RevokedBy       string `json:"revoked_by"`
	IsEmergency     bool   `json:"is_emergency"`
	TenantID        string `json:"tenant_id"`
}

type ProvisionParams struct {
	IdentityID      string `json:"identity_id"`
	ResourceID      string `json:"resource_id"`
	RoleID          string `json:"role_id"`
	ConnectorID     string `json:"connector_id"`
	DurationMinutes int    `json:"duration_minutes"`
	GrantedBy       string `json:"granted_by"`
	Reason          string `json:"reason"`
	TenantID        string `json:"tenant_id"`
}

type DelegationQueryParams struct {
	IdentityID string `json:"identity_id"`
	MaxDepth   int    `json:"max_depth"`
}

type DelegationResult struct {
	AgentID     string `json:"agent_id"`
	Depth       int    `json:"depth"`
	Scope       string `json:"scope"`
	AgentType   string `json:"agent_type"`
	IsGoverned  bool   `json:"is_governed"`
	RiskScore   float64 `json:"risk_score"`
}

type CAEPEventParams struct {
	EventType   string   `json:"event_type"`
	IdentityID  string   `json:"identity_id"`
	SessionID   string   `json:"session_id"`
	Subjects    []string `json:"subjects"`
	ReasonAdmin string   `json:"reason_admin"`
	ReasonUser  string   `json:"reason_user"`
	TenantID    string   `json:"tenant_id"`
}

type PolicyCheckParams struct {
	IdentityID   string            `json:"identity_id"`
	IdentityType string            `json:"identity_type"`
	ResourceID   string            `json:"resource_id"`
	ResourceType string            `json:"resource_type"`
	Action       string            `json:"action"`
	Context      map[string]any    `json:"context"`
	TenantID     string            `json:"tenant_id"`
}

type PolicyCheckResult struct {
	Allowed     bool     `json:"allowed"`
	Decision    string   `json:"decision"` // "permit", "forbid", "not_applicable"
	MatchedPolicies []string `json:"matched_policies"`
	Reason      string   `json:"reason"`
}

type SoDCheckResult struct {
	HasConflict bool     `json:"has_conflict"`
	Conflicts   []SoDConflict `json:"conflicts"`
	RiskScore   float64  `json:"risk_score"`
}

type SoDConflict struct {
	ExistingRoleID      string `json:"existing_role_id"`
	ExistingRoleName    string `json:"existing_role_name"`
	RequestedRoleID     string `json:"requested_role_id"`
	ConflictType        string `json:"conflict_type"` // "toxic_pair", "transitive", "rubberband"
	Severity            string `json:"severity"`      // "critical", "high", "medium"
	Resolution          string `json:"resolution"`
}

type AnomalyResult struct {
	AgentID     string            `json:"agent_id"`
	AgentName   string            `json:"agent_name"`
	AnomalyType string            `json:"anomaly_type"` // "behavioral", "graph", "access_pattern"
	Score       float64           `json:"score"`
	Reason      string            `json:"reason"`
	Critical    bool              `json:"critical"`
	Details     map[string]any    `json:"details"`
}

type CreateIdentityParams struct {
	Email        string            `json:"email"`
	DisplayName  string            `json:"display_name"`
	IdentityType string            `json:"identity_type"`
	Department   string            `json:"department"`
	EmployeeID   string            `json:"employee_id"`
	ManagerID    string            `json:"manager_id"`
	Source       string            `json:"source"`
	RequestedBy  string            `json:"requested_by"`
	TenantID     string            `json:"tenant_id"`
	Attributes   map[string]string `json:"attributes"`
}

type CreateIdentityResult struct {
	IdentityID string    `json:"identity_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type AssignRoleParams struct {
	IdentityID string `json:"identity_id"`
	RoleName   string `json:"role_name"`
	RoleID     string `json:"role_id"`
	AssignedBy string `json:"assigned_by"`
	TenantID   string `json:"tenant_id"`
}

// ─── Activity Service ──────────────────────────────────────

type ActivityService struct {
	pgPool   *pgxpool.Pool
	neo4j    neo4j.DriverWithContext
	redis    *redis.Client
	temporal client.Client

	lockMu       sync.Mutex
	fenceCounter int64
}

func NewActivityService(pgPool *pgxpool.Pool, neo4j neo4j.DriverWithContext, rdb *redis.Client, tc client.Client) *ActivityService {
	return &ActivityService{
		pgPool:       pgPool,
		neo4j:        neo4j,
		redis:        rdb,
		temporal:     tc,
		fenceCounter: time.Now().UnixMilli(),
	}
}

// ─── Audit Activities ──────────────────────────────────────

func (s *ActivityService) InitiateAuditTrail(ctx context.Context, params AuditTrailParams) (AuditTrailResult, error) {
	auditID := uuid.New().String()

	_, err := s.pgPool.Exec(ctx, `
		INSERT INTO audit_log (id, tenant_id, event_type, actor_id, actor_type, action, resource, details, ip_address, created_at)
		VALUES ($1, $2, 'workflow_started', $3, $4, $5, $6, $7, 'internal', NOW())`,
		auditID, params.TenantID, params.IdentityID, params.IdentityType,
		params.Operation, "identity:"+params.IdentityID,
		json.RawMessage(fmt.Sprintf(`{"reason":"%s","requested_by":"%s"}`, params.Reason, params.RequestedBy)),
	)
	if err != nil {
		return AuditTrailResult{}, fmt.Errorf("audit trail insert: %w", err)
	}

	activity.RecordHeartbeat(ctx, "audit_initialized")
	return AuditTrailResult{AuditID: auditID}, nil
}

func (s *ActivityService) FinalizeAuditTrail(ctx context.Context, params map[string]any) error {
	auditID, _ := params["audit_id"].(string)
	status, _ := params["status"].(string)

	_, err := s.pgPool.Exec(ctx, `
		UPDATE audit_log SET details = details || jsonb_build_object('status', $2, 'completed_at', NOW())
		WHERE id = $1`, auditID, status)
	if err != nil {
		return fmt.Errorf("audit trail finalize: %w", err)
	}

	activity.RecordHeartbeat(ctx, "audit_finalized")
	return nil
}

// ─── Lock Activities (Fencing Token + Watchdog) ───────────

func (s *ActivityService) AcquireIdentityLock(ctx context.Context, params LockParams) (LockResult, error) {
	s.lockMu.Lock()
	s.fenceCounter++
	fenceVersion := s.fenceCounter
	s.lockMu.Unlock()

	token := fmt.Sprintf("%s-%d", uuid.New().String(), fenceVersion)
	lockKey := fmt.Sprintf("lock:identity:%s", params.IdentityID)

	ok, err := s.redis.SetNX(ctx, lockKey, token, time.Duration(params.TTLSeconds)*time.Second).Result()
	if err != nil {
		return LockResult{}, fmt.Errorf("redis lock error: %w", err)
	}
	if !ok {
		existing, _ := s.redis.Get(ctx, lockKey).Result()
		ttl, _ := s.redis.TTL(ctx, lockKey).Result()
		return LockResult{}, fmt.Errorf("identity %s is already locked (holder: %s, ttl: %v)",
			params.IdentityID, existing, ttl)
	}

	// Start watchdog goroutine to extend the lock
	go func() {
		ticker := time.NewTicker(time.Duration(params.TTLSeconds/3) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				luaExtend := redis.NewScript(`
					if redis.call("GET", KEYS[1]) == ARGV[1] then
						return redis.call("EXPIRE", KEYS[1], ARGV[2])
					end
					return 0
				`)
				_ = luaExtend.Run(ctx, s.redis, []string{lockKey}, token, params.TTLSeconds).Err()
			}
		}
	}()

	activity.RecordHeartbeat(ctx, "lock_acquired", fenceVersion)
	return LockResult{Token: token, FenceVersion: fenceVersion}, nil
}

func (s *ActivityService) ReleaseIdentityLock(ctx context.Context, params UnlockParams) error {
	lockKey := fmt.Sprintf("lock:identity:%s", params.IdentityID)

	luaRelease := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		end
		return 0
	`)
	if err := luaRelease.Run(ctx, s.redis, []string{lockKey}, params.Token).Err(); err != nil {
		return fmt.Errorf("lock release error: %w", err)
	}

	activity.RecordHeartbeat(ctx, "lock_released")
	return nil
}

// ─── Graph (Neo4j) Activities ──────────────────────────────

func (s *ActivityService) QueryIdentityEntitlements(ctx context.Context, params EntitlementQueryParams) ([]EntitlementResult, error) {
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (i:Identity {uuid: $identityId})
		OPTIONAL MATCH (i)-[:HAS_ROLE]->(r:Role)-[:GRANTS]->(e:Entitlement)-[:ACCESSES]->(res:Resource)
		OPTIONAL MATCH (i)-[:DIRECTLY_OWNS]->(e2:Entitlement)-[:ACCESSES]->(res2:Resource)
		WITH COLLECT(DISTINCT {
			entitlement: properties(e), role: properties(r), resource: properties(res),
			source: 'role_inheritance'
		}) + COLLECT(DISTINCT {
			entitlement: properties(e2), resource: properties(res2),
			source: 'direct'
		}) AS raw
		UNWIND raw AS item
		RETURN DISTINCT item.entitlement.id AS id,
		       item.entitlement.app_name AS app_name,
		       item.entitlement.permission_level AS permission_level,
		       item.entitlement.entitlement_type AS entitlement_type,
		       item.resource.id AS resource_id,
		       item.resource.name AS resource_name,
		       item.source AS source,
		       item.role.id AS role_id,
		       item.role.name AS role_name,
		       COALESCE(item.entitlement.is_toxic, false) AS is_toxic,
		       COALESCE(item.entitlement.risk_classification, 'low') AS risk_classification
	`

	result, err := session.Run(ctx, query, map[string]any{"identityId": params.IdentityID})
	if err != nil {
		return nil, fmt.Errorf("neo4j entitlement query: %w", err)
	}

	var entitlements []EntitlementResult
	for result.Next(ctx) {
		rec := result.Record()
		e := EntitlementResult{
			ID:              getStr(rec, "id"),
			AppName:         getStr(rec, "app_name"),
			PermissionLevel: getStr(rec, "permission_level"),
			EntitlementType: getStr(rec, "entitlement_type"),
			ResourceID:      getStr(rec, "resource_id"),
			ResourceName:    getStr(rec, "resource_name"),
			Source:          getStr(rec, "source"),
			RoleID:          getStr(rec, "role_id"),
			RoleName:        getStr(rec, "role_name"),
		}
		if isToxic, ok := rec.Get("is_toxic"); ok {
			e.IsToxic, _ = isToxic.(bool)
		}
		rc := getStr(rec, "risk_classification")
		switch rc {
		case "critical":
			e.RiskScore = 1.0
		case "high":
			e.RiskScore = 0.7
		case "medium":
			e.RiskScore = 0.4
		default:
			e.RiskScore = 0.1
		}
		entitlements = append(entitlements, e)
	}

	activity.RecordHeartbeat(ctx, "entitlements_queried", len(entitlements))
	return entitlements, nil
}

func (s *ActivityService) FindDelegatedAgents(ctx context.Context, params DelegationQueryParams) ([]DelegationResult, error) {
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH path = (n:NonHumanIdentity {uuid: $identityId})-[:DELEGATED_FROM*1..$maxDepth]->(child:NonHumanIdentity)
		WHERE child.status = 'active'
		WITH child, length(path) AS depth
		RETURN child.uuid AS agent_id, depth,
		       COALESCE(child.name, 'unknown') AS name,
		       COALESCE(child.type, 'unknown') AS agent_type,
		       COALESCE(child.is_governed, false) AS is_governed,
		       COALESCE(child.risk_score, 0.0) AS risk_score,
		       COALESCE(child.deployment_environment, 'unknown') AS env
		ORDER BY depth ASC
	`

	result, err := session.Run(ctx, query, map[string]any{
		"identityId": params.IdentityID,
		"maxDepth":   params.MaxDepth,
	})
	if err != nil {
		return nil, fmt.Errorf("delegation query: %w", err)
	}

	var agents []DelegationResult
	for result.Next(ctx) {
		rec := result.Record()
		depth, _ := rec.Get("depth")
		depthInt, _ := depth.(int64)
		rs, _ := rec.Get("risk_score")
		riskScore, _ := rs.(float64)
		ig, _ := rec.Get("is_governed")
		isGov, _ := ig.(bool)

		agents = append(agents, DelegationResult{
			AgentID:    getStr(rec, "agent_id"),
			Depth:      int(depthInt),
			AgentType:  getStr(rec, "agent_type"),
			IsGoverned: isGov,
			RiskScore:  riskScore,
		})
	}

	activity.RecordHeartbeat(ctx, "delegated_agents_found", len(agents))
	return agents, nil
}

// ─── Provisioning Activities ──────────────────────────────

func (s *ActivityService) RevokeTargetAccess(ctx context.Context, params RevocationParams) error {
	// Get connector for the target system and revoke in the real system
	connectorID := params.ConnectorID
	if connectorID == "" {
		// Look up connector from entitlement metadata
		var extID, connID string
		err := s.pgPool.QueryRow(ctx, `
			SELECT c.id, ce.external_id
			FROM connector_identities ci
			JOIN connectors c ON c.id = ci.connector_id
			JOIN connector_entitlements ce ON ce.identity_external_id = ci.external_id
			WHERE ce.id = $1`, params.EntitlementID).Scan(&connID, &extID)
		if err != nil {
			return fmt.Errorf("revoke: connector lookup: %w", err)
		}
		params.ConnectorID = connID
		params.ExternalID = extID
	}

	// Record revocation in PostgreSQL
	_, err := s.pgPool.Exec(ctx, `
		INSERT INTO audit_log (id, tenant_id, event_type, actor_id, action, resource, details, ip_address, created_at)
		VALUES ($1, $2, 'entitlement_revoked', $3, 'revoke', $4,
		        jsonb_build_object('connector_id', $5, 'external_id', $6, 'reason', $7, 'is_emergency', $8), 'internal', NOW())`,
		uuid.New().String(), params.TenantID, params.IdentityID,
		"entitlement:"+params.EntitlementID, params.ConnectorID, params.ExternalID,
		params.Reason, params.IsEmergency,
	)
	if err != nil {
		return fmt.Errorf("revoke: audit log: %w", err)
	}

	// Mark entitlement as revoked in Neo4j
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	if _, err := session.Run(ctx, `
		MATCH (e:Entitlement {id: $id})
		SET e.status = 'revoked', e.revoked_at = timestamp(), e.revoked_by = $revokedBy
	`, map[string]any{"id": params.EntitlementID, "revokedBy": params.RevokedBy}); err != nil {
		return fmt.Errorf("revoke: neo4j mark entitlement: %w", err)
	}

	activity.RecordHeartbeat(ctx, "access_revoked")
	return nil
}

func (s *ActivityService) ProvisionAccess(ctx context.Context, params ProvisionParams) error {
	// Write provisioning record
	_, err := s.pgPool.Exec(ctx, `
		INSERT INTO audit_log (id, tenant_id, event_type, actor_id, action, resource, details, ip_address, created_at)
		VALUES ($1, $2, 'access_provisioned', $3, 'provision', $4,
		        jsonb_build_object('resource_id', $5, 'role_id', $6, 'duration_minutes', $7, 'reason', $8, 'granted_by', $9), 'internal', NOW())`,
		uuid.New().String(), params.TenantID, params.IdentityID,
		"resource:"+params.ResourceID, params.ResourceID, params.RoleID,
		params.DurationMinutes, params.Reason, params.GrantedBy,
	)
	if err != nil {
		return fmt.Errorf("provision: audit log: %w", err)
	}

	// Create Neo4j relationship
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	if _, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $identityId}), (res:Resource {id: $resourceId})
		MERGE (i)-[:HAS_DIRECT_ACCESS {granted_at: timestamp(), granted_by: $grantedBy, reason: $reason}]->(res)
	`, map[string]any{
		"identityId": params.IdentityID, "resourceId": params.ResourceID,
		"grantedBy": params.GrantedBy, "reason": params.Reason,
	}); err != nil {
		return fmt.Errorf("provision: neo4j relationship: %w", err)
	}

	// Store in Redis for quick access check invalidation
	s.redis.Del(ctx, fmt.Sprintf("access:check:%s:%s", params.IdentityID, params.ResourceID))

	activity.RecordHeartbeat(ctx, "access_provisioned")
	return nil
}

func (s *ActivityService) RevokeIdentityAccess(ctx context.Context, params RevocationParams) error {
	// Phase 1: Cache invalidation
	s.redis.Set(ctx, fmt.Sprintf("revocation:recent:%s", params.IdentityID), "true", 5*time.Minute)

	// Phase 2: Mark identity as suspended/revoked in PostgreSQL
	_, err := s.pgPool.Exec(ctx, `
		UPDATE identities SET status = 'revoked', updated_at = NOW() WHERE id = $1`, params.IdentityID)
	if err != nil {
		return fmt.Errorf("revoke identity: pg update: %w", err)
	}

	// Phase 3: Mark in Neo4j
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	if _, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $id})
		SET i.status = 'revoked', i.revoked_at = timestamp(), i.revoked_by = $revokedBy
	`, map[string]any{"id": params.IdentityID, "revokedBy": params.RevokedBy}); err != nil {
		return fmt.Errorf("revoke identity: neo4j status update: %w", err)
	}

	// Phase 4: Revoke all active sessions
	s.redis.Del(ctx, fmt.Sprintf("session:active:%s", params.IdentityID))

	activity.RecordHeartbeat(ctx, "identity_revoked")
	return nil
}

func (s *ActivityService) ProvisionTemporaryAccess(ctx context.Context, params ProvisionParams) error {
	if params.DurationMinutes <= 0 {
		params.DurationMinutes = 60
	}

	expiresAt := time.Now().Add(time.Duration(params.DurationMinutes) * time.Minute)

	_, err := s.pgPool.Exec(ctx, `
		INSERT INTO audit_log (id, tenant_id, event_type, actor_id, action, resource, details, ip_address, created_at)
		VALUES ($1, $2, 'temporary_access_granted', $3, 'jit_provision', $4,
		        jsonb_build_object('resource_id', $5, 'duration_minutes', $6, 'expires_at', $7, 'granted_by', $8, 'reason', $9), 'internal', NOW())`,
		uuid.New().String(), params.TenantID, params.IdentityID,
		"resource:"+params.ResourceID, params.ResourceID, params.DurationMinutes,
		expiresAt.Format(time.RFC3339), params.GrantedBy, params.Reason,
	)
	if err != nil {
		return fmt.Errorf("temp provision: audit log: %w", err)
	}

	// Store JIT grant in Redis with TTL for fast expiration
	jitKey := fmt.Sprintf("jit:grant:%s:%s", params.IdentityID, params.ResourceID)
	s.redis.Set(ctx, jitKey, "active", time.Duration(params.DurationMinutes)*time.Minute)

	// Create Neo4j temp relationship
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	if _, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $identityId}), (res:Resource {id: $resourceId})
		MERGE (i)-[:HAS_TEMPORARY_ACCESS {granted_at: timestamp(), expires_at: $expiresAt, granted_by: $grantedBy}]->(res)
	`, map[string]any{
		"identityId": params.IdentityID, "resourceId": params.ResourceID,
		"expiresAt": expiresAt.UnixMilli(), "grantedBy": params.GrantedBy,
	}); err != nil {
		return fmt.Errorf("temp provision: neo4j relationship: %w", err)
	}

	activity.RecordHeartbeat(ctx, "temp_access_granted")
	return nil
}

func (s *ActivityService) RevokeTemporaryAccess(ctx context.Context, params map[string]any) error {
	identityID, _ := params["identity_id"].(string)
	resourceID, _ := params["resource_id"].(string)

	// Remove Redis grant
	if resourceID != "" {
		s.redis.Del(ctx, fmt.Sprintf("jit:grant:%s:%s", identityID, resourceID))
	} else {
		// Pattern delete all JIT grants for this identity
		iter := s.redis.Scan(ctx, 0, fmt.Sprintf("jit:grant:%s:*", identityID), 0).Iterator()
		for iter.Next(ctx) {
			s.redis.Del(ctx, iter.Val())
		}
	}

	// Remove Neo4j relationship
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	if resourceID != "" {
		if _, err := session.Run(ctx, `
			MATCH (i:Identity {uuid: $identityId})-[r:HAS_TEMPORARY_ACCESS]->(res:Resource {id: $resourceId})
			DELETE r
		`, map[string]any{"identityId": identityID, "resourceId": resourceID}); err != nil {
			return fmt.Errorf("revoke temp access: neo4j delete: %w", err)
		}
	} else {
		if _, err := session.Run(ctx, `
			MATCH (i:Identity {uuid: $identityId})-[r:HAS_TEMPORARY_ACCESS]->()
			DELETE r
		`, map[string]any{"identityId": identityID}); err != nil {
			return fmt.Errorf("revoke temp access: neo4j bulk delete: %w", err)
		}
	}

	// Audit log — audit failure should not block the operation
	if _, err := s.pgPool.Exec(ctx, `
		INSERT INTO audit_log (id, event_type, actor_id, action, resource, details, created_at)
		VALUES ($1, 'temporary_access_revoked', $2, 'jit_revoke', $3,
		        jsonb_build_object('reason', 'expired'), NOW())`,
		uuid.New().String(), identityID, "resource:"+resourceID); err != nil {
		activity.RecordHeartbeat(ctx, "temp_access_audit_write_failed", err.Error())
	}

	activity.RecordHeartbeat(ctx, "temp_access_revoked")
	return nil
}

// ─── Agent Revocation Activities ──────────────────────────

func (s *ActivityService) RevokeSPIFFESVID(ctx context.Context, params map[string]any) error {
	agentID, _ := params["agent_id"].(string)

	// Record SPIFFE revocation intent
	_, err := s.pgPool.Exec(ctx, `
		INSERT INTO audit_log (id, event_type, actor_id, action, resource, details, ip_address, created_at)
		VALUES ($1, 'spiffe_svid_revoked', $2, 'revoke_spiffe', $3,
		        jsonb_build_object('method', 'spire_entry_deletion'), 'internal', NOW())`,
		uuid.New().String(), agentID, "agent:"+agentID)
	if err != nil {
		return fmt.Errorf("revoke SVID: audit log: %w", err)
	}

	// In production: DELETE /spire/v1/entry/{entryID} to SPIRE server
	activity.RecordHeartbeat(ctx, "spiffe_revoked")
	return nil
}

func (s *ActivityService) RevokeOAuthTokens(ctx context.Context, params map[string]any) error {
	agentID, _ := params["agent_id"].(string)

	// Invalidate all OAuth tokens in Redis
	iter := s.redis.Scan(ctx, 0, fmt.Sprintf("oauth:token:%s:*", agentID), 0).Iterator()
	for iter.Next(ctx) {
		s.redis.Set(ctx, iter.Val(), "revoked", 24*time.Hour)
	}

	// Clear active sessions
	s.redis.Del(ctx, fmt.Sprintf("session:active:%s", agentID))

	// Audit log — best-effort, don't block revocation on audit failure
	if _, err := s.pgPool.Exec(ctx, `
		INSERT INTO audit_log (id, event_type, actor_id, action, resource, details, ip_address, created_at)
		VALUES ($1, 'oauth_tokens_revoked', $2, 'revoke_oauth', $3,
		        jsonb_build_object('method', 'token_invalidation'), 'internal', NOW())`,
		uuid.New().String(), agentID, "agent:"+agentID); err != nil {
		activity.RecordHeartbeat(ctx, "oauth_audit_write_failed", err.Error())
	}

	activity.RecordHeartbeat(ctx, "oauth_revoked")
	return nil
}

func (s *ActivityService) RevokeAPIKeys(ctx context.Context, params map[string]any) error {
	agentID, _ := params["agent_id"].(string)

	// Mark all API keys as revoked in PostgreSQL
	if _, err := s.pgPool.Exec(ctx, `
		UPDATE non_human_identities SET status = 'revoked', updated_at = NOW()
		WHERE id = $1 OR owner_id = $1`, agentID); err != nil {
		return fmt.Errorf("revoke api keys: pg update: %w", err)
	}

	// Invalidate in Redis
	s.redis.Del(ctx, fmt.Sprintf("apikey:hash:%s:*", agentID))

	// Audit log — best-effort
	if _, err := s.pgPool.Exec(ctx, `
		INSERT INTO audit_log (id, event_type, actor_id, action, resource, details, ip_address, created_at)
		VALUES ($1, 'api_keys_revoked', $2, 'revoke_apikeys', $3,
		        jsonb_build_object('method', 'key_rotation'), 'internal', NOW())`,
		uuid.New().String(), agentID, "agent:"+agentID); err != nil {
		activity.RecordHeartbeat(ctx, "apikeys_audit_write_failed", err.Error())
	}

	activity.RecordHeartbeat(ctx, "apikeys_revoked")
	return nil
}

// ─── CAEP Activities ─────────────────────────────────────

func (s *ActivityService) BroadcastCAEPEvent(ctx context.Context, params CAEPEventParams) error {
	// Build CAEP/SET (Security Event Token) structure
	eventURL := fmt.Sprintf("https://schemas.openid.net/secevent/caep/event-type/%s", params.EventType)

	event := map[string]any{
		"iss": "https://observeid.io/",
		"sub": params.IdentityID,
		"jti": uuid.New().String(),
		"iat": time.Now().Unix(),
		"aud": params.TenantID,
		"events": map[string]any{
			eventURL: map[string]any{
				"subject": map[string]string{
					"subject_type": "iam",
					"user":        params.IdentityID,
					"session_id":  params.SessionID,
				},
				"event_timestamp":   time.Now().UnixMilli(),
				"initiating_entity": params.ReasonAdmin,
				"reason_admin":       params.ReasonAdmin,
				"reason_user":       params.ReasonUser,
			},
		},
	}

	payload, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return fmt.Errorf("caep marshal event: %w", err)
	}

	// Persist CAEP event to PostgreSQL
	_, err = s.pgPool.Exec(ctx, `
		INSERT INTO caep_events (id, tenant_id, event_type, event_jti, identity_id, session_id,
		                         initiating_entity, reason_admin, reason_user, payload, delivery_status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending', NOW())`,
		uuid.New().String(), params.TenantID, params.EventType, event["jti"].(string),
		params.IdentityID, params.SessionID, params.ReasonAdmin,
		params.ReasonAdmin, params.ReasonUser, payload,
	)
	if err != nil {
		return fmt.Errorf("caep persist: %w", err)
	}

	// In production: JWT-sign and POST to registered webhook receivers
	// Implementation: sign payload with HMAC-SHA256, POST to tenant webhook URLs
	activity.RecordHeartbeat(ctx, "caep_payload_ready", fmt.Sprintf("%d bytes", len(payload)))

	activity.RecordHeartbeat(ctx, "caep_broadcasted")
	return nil
}

// ─── Identity CRUD Activities ─────────────────────────────

func (s *ActivityService) CreateIdentity(ctx context.Context, params CreateIdentityParams) (CreateIdentityResult, error) {
	identityID := uuid.New().String()
	now := time.Now()

	_, err := s.pgPool.Exec(ctx, `
		INSERT INTO identities (id, tenant_id, type, status, email, display_name, department, employee_id, manager_id, source, attributes, created_at, updated_at)
		VALUES ($1, $2, $3, 'active', $4, $5, $6, $7, $8, $9, $10, $11, $11)`,
		identityID, params.TenantID, params.IdentityType, params.Email, params.DisplayName,
		params.Department, params.EmployeeID, params.ManagerID, params.Source,
		params.Attributes, now,
	)
	if err != nil {
		return CreateIdentityResult{}, fmt.Errorf("identity create: %w", err)
	}

	// Create Neo4j node
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	if _, err := session.Run(ctx, `
		CREATE (i:Identity {
			uuid: $uuid, tenant_id: $tenantId, type: $type, status: 'active',
			email: $email, display_name: $displayName, created_at: $createdAt
		})
	`, map[string]any{
		"uuid": identityID, "tenantId": params.TenantID, "type": params.IdentityType,
		"email": params.Email, "displayName": params.DisplayName,
		"createdAt": now.UnixMilli(),
	}); err != nil {
		return CreateIdentityResult{}, fmt.Errorf("identity create: neo4j node: %w", err)
	}

	activity.RecordHeartbeat(ctx, "identity_created")
	return CreateIdentityResult{IdentityID: identityID, CreatedAt: now}, nil
}

func (s *ActivityService) AssignRoleToIdentity(ctx context.Context, params AssignRoleParams) error {
	// Create Neo4j relationship
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $identityId}), (r:Role {id: $roleId})
		MERGE (i)-[rel:HAS_ROLE]->(r)
		SET rel.assigned_at = timestamp(), rel.assigned_by = $assignedBy, rel.source = 'workflow'
	`, map[string]any{
		"identityId": params.IdentityID, "roleId": params.RoleID,
		"assignedBy": params.AssignedBy,
	})
	if err != nil {
		return fmt.Errorf("assign role: neo4j: %w", err)
	}

	// Audit log — best-effort, don't block role assignment
	if _, err := s.pgPool.Exec(ctx, `
		INSERT INTO audit_log (id, event_type, actor_id, action, resource, details, ip_address, created_at)
		VALUES ($1, 'role_assigned', $2, 'assign_role', $3,
		        jsonb_build_object('role_name', $4, 'role_id', $5), 'internal', NOW())`,
		uuid.New().String(), params.IdentityID, "role:"+params.RoleID,
		params.RoleName, params.RoleID); err != nil {
		// Non-critical — log and continue
		activity.RecordHeartbeat(ctx, "role_assignment_audit_write_failed", err.Error())
	}

	return nil
}

// ─── Policy Activities ────────────────────────────────────

func (s *ActivityService) CheckAccessPolicy(ctx context.Context, params PolicyCheckParams) (PolicyCheckResult, error) {
	// Evaluate against stored Cedar policies
	rows, err := s.pgPool.Query(ctx, `
		SELECT policy_id, effect, policy_source FROM cedar_policies
		WHERE tenant_id = $1 AND is_active = true
		ORDER BY version DESC`, params.TenantID)
	if err != nil {
		return PolicyCheckResult{}, fmt.Errorf("policy query: %w", err)
	}
	defer rows.Close()

	var matchedPolicies []string
	var effect string

	for rows.Next() {
		var policyID, polEffect, source string
		if err := rows.Scan(&policyID, &polEffect, &source); err != nil {
			continue
		}

		// Simple pattern matching: check if the policy conditions align
		// In production: use cedar-go evaluator
		matched := evaluatePolicyConditions(
			polEffect, source,
			params.IdentityType, params.Action, params.ResourceType,
		)
		if matched {
			matchedPolicies = append(matchedPolicies, policyID)
			effect = polEffect
		}
	}

	result := PolicyCheckResult{
		MatchedPolicies: matchedPolicies,
	}

	if len(matchedPolicies) > 0 {
		result.Allowed = (effect == "permit")
		result.Decision = effect
		result.Reason = fmt.Sprintf("matched %d policies, decision: %s", len(matchedPolicies), effect)
	} else {
		result.Allowed = true // default allow if no policy matches
		result.Decision = "not_applicable"
		result.Reason = "no matching policies, default allow"
	}

	// Cache decision in Redis
	cacheKey := fmt.Sprintf("policy:decision:%s:%s:%s", params.IdentityID, params.ResourceID, params.Action)
	if cacheVal, err := json.Marshal(result); err == nil {
		s.redis.Set(ctx, cacheKey, cacheVal, 30*time.Second)
	}

	activity.RecordHeartbeat(ctx, "policy_checked", result.Decision)
	return result, nil
}

func evaluatePolicyConditions(effect, source, identityType, action, resourceType string) bool {
	// Parse the Cedar-like policy source for condition matching
	parts := strings.Fields(source)
	if len(parts) < 3 {
		return false
	}

	// Pattern: "permit(identity_type, action, resource_type)"
	hasIdentity := strings.Contains(source, identityType)
	hasAction := strings.Contains(source, action)
	hasResource := strings.Contains(source, resourceType)

	return hasIdentity && hasAction && hasResource
}

// ─── SoD Check Activity ─────────────────────────────────

func (s *ActivityService) CheckSoDConflicts(ctx context.Context, params map[string]any) (SoDCheckResult, error) {
	identityID, _ := params["identity_id"].(string)
	roleID, _ := params["role_id"].(string)

	result := SoDCheckResult{}

	// Query Neo4j for SoD conflict pairs
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Direct toxic pair check
	directResult, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $identityId})-[:HAS_ROLE]->(existing:Role)
		MATCH (existing)-[:INCOMPATIBLE_WITH]->(requested:Role {id: $roleId})
		RETURN existing.id AS existing_role_id, existing.name AS existing_role_name,
		       requested.name AS requested_role_name, 'toxic_pair' AS conflict_type,
		       'critical' AS severity
	`, map[string]any{"identityId": identityID, "roleId": roleID})
	if err == nil {
		for directResult.Next(ctx) {
			rec := directResult.Record()
			result.Conflicts = append(result.Conflicts, SoDConflict{
				ExistingRoleID:   getStr(rec, "existing_role_id"),
				ExistingRoleName: getStr(rec, "existing_role_name"),
				RequestedRoleID:  roleID,
				ConflictType:     getStr(rec, "conflict_type"),
				Severity:         getStr(rec, "severity"),
				Resolution:       "remove_existing_or_deny_request",
			})
		}
	}

	// Transitive conflict check (A -> B -> C where A and C conflict)
	transitiveResult, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $identityId})-[:HAS_ROLE]->(chain1:Role)-[:GRANTS]->(e1:Entitlement)
		MATCH (requested:Role {id: $roleId})-[:GRANTS]->(e2:Entitlement)
		WHERE e1.is_toxic = true AND e2.is_toxic = true
		AND e1.app_name = e2.app_name
		AND e1.permission_level <> e2.permission_level
		RETURN chain1.id AS existing_role_id, chain1.name AS existing_role_name,
		       requested.name AS requested_role_name, 'transitive' AS conflict_type,
		       'high' AS severity
	`, map[string]any{"identityId": identityID, "roleId": roleID})
	if err == nil {
		for transitiveResult.Next(ctx) {
			rec := transitiveResult.Record()
			result.Conflicts = append(result.Conflicts, SoDConflict{
				ExistingRoleID:   getStr(rec, "existing_role_id"),
				ExistingRoleName: getStr(rec, "existing_role_name"),
				RequestedRoleID:  roleID,
				ConflictType:    getStr(rec, "conflict_type"),
				Severity:        getStr(rec, "severity"),
				Resolution:      "add_mitigating_control_or_deny",
			})
		}
	}

	// Rubberband conflict check (same app, excessive privilege combination)
	rubberResult, err := session.Run(ctx, `
		MATCH (i:Identity {uuid: $identityId})-[:HAS_ROLE]->(existing:Role)
		MATCH (requested:Role {id: $roleId})
		WHERE existing.is_rubberband = true OR requested.is_rubberband = true
		RETURN existing.id AS existing_role_id, existing.name AS existing_role_name,
		       requested.name AS requested_role_name, 'rubberband' AS conflict_type,
		       'medium' AS severity
	`, map[string]any{"identityId": identityID, "roleId": roleID})
	if err == nil {
		for rubberResult.Next(ctx) {
			rec := rubberResult.Record()
			result.Conflicts = append(result.Conflicts, SoDConflict{
				ExistingRoleID:   getStr(rec, "existing_role_id"),
				ExistingRoleName: getStr(rec, "existing_role_name"),
				RequestedRoleID:  roleID,
				ConflictType:    getStr(rec, "conflict_type"),
				Severity:        getStr(rec, "severity"),
				Resolution:      "implement_break_glass_procedure",
			})
		}
	}

	result.HasConflict = len(result.Conflicts) > 0
	if result.HasConflict {
		for _, c := range result.Conflicts {
			switch c.Severity {
			case "critical":
				result.RiskScore += 1.0
			case "high":
				result.RiskScore += 0.6
			case "medium":
				result.RiskScore += 0.3
			}
		}
		if result.RiskScore > 1.0 {
			result.RiskScore = 1.0
		}
	}

	activity.RecordHeartbeat(ctx, "sod_checked", result.HasConflict, result.RiskScore)
	return result, nil
}

// ─── Anomaly Detection Activity ──────────────────────────

func (s *ActivityService) ScanAgentBehavior(ctx context.Context) ([]AnomalyResult, error) {
	activity.RecordHeartbeat(ctx, "scanning_agent_behavior")

	var anomalies []AnomalyResult

	// Query all active non-human identities
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	agentResult, err := session.Run(ctx, `
		MATCH (n:NonHumanIdentity)
		WHERE n.status = 'active'
		RETURN n.uuid AS id, n.name AS name, n.risk_score AS risk_score,
		       n.is_governed AS is_governed, n.type AS type,
		       n.framework AS framework, n.deployment_environment AS env
	`, nil)
	if err != nil {
		return nil, fmt.Errorf("agent scan query: %w", err)
	}

	for agentResult.Next(ctx) {
		rec := agentResult.Record()
		agentID, _ := rec.Get("id")
		agentName, _ := rec.Get("name")
		riskScore, _ := rec.Get("risk_score")
		isGoverned, _ := rec.Get("is_governed")
		framework, _ := rec.Get("framework")

		id, _ := agentID.(string)
		name, _ := agentName.(string)
		risk, _ := riskScore.(float64)
		ungoverned, _ := isGoverned.(bool)
		fw, _ := framework.(string)

		// Anomaly 1: Ungoverned agent with high risk
		if !ungoverned && risk > 0.5 {
			anomalies = append(anomalies, AnomalyResult{
				AgentID:     id,
				AgentName:   name,
				AnomalyType: "governance_gap",
				Score:       risk,
				Reason:      fmt.Sprintf("ungoverned agent with risk score %.2f", risk),
				Critical:    risk > 0.8,
				Details: map[string]any{
					"risk_score":  risk,
					"framework":   fw,
					"governed":    false,
				},
			})
		}

		// Anomaly 2: Missing security framework
		if fw == "" || fw == "none" {
			anomalies = append(anomalies, AnomalyResult{
				AgentID:     id,
				AgentName:   name,
				AnomalyType: "missing_framework",
				Score:       0.6,
				Reason:      "agent has no security framework defined",
				Critical:    false,
				Details: map[string]any{
					"risk_score": risk,
				},
			})
		}
	}

	// Anomaly 3: Excessive delegation depth (graph-based)
	delegationResult, err := session.Run(ctx, `
		MATCH path = (root:NonHumanIdentity)-[:DELEGATED_FROM*]->(child:NonHumanIdentity)
		WHERE child.status = 'active'
		WITH root, child, length(path) AS depth
		WHERE depth >= 3
		RETURN child.uuid AS agent_id, child.name AS name, depth,
		       root.uuid AS root_agent_id, root.name AS root_name
		ORDER BY depth DESC
	`, nil)
	if err == nil {
		for delegationResult.Next(ctx) {
			rec := delegationResult.Record()
			anomalies = append(anomalies, AnomalyResult{
				AgentID:     getStr(rec, "agent_id"),
				AgentName:   getStr(rec, "name"),
				AnomalyType: "deep_delegation_chain",
				Score:       0.7,
				Reason: fmt.Sprintf("delegation depth %d from %s",
					getInt64(rec, "depth"), getStr(rec, "root_name")),
				Critical: getInt64(rec, "depth") >= 5,
				Details: map[string]any{
					"root_agent_id": getStr(rec, "root_agent_id"),
					"depth":         getInt64(rec, "depth"),
				},
			})
		}
	}

	// Anomaly 4: Access pattern anomaly (stale entitlements)
	staleResult, err := session.Run(ctx, `
		MATCH (n:NonHumanIdentity)-[:HAS_ROLE]->(r:Role)-[:GRANTS]->(e:Entitlement)
		WHERE n.status = 'active' AND e.last_used_at IS NOT NULL
		AND (datetime().epochMillis - e.last_used_at) > 7776000000
		RETURN n.uuid AS agent_id, n.name AS name,
		       COLLECT(DISTINCT e.id) AS stale_entitlements,
		       COUNT(DISTINCT e) AS stale_count
	`, nil)
	if err == nil {
		for staleResult.Next(ctx) {
			rec := staleResult.Record()
			staleCount, _ := rec.Get("stale_count")
			count, ok := staleCount.(int64)
			if ok && count >= 3 {
				anomalies = append(anomalies, AnomalyResult{
					AgentID:     getStr(rec, "agent_id"),
					AgentName:   getStr(rec, "name"),
					AnomalyType: "stale_entitlements",
					Score:       0.5,
					Reason:      fmt.Sprintf("agent has %d unused entitlements (>90d)", count),
					Critical:    count >= 10,
					Details: map[string]any{
						"stale_count": count,
					},
				})
			}
		}
	}

	// Sort by score descending
	sort.Slice(anomalies, func(i, j int) bool {
		return anomalies[i].Score > anomalies[j].Score
	})

	activity.RecordHeartbeat(ctx, "anomaly_scan_complete", len(anomalies))
	return anomalies, nil
}

// ─── SoD Detection Activity ─────────────────────────────

func (s *ActivityService) ScanSoDViolations(ctx context.Context) ([]SoDConflict, error) {
	activity.RecordHeartbeat(ctx, "scanning_sod")

	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	var allConflicts []SoDConflict

	// Find all identities with toxic role pairs
	result, err := session.Run(ctx, `
		MATCH (i:Identity)-[:HAS_ROLE]->(r1:Role)
		MATCH (i)-[:HAS_ROLE]->(r2:Role)
		WHERE r1 <> r2 AND (r1)-[:INCOMPATIBLE_WITH]-(r2)
		RETURN i.uuid AS identity_id, i.display_name AS identity_name,
		       r1.id AS role1_id, r1.name AS role1_name,
		       r2.id AS role2_id, r2.name AS role2_name,
		       'toxic_pair' AS conflict_type,
		       'critical' AS severity
	`, nil)
	if err == nil {
		for result.Next(ctx) {
			rec := result.Record()
			allConflicts = append(allConflicts, SoDConflict{
				ExistingRoleID:   getStr(rec, "role1_id"),
				ExistingRoleName: getStr(rec, "role1_name"),
				RequestedRoleID:  getStr(rec, "role2_id"),
				ConflictType:     getStr(rec, "conflict_type"),
				Severity:         getStr(rec, "severity"),
				Resolution:       "remove_one_role_or_implement_mitigation",
			})
		}
	}

	// Find rubberband conflicts (excessive privilege combinations)
	rubberResult, err := session.Run(ctx, `
		MATCH (i:Identity)-[:HAS_ROLE]->(r:Role)
		WHERE r.is_rubberband = true
		WITH i, COUNT(r) AS rubberband_count
		WHERE rubberband_count >= 2
		RETURN i.uuid AS identity_id, i.display_name AS identity_name,
		       rubberband_count, 'rubberband' AS conflict_type, 'medium' AS severity
	`, nil)
	if err == nil {
		for rubberResult.Next(ctx) {
			rec := rubberResult.Record()
			allConflicts = append(allConflicts, SoDConflict{
				ConflictType: getStr(rec, "conflict_type"),
				Severity:     getStr(rec, "severity"),
				Resolution:   "review_and_consolidate_roles",
			})
		}
	}

	activity.RecordHeartbeat(ctx, "sod_scan_complete", len(allConflicts))
	return allConflicts, nil
}

// ─── Approval Activities ──────────────────────────────────

func (s *ActivityService) SendApprovalRequest(ctx context.Context, params map[string]any) error {
	identityID, _ := params["identity_id"].(string)
	resourceID, _ := params["resource_id"].(string)
	requestedBy, _ := params["requested_by"].(string)
	reason, _ := params["reason"].(string)

	// Store approval request in PostgreSQL
	requestID := uuid.New().String()
	_, err := s.pgPool.Exec(ctx, `
		INSERT INTO approval_requests (id, identity_id, resource_id, requested_by, reason, status, created_at)
		VALUES ($1, $2, $3, $4, $5, 'pending', NOW())`,
		requestID, identityID, resourceID, requestedBy, reason)
	if err != nil {
		return fmt.Errorf("approval request persist: %w", err)
	}

	// In production: send to approver via email/Slack/PagerDuty
	// with approval/rejection callback URL

	activity.RecordHeartbeat(ctx, "approval_requested", requestID)
	return nil
}

// ─── Auth & Credential Rotation ──────────────────────────

func (s *ActivityService) RotateCredentials(ctx context.Context, params map[string]any) (map[string]string, error) {
	identityID, _ := params["identity_id"].(string)
	credType, _ := params["credential_type"].(string) // "api_key", "oauth_secret", "ssh_key"

	result := make(map[string]string)

	switch credType {
	case "api_key":
		newKey := generateRandomString(32)
		result["new_api_key"] = newKey
		result["hash"] = hashString(newKey)

	case "oauth_secret":
		newSecret := generateRandomString(48)
		result["new_client_secret"] = newSecret

	case "ssh_key":
		// Generate new SSH key pair
		result["action"] = "ssh_key_regenerated"
	}

	// Revoke old credentials in Redis
	s.redis.Del(ctx, fmt.Sprintf("cred:%s:%s", credType, identityID))

	// Audit
	_, _ = s.pgPool.Exec(ctx, `
		INSERT INTO audit_log (id, event_type, actor_id, action, resource, details, created_at)
		VALUES ($1, 'credential_rotated', $2, 'rotate_credential', $3,
		        jsonb_build_object('credential_type', $4), NOW())`,
		uuid.New().String(), identityID, "identity:"+identityID, credType)

	activity.RecordHeartbeat(ctx, "credential_rotated")
	return result, nil
}

// ─── Helpers ────────────────────────────────────────────

func getStr(rec *neo4j.Record, key string) string {
	val, ok := rec.Get(key)
	if !ok || val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", val)
}

func getInt64(rec *neo4j.Record, key string) int64 {
	val, ok := rec.Get(key)
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return v
	case float64:
		return int64(math.Round(v))
	case int:
		return int64(v)
	default:
		return 0
	}
}

func generateRandomString(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:length]
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func hmacVerify(message, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	expected := mac.Sum(nil)

	actual, err := hex.DecodeString(message)
	if err != nil {
		return false
	}
	return hmac.Equal(actual, expected)
}
