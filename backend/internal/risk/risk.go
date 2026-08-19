package risk

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// RiskFactors describes the qualitative inputs used to produce a score.
type RiskFactors struct {
	S1Score           float64 `json:"s1_score"`
	S2Score           float64 `json:"s2_score"`
	EntitlementCount  int     `json:"entitlement_count"`
	ResourceCount     int     `json:"resource_count"`
	MaxDepth          int     `json:"max_depth"`
	StandingPrivilege int     `json:"standing_privilege_count"`
	JITPrivilege      int     `json:"jit_privilege_count"`
}

// entitlementRiskScore maps PG/Neo4j risk_classification values to a 0-100 scale.
func entitlementRiskScore(class string) float64 {
	switch strings.ToLower(class) {
	case "critical":
		return 100
	case "high":
		return 70
	case "medium":
		return 40
	case "low":
		return 10
	default:
		return 20
	}
}

// resourceCriticalityScore maps resource criticality to a 0-100 scale.
// Used for direct access edges that do not pass through an entitlement node.
func resourceCriticalityScore(criticality string) float64 {
	switch strings.ToLower(criticality) {
	case "p0", "critical":
		return 100
	case "p1", "high":
		return 70
	case "p2", "medium":
		return 40
	case "p3", "low":
		return 10
	default:
		return 20
	}
}

