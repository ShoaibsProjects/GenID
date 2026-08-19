package stores

import (
	"context"
	"strconv"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// BlastRadiusGraph returns the graph-native view of an identity's blast
// radius: every node (Identity/NonHumanIdentity, Role, Entitlement, Resource)
// and every relationship (HAS_ROLE, DIRECTLY_OWNS, DELEGATED_FROM, ACCESSES)
// on the paths, deduplicated. Powers the force-graph visualization.
func (s *Store) BlastRadiusGraph(ctx context.Context, id string) map[string]any {
	session := s.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	nodes := map[string]map[string]any{}
	var links []map[string]any
	type linkKey struct{ s, t, typ string }
	seenLinks := map[linkKey]bool{}

	addNode := func(m map[string]any) {
		if nid, ok := m["id"].(string); ok && nid != "" {
			if _, exists := nodes[nid]; !exists {
				nodes[nid] = m
			}
		}
	}

	// Center node — Identity or NonHumanIdentity (agents have blast radius too)
	centerRes, err := session.Run(ctx, `
		MATCH (i {uuid: $id}) WHERE i:Identity OR i:NonHumanIdentity
		RETURN i.uuid AS id,
		       COALESCE(i.display_name, i.name, i.email, i.uuid) AS label,
		       labels(i)[0] AS type
	`, map[string]any{"id": id})
	if err == nil {
		for centerRes.Next(ctx) {
			rec := centerRes.Record()
			addNode(map[string]any{
				"id":    GetRecordVal(rec, "id"),
				"label": GetRecordVal(rec, "label"),
				"type":  GetRecordVal(rec, "type"),
			})
		}
	}

	// Paths → nodes + links
	pathRes, err := session.Run(ctx, `
		MATCH (i {uuid: $id}) WHERE i:Identity OR i:NonHumanIdentity
		OPTIONAL MATCH pathRole = (i)-[:HAS_ROLE]->(:Role)-[:GRANTS]->(e:Entitlement)-[:ACCESSES]->(r:Resource)
		OPTIONAL MATCH pathDirectEnt = (i)-[:HAS_ENTITLEMENT]->(e2:Entitlement)-[:ACCESSES]->(r2:Resource)
		OPTIONAL MATCH pathDirect = (i)-[:HAS_DIRECT_ACCESS]->(r3:Resource)
		OPTIONAL MATCH pathTemp = (i)-[:HAS_TEMPORARY_ACCESS]->(r4:Resource)
		WITH i,
		     COLLECT(DISTINCT CASE WHEN pathRole IS NOT NULL THEN pathRole END) AS rolePaths,
		     COLLECT(DISTINCT CASE WHEN pathDirectEnt IS NOT NULL THEN pathDirectEnt END) AS directEntPaths,
		     COLLECT(DISTINCT CASE WHEN pathDirect IS NOT NULL THEN pathDirect END) AS directPaths,
		     COLLECT(DISTINCT CASE WHEN pathTemp IS NOT NULL THEN pathTemp END) AS tempPaths
		WITH [p IN rolePaths + directEntPaths + directPaths + tempPaths WHERE p IS NOT NULL] AS paths
		UNWIND paths AS path
		RETURN [n IN nodes(path) | {
			id: n.uuid,
			label: COALESCE(n.display_name, n.name, n.app_name, n.email, n.uuid),
			type: labels(n)[0],
			criticality: n.criticality,
			permission_level: n.permission_level
		}] AS ns,
		[rel IN relationships(path) | {
			source: startNode(rel).uuid,
			target: endNode(rel).uuid,
			type: type(rel)
		}] AS rs
		LIMIT 200
	`, map[string]any{"id": id})
	if err != nil {
		return map[string]any{"nodes": nodes, "links": links}
	}

	for pathRes.Next(ctx) {
		rec := pathRes.Record()
		if nsRaw, ok := rec.Get("ns"); ok {
			if nsList, ok := nsRaw.([]any); ok {
				for _, nRaw := range nsList {
					if nMap, ok := nRaw.(map[string]any); ok {
						addNode(nMap)
					}
				}
			}
		}
		if rsRaw, ok := rec.Get("rs"); ok {
			if rsList, ok := rsRaw.([]any); ok {
				for _, lRaw := range rsList {
					if lMap, ok := lRaw.(map[string]any); ok {
						src, _ := lMap["source"].(string)
						tgt, _ := lMap["target"].(string)
						typ, _ := lMap["type"].(string)
						k := linkKey{src, tgt, typ}
						if src != "" && tgt != "" && !seenLinks[k] {
							seenLinks[k] = true
							links = append(links, map[string]any{"source": src, "target": tgt, "type": typ})
						}
					}
				}
			}
		}
	}

	nodeList := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		nodeList = append(nodeList, n)
	}
	if links == nil {
		links = []map[string]any{}
	}
	return map[string]any{"nodes": nodeList, "links": links}
}

// ─── Neo4j record helpers ─────────────────────────────────

// GetRecordString extracts a Neo4j value as a string ("" when absent).
func GetRecordString(record *neo4j.Record, key string) string {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

// GetRecordStrings extracts a Neo4j list value as []string (nil when absent).
func GetRecordStrings(record *neo4j.Record, key string) []string {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return nil
	}
	if list, ok := val.([]any); ok {
		result := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// GetRecordVal extracts a Neo4j value as a string, rendering scalar types
// (int64, float64, bool) to their string form; "" when absent.
func GetRecordVal(record *neo4j.Record, key string) string {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return ""
	}
}

// GetRecordFloat extracts a numeric Neo4j value as float64 (0 when absent).
func GetRecordFloat(record *neo4j.Record, key string) float64 {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	default:
		return 0
	}
}

// GetRecordBool extracts a boolean Neo4j value (also accepts "true"/"false"
// strings).
func GetRecordBool(record *neo4j.Record, key string) bool {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	default:
		return false
	}
}
