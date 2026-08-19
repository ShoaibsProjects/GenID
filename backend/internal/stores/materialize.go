package stores

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// MaterializeResult summarizes what a materialization pass changed so
// callers can surface it in sync responses and audit entries.
type MaterializeResult struct {
	EntitlementsUpserted int      `json:"entitlements_upserted"`
	ResourcesUpserted    int      `json:"resources_upserted"`
	GroupsUpserted       int      `json:"groups_upserted"`
	GraphNodesWritten    int      `json:"graph_nodes_written"`
	GraphEdgesWritten    int      `json:"graph_edges_written"`
	UncorrelatedUsers    int      `json:"uncorrelated_users"`
	Errors               []string `json:"errors,omitempty"`
}

// MaterializeConnectorData promotes a connector's synced cache into the
// canonical data model:
//
//  1. connector_entitlements  → entitlements            (dedup + risk scoring)
//  2. connector_resources     → resources              (dedup)
//  3. connector_groups        → roles                  (directory groups become roles)
//  4. Neo4j access graph      → Entitlement/Resource/Role nodes + HAS_ENTITLEMENT /
//                              HAS_ROLE / ACCESSES edges correlated to canonical
//                              identities by email.
//
// Mirrors how SailPoint/Saviynt materialize aggregated accounts into the
// authoritative identity model: the connector cache is a staging area, the
// canonical model is the source of truth, and the graph is the joined view.
func (s *Store) MaterializeConnectorData(ctx context.Context, tenantID, connectorID string) (*MaterializeResult, error) {
	res := &MaterializeResult{}

	// ── 1. Canonical entitlements (dedup on app_name + permission_level) ──
	entMap, err := s.materializeEntitlements(ctx, tenantID, connectorID, res)
	if err != nil {
		return res, err
	}

	// ── 2. Canonical resources ──
	resMap, err := s.materializeResources(ctx, tenantID, connectorID, res)
	if err != nil {
		return res, err
	}

	// ── 3. Directory groups → roles ──
	roleMap, err := s.materializeGroups(ctx, tenantID, connectorID, res)
	if err != nil {
		return res, err
	}

	// ── 4. Neo4j access graph ──
	if err := s.materializeGraph(ctx, tenantID, connectorID, entMap, resMap, roleMap, res); err != nil {
		return res, err
	}

	return res, nil
}

// materializeEntitlements upserts connector entitlements into the canonical
// entitlements table and returns a map of "app_name\x00permission_level" → UUID.
func (s *Store) materializeEntitlements(ctx context.Context, tenantID, connectorID string, res *MaterializeResult) (map[string]string, error) {
	rows, err := s.pg.Query(ctx, `
		SELECT app_name, source_name, source_id, entitlement_type, raw_attributes
		FROM connector_entitlements
		WHERE connector_id = $1 AND is_active = true
	`, connectorID)
	if err != nil {
		return nil, fmt.Errorf("query connector entitlements: %w", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	var appNames, levels, types []string
	for rows.Next() {
		var appName, sourceName, sourceID, entType string
		var raw map[string]any
		if err := rows.Scan(&appName, &sourceName, &sourceID, &entType, &raw); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("scan entitlement: %v", err))
			continue
		}
		app := appName
		if app == "" {
			app = "Microsoft Entra ID"
		}
		level := sourceName
		if level == "" {
			level = sourceID
		}
		if level == "" {
			level = entType
		}
		key := app + "\x00" + level
		if seen[key] {
			continue
		}
		seen[key] = true
		appNames = append(appNames, app)
		levels = append(levels, level)
		types = append(types, entType)
	}

	entMap := map[string]string{}
	for i := range appNames {
		risk, toxic, rubberband := classifyEntitlement(types[i], appNames[i], levels[i])

		var id string
		err := s.pg.QueryRow(ctx, `
			INSERT INTO entitlements
				(tenant_id, app_name, permission_level, entitlement_type, risk_classification, is_toxic, is_rubberband)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (tenant_id, app_name, permission_level) DO UPDATE SET
				entitlement_type    = EXCLUDED.entitlement_type,
				risk_classification = EXCLUDED.risk_classification,
				is_toxic            = EXCLUDED.is_toxic,
				is_rubberband       = EXCLUDED.is_rubberband,
				updated_at          = NOW()
			RETURNING id
		`, tenantID, appNames[i], levels[i], types[i], risk, toxic, rubberband).Scan(&id)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("upsert entitlement %s/%s: %v", appNames[i], levels[i], err))
			continue
		}
		entMap[appNames[i]+"\x00"+levels[i]] = id
		res.EntitlementsUpserted++
	}
	return entMap, nil
}

