package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/activity"
)

// ─── Activity Service ──────────────────────────────────────

type ActivityService struct {
	pgPool    interface{}  // Will use pgx in production
	neo4j     neo4j.DriverWithContext
	redis     *redis.Client
	temporal  client.Client
}

func NewActivityService(pgPool interface{}, neo4j neo4j.DriverWithContext, rdb *redis.Client, tc client.Client) *ActivityService {
	return &ActivityService{
		pgPool:   pgPool,
		neo4j:    neo4j,
		redis:    rdb,
		temporal: tc,
	}
}

// ─── Audit Activities ──────────────────────────────────────

func (s *ActivityService) InitiateAuditTrail(ctx context.Context, params map[string]any) (string, error) {
	auditID := uuid.New().String()
	log.Printf("[AUDIT] %s: %s on %s by %s", auditID, params["operation"], params["identity_id"], params["requested_by"])
	return auditID, nil
}

func (s *ActivityService) FinalizeAuditTrail(ctx context.Context, params map[string]any) error {
	log.Printf("[AUDIT] %s: finalized with status %s", params["audit_id"], params["status"])

	// In production, write to Amazon QLDB here:
	// qldbSession.Execute("INSERT INTO audit_log VALUES ?", params)
	return nil
}

// ─── Lock Activities ───────────────────────────────────────

func (s *ActivityService) AcquireIdentityLock(ctx context.Context, params map[string]any) (string, error) {
	identityID := params["identity_id"].(string)
	ttl := params["ttl_seconds"].(int)
	token := uuid.New().String()

	ok, err := s.redis.SetNX(ctx, fmt.Sprintf("lock:identity:%s", identityID), token, time.Duration(ttl)*time.Second).Result()
	if err != nil {
		return "", fmt.Errorf("redis lock error: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("identity %s is already locked", identityID)
	}

	log.Printf("[LOCK] Acquired lock for identity %s (token: %s)", identityID, token)
	return token, nil
}

func (s *ActivityService) ReleaseIdentityLock(ctx context.Context, params map[string]any) error {
	identityID := params["identity_id"].(string)
	token := params["token"].(string)

	storedToken, err := s.redis.Get(ctx, fmt.Sprintf("lock:identity:%s", identityID)).Result()
	if err != nil {
		return fmt.Errorf("lock not found: %w", err)
	}
	if storedToken != token {
		return fmt.Errorf("lock token mismatch: got %s, expected %s", token, storedToken)
	}

	s.redis.Del(ctx, fmt.Sprintf("lock:identity:%s", identityID))
	log.Printf("[LOCK] Released lock for identity %s", identityID)
	return nil
}

// ─── Graph (Neo4j) Activities ──────────────────────────────

func (s *ActivityService) QueryIdentityEntitlements(ctx context.Context, params map[string]any) ([]map[string]any, error) {
	identityID := params["identity_id"].(string)

	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (i:Identity {uuid: $identityId})
		OPTIONAL MATCH (i)-[:HAS_ROLE]->(r:Role)-[:GRANTS]->(e:Entitlement)-[:ACCESSES]->(res:Resource)
		OPTIONAL MATCH (i)-[:DIRECTLY_OWNS]->(e2:Entitlement)-[:ACCESSES]->(res2:Resource)
		RETURN COLLECT(DISTINCT {
			entitlement: e,
			role: r,
			resource: res,
			source: 'role_inheritance'
		}) + COLLECT(DISTINCT {
			entitlement: e2,
			resource: res2,
			source: 'direct'
		}) AS entitlements
	`

	result, err := session.Run(ctx, query, map[string]any{"identityId": identityID})
	if err != nil {
		return nil, fmt.Errorf("neo4j query error: %w", err)
	}

	var entitlements []map[string]any
	if result.Next(ctx) {
		val, _ := result.Record().Get("entitlements")
		entitlements, _ = val.([]map[string]any)
	}

	return entitlements, nil
}

func (s *ActivityService) FindDelegatedAgents(ctx context.Context, params map[string]any) ([]string, error) {
	identityID := params["identity_id"].(string)

	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (n:NonHumanIdentity {uuid: $identityId})-[:DELEGATED_FROM*1..3]->(child:NonHumanIdentity)
		WHERE child.status = 'active'
		RETURN COLLECT(DISTINCT child.uuid) AS agent_ids
	`

	result, err := session.Run(ctx, query, map[string]any{"identityId": identityID})
	if err != nil {
		return nil, fmt.Errorf("delegation query error: %w", err)
	}

	var agentIDs []string
	if result.Next(ctx) {
		val, _ := result.Record().Get("agent_ids")
		agentIDs, _ = val.([]string)
	}

	return agentIDs, nil
}

// ─── Provisioning Activities ──────────────────────────────

func (s *ActivityService) GrantOktaAccess(ctx context.Context, params map[string]any) error {
	log.Printf("[PROVISION] Granting Okta access: identity=%s, resource=%s",
		params["identity_id"], params["resource_id"])
	// TODO: Okta API call
	// Simulate work
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	return nil
}

func (s *ActivityService) RevokeAWSAccess(ctx context.Context, params map[string]any) error {
	log.Printf("[PROVISION] Revoking AWS access: identity=%s",
		params["identity_id"])
	// TODO: AWS IAM API call (DetachUserPolicy, etc.)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(300 * time.Millisecond):
	}
	return nil
}

