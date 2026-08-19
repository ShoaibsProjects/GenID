package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.temporal.io/sdk/client"

	"github.com/observeid/genid/internal/audit"
	"github.com/observeid/genid/internal/connector"
	"github.com/observeid/genid/internal/workflow"
)

// JMLResult summarises the propagation of a connector sync into the
// canonical identity model.
type JMLResult struct {
	IdentitiesCreated    int      `json:"identities_created"`
	IdentitiesUpdated    int      `json:"identities_updated"`
	IdentitiesTerminated int      `json:"identities_terminated"`
	CreatedIDs           []string `json:"created_ids,omitempty"`
	UpdatedIDs           []string `json:"updated_ids,omitempty"`
	TerminatedIDs        []string `json:"terminated_ids,omitempty"`
}

// ApplyJMLFromConnectorUsers diffs a freshly-synced connector roster against
// the canonical identities table and emits the right outbox events for each
// kind of change. Safe to run repeatedly: unchanged rows produce no events.
//
// Identity resolution order: employee_id first (within tenant), falling back
// to email.
//
// Termination: any existing identity with source='hris' within the same
// tenant, whose employee_id (or email) is NOT in the new roster and whose
// status is still active/inactive/on_leave, is marked status='terminated'
// and emits identity.deleted. Pending/suspended/terminated rows are skipped.
func (s *Service) ApplyJMLFromConnectorUsers(ctx context.Context, tenantID, connectorID string, users []connector.ConnectorUser) (*JMLResult, error) {
	res := &JMLResult{}

	type rosterKey struct{ empID, email string }
	inRoster := make(map[rosterKey]bool, len(users))
	for _, u := range users {
		inRoster[rosterKey{empID: u.EmployeeID, email: strings.ToLower(u.Email)}] = true
	}

	for _, u := range users {
		if u.Email == "" || u.DisplayName == "" {
			continue
		}
		var (
			existingID, existingDept, existingManager string
			existingStatus                            string
			existingEmpID                             *string
		)
		err := s.pgPool.QueryRow(ctx, `
			SELECT id, COALESCE(department,''), COALESCE(manager_id::text,''),
			       status, employee_id
			FROM identities
			WHERE tenant_id = $1 AND (
				(employee_id = $2 AND $2 <> '')
				OR (email = $3 AND $3 <> '')
			)
			ORDER BY (employee_id = $2 AND $2 <> '') DESC
			LIMIT 1
		`, tenantID, u.EmployeeID, strings.ToLower(u.Email)).Scan(
			&existingID, &existingDept, &existingManager,
			&existingStatus, &existingEmpID)

		newStatus := "active"
		if !u.Enabled {
			newStatus = "inactive"
		}
		newSource := "hris"

		var managerID interface{} // NULL unless successfully resolved to a UUID
		attrs := map[string]string{}
		if u.Attributes != nil {
			attrs = u.Attributes
		}
		attrs["hr_source_connector_id"] = connectorID
		attrs["title"] = u.Title
		attrs["first_name"] = u.FirstName
		attrs["last_name"] = u.LastName
		if u.Manager != "" {
			attrs["manager_email"] = u.Manager // always keep the email for reference
			var mgrID string
			_ = s.pgPool.QueryRow(ctx, `SELECT id FROM identities WHERE email = $1 LIMIT 1`, u.Manager).Scan(&mgrID)
			if mgrID != "" {
				managerID = mgrID
			}
			// else: manager_id stays NULL; the email is preserved in attributes.manager_email
		}
		attrsJSON, _ := json.Marshal(attrs)
		if attrsJSON == nil {
			attrsJSON = []byte("{}")
		}

		tx, err := s.pgPool.Begin(ctx)
		if err != nil {
			return res, fmt.Errorf("begin tx: %w", err)
		}
		// Rollback is safe to call even after Commit (returns ErrTxClosed).
		defer tx.Rollback(ctx)

		if err == nil && existingID != "" {
			// UPDATE path.
			deptChanged := existingDept != u.Department
			mgrChanged := existingManager != asString(managerID)
			statusChanged := existingStatus != newStatus
			if !deptChanged && !mgrChanged && !statusChanged {
				_ = tx.Commit(ctx)
				continue
			}

			// Manager update: only when we successfully resolved the email to a UUID.
			// Pass the manager UUID as a string and let Postgres cast it.
			var mgrParam any = nil
			if managerID != nil {
				mgrParam = asString(managerID)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE identities
				SET department = COALESCE(NULLIF($2,''), department),
				    manager_id = CASE WHEN $3::text IS NOT NULL AND $3 <> '' THEN $3::uuid ELSE manager_id END,
				    status     = $4,
				    source     = $5,
				    attributes = $6,
				    updated_at = NOW()
				WHERE id = $1 AND tenant_id = $7
			`, existingID, u.Department, mgrParam, newStatus, newSource, attrsJSON, tenantID); err != nil {
				return res, fmt.Errorf("update identity %s: %w", existingID, err)
			}

			err = s.outbox.Publish(ctx, tx, "identity.updated", "identity", existingID,
				map[string]any{
					"tenant_id":    tenantID,
					"email":        u.Email,
					"display_name": u.DisplayName,
					"status":       newStatus,
					"department":   u.Department,
					"title":        u.Title,
					"manager":      u.Manager,
					"source":       newSource,
					"changes":      changesMap(deptChanged, mgrChanged, statusChanged),
					"connector_id": connectorID,
				},
				map[string]any{"method": "csv_hr_sync"})
			if err != nil {
				return res, fmt.Errorf("outbox updated %s: %w", existingID, err)
			}
			if err := tx.Commit(ctx); err != nil {
				return res, fmt.Errorf("commit update %s: %w", existingID, err)
			}
			res.IdentitiesUpdated++
			res.UpdatedIDs = append(res.UpdatedIDs, existingID)
			continue
		}

		// CREATE path.
		newID := uuid.New().String()
		// NOTE: first_name / last_name / title are stashed in attributes
		// JSONB; the canonical `identities` table only has department /
		// employee_id / manager_id as proper columns.
		if _, err := tx.Exec(ctx, `
			INSERT INTO identities (id, tenant_id, type, status, email, display_name,
			                        department,
			                        employee_id, manager_id, source, attributes,
			                        risk_score)
			VALUES ($1, $2, $3, $4, $5, $6,
			        $7,
			        $8, $9, $10, $11, 0.0)
			ON CONFLICT (tenant_id, email) DO NOTHING
		`, newID, tenantID, "human", newStatus, strings.ToLower(u.Email), u.DisplayName,
			u.Department,
			u.EmployeeID, managerID, newSource, attrsJSON); err != nil {
			return res, fmt.Errorf("insert identity %s: %w", u.Email, err)
		}

		err = s.outbox.Publish(ctx, tx, "identity.created", "identity", newID,
			map[string]any{
				"tenant_id":    tenantID,
				"email":        strings.ToLower(u.Email),
				"display_name": u.DisplayName,
				"status":       newStatus,
				"type":         "human",
				"source":       newSource,
				"department":   u.Department,
				"title":        u.Title,
				"employee_id":  u.EmployeeID,
				"connector_id": connectorID,
			},
			map[string]any{"method": "csv_hr_sync"})
		if err != nil {
			return res, fmt.Errorf("outbox created %s: %w", u.Email, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return res, fmt.Errorf("commit create %s: %w", u.Email, err)
		}
		res.IdentitiesCreated++
		res.CreatedIDs = append(res.CreatedIDs, newID)
	}

	// Termination diff: hris-sourced identities missing from the new roster.
	// Scope by connector_id (stored in attributes.hr_source_connector_id) so
	// one HR-source connector's roster only terminates identities IT created.
	rows, err := s.pgPool.Query(ctx, `
		SELECT id::text, COALESCE(employee_id,''), email
		FROM identities
		WHERE tenant_id = $1
		  AND source = 'hris'
		  AND status IN ('active','inactive')
		  AND COALESCE(attributes->>'hr_source_connector_id','') = $2
	`, tenantID, connectorID)
	if err != nil {
		return res, fmt.Errorf("query hris identities: %w", err)
	}
	type hrr struct {
		id, empID, email string
	}
	var hrrRows []hrr
	for rows.Next() {
		var r hrr
		if err := rows.Scan(&r.id, &r.empID, &r.email); err != nil {
			continue
		}
		hrrRows = append(hrrRows, r)
	}
	rows.Close()

	for _, r := range hrrRows {
		key := rosterKey{r.empID, strings.ToLower(r.email)}
		if inRoster[key] {
			continue
		}
		tx, err := s.pgPool.Begin(ctx)
		if err != nil {
			return res, fmt.Errorf("begin term tx: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE identities
			SET status = 'terminated',
			    attributes = jsonb_set(COALESCE(attributes,'{}'::jsonb), '{terminated_at}', to_jsonb(NOW())),
			    updated_at = NOW()
			WHERE id = $1 AND tenant_id = $2
		`, r.id, tenantID); err != nil {
			_ = tx.Rollback(ctx)
			return res, fmt.Errorf("terminate identity %s: %w", r.id, err)
		}
		if err := s.outbox.Publish(ctx, tx, "identity.deleted", "identity", r.id,
			map[string]any{
				"tenant_id":    tenantID,
				"email":        r.email,
				"employee_id":  r.empID,
				"reason":       "hr_source_removed",
				"connector_id": connectorID,
			},
			map[string]any{"method": "csv_hr_sync"}); err != nil {
			_ = tx.Rollback(ctx)
			return res, fmt.Errorf("outbox delete %s: %w", r.id, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return res, fmt.Errorf("commit term %s: %w", r.id, err)
		}
		res.IdentitiesTerminated++
		res.TerminatedIDs = append(res.TerminatedIDs, r.id)
	}

	s.auditLog.Append(audit.Entry{
		Level: audit.LevelInfo, Service: "connector-hr", Path: "/sync-hr",
		Message: fmt.Sprintf("HR sync: %d created, %d updated, %d terminated",
			res.IdentitiesCreated, res.IdentitiesUpdated, res.IdentitiesTerminated),
		Tags: []string{"connector", "hr", "jml"},
	})

	return res, nil
}

// asString renders any value as a string ("" for nil).
func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// changesMap summarises which identity fields changed for outbox events.
func changesMap(dept, mgr, status bool) map[string]bool {
	return map[string]bool{
		"department": dept,
		"manager":    mgr,
		"status":     status,
	}
}

// SanitizeConfig redacts secrets from a connector config before it leaves
// the server.
func SanitizeConfig(cfg connector.ConnectorConfig) connector.ConnectorConfig {
	if cfg.ClientSecret != "" {
		cfg.ClientSecret = "[redacted]"
	}
	if cfg.Password != "" {
		cfg.Password = "[redacted]"
	}
	if cfg.Cert != "" {
		cfg.Cert = "[redacted]"
	}
	// Redact bearer tokens stored in properties
	if cfg.Properties != nil {
		if _, ok := cfg.Properties["bearer_token"]; ok {
			cfg.Properties["bearer_token"] = "[redacted]"
		}
	}
	return cfg
}

// logError prints an error with a component prefix.
func logError(component string, err error) {
	fmt.Printf("[ERROR] %s: %v\n", component, err)
}

// StartJustInTimeWorkflow launches the JIT access Temporal workflow.
func (s *Service) StartJustInTimeWorkflow(ctx context.Context, input workflow.JustInTimeInput) (string, error) {
	workflowID := fmt.Sprintf("jit-access-%s-%s", input.IdentityID, uuid.New().String()[:8])
	_, err := s.temporal.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "critical-offboarding",
	}, workflow.JustInTimeAccessWorkflow, input)
	if err != nil {
		return "", fmt.Errorf("start jit workflow: %w", err)
	}
	return workflowID, nil
}