// materializeResources upserts connector resources into the canonical
// resources table and returns a map of "name" → UUID.
func (s *Store) materializeResources(ctx context.Context, tenantID, connectorID string, res *MaterializeResult) (map[string]string, error) {
	rows, err := s.pg.Query(ctx, `
		SELECT name, resource_type, description
		FROM connector_resources
		WHERE connector_id = $1
	`, connectorID)
	if err != nil {
		return nil, fmt.Errorf("query connector resources: %w", err)
	}
	defer rows.Close()

	resMap := map[string]string{}
	for rows.Next() {
		var name, rtype, desc string
		if err := rows.Scan(&name, &rtype, &desc); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("scan resource: %v", err))
			continue
		}
		if name == "" {
			continue
		}
		criticality := resourceCriticality(rtype)
		var id string
		err := s.pg.QueryRow(ctx, `
			INSERT INTO resources
				(tenant_id, name, resource_type, criticality, data_classification, health_status, attributes)
			VALUES ($1, $2, $3, $4, 'internal', 'connected',
				jsonb_build_object('source', 'connector', 'connector_id', $5::text))
			ON CONFLICT (tenant_id, name) DO UPDATE SET
				resource_type = EXCLUDED.resource_type,
				criticality   = EXCLUDED.criticality,
				health_status = EXCLUDED.health_status,
				updated_at    = NOW()
			RETURNING id
		`, tenantID, name, rtype, criticality, connectorID).Scan(&id)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("upsert resource %s: %v", name, err))
			continue
		}
		resMap[name] = id
		res.ResourcesUpserted++
	}
	return resMap, nil
}

// materializeGroups upserts connector directory groups into the canonical
// roles table (a group is a role whose members derive from the directory)
// and returns a map of group name → role UUID.
func (s *Store) materializeGroups(ctx context.Context, tenantID, connectorID string, res *MaterializeResult) (map[string]string, error) {
	rows, err := s.pg.Query(ctx, `
		SELECT name, description, group_type
		FROM connector_groups
		WHERE connector_id = $1
	`, connectorID)
	if err != nil {
		return nil, fmt.Errorf("query connector groups: %w", err)
	}
	defer rows.Close()

	roleMap := map[string]string{}
	for rows.Next() {
		var name, desc, groupType string
		if err := rows.Scan(&name, &desc, &groupType); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("scan group: %v", err))
			continue
		}
		if name == "" {
			continue
		}
		roleType := "directory"
		if groupType == "unified" {
			roleType = "unified"
		}
		attr := fmt.Sprintf(`{"source": "connector", "connector_id": %q}`, connectorID)

		var id string
		err := s.pg.QueryRow(ctx, `
			INSERT INTO roles (tenant_id, name, description, role_type, is_auto_assigned, attributes)
			VALUES ($1, $2, $3, $4, TRUE, $5::jsonb)
			ON CONFLICT (tenant_id, name) DO UPDATE SET
				description = EXCLUDED.description,
				role_type   = EXCLUDED.role_type,
				updated_at  = NOW()
			RETURNING id
		`, tenantID, name, desc, roleType, attr).Scan(&id)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("upsert group->role %s: %v", name, err))
			continue
		}
		roleMap[name] = id
		res.GroupsUpserted++
	}
	return roleMap, nil
}

