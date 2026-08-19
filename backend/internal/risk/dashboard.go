package risk

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Dashboard struct {
	neo4jDriver neo4j.DriverWithContext
}

func NewDashboard(neo4jDriver neo4j.DriverWithContext) *Dashboard {
	return &Dashboard{neo4jDriver: neo4jDriver}
}

type RiskDashboard struct {
	TotalIdentities   int64          `json:"total_identities"`
	CriticalCount     int64          `json:"critical_count"`
	HighCount         int64          `json:"high_count"`
	ElevatedCount     int64          `json:"elevated_count"`
	LowCount          int64          `json:"low_count"`
	MinimalCount      int64          `json:"minimal_count"`
	AverageScore      float64        `json:"average_score"`
	TopRiskIdentities []RiskIdentity `json:"top_risk_identities"`
}

type RiskIdentity struct {
	IdentityID  string  `json:"identity_id"`
	DisplayName string  `json:"display_name"`
	RiskScore   float64 `json:"risk_score"`
	RiskBand    string  `json:"risk_band"`
	Department  string  `json:"department"`
	LastEvent   string  `json:"last_event"`
}

func getInt64(rec *neo4j.Record, key string) int64 {
	v, _ := rec.Get(key)
	return toInt64(v)
}

func getFloat64(rec *neo4j.Record, key string) float64 {
	v, _ := rec.Get(key)
	return toFloat64(v)
}

func getStr(rec *neo4j.Record, key string) string {
	v, _ := rec.Get(key)
	return fmt.Sprintf("%v", v)
}

func (d *Dashboard) GetDashboard(ctx context.Context) (*RiskDashboard, error) {
	session := d.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	dashboard := &RiskDashboard{}

	countQuery := `
		MATCH (i:Identity)
		RETURN count(i) AS total,
		       count(CASE WHEN i.risk_band = 'critical' THEN 1 END) AS critical,
		       count(CASE WHEN i.risk_band = 'high' THEN 1 END) AS high,
		       count(CASE WHEN i.risk_band = 'elevated' THEN 1 END) AS elevated,
		       count(CASE WHEN i.risk_band = 'low' THEN 1 END) AS low,
		       count(CASE WHEN i.risk_band = 'minimal' THEN 1 END) AS minimal,
		       avg(coalesce(i.risk_score, 0)) AS avgScore
	`

	result, err := session.Run(ctx, countQuery, nil)
	if err != nil {
		return nil, fmt.Errorf("dashboard counts: %w", err)
	}

	if result.Next(ctx) {
		rec := result.Record()
		dashboard.TotalIdentities = getInt64(rec, "total")
		dashboard.CriticalCount = getInt64(rec, "critical")
		dashboard.HighCount = getInt64(rec, "high")
		dashboard.ElevatedCount = getInt64(rec, "elevated")
		dashboard.LowCount = getInt64(rec, "low")
		dashboard.MinimalCount = getInt64(rec, "minimal")
		dashboard.AverageScore = getFloat64(rec, "avgScore")
	}

	topQuery := `
		MATCH (i:Identity)
		WHERE i.risk_score > 0
		RETURN i.uuid AS id, i.display_name AS name, i.risk_score AS score,
		       i.risk_band AS band, i.department AS dept, i.risk_last_event AS event
		ORDER BY i.risk_score DESC
		LIMIT 10
	`

	result, err = session.Run(ctx, topQuery, nil)
	if err != nil {
		return nil, fmt.Errorf("top risk: %w", err)
	}

	for result.Next(ctx) {
		rec := result.Record()
		identity := RiskIdentity{
			IdentityID:  getStr(rec, "id"),
			DisplayName: getStr(rec, "name"),
			RiskScore:   getFloat64(rec, "score"),
			RiskBand:    getStr(rec, "band"),
			Department:  getStr(rec, "dept"),
			LastEvent:   getStr(rec, "event"),
		}
		dashboard.TopRiskIdentities = append(dashboard.TopRiskIdentities, identity)
	}

	return dashboard, nil
}

func (d *Dashboard) GetIdentityRisk(ctx context.Context, identityID string) (map[string]any, error) {
	session := d.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (i {uuid: $identityId}) WHERE i:Identity OR i:NonHumanIdentity
		RETURN i.uuid AS id, i.display_name AS name, i.risk_score AS score,
		       i.risk_static AS static, i.risk_dynamic AS dynamic,
		       i.risk_peer AS peer, i.risk_band AS band,
		       i.risk_factors AS factors, i.risk_event_count AS eventCount,
		       i.risk_last_event AS lastEvent, i.risk_last_source AS lastSource,
		       i.risk_calculated_at AS calculatedAt
	`

	result, err := session.Run(ctx, query, map[string]any{"identityId": identityID})
	if err != nil {
		return nil, err
	}

	if result.Next(ctx) {
		rec := result.Record()
		id, _ := rec.Get("id")
		name, _ := rec.Get("name")
		score, _ := rec.Get("score")
		static, _ := rec.Get("static")
		dynamic, _ := rec.Get("dynamic")
		peer, _ := rec.Get("peer")
		band, _ := rec.Get("band")
		factors, _ := rec.Get("factors")
		eventCount, _ := rec.Get("eventCount")
		lastEvent, _ := rec.Get("lastEvent")
		lastSource, _ := rec.Get("lastSource")
		calculatedAt, _ := rec.Get("calculatedAt")

		return map[string]any{
			"identityId":   id,
			"displayName":  name,
			"riskScore":    score,
			"staticScore":  static,
			"dynamicScore": dynamic,
			"peerScore":    peer,
			"riskBand":     band,
			"factors":      factors,
			"eventCount":   eventCount,
			"lastEvent":    lastEvent,
			"lastSource":    lastSource,
			"calculatedAt": calculatedAt,
		}, nil
	}

	return nil, fmt.Errorf("identity not found")
}
