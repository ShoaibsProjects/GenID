package risk

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type SessionManager struct {
	neo4jDriver neo4j.DriverWithContext
}

func NewSessionManager(neo4jDriver neo4j.DriverWithContext) *SessionManager {
	return &SessionManager{neo4jDriver: neo4jDriver}
}

func (s *SessionManager) TerminateAllSessions(ctx context.Context, identityID string, reason string) (int, error) {
	session := s.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (i {uuid: $identityId}) WHERE i:Identity OR i:NonHumanIdentity
		MATCH (i)-[:HAS_SESSION]->(s:Session)
		WHERE s.status = 'active'
		SET s.status = 'terminated',
		    s.terminated_at = datetime(),
		    s.termination_reason = $reason
		RETURN count(s) AS terminatedCount
	`

	result, err := session.Run(ctx, query, map[string]any{
		"identityId": identityID,
		"reason":     reason,
	})
	if err != nil {
		return 0, fmt.Errorf("terminate sessions: %w", err)
	}

	if result.Next(ctx) {
		rec := result.Record()
		count, _ := rec.Get("terminatedCount")
		switch n := count.(type) {
		case int64:
			return int(n), nil
		}
	}

	return 0, nil
}

func (s *SessionManager) CreateSession(ctx context.Context, identityID, source, ipAddress string) (string, error) {
	session := s.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	sessionID := fmt.Sprintf("sess-%d", time.Now().UnixNano())

	query := `
		MATCH (i {uuid: $identityId}) WHERE i:Identity OR i:NonHumanIdentity
		CREATE (s:Session {
			uuid: $sessionId,
			status: 'active',
			source: $source,
			ip_address: $ipAddress,
			created_at: datetime(),
			last_activity: datetime()
		})
		CREATE (i)-[:HAS_SESSION]->(s)
		RETURN s.uuid
	`

	_, err := session.Run(ctx, query, map[string]any{
		"identityId": identityID,
		"sessionId":  sessionID,
		"source":     source,
		"ipAddress":  ipAddress,
	})

	return sessionID, err
}

func (s *SessionManager) GetActiveSessions(ctx context.Context, identityID string) ([]map[string]any, error) {
	session := s.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (i {uuid: $identityId})-[:HAS_SESSION]->(s:Session)
		WHERE s.status = 'active'
		RETURN s.uuid AS sessionId, s.source AS source,
		       s.ip_address AS ipAddress, s.created_at AS createdAt
		ORDER BY s.created_at DESC
	`

	result, err := session.Run(ctx, query, map[string]any{"identityId": identityID})
	if err != nil {
		return nil, err
	}

	var sessions []map[string]any
	for result.Next(ctx) {
		rec := result.Record()
		sessionId, _ := rec.Get("sessionId")
		source, _ := rec.Get("source")
		ipAddress, _ := rec.Get("ipAddress")
		createdAt, _ := rec.Get("createdAt")
		sess := map[string]any{
			"sessionId": sessionId,
			"source":    source,
			"ipAddress": ipAddress,
			"createdAt": createdAt,
		}
		sessions = append(sessions, sess)
	}

	return sessions, nil
}