// materializeGraph writes the access graph to Neo4j. It correlates connector
// identities to canonical identities by email (case-insensitive), creates
// Entitlement / Resource / Role nodes, and links:
//
//	(i:Identity)-[:HAS_ENTITLEMENT]->(e:Entitlement)-[:ACCESSES]->(r:Resource)
//	(i:Identity)-[:HAS_ROLE]->(r:Role)
func (s *Store) materializeGraph(ctx context.Context, tenantID, connectorID string, entMap, resMap, roleMap map[string]string, res *MaterializeResult) error {
	if s.neo4j == nil {
		return nil
	}
	if len(entMap) == 0 && len(resMap) == 0 && len(roleMap) == 0 {
		return nil
	}

	// Correlate connector identities → canonical identity UUIDs by email.
	corr, err := s.correlateIdentities(ctx, tenantID, connectorID, res)
	if err != nil {
		return err
	}

	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Resource + Entitlement + Role nodes.
	for name, id := range resMap {
		_, err := session.Run(ctx, `
			MERGE (r:Resource {id: $id})
			SET r.name = $name, r.tenant_id = $tenant_id
		`, map[string]any{"id": id, "name": name, "tenant_id": tenantID})
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("neo4j resource node %s: %v", name, err))
			continue
		}
		res.GraphNodesWritten++
	}

	for key, id := range entMap {
		parts := strings.SplitN(key, "\x00", 2)
		app, level := "Microsoft Entra ID", key
		if len(parts) == 2 {
			app, level = parts[0], parts[1]
		}
		_, err := session.Run(ctx, `
			MERGE (e:Entitlement {id: $id})
			SET e.app_name = $app, e.permission_level = $level,
			    e.entitlement_type = $ent_type, e.tenant_id = $tenant_id,
			    e.risk_classification = $risk, e.is_toxic = $toxic
		`, map[string]any{
			"id": id, "app": app, "level": level, "ent_type": classifyEntType(app, level),
			"tenant_id": tenantID, "risk": classifyRisk(app, level), "toxic": classifyToxic(app, level),
		})
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("neo4j entitlement node %s: %v", key, err))
			continue
		}
		res.GraphNodesWritten++
	}

	for name, id := range roleMap {
		_, err := session.Run(ctx, `
			MERGE (r:Role {uuid: $id})
			SET r.name = $name, r.tenant_id = $tenant_id, r.is_active = true
		`, map[string]any{"id": id, "name": name, "tenant_id": tenantID})
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("neo4j role node %s: %v", name, err))
			continue
		}
		res.GraphNodesWritten++
	}

	// HAS_ENTITLEMENT / HAS_ROLE / ACCESSES edges, correlated by email.
	if err := s.materializeEdges(ctx, session, tenantID, connectorID, corr, entMap, resMap, roleMap, res); err != nil {
		return err
	}

	return nil
}

