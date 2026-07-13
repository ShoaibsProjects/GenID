package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/observeid/identity-platform/internal/audit"
	"github.com/observeid/identity-platform/internal/connector"
	"github.com/observeid/identity-platform/internal/domain"
)

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }

func (q *queryResolver) getIdentityByID(ctx context.Context, id string) (*Identity, error) {
	row := q.Svc.Pool().QueryRow(ctx, `
		SELECT id, tenant_id, type, status, email, display_name,
		       COALESCE(department,''), COALESCE(employee_id,''), COALESCE(manager_id::text,''),
		       COALESCE(source,'manual'), risk_score, risk_factors, assurance_level, attributes,
		       created_at, updated_at
		FROM identities WHERE id = $1`, id)
	var d domain.Identity
	var dept, eid, mid, src string
	err := row.Scan(&d.ID, &d.TenantID, &d.Type, &d.Status, &d.Email, &d.DisplayName,
		&dept, &eid, &mid, &src, &d.RiskScore, &d.RiskFactors, &d.AssuranceLevel, &d.Attributes,
		&d.CreatedAt, &d.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("graphql: get identity: %w", err)
	}
	d.Department = dept
	d.EmployeeID = eid
	d.ManagerID = mid
	return identityToGQL(d, src), nil
}

func (q *queryResolver) getConnectorByID(ctx context.Context, id string) (*Connector, error) {
	cfg, err := q.Svc.ConnectorManager().GetConfig(id)
	if err != nil {
		return nil, fmt.Errorf("graphql: get connector: %w", err)
	}
	return configToGQLConnector(cfg), nil
}

func identityToGQL(d domain.Identity, src string) *Identity {
	attrs := map[string]any{
		"type":       d.Type,
		"status":     d.Status,
		"email":      d.Email,
		"risk_score": d.RiskScore,
	}
	if d.Attributes != nil {
		attrs["custom"] = d.Attributes
	}
	dept := d.Department
	eid := d.EmployeeID
	mid := d.ManagerID
	return &Identity{
		ID:             d.ID,
		TenantID:       d.TenantID,
		Type:           IdentityType(d.Type),
		Status:         IdentityStatus(d.Status),
		Email:          d.Email,
		DisplayName:    d.DisplayName,
		Department:     &dept,
		EmployeeID:     &eid,
		ManagerID:      &mid,
		Source:         src,
		RiskScore:      d.RiskScore,
		RiskFactors:    d.RiskFactors,
		AssuranceLevel: d.AssuranceLevel,
		Attributes:     attrs,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
}

func nhiToGQL(d domain.NonHumanIdentity) *NonHumanIdentity {
	owner := d.OwnerID
	team := d.TeamID
	card := d.AgentCardID
	return &NonHumanIdentity{
		ID:          d.ID,
		TenantID:    d.TenantID,
		Name:        d.Name,
		Type:        IdentityType(d.Type),
		Status:      IdentityStatus(d.Status),
		AgentCardID: &card,
		Protocols:   d.Protocols,
		OwnerID:     &owner,
		TeamID:      &team,
		IsGoverned:  d.IsGoverned,
		RiskScore:   d.RiskScore,
		ExpiresAt:   d.ExpiresAt,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}

func configToGQLConnector(cfg connector.ConnectorConfig) *Connector {
	ls := cfg.LastSyncAt
	le := cfg.LastError
	return &Connector{
		ID:            cfg.ID,
		TenantID:      cfg.TenantID,
		Name:          cfg.Name,
		ConnectorType: ConnectorType(cfg.Type),
		Status:        ConnectorStatus(cfg.Status),
		LastSyncAt:    ls,
		LastError:     &le,
		CreatedAt:     cfg.CreatedAt,
		UpdatedAt:     cfg.UpdatedAt,
	}
}

// ─── Query Resolvers ──────────────────────────────────────

func (r *queryResolver) Identities(ctx context.Context, limit *int, offset *int) ([]*Identity, error) {
	l := 100
	o := 0
	if limit != nil {
		l = *limit
	}
	if offset != nil {
		o = *offset
	}

	rows, err := r.Svc.Pool().Query(ctx, `
		SELECT id, tenant_id, type, status, email, display_name,
		       COALESCE(department,''), COALESCE(employee_id,''), COALESCE(manager_id::text,''),
		       COALESCE(source,'manual'), risk_score, risk_factors, assurance_level, attributes,
		       created_at, updated_at
		FROM identities ORDER BY created_at DESC LIMIT $1 OFFSET $2`, l, o)
	if err != nil {
		return nil, fmt.Errorf("graphql: list identities: %w", err)
	}
	defer rows.Close()

	var result []*Identity
	for rows.Next() {
		var d domain.Identity
		var dept, eid, mid, src string
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Type, &d.Status, &d.Email, &d.DisplayName,
			&dept, &eid, &mid, &src, &d.RiskScore, &d.RiskFactors, &d.AssuranceLevel, &d.Attributes,
			&d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.Department = dept
		d.EmployeeID = eid
		d.ManagerID = mid
		result = append(result, identityToGQL(d, src))
	}
	return result, nil
}

func (r *queryResolver) Identity(ctx context.Context, id string) (*Identity, error) {
	return r.getIdentityByID(ctx, id)
}

func (r *queryResolver) Agents(ctx context.Context, limit *int, offset *int) ([]*NonHumanIdentity, error) {
	l := 100
	o := 0
	if limit != nil {
		l = *limit
	}
	if offset != nil {
		o = *offset
	}

	rows, err := r.Svc.Pool().Query(ctx, `
		SELECT id, tenant_id, name, type, status, COALESCE(agent_card_id,''),
		       protocols, COALESCE(owner_id,''), COALESCE(team_id,''),
		       is_governed, risk_score, expires_at, created_at, updated_at
		FROM non_human_identities ORDER BY created_at DESC LIMIT $1 OFFSET $2`, l, o)
	if err != nil {
		return nil, fmt.Errorf("graphql: list agents: %w", err)
	}
	defer rows.Close()

	var result []*NonHumanIdentity
	for rows.Next() {
		var d domain.NonHumanIdentity
		var owner, team, card string
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Name, &d.Type, &d.Status,
			&card, &d.Protocols, &owner, &team,
			&d.IsGoverned, &d.RiskScore, &d.ExpiresAt, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.OwnerID = owner
		d.TeamID = team
		d.AgentCardID = card
		result = append(result, nhiToGQL(d))
	}
	return result, nil
}

func (r *queryResolver) Agent(ctx context.Context, id string) (*NonHumanIdentity, error) {
	row := r.Svc.Pool().QueryRow(ctx, `
		SELECT id, tenant_id, name, type, status, COALESCE(agent_card_id,''),
		       protocols, COALESCE(owner_id,''), COALESCE(team_id,''),
		       is_governed, risk_score, expires_at, created_at, updated_at
		FROM non_human_identities WHERE id = $1`, id)
	var d domain.NonHumanIdentity
	var owner, team, card string
	err := row.Scan(&d.ID, &d.TenantID, &d.Name, &d.Type, &d.Status,
		&card, &d.Protocols, &owner, &team,
		&d.IsGoverned, &d.RiskScore, &d.ExpiresAt, &d.CreatedAt, &d.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("graphql: get agent: %w", err)
	}
	d.OwnerID = owner
	d.TeamID = team
	d.AgentCardID = card
	return nhiToGQL(d), nil
}

func (r *queryResolver) Connectors(ctx context.Context) ([]*Connector, error) {
	configs := r.Svc.ConnectorManager().List()
	result := make([]*Connector, len(configs))
	for i, cfg := range configs {
		result[i] = configToGQLConnector(cfg)
	}
	return result, nil
}

func (r *queryResolver) Connector(ctx context.Context, id string) (*Connector, error) {
	return r.getConnectorByID(ctx, id)
}

func (r *queryResolver) ConnectorUsers(ctx context.Context, connectorID string, limit *int, offset *int) ([]*ConnectorUser, error) {
	users, err := r.Svc.ConnectorManager().GetConnectorUsers(ctx, connectorID)
	if err != nil {
		return nil, fmt.Errorf("graphql: connector users: %w", err)
	}
	l := len(users)
	if limit != nil && *limit < l {
		l = *limit
	}
	o := 0
	if offset != nil {
		o = *offset
	}
	if o >= len(users) {
		return nil, nil
	}
	users = users[o:l]

	result := make([]*ConnectorUser, len(users))
	for i, u := range users {
		un := u.Username
		em := u.Email
		dn := u.DisplayName
		dp := u.Department
		result[i] = &ConnectorUser{
			ExternalID:  u.ExternalID,
			Username:    &un,
			Email:       &em,
			DisplayName: &dn,
			Department:  &dp,
			Enabled:     u.Enabled,
		}
	}
	return result, nil
}

func (r *queryResolver) ConnectorGroups(ctx context.Context, connectorID string) ([]*ConnectorGroup, error) {
	groups, err := r.Svc.ConnectorManager().SyncGroups(ctx, connectorID)
	if err != nil {
		return nil, fmt.Errorf("graphql: connector groups: %w", err)
	}
	result := make([]*ConnectorGroup, len(groups))
	for i, g := range groups {
		desc := g.Description
		gt := g.Type
		result[i] = &ConnectorGroup{
			ExternalID:  g.ExternalID,
			Name:        g.Name,
			Description: &desc,
			GroupType:   &gt,
		}
	}
	return result, nil
}

func (r *queryResolver) ConnectorHealth(ctx context.Context, connectorID string) (*ConnectorHealth, error) {
	h, err := r.Svc.ConnectorManager().GetConnectorHealth(connectorID)
	if err != nil {
		return nil, fmt.Errorf("graphql: connector health: %w", err)
	}
	return &ConnectorHealth{
		ConnectorID:    h.ConnectorID,
		ConnectorName:  h.ConnectorName,
		Status:         h.Status,
		DeltaSupported: h.DeltaSupported,
		SupportsSchema: h.SupportsSchema,
	}, nil
}

func (r *queryResolver) AuditLogs(ctx context.Context, limit *int, offset *int, level *string, path *string) ([]*AuditEntry, error) {
	l := 100
	o := 0
	if limit != nil {
		l = *limit
	}
	if offset != nil {
		o = *offset
	}
	lvl := ""
	if level != nil {
		lvl = *level
	}
	pth := ""
	if path != nil {
		pth = *path
	}

	store := r.Svc.AuditStore()
	entries := store.List(l, o, audit.Level(lvl), pth)
	result := make([]*AuditEntry, len(entries))
	for i, e := range entries {
		method := e.Method
		epath := e.Path
		status := e.Status
		latency := e.Latency
		src := e.SourceIP
		result[i] = &AuditEntry{
			ID:        e.ID,
			Timestamp: e.Timestamp,
			Level:     string(e.Level),
			Service:   e.Service,
			Method:    &method,
			Path:      &epath,
			Status:    &status,
			Latency:   &latency,
			Message:   e.Message,
			SourceIP:  &src,
			Tags:      e.Tags,
		}
	}
	return result, nil
}

func (r *queryResolver) Health(ctx context.Context) (*HealthStatus, error) {
	return &HealthStatus{
		Status:  "ok",
		Service: "observeid-identity",
		Version: "1.0.0",
	}, nil
}

func (r *queryResolver) Ready(ctx context.Context) (*ReadinessResult, error) {
	checks := map[string]string{}
	if err := r.Svc.Redis().Ping(ctx).Err(); err != nil {
		checks["redis"] = "down"
	} else {
		checks["redis"] = "ok"
	}
	if err := r.Svc.Pool().Ping(ctx); err != nil {
		checks["postgres"] = "down"
	} else {
		checks["postgres"] = "ok"
	}
	if err := r.Svc.Neo4j().VerifyConnectivity(ctx); err != nil {
		checks["neo4j"] = "down"
	} else {
		checks["neo4j"] = "ok"
	}

	status := "ready"
	for _, v := range checks {
		if v == "down" {
			status = "unavailable"
			break
		}
	}

	return &ReadinessResult{
		Status: status,
		Checks: &ReadinessChecks{
			Redis:    checks["redis"],
			Postgres: checks["postgres"],
			Neo4j:    checks["neo4j"],
		},
	}, nil
}

// ─── Mutation Resolvers ──────────────────────────────────

func (r *mutationResolver) CreateIdentity(ctx context.Context, input CreateIdentityInput) (*Identity, error) {
	attrs := map[string]string{}
	if input.Attributes != nil {
		if m, ok := input.Attributes.(map[string]any); ok {
			for k, v := range m {
				attrs[k] = fmt.Sprintf("%v", v)
			}
		}
	}
	src := "manual"
	if input.Source != nil {
		src = *input.Source
	}
	dept := ""
	if input.Department != nil {
		dept = *input.Department
	}
	eid := ""
	if input.EmployeeID != nil {
		eid = *input.EmployeeID
	}

	identityType := strings.ToLower(string(input.Type))
	var id string
	err := r.Svc.Pool().QueryRow(ctx, `
		INSERT INTO identities (tenant_id, type, status, email, display_name, department, employee_id, source, attributes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id`,
		"00000000-0000-0000-0000-000000000001",
		identityType, "active", input.Email, input.DisplayName,
		dept, eid, src, attrs,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("graphql: create identity: %w", err)
	}

	return (&queryResolver{r.Resolver}).getIdentityByID(ctx, id)
}

func (r *mutationResolver) UpdateIdentity(ctx context.Context, id string, input UpdateIdentityInput) (*Identity, error) {
	attrs := map[string]string{}
	attrsJSON, _ := json.Marshal(attrs)

	statusStr := (*string)(nil)
	if input.Status != nil {
		s := strings.ToLower(string(*input.Status))
		statusStr = &s
	}
	_, err := r.Svc.Pool().Exec(ctx, `
		UPDATE identities SET
			display_name = COALESCE($2, display_name),
			department   = COALESCE($3, department),
			email        = COALESCE($4, email),
			status       = COALESCE($5, status),
			attributes   = COALESCE($6::jsonb, attributes),
			updated_at   = NOW()
		WHERE id = $1`,
		id,
		input.DisplayName,
		input.Department,
		input.Email,
		statusStr,
		string(attrsJSON),
	)
	if err != nil {
		return nil, fmt.Errorf("graphql: update identity: %w", err)
	}
	return (&queryResolver{r.Resolver}).getIdentityByID(ctx, id)
}

func (r *mutationResolver) DeleteIdentity(ctx context.Context, id string) (bool, error) {
	_, err := r.Svc.Pool().Exec(ctx, `UPDATE identities SET status = 'terminated', updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("graphql: delete identity: %w", err)
	}
	return true, nil
}

func (r *mutationResolver) CreateConnector(ctx context.Context, input CreateConnectorInput) (*Connector, error) {
	cfgJSON, _ := json.Marshal(input.Config)
	cfg := connector.ConnectorConfig{
		ID:       fmt.Sprintf("conn-%d", time.Now().UnixNano()),
		TenantID: "00000000-0000-0000-0000-000000000001",
		Name:     input.Name,
		Type:     connector.ConnectorType(input.ConnectorType),
		Status:   connector.ConnectorStatusDisconnected,
		Properties: map[string]string{
			"raw_config": string(cfgJSON),
		},
	}
	id, err := r.Svc.ConnectorManager().Register(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("graphql: create connector: %w", err)
	}
	cfg.ID = id
	return configToGQLConnector(cfg), nil
}

func (r *mutationResolver) DeleteConnector(ctx context.Context, id string) (bool, error) {
	if err := r.Svc.ConnectorManager().Unregister(ctx, id); err != nil {
		return false, fmt.Errorf("graphql: delete connector: %w", err)
	}
	return true, nil
}

func (r *mutationResolver) ConnectConnector(ctx context.Context, id string) (*Connector, error) {
	if err := r.Svc.ConnectorManager().Connect(ctx, id); err != nil {
		return nil, fmt.Errorf("graphql: connect connector: %w", err)
	}
	return (&queryResolver{r.Resolver}).getConnectorByID(ctx, id)
}

func (r *mutationResolver) DisconnectConnector(ctx context.Context, id string) (*Connector, error) {
	if err := r.Svc.ConnectorManager().Disconnect(ctx, id); err != nil {
		return nil, fmt.Errorf("graphql: disconnect connector: %w", err)
	}
	return (&queryResolver{r.Resolver}).getConnectorByID(ctx, id)
}

func (r *mutationResolver) SyncConnector(ctx context.Context, id string) (*Connector, error) {
	if _, err := r.Svc.ConnectorManager().SyncUsers(ctx, id); err != nil {
		return nil, fmt.Errorf("graphql: sync connector: %w", err)
	}
	return (&queryResolver{r.Resolver}).getConnectorByID(ctx, id)
}

func (r *mutationResolver) SyncConnectorDelta(ctx context.Context, id string) (*Connector, error) {
	if _, err := r.Svc.ConnectorManager().SyncUsersDelta(ctx, id); err != nil {
		return nil, fmt.Errorf("graphql: sync delta: %w", err)
	}
	return (&queryResolver{r.Resolver}).getConnectorByID(ctx, id)
}