func (s *ActivityService) RevokeTargetAccess(ctx context.Context, params map[string]any) error {
	entitlementID := params["entitlement_id"].(string)
	log.Printf("[PROVISION] Revoking target access: entitlement=%s", entitlementID)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	return nil
}

func (s *ActivityService) ProvisionAccess(ctx context.Context, params map[string]any) error {
	log.Printf("[PROVISION] Provisioning access: identity=%s, resource=%s",
		params["identity_id"], params["resource_id"])
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(300 * time.Millisecond):
	}
	return nil
}

func (s *ActivityService) ProvisionTemporaryAccess(ctx context.Context, params map[string]any) error {
	log.Printf("[PROVISION] Granting temporary access: identity=%s, duration=%d min",
		params["identity_id"], params["duration_minutes"])
	return nil
}

func (s *ActivityService) RevokeTemporaryAccess(ctx context.Context, params map[string]any) error {
	log.Printf("[PROVISION] Revoking temporary access: identity=%s",
		params["identity_id"])
	return nil
}

func (s *ActivityService) RevokeIdentityAccess(ctx context.Context, params map[string]any) error {
	log.Printf("[PROVISION] Emergency revoke: identity=%s, emergency=%v",
		params["identity_id"], params["is_emergency"])
	return nil
}

// ─── Agent Revocation Activities ──────────────────────────

func (s *ActivityService) RevokeSPIFFESVID(ctx context.Context, params map[string]any) error {
	log.Printf("[KILL] Revoking SPIFFE SVID for agent: %s", params["agent_id"])
	return nil
}

func (s *ActivityService) RevokeOAuthTokens(ctx context.Context, params map[string]any) error {
	log.Printf("[KILL] Revoking OAuth tokens for agent: %s", params["agent_id"])
	return nil
}

func (s *ActivityService) RevokeAPIKeys(ctx context.Context, params map[string]any) error {
	log.Printf("[KILL] Revoking API keys for agent: %s", params["agent_id"])
	return nil
}

// ─── CAEP Activities ─────────────────────────────────────

func (s *ActivityService) BroadcastCAEPEvent(ctx context.Context, params map[string]any) error {
	eventType := params["event_type"].(string)
	identityID := params["identity_id"].(string)

	event := map[string]any{
		"iss": "https://observeid.io/",
		"jti": uuid.New().String(),
		"iat": time.Now().Unix(),
		"events": map[string]any{
			fmt.Sprintf("https://schemas.openid.net/secevent/caep/event-type/%s", eventType): map[string]any{
				"subject": map[string]string{
					"user": identityID,
				},
				"event_timestamp":  time.Now().UnixMilli(),
				"initiating_entity": params["reason_admin"],
			},
		},
	}

	payload, _ := json.Marshal(event)
	log.Printf("[CAEP] Broadcasting %s event for %s", eventType, identityID)
	_ = payload

	// In production: POST to each downstream receiver's webhook endpoint
	// with JWT-signed SET (Security Event Token)
	return nil
}

// ─── Identity CRUD Activities ─────────────────────────────

func (s *ActivityService) CreateIdentity(ctx context.Context, params map[string]any) (string, error) {
	identityID := uuid.New().String()
	log.Printf("[IDENTITY] Created %s: %s (%s)", params["identity_type"], params["email"], identityID)
	return identityID, nil
}

func (s *ActivityService) AssignRoleToIdentity(ctx context.Context, params map[string]any) error {
	log.Printf("[IDENTITY] Assigned role %s to %s", params["role_name"], params["identity_id"])
	return nil
}

// ─── Policy Activities ────────────────────────────────────

func (s *ActivityService) CheckAccessPolicy(ctx context.Context, params map[string]any) (bool, error) {
	log.Printf("[POLICY] Checking access: identity=%s, resource=%s, action=%s",
		params["identity_id"], params["resource_id"], params["action"])
	// In production: call embedded Cedar PDP
	return true, nil
}

// ─── Anomaly & SoD Activities ─────────────────────────────

func (s *ActivityService) ScanAgentBehavior(ctx context.Context) ([]map[string]any, error) {
	activity.RecordHeartbeat(ctx, "scanning")
	log.Printf("[ANALYSIS] Scanning agent behavior for anomalies")
	return nil, nil
}

func (s *ActivityService) ScanSoDViolations(ctx context.Context) ([]map[string]any, error) {
	activity.RecordHeartbeat(ctx, "scanning")
	log.Printf("[ANALYSIS] Scanning for SoD violations")
	return nil, nil
}

// ─── Approval Activities ──────────────────────────────────

func (s *ActivityService) SendApprovalRequest(ctx context.Context, params map[string]any) error {
	log.Printf("[APPROVAL] Request sent: identity=%s requested access to %s",
		params["identity_id"], params["resource_id"])
	return nil
}