// CalculateIdentityRisk computes a dynamic 0-100 risk score for an identity using
// existing graph and relational data.
//
// Formula (v1):
//   S1 = average risk score of assigned entitlements (Low=10, Med=40, High=70, Critical=100)
//   S2 = blast-radius expansion: f(resource_count, max_depth, standing privilege count)
//        S2 = (resource_count * 5) + (max_depth * 10) + (standing_privilege_count * 3)
//   Risk = min(100, S1*0.5 + S2*0.5)
//
// Returns the score, a list of human-readable risk factors, and any error.
func CalculateIdentityRisk(
	ctx context.Context,
	neo4jDriver neo4j.DriverWithContext,
	pgPool *pgxpool.Pool,
	tenantID string,
	identityID string,
) (float64, []string, error) {
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	session := neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (i {uuid: $identityId}) WHERE i:Identity OR i:NonHumanIdentity
		OPTIONAL MATCH pathRole = (i)-[:HAS_ROLE]->(:Role)-[:GRANTS]->(e:Entitlement)-[:ACCESSES]->(r:Resource)
		OPTIONAL MATCH pathDirectEnt = (i)-[:HAS_ENTITLEMENT]->(e2:Entitlement)-[:ACCESSES]->(r2:Resource)
		OPTIONAL MATCH pathDirect = (i)-[:HAS_DIRECT_ACCESS]->(r3:Resource)
		OPTIONAL MATCH pathTemp = (i)-[:HAS_TEMPORARY_ACCESS]->(r4:Resource)
		WITH i,
		     COLLECT(DISTINCT CASE WHEN pathRole IS NOT NULL THEN {
				ent_id: e.id, ent_risk: e.risk_classification,
				res_id: r.uuid, res_crit: r.criticality,
				depth: length(pathRole), source: 'role'
			} END) AS rolePaths,
		     COLLECT(DISTINCT CASE WHEN pathDirectEnt IS NOT NULL THEN {
				ent_id: e2.id, ent_risk: e2.risk_classification,
				res_id: r2.uuid, res_crit: r2.criticality,
				depth: length(pathDirectEnt), source: 'direct_entitlement'
			} END) AS directEntPaths,
		     COLLECT(DISTINCT CASE WHEN pathDirect IS NOT NULL THEN {
				ent_id: null, ent_risk: null,
				res_id: r3.uuid, res_crit: r3.criticality,
				depth: length(pathDirect), source: 'direct_access'
			} END) AS directPaths,
		     COLLECT(DISTINCT CASE WHEN pathTemp IS NOT NULL THEN {
				ent_id: null, ent_risk: null,
				res_id: r4.uuid, res_crit: r4.criticality,
				depth: length(pathTemp), source: 'temporary_access'
			} END) AS tempPaths
		RETURN [p IN rolePaths + directEntPaths + directPaths + tempPaths WHERE p IS NOT NULL] AS paths
	`

	result, err := session.Run(ctx, query, map[string]any{
		"identityId": identityID,
		"tenantId":   tenantID,
	})
	if err != nil {
		return 0, nil, fmt.Errorf("risk: neo4j query: %w", err)
	}

	if !result.Next(ctx) {
		return 0, []string{"no_access_paths"}, nil
	}

	pathsRaw, _ := result.Record().Get("paths")
	paths, _ := pathsRaw.([]any)

	// Aggregate.
	resourceIDs := map[string]bool{}
	var maxDepth int64
	standingResources := map[string]bool{}
	jitResources := map[string]bool{}
	var entitlementScores []float64

	for _, pRaw := range paths {
		p, ok := pRaw.(map[string]any)
		if !ok {
			continue
		}
		resID, _ := p["res_id"].(string)
		if resID == "" {
			continue
		}
		resourceIDs[resID] = true

		depth := toInt64(p["depth"])
		if depth > maxDepth {
			maxDepth = depth
		}

		source, _ := p["source"].(string)
		switch source {
		case "role", "direct_entitlement", "direct_access":
			standingResources[resID] = true
		case "temporary_access":
			jitResources[resID] = true
		}

		if entRisk, ok := p["ent_risk"].(string); ok && entRisk != "" {
			entitlementScores = append(entitlementScores, entitlementRiskScore(entRisk))
		} else {
			// No entitlement in path (direct resource access) — use resource criticality.
			resCrit, _ := p["res_crit"].(string)
			entitlementScores = append(entitlementScores, resourceCriticalityScore(resCrit))
		}
	}

	resourceCount := len(resourceIDs)
	standingPriv := len(standingResources)
	jitPriv := len(jitResources)

	avgEntitlementRisk := 0.0
	if len(entitlementScores) > 0 {
		var sum float64
		for _, v := range entitlementScores {
			sum += v
		}
		avgEntitlementRisk = sum / float64(len(entitlementScores))
	}

	risk, factors := ComputeRiskScoreV1(avgEntitlementRisk, resourceCount, int(maxDepth), standingPriv, jitPriv)

	// Persist the score to PostgreSQL and Neo4j for API reads.
	if err := persistRiskScore(ctx, neo4jDriver, pgPool, tenantID, identityID, risk, factors); err != nil {
		// Persistence failure should not break the calculation; callers may log.
		_ = err
	}

	return risk, factors, nil
}

// ComputeRiskScoreV1 implements the v1 risk formula using pre-aggregated inputs.
// It is exposed for unit testing and for callers that already have the raw counts.
func ComputeRiskScoreV1(avgEntitlementRisk float64, resourceCount, maxDepth, standingPriv, jitPriv int) (float64, []string) {
	s1 := avgEntitlementRisk
	s2 := float64(resourceCount)*5.0 + float64(maxDepth)*10.0 + float64(standingPriv)*3.0

	risk := math.Min(100.0, s1*0.5+s2*0.5)

	factors := []string{}
	if avgEntitlementRisk > 0 {
		factors = append(factors, fmt.Sprintf("avg_entitlement_risk=%.0f", s1))
	}
	if resourceCount > 0 {
		factors = append(factors, fmt.Sprintf("reachable_resources=%d", resourceCount))
	}
	if maxDepth > 0 {
		factors = append(factors, fmt.Sprintf("max_path_depth=%d", maxDepth))
	}
	if standingPriv > 0 {
		factors = append(factors, fmt.Sprintf("standing_privileges=%d", standingPriv))
	}
	if jitPriv > 0 {
		factors = append(factors, fmt.Sprintf("jit_privileges=%d", jitPriv))
	}
	if len(factors) == 0 {
		factors = []string{"no_access_paths"}
	}

	return risk, factors
}

func persistRiskScore(
	ctx context.Context,
	neo4jDriver neo4j.DriverWithContext,
	pgPool *pgxpool.Pool,
	tenantID string,
	identityID string,
	score float64,
	factors []string,
) error {
	if pgPool != nil {
		tx, err := pgPool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("persist identity risk begin: %w", err)
		}
		defer tx.Rollback(ctx)

		if tenantID != "" {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.current_tenant', $1, true)`, tenantID); err != nil {
				return fmt.Errorf("persist identity risk set tenant: %w", err)
			}
		}

		_, err = tx.Exec(ctx, `
			UPDATE identities
			SET risk_score = $1, risk_factors = $2, updated_at = NOW()
			WHERE id = $3 AND tenant_id = $4
		`, score, factors, identityID, tenantID)
		if err != nil {
			return fmt.Errorf("persist identity risk: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("persist identity risk commit: %w", err)
		}
	}

	if neo4jDriver != nil {
		session := neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)
		_, err := session.Run(ctx, `
			MATCH (i {uuid: $identityId}) WHERE i:Identity OR i:NonHumanIdentity
			SET i.risk_score = $score, i.risk_factors = $factors, i.updated_at = datetime()
		`, map[string]any{
			"identityId": identityID,
			"score":      score,
			"factors":    factors,
		})
		if err != nil {
			return fmt.Errorf("persist neo4j risk: %w", err)
		}
	}

	return nil
}

func toInt64(v any) int64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}
