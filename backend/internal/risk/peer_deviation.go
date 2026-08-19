package risk

import (
	"context"
	"fmt"
	"math"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type PeerDeviation struct {
	neo4jDriver neo4j.DriverWithContext
}

func NewPeerDeviation(neo4jDriver neo4j.DriverWithContext) *PeerDeviation {
	return &PeerDeviation{neo4jDriver: neo4jDriver}
}

func (p *PeerDeviation) Calculate(ctx context.Context, identityID string) (float64, []string, error) {
	session := p.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (i:Identity {uuid: $identityId})
		WITH i, coalesce(i.department, '') AS dept
		OPTIONAL MATCH (peer:Identity)
		WHERE peer.department = dept AND peer.uuid <> i.uuid AND peer:Identity
		WITH i, dept, collect(peer) AS peers
		WITH i, dept, size(peers) AS peerCount,
		     [p IN peers | size([(p)-[:HAS_ROLE]->(r) | r])] AS peerRoleCounts,
		     [p IN peers | size([(p)-[:HAS_ENTITLEMENT]->(e) | e])] AS peerEntCounts,
		     size([(i)-[:HAS_ROLE]->(r) | r]) AS myRoles,
		     size([(i)-[:HAS_ENTITLEMENT]->(e) | e]) AS myEnts,
		     size([(i)-[:HAS_DIRECT_ACCESS]->(res) | res]) AS myDirectAccess
		WITH i, dept, peerCount, myRoles, myEnts, myDirectAccess,
		     CASE WHEN peerCount > 1
		          THEN reduce(s = 0.0, c IN peerRoleCounts | s + c) / peerCount
		          ELSE 0 END AS avgPeerRoles,
		     CASE WHEN peerCount > 1
		          THEN reduce(s = 0.0, c IN peerEntCounts | s + c) / peerCount
		          ELSE 0 END AS avgPeerEnts
		WITH i, dept, peerCount, myRoles, myEnts, myDirectAccess,
		     avgPeerRoles, avgPeerEnts,
		     abs(myRoles - avgPeerRoles) AS roleDeviation,
		     abs(myEnts - avgPeerEnts) AS entDeviation
		RETURN dept, peerCount, myRoles, myEnts, myDirectAccess,
		       avgPeerRoles, avgPeerEnts, roleDeviation, entDeviation
	`

	result, err := session.Run(ctx, query, map[string]any{"identityId": identityID})
	if err != nil {
		return 0, nil, fmt.Errorf("peer deviation query: %w", err)
	}

	if !result.Next(ctx) {
		return 0, []string{"no_peers_found"}, nil
	}

	rec := result.Record()
	peerCount, _ := rec.Get("peerCount")
	myDirect, _ := rec.Get("myDirectAccess")
	roleDev, _ := rec.Get("roleDeviation")
	entDev, _ := rec.Get("entDeviation")

	pc := toFloat64(peerCount)
	md := toFloat64(myDirect)
	rd := toFloat64(roleDev)
	ed := toFloat64(entDev)

	var factors []string
	var score float64

	if pc > 0 {
		roleScore := math.Min(100, rd*10)
		entScore := math.Min(150, ed*15)
		directScore := math.Min(50, md*5)
		score = roleScore + entScore + directScore

		if rd > 2 {
			factors = append(factors, fmt.Sprintf("role_deviation=%.0f_above_peers", rd))
		}
		if ed > 3 {
			factors = append(factors, fmt.Sprintf("entitlement_deviation=%.0f_above_peers", ed))
		}
		if md > 2 {
			factors = append(factors, fmt.Sprintf("direct_access=%d_unusual", int(md)))
		}
	}

	if len(factors) == 0 {
		factors = []string{"peer_aligned"}
	}

	return math.Min(200, score), factors, nil
}

func toFloat64(v any) float64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0
	}
}

func toFloat64FromRecord(rec *neo4j.Record, key string) float64 {
	v, _ := rec.Get(key)
	return toFloat64(v)
}

func toInt64FromRecord(rec *neo4j.Record, key string) int64 {
	v, _ := rec.Get(key)
	return toInt64(v)
}
