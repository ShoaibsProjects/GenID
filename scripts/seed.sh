#!/bin/bash
# ─── ObserveID Reimagined Seed Data Loader ───────────────
set -e

echo "🌱 Loading seed data..."

# Seed PostgreSQL
echo "  → PostgreSQL: seeding identities and roles..."
docker exec -i observeid-postgres psql -U observeid -d observeid <<'SQL'
INSERT INTO identities (id, tenant_id, type, status, email, display_name, department, employee_id, source, assurance_level)
VALUES
  ('a0000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'human', 'active', 'alice@observeid.io', 'Alice Johnson', 'Engineering', 'EMP-002', 'hris', 'aal2'),
  ('a0000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 'human', 'active', 'bob@observeid.io', 'Bob Smith', 'Engineering', 'EMP-003', 'scim', 'aal2'),
  ('a0000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'human', 'active', 'charlie@observeid.io', 'Charlie Davis', 'Finance', 'EMP-004', 'hris', 'aal2'),
  ('a0000000-0000-0000-0000-000000000004', '00000000-0000-0000-0000-000000000001', 'human', 'active', 'diana@partner.com', 'Diana Moore', 'Engineering', 'CON-001', 'scim', 'aal1'),
  ('a0000000-0000-0000-0000-000000000005', '00000000-0000-0000-0000-000000000001', 'human', 'active', 'eve@observeid.io', 'Eve Wilson', 'HR', 'EMP-005', 'manual', 'aal2')
ON CONFLICT (tenant_id, email) DO NOTHING;

INSERT INTO non_human_identities (id, tenant_id, name, type, status, owner_id, capabilities, is_governed)
VALUES
  ('b0000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'deploy-bot', 'ai_agent', 'active', 'a0000000-0000-0000-0000-000000000001', ARRAY['read:github','write:github'], true),
  ('b0000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 'doc-analyzer', 'ai_agent', 'active', 'a0000000-0000-0000-0000-000000000001', ARRAY['read:docs','read:database'], true),
  ('b0000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'prod-db-reader', 'service_account', 'active', 'a0000000-0000-0000-0000-000000000003', ARRAY['read:database'], false)
ON CONFLICT (tenant_id, name) DO NOTHING;
SQL

echo "  ✅ PostgreSQL seed complete"

# Seed Neo4j
echo "  → Neo4j: seeding graph data..."
docker exec -i observeid-neo4j cypher-shell -u neo4j -p observeid123 <<'CYPHER'
CREATE (alice:Identity {
  uuid: "a0000000-0000-0000-0000-000000000001",
  tenant_id: "00000000-0000-0000-0000-000000000001",
  display_name: "Alice Johnson",
  email: "alice@observeid.io",
  department: "Engineering",
  status: "active",
  type: "human",
  risk_score: 0.15,
  assurance_level: "aal2"
})
CREATE (deployBot:NonHumanIdentity {
  uuid: "b0000000-0000-0000-0000-000000000001",
  name: "deploy-bot",
  type: "ai_agent",
  status: "active",
  risk_score: 0.35,
  is_governed: true,
  protocols: ["mcp", "a2a"],
  capabilities: ["read:github", "write:github"]
})
CREATE (deployBot)-[:OWNED_BY {ownership_type: "primary"}]->(alice)
RETURN "Seeded: " + alice.display_name + " and " + deployBot.name
CYPHER

echo "  ✅ Neo4j seed complete"
echo ""
echo "🌱 Seed data loaded successfully!"
echo ""