// correlateIdentities returns a map of connector external_id → canonical
// identity UUID, matched by normalized email. Uncorrelated users are counted
// (SailPoint's "orphan accounts" signal).
func (s *Store) correlateIdentities(ctx context.Context, tenantID, connectorID string, res *MaterializeResult) (map[string]string, error) {
	rows, err := s.pg.Query(ctx, `
		SELECT external_id, LOWER(email)
		FROM connector_identities
		WHERE connector_id = $1 AND email IS NOT NULL AND email <> ''
	`, connectorID)
	if err != nil {
		return nil, fmt.Errorf("query connector identities: %w", err)
	}
	defer rows.Close()

	type ext struct {
		externalID string
		email      string
	}
	var exts []ext
	for rows.Next() {
		var e ext
		if err := rows.Scan(&e.externalID, &e.email); err != nil {
			continue
		}
		exts = append(exts, e)
	}
	if len(exts) == 0 {
		return map[string]string{}, nil
	}

	// One query per batch: fetch canonical identity UUIDs by email in tenant.
	seen := map[string]string{}
	for i := 0; i < len(exts); i += 200 {
		end := i + 200
		if end > len(exts) {
			end = len(exts)
		}
		batch := exts[i:end]
		args := []any{tenantID}
		ph := make([]string, len(batch))
		for j, e := range batch {
			ph[j] = fmt.Sprintf("$%d", j+2)
			args = append(args, e.email)
		}
		rows, err := s.pg.Query(ctx, fmt.Sprintf(`
			SELECT email, id::text FROM identities
			WHERE tenant_id = $1 AND LOWER(email) IN (%s)
		`, strings.Join(ph, ",")), args...)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("correlate batch: %v", err))
			continue
		}
		emailToID := map[string]string{}
		for rows.Next() {
			var email, id string
			if err := rows.Scan(&email, &id); err == nil {
				emailToID[strings.ToLower(email)] = id
			}
		}
		rows.Close()

		for _, e := range batch {
			if id, ok := emailToID[e.email]; ok {
				seen[e.externalID] = id
			} else {
				res.UncorrelatedUsers++
			}
		}
	}
	return seen, nil
}

// materializeEdges writes HAS_ENTITLEMENT / HAS_ROLE / ACCESSES edges.
func (s *Store) materializeEdges(ctx context.Context, session neo4j.SessionWithContext, tenantID, connectorID string, corr map[string]string, entMap, resMap, roleMap map[string]string, res *MaterializeResult) error {
	// Entitlement → Resource via app_name.
	for key, entID := range entMap {
		parts := strings.SplitN(key, "\x00", 2)
		app := "Microsoft Entra ID"
		if len(parts) == 2 {
			app = parts[0]
		}
		if resID, ok := resMap[app]; ok {
			if _, err := session.Run(ctx, `
				MATCH (e:Entitlement {id: $ent_id})
				MATCH (r:Resource {id: $res_id})
				MERGE (e)-[:ACCESSES]->(r)
			`, map[string]any{"ent_id": entID, "res_id": resID}); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("neo4j access edge %s: %v", app, err))
				continue
			}
			res.GraphEdgesWritten++
		}
	}

	// Identity → Entitlement + Identity → Role.
	rows, err := s.pg.Query(ctx, `
		SELECT identity_external_id, app_name, source_name, source_id, entitlement_type, groups
		FROM connector_entitlements ce
		LEFT JOIN connector_identities ci
			ON ci.connector_id = ce.connector_id AND ci.external_id = ce.identity_external_id
		WHERE ce.connector_id = $1 AND ce.is_active = true
	`, connectorID)
	if err != nil {
		return fmt.Errorf("query connector entitlements for graph: %w", err)
	}
	defer rows.Close()

	seenEnt := map[string]bool{}
	for rows.Next() {
		var extID, app, sourceName, sourceID, entType string
		var groups []string
		if err := rows.Scan(&extID, &app, &sourceName, &sourceID, &entType, &groups); err != nil {
			continue
		}
		identityUUID, ok := corr[extID]
		if !ok {
			continue
		}
		if app == "" {
			app = "Microsoft Entra ID"
		}
		level := sourceName
		if level == "" {
			level = sourceID
		}
		if level == "" {
			level = entType
		}
		entID, ok := entMap[app+"\x00"+level]
		if !ok {
			continue
		}
		edgeKey := identityUUID + "|" + entID
		if seenEnt[edgeKey] {
			continue
		}
		seenEnt[edgeKey] = true
		if _, err := session.Run(ctx, `
			MATCH (i:Identity {uuid: $uid})
			MATCH (e:Entitlement {id: $eid})
			MERGE (i)-[:HAS_ENTITLEMENT]->(e)
		`, map[string]any{"uid": identityUUID, "eid": entID}); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("neo4j entitlement edge: %v", err))
			continue
		}
		res.GraphEdgesWritten++
	}

	// Identity → Role from connector group memberships.
	grows, err := s.pg.Query(ctx, `
		SELECT g.name, ci.external_id
		FROM connector_groups g
		JOIN connector_identities ci ON ci.connector_id = g.connector_id
		WHERE g.connector_id = $1 AND ci.external_id = ANY(g.member_ids)
	`, connectorID)
	if err != nil {
		return fmt.Errorf("query group memberships: %w", err)
	}
	defer grows.Close()

	seenRole := map[string]bool{}
	for grows.Next() {
		var groupName, extID string
		if err := grows.Scan(&groupName, &extID); err != nil {
			continue
		}
		identityUUID, ok := corr[extID]
		if !ok {
			continue
		}
		roleID, ok := roleMap[groupName]
		if !ok {
			continue
		}
		edgeKey := identityUUID + "|" + roleID
		if seenRole[edgeKey] {
			continue
		}
		seenRole[edgeKey] = true
		if _, err := session.Run(ctx, `
			MATCH (i:Identity {uuid: $uid})
			MATCH (r:Role {uuid: $rid})
			MERGE (i)-[:HAS_ROLE]->(r)
		`, map[string]any{"uid": identityUUID, "rid": roleID}); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("neo4j role edge: %v", err))
			continue
		}
		res.GraphEdgesWritten++
	}

	return nil
}

