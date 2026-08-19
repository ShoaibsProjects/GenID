package risk

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type MicroReview struct {
	neo4jDriver neo4j.DriverWithContext
}

func NewMicroReview(neo4jDriver neo4j.DriverWithContext) *MicroReview {
	return &MicroReview{neo4jDriver: neo4jDriver}
}

type ReviewTrigger struct {
	IdentityID  string
	TriggerType string
	RiskScore   float64
	RiskBand    string
	Description string
	CreatedAt   time.Time
}

func (m *MicroReview) TriggerReview(ctx context.Context, trigger ReviewTrigger) (string, error) {
	session := m.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	reviewID := fmt.Sprintf("review-%d", time.Now().UnixNano())

	query := `
		MATCH (i {uuid: $identityId}) WHERE i:Identity OR i:NonHumanIdentity
		CREATE (r:Review {
			uuid: $reviewId,
			trigger_type: $triggerType,
			risk_score: $riskScore,
			risk_band: $riskBand,
			description: $description,
			status: 'pending',
			created_at: datetime(),
			due_date: datetime() + duration('P3D')
		})
		CREATE (i)-[:HAS_REVIEW]->(r)
		RETURN r.uuid
	`

	_, err := session.Run(ctx, query, map[string]any{
		"identityId":  trigger.IdentityID,
		"reviewId":    reviewID,
		"triggerType": trigger.TriggerType,
		"riskScore":   trigger.RiskScore,
		"riskBand":    trigger.RiskBand,
		"description":  trigger.Description,
	})

	return reviewID, err
}

func (m *MicroReview) GetPendingReviews(ctx context.Context, identityID string) ([]map[string]any, error) {
	session := m.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (i {uuid: $identityId})-[:HAS_REVIEW]->(r:Review)
		WHERE r.status = 'pending'
		RETURN r.uuid AS reviewId, r.trigger_type AS triggerType,
		       r.risk_score AS riskScore, r.risk_band AS riskBand,
		       r.description AS description, r.created_at AS createdAt,
		       r.due_date AS dueDate
		ORDER BY r.risk_score DESC
	`

	result, err := session.Run(ctx, query, map[string]any{"identityId": identityID})
	if err != nil {
		return nil, err
	}

	var reviews []map[string]any
	for result.Next(ctx) {
		rec := result.Record()
		review := map[string]any{
			"reviewId":    getStr(rec, "reviewId"),
			"triggerType": getStr(rec, "triggerType"),
			"riskScore":   getFloat64(rec, "riskScore"),
			"riskBand":    getStr(rec, "riskBand"),
			"description": getStr(rec, "description"),
			"createdAt":   getStr(rec, "createdAt"),
			"dueDate":     getStr(rec, "dueDate"),
		}
		reviews = append(reviews, review)
	}

	return reviews, nil
}

func (m *MicroReview) CompleteReview(ctx context.Context, reviewID, decision, reviewer string) error {
	session := m.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (r:Review {uuid: $reviewId})
		SET r.status = 'completed',
		    r.decision = $decision,
		    r.reviewer = $reviewer,
		    r.completed_at = datetime()
	`

	_, err := session.Run(ctx, query, map[string]any{
		"reviewId":  reviewID,
		"decision":  decision,
		"reviewer":  reviewer,
	})

	return err
}
