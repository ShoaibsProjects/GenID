package risk

import (
	"context"
	"fmt"
	"math"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type StaticRisk struct {
	neo4jDriver neo4j.DriverWithContext
}

func NewStaticRisk(neo4jDriver neo4j.DriverWithContext) *StaticRisk {
	return &StaticRisk{neo4jDriver: neo4jDriver}
}

func (s *StaticRisk) Calculate(ctx context.Context, identityID string) (float64, []string, error) {
	session := s.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (i:Identity {uuid: $identityId})
		OPTIONAL MATCH (i)-[:HAS_ROLE]->(r:Role)
		OPTIONAL MATCH (r)-[:GRANTS]->(e:Entitlement)-[:ACCESSES]->(res:Resource)
		OPTIONAL MATCH (i)-[:HAS_ENTITLEMENT]->(e2:Entitlement)-[:ACCESSES]->(res2:Resource)
		OPTIONAL MATCH (i)-[:HAS_DIRECT_ACCESS]->(res3:Resource)
		WITH i,
		     collect(DISTINCT r) AS roles,
		     collect(DISTINCT e) + collect(DISTINCT e2) AS entitlements,
		     collect(DISTINCT res) + collect(DISTINCT res2) + collect(DISTINCT res3) AS resources
		WITH i, roles, entitlements, resources,
		     [e IN entitlements | CASE coalesce(e.risk_classification, 'medium')
		       WHEN 'critical' THEN 1000
		       WHEN 'high' THEN 700
		       WHEN 'medium' THEN 400
		       WHEN 'low' THEN 100
		       ELSE 200 END] AS entRiskScores,
		     [res IN resources | CASE coalesce(res.criticality, 'medium')
		       WHEN 'p0' THEN 1000
		       WHEN 'critical' THEN 1000
		       WHEN 'p1' THEN 700
		       WHEN 'high' THEN 700
		       WHEN 'p2' THEN 400
		       WHEN 'medium' THEN 400
		       WHEN 'p3' THEN 100
		       WHEN 'low' THEN 100
		       ELSE 200 END] AS resRiskScores
		WITH i, roles, entitlements, resources, entRiskScores, resRiskScores,
		     CASE WHEN size(entRiskScores) > 0
		          THEN reduce(s = 0.0, sc IN entRiskScores | s + sc) / size(entRiskScores)
		          ELSE 0 END AS avgEntRisk,
		     CASE WHEN size(resRiskScores) > 0
		          THEN reduce(s = 0.0, sc IN resRiskScores | s + sc) / size(resRiskScores)
		          ELSE 0 END AS avgResRisk,
		     size(roles) AS roleCount,
		     size(entitlements) AS entCount,
		     size(resources) AS resCount
		RETURN roleCount, entCount, resCount, avgEntRisk, avgResRisk
	`

	result, err := session.Run(ctx, query, map[string]any{"identityId": identityID})
	if err != nil {
		return 0, nil, fmt.Errorf("static risk query: %w", err)
	}

	if !result.Next(ctx) {
		return 0, []string{"no_access_data"}, nil
	}

	rec := result.Record()
	roleCount, _ := rec.Get("roleCount")
	entCount, _ := rec.Get("entCount")
	resCount, _ := rec.Get("resCount")
	avgEntRisk, _ := rec.Get("avgEntRisk")
	avgResRisk, _ := rec.Get("avgResRisk")

	rc := toFloat64(roleCount)
	ec := toFloat64(entCount)
	rc2 := toFloat64(resCount)
	aer := toFloat64(avgEntRisk)
	arr := toFloat64(avgResRisk)

	var factors []string
	var score float64

	entScore := math.Min(400, aer*0.4)
	resScore := math.Min(300, arr*0.3)
	volumeScore := math.Min(300, (rc*30 + ec*20 + rc2*10))

	score = entScore + resScore + volumeScore

	if aer > 600 {
		factors = append(factors, "high_risk_entitlements")
	}
	if arr > 600 {
		factors = append(factors, "critical_resource_access")
	}
	if ec > 10 {
		factors = append(factors, fmt.Sprintf("excessive_entitlements=%d", int(ec)))
	}
	if rc > 5 {
		factors = append(factors, fmt.Sprintf("excessive_roles=%d", int(rc)))
	}
	if rc2 > 15 {
		factors = append(factors, fmt.Sprintf("excessive_resources=%d", int(rc2)))
	}

	if len(factors) == 0 {
		factors = []string{"standard_access_profile"}
	}

	return math.Min(1000, score), factors, nil
}