// ─── Risk Classification (SailPoint-style) ──────────────────

func classifyEntitlement(entType, app, level string) (risk string, toxic, rubberband bool) {
	risk = classifyRisk(app, level)
	toxic = classifyToxic(app, level)
	// Directory roles and app roles assigned outside a governing role are
	// rubber-band entitlements until a role governs them.
	rubberband = entType == "directory_role" || entType == "app_role"
	return risk, toxic, rubberband
}

func classifyEntType(app, level string) string {
	l := strings.ToLower(level)
	switch {
	case strings.Contains(l, "administrator"), strings.Contains(l, "admin"):
		return "directory_role"
	case strings.Contains(l, ".all"), strings.Contains(l, "readwrite"), strings.Contains(l, "write"):
		return "oauth2_permission"
	default:
		return "app_role"
	}
}

func classifyRisk(app, level string) string {
	l := strings.ToLower(level)
	switch {
	case strings.Contains(l, "global administrator"),
		strings.Contains(l, "privileged role administrator"),
		strings.Contains(l, "application administrator"),
		strings.Contains(l, "hybrid identity administrator"),
		strings.Contains(l, "conditional access administrator"),
		strings.Contains(l, "security administrator"),
		strings.Contains(l, ".readwrite.all"),
		strings.Contains(l, ".write.all"),
		strings.Contains(l, "rolemanagement.readwrite"):
		return "critical"
	case strings.Contains(l, "administrator"),
		strings.Contains(l, "admin"),
		strings.Contains(l, ".read.all"),
		strings.Contains(l, ".write"),
		strings.Contains(l, "user administrator"),
		strings.Contains(l, "exchange administrator"):
		return "high"
	case strings.Contains(l, "read"), strings.Contains(l, "member"):
		return "medium"
	default:
		return "medium"
	}
}

func classifyToxic(app, level string) bool {
	l := strings.ToLower(level)
	switch {
	case strings.Contains(l, "global administrator"),
		strings.Contains(l, "privileged role administrator"),
		strings.Contains(l, "application administrator"),
		strings.Contains(l, "hybrid identity administrator"),
		strings.Contains(l, "conditional access administrator"),
		strings.Contains(l, "security administrator"),
		strings.Contains(l, ".readwrite.all"),
		strings.Contains(l, ".write.all"),
		strings.Contains(l, "rolemanagement.readwrite"),
		strings.Contains(l, "user administrator"),
		strings.Contains(l, "exchange administrator"):
		return true
	default:
		return false
	}
}

func resourceCriticality(rtype string) string {
	switch strings.ToLower(rtype) {
	case "service_principal", "application":
		return "p2"
	case "device":
		return "p3"
	default:
		return "p3"
	}
}