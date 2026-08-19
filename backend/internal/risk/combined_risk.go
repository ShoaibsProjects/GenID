package risk

import (
	"context"
	"fmt"
	"math"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type CombinedRisk struct {
	neo4jDriver neo4j.DriverWithContext
	staticCalc  *StaticRisk
	peerCalc    *PeerDeviation
}

type RiskBreakdown struct {
	FinalScore    float64
	StaticScore   float64
	DynamicScore  float64
	PeerScore     float64
	Band          string
	StaticFactors []string
	PeerFactors   []string
}

func NewCombinedRisk(neo4jDriver neo4j.DriverWithContext) *CombinedRisk {
	return &CombinedRisk{
		neo4jDriver: neo4jDriver,
		staticCalc:  NewStaticRisk(neo4jDriver),
		peerCalc:    NewPeerDeviation(neo4jDriver),
	}
}

func (c *CombinedRisk) Calculate(ctx context.Context, identityID string) (*RiskBreakdown, error) {
	staticScore, staticFactors, err := c.staticCalc.Calculate(ctx, identityID)
	if err != nil {
		return nil, fmt.Errorf("static risk: %w", err)
	}

	peerScore, peerFactors, err := c.peerCalc.Calculate(ctx, identityID)
	if err != nil {
		return nil, fmt.Errorf("peer deviation: %w", err)
	}

	dynamicScore, err := c.getDynamicScore(ctx, identityID)
	if err != nil {
		return nil, fmt.Errorf("dynamic risk: %w", err)
	}

	finalScore := math.Min(1000, staticScore*0.3+dynamicScore*0.5+peerScore*0.2)

	return &RiskBreakdown{
		FinalScore:    finalScore,
		StaticScore:   staticScore,
		DynamicScore:  dynamicScore,
		PeerScore:     peerScore,
		Band:          scoreToBand(finalScore),
		StaticFactors: staticFactors,
		PeerFactors:   peerFactors,
	}, nil
}

func (c *CombinedRisk) getDynamicScore(ctx context.Context, identityID string) (float64, error) {
	session := c.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (i:Identity {uuid: $identityId})
		RETURN coalesce(i.risk_score, 0) AS dynamicScore
	`

	result, err := session.Run(ctx, query, map[string]any{"identityId": identityID})
	if err != nil {
		return 0, err
	}

	if result.Next(ctx) {
		rec := result.Record()
		score, _ := rec.Get("dynamicScore")
		switch n := score.(type) {
		case float64:
			return n, nil
		case int64:
			return float64(n), nil
		}
	}

	return 0, nil
}

func (c *CombinedRisk) Persist(ctx context.Context, identityID string, breakdown *RiskBreakdown) error {
	session := c.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (i {uuid: $identityId}) WHERE i:Identity OR i:NonHumanIdentity
		SET i.risk_score = $finalScore,
		    i.risk_static = $staticScore,
		    i.risk_dynamic = $dynamicScore,
		    i.risk_peer = $peerScore,
		    i.risk_band = $band,
		    i.risk_factors = $factors,
		    i.risk_calculated_at = datetime(),
		    i.updated_at = datetime()
	`

	factors := append(breakdown.StaticFactors, breakdown.PeerFactors...)

	_, err := session.Run(ctx, query, map[string]any{
		"identityId":  identityID,
		"finalScore":  math.Round(breakdown.FinalScore*100) / 100,
		"staticScore": math.Round(breakdown.StaticScore*100) / 100,
		"dynamicScore": math.Round(breakdown.DynamicScore*100) / 100,
		"peerScore":   math.Round(breakdown.PeerScore*100) / 100,
		"band":        breakdown.Band,
		"factors":     factors,
	})

	return err
}

func scoreToBand(score float64) string {
	switch {
	case score >= 800:
		return "critical"
	case score >= 600:
		return "high"
	case score >= 300:
		return "elevated"
	case score >= 100:
		return "low"
	default:
		return "minimal"
	}
}