// RiskBandFromScore maps a 0-1000 risk score to the band names used
// by the approval routing engine.
func RiskBandFromScore(score float64) string {
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

// RevokeTemporaryAccess mirrors the RevokeTemporaryAccess activity logic —
// duplicated here so we don't need to round-trip Temporal just to revoke a
// session. Cleans Redis JIT grants and the Neo4j HAS_TEMPORARY_ACCESS edge.
func (s *Service) RevokeTemporaryAccess(ctx context.Context, identityID, resourceID, reason, revokedBy string) error {
	if identityID == "" {
		return fmt.Errorf("identity_id required")
	}
	// Redis cleanup
	if resourceID != "" {
		s.redis.Del(ctx, fmt.Sprintf("jit:grant:%s:%s", identityID, resourceID))
	} else {
		iter := s.redis.Scan(ctx, 0, fmt.Sprintf("jit:grant:%s:*", identityID), 0).Iterator()
		for iter.Next(ctx) {
			s.redis.Del(ctx, iter.Val())
		}
	}
	// Neo4j cleanup
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	if resourceID != "" {
		_, _ = session.Run(ctx, `
			MATCH (i)-[r:HAS_TEMPORARY_ACCESS]->(res:Resource {uuid:$rid})
			WHERE (i:Identity OR i:NonHumanIdentity) AND i.uuid = $iid
			DELETE r
		`, map[string]any{"iid": identityID, "rid": resourceID})
	} else {
		_, _ = session.Run(ctx, `
			MATCH (i)-[r:HAS_TEMPORARY_ACCESS]->(res:Resource)
			WHERE (i:Identity OR i:NonHumanIdentity) AND i.uuid = $iid
			DELETE r
		`, map[string]any{"iid": identityID})
	}
	_ = s.wfStore.AppendAudit(ctx, "", "activity.completed", "system", map[string]any{
		"action":      "revoke_temporary_access",
		"identity_id": identityID,
		"resource_id": resourceID,
		"reason":      reason,
		"revoked_by":  revokedBy,
	})
	return nil
}
