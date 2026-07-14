// ─── ObserveID Reimagined Neo4j Graph Schema ──────────────
// Identity Fabric graph initialization

// ─── Constraints (Uniqueness) ────────────────────────────────
CREATE CONSTRAINT identity_uuid IF NOT EXISTS FOR (i:Identity) REQUIRE i.uuid IS UNIQUE;
CREATE CONSTRAINT identity_email IF NOT EXISTS FOR (i:Identity) REQUIRE (i.tenant_id, i.email) IS UNIQUE;
CREATE CONSTRAINT nhi_uuid IF NOT EXISTS FOR (n:NonHumanIdentity) REQUIRE n.uuid IS UNIQUE;
CREATE CONSTRAINT entitlement_uuid IF NOT EXISTS FOR (e:Entitlement) REQUIRE e.uuid IS UNIQUE;
CREATE CONSTRAINT resource_uuid IF NOT EXISTS FOR (r:Resource) REQUIRE r.uuid IS UNIQUE;
CREATE CONSTRAINT role_uuid IF NOT EXISTS FOR (r:Role) REQUIRE r.uuid IS UNIQUE;
CREATE CONSTRAINT session_uuid IF NOT EXISTS FOR (s:Session) REQUIRE s.session_id IS UNIQUE;
CREATE CONSTRAINT policy_uuid IF NOT EXISTS FOR (p:Policy) REQUIRE p.uuid IS UNIQUE;

// ─── Indexes (Performance) ───────────────────────────────────
// Primary lookup
CREATE INDEX identity_lookup IF NOT EXISTS FOR (i:Identity) ON (i.tenant_id, i.status);
CREATE INDEX identity_email_idx IF NOT EXISTS FOR (i:Identity) ON (i.email);
CREATE INDEX identity_department IF NOT EXISTS FOR (i:Identity) ON (i.department);

// NIH indexes
CREATE INDEX nhi_owner IF NOT EXISTS FOR (n:NonHumanIdentity) ON (n.owner_id);
CREATE INDEX nhi_status IF NOT EXISTS FOR (n:NonHumanIdentity) ON (n.status);
CREATE INDEX nhi_governed IF NOT EXISTS FOR (n:NonHumanIdentity) ON (n.is_governed);

// Entitlement indexes
CREATE INDEX entitlement_app IF NOT EXISTS FOR (e:Entitlement) ON (e.app_name);
CREATE INDEX entitlement_toxic IF NOT EXISTS FOR (e:Entitlement) ON (e.is_toxic);
CREATE INDEX entitlement_risk IF NOT EXISTS FOR (e:Entitlement) ON (e.risk_classification);

// Resource indexes
CREATE INDEX resource_type IF NOT EXISTS FOR (r:Resource) ON (r.type);
CREATE INDEX resource_criticality IF NOT EXISTS FOR (r:Resource) ON (r.criticality);
CREATE INDEX resource_classification IF NOT EXISTS FOR (r:Resource) ON (r.data_classification);

// Role indexes
CREATE INDEX role_name IF NOT EXISTS FOR (r:Role) ON (r.name);

// Session indexes
CREATE INDEX session_active IF NOT EXISTS FOR (s:Session) ON (s.is_active);
CREATE INDEX session_identity IF NOT EXISTS FOR (s:Session) ON (s.identity_uuid);

// Full-text search for identity discovery
CREATE FULLTEXT INDEX identity_search IF NOT EXISTS FOR (i:Identity) ON EACH [i.display_name, i.email];
CREATE FULLTEXT INDEX nhi_search IF NOT EXISTS FOR (n:NonHumanIdentity) ON EACH [n.name];

// ─── Composite Index for Temporal Access Evaluation ─────────
CREATE INDEX access_eval IF NOT EXISTS FOR ()-[r:HAS_ROLE]-() ON (r.expires_at, r.is_active);
CREATE INDEX delegation_depth IF NOT EXISTS FOR ()-[r:DELEGATED_FROM]-() ON (r.max_depth_remaining);

// ─── Seed Data ───────────────────────────────────────────────
// Default tenant admin identity
CREATE (admin:Identity {
    uuid: "00000000-0000-0000-0000-000000000002",
    tenant_id: "00000000-0000-0000-0000-000000000001",
    status: "active",
    type: "human",
    risk_score: 0.0,
    risk_factors: [],
    department: "Engineering",
    email: "admin@observeid.io",
    display_name: "System Admin",
    employee_id: "ADMIN-001",
    employment_type: "employee",
    assurance_level: "aal2",
    created_at: datetime(),
    last_reviewed_at: datetime(),
    last_accessed_at: datetime()
})

// Roles
CREATE (adminRole:Role {
    uuid: "00000000-0000-0000-0000-000000000010",
    name: "Administrator",
    description: "Full system access",
    type: "technical",
    is_auto_assigned: false,
    is_active: true
})

CREATE (secRole:Role {
    uuid: "00000000-0000-0000-0000-000000000011",
    name: "Security Reviewer",
    description: "Access review permissions",
    type: "business",
    is_auto_assigned: false,
    is_active: true
})

CREATE (auditorRole:Role {
    uuid: "00000000-0000-0000-0000-000000000012",
    name: "Auditor",
    description: "Read-only audit access",
    type: "business",
    is_auto_assigned: false,
    is_active: true
})

CREATE (engRole:Role {
    uuid: "00000000-0000-0000-0000-000000000013",
    name: "Engineer",
    description: "Standard engineering access",
    type: "technical",
    is_auto_assigned: true,
    is_active: true
})

CREATE (contractorRole:Role {
    uuid: "00000000-0000-0000-0000-000000000014",
    name: "Contractor",
    description: "Limited contractor access",
    type: "business",
    is_auto_assigned: false,
    approval_required: true,
    is_active: true
})

// Resources (target systems)
CREATE (aws:Resource {
    uuid: "00000000-0000-0000-0000-000000000020",
    name: "AWS Production",
    type: "cloud",
    criticality: "p0",
    data_classification: "critical",
    owner_team: "Platform Engineering",
    connection_type: "iam"
})

CREATE (okta_instance:Resource {
    uuid: "00000000-0000-0000-0000-000000000021",
    name: "Okta",
    type: "saas",
    criticality: "p1",
    data_classification: "critical",
    owner_team: "IT",
    connection_type: "scim"
})

CREATE (github:Resource {
    uuid: "00000000-0000-0000-0000-000000000022",
    name: "GitHub Enterprise",
    type: "saas",
    criticality: "p1",
    data_classification: "internal",
    owner_team: "Engineering",
    connection_type: "oauth"
})

CREATE (slack:Resource {
    uuid: "00000000-0000-0000-0000-000000000023",
    name: "Slack",
    type: "saas",
    criticality: "p2",
    data_classification: "internal",
    owner_team: "IT",
    connection_type: "scim"
})

CREATE (snowflake:Resource {
    uuid: "00000000-0000-0000-0000-000000000024",
    name: "Snowflake",
    type: "saas",
    criticality: "p1",
    data_classification: "pii",
    owner_team: "Data Engineering",
    connection_type: "oauth"
})

// Entitlements (Permissions)
CREATE (adminEnt:Entitlement {
    uuid: "00000000-0000-0000-0000-000000000030",
    app_name: "AWS",
    permission_level: "AdministratorAccess",
    entitlement_type: "admin",
    is_toxic: true,
    risk_classification: "critical"
})

CREATE (s3Read:Entitlement {
    uuid: "00000000-0000-0000-0000-000000000031",
    app_name: "AWS",
    permission_level: "AmazonS3ReadOnlyAccess",
    entitlement_type: "read",
    is_toxic: false,
    risk_classification: "low"
})

CREATE (oktaAdmin:Entitlement {
    uuid: "00000000-0000-0000-0000-000000000032",
    app_name: "Okta",
    permission_level: "SuperAdmin",
    entitlement_type: "admin",
    is_toxic: true,
    risk_classification: "critical"
})

CREATE (githubWrite:Entitlement {
    uuid: "00000000-0000-0000-0000-000000000033",
    app_name: "GitHub",
    permission_level: "Write",
    entitlement_type: "write",
    is_toxic: false,
    risk_classification: "medium"
})

CREATE (githubRead:Entitlement {
    uuid: "00000000-0000-0000-0000-000000000034",
    app_name: "GitHub",
    permission_level: "Read",
    entitlement_type: "read",
    is_toxic: false,
    risk_classification: "low"
})

CREATE (slackWrite:Entitlement {
    uuid: "00000000-0000-0000-0000-000000000035",
    app_name: "Slack",
    permission_level: "User",
    entitlement_type: "write",
    is_toxic: false,
    risk_classification: "low"
})

CREATE (snowflakeRead:Entitlement {
    uuid: "00000000-0000-0000-0000-000000000036",
    app_name: "Snowflake",
    permission_level: "ReadOnly",
    entitlement_type: "read",
    is_toxic: false,
    risk_classification: "high"
})

// ─── Relationships ───────────────────────────────────────────

// Roles → Entitlements
CREATE (adminRole)-[:GRANTS {assigned_at: datetime()}]->(adminEnt)
CREATE (adminRole)-[:GRANTS {assigned_at: datetime()}]->(oktaAdmin)
CREATE (adminRole)-[:GRANTS {assigned_at: datetime()}]->(aws)
CREATE (adminRole)-[:GRANTS {assigned_at: datetime()}]->(okta_instance)
CREATE (adminRole)-[:GRANTS {assigned_at: datetime()}]->(github)
CREATE (adminRole)-[:GRANTS {assigned_at: datetime()}]->(slack)
CREATE (adminRole)-[:GRANTS {assigned_at: datetime()}]->(snowflake)

CREATE (engRole)-[:GRANTS {assigned_at: datetime()}]->(s3Read)
CREATE (engRole)-[:GRANTS {assigned_at: datetime()}]->(githubWrite)
CREATE (engRole)-[:GRANTS {assigned_at: datetime()}]->(slackWrite)

CREATE (contractorRole)-[:GRANTS {assigned_at: datetime()}]->(githubRead)
CREATE (contractorRole)-[:GRANTS {assigned_at: datetime()}]->(slackWrite)

// Admin → Administrator Role
CREATE (admin)-[:HAS_ROLE {
    assigned_at: datetime(),
    assigned_by: admin.uuid,
    source: "direct",
    is_active: true
}]->(adminRole)

// Entitlements → Resources
CREATE (adminEnt)-[:ACCESSES {access_type: "direct"}]->(aws)
CREATE (s3Read)-[:ACCESSES {access_type: "direct"}]->(aws)
CREATE (oktaAdmin)-[:ACCESSES {access_type: "direct"}]->(okta_instance)
CREATE (githubWrite)-[:ACCESSES {access_type: "direct"}]->(github)
CREATE (githubRead)-[:ACCESSES {access_type: "direct"}]->(github)
CREATE (slackWrite)-[:ACCESSES {access_type: "direct"}]->(slack)
CREATE (snowflakeRead)-[:ACCESSES {access_type: "direct"}]->(snowflake)

// SoD Conflict Example: Admin Access conflicting with Read-Only
CREATE (adminEnt)-[:CONFLICTS_WITH {
    conflict_type: "privilege_escalation",
    severity: "critical",
    detected_at: datetime()
}]->(s3Read)

// ─── Sample Identity for Testing ─────────────────────────────
CREATE (alice:Identity {
    uuid: "00000000-0000-0000-0000-000000000100",
    tenant_id: "00000000-0000-0000-0000-000000000001",
    status: "active",
    type: "human",
    risk_score: 0.15,
    risk_factors: [],
    department: "Engineering",
    email: "alice@observeid.io",
    display_name: "Alice Johnson",
    employee_id: "EMP-002",
    employment_type: "employee",
    assurance_level: "aal2",
    created_at: datetime(),
    last_reviewed_at: datetime(),
    last_accessed_at: datetime()
})

CREATE (alice)-[:HAS_ROLE {
    assigned_at: datetime(),
    assigned_by: admin.uuid,
    source: "direct",
    is_active: true
}]->(engRole)

// ─── Sample NHI (AI Agent) ──────────────────────────────────
CREATE (deployBot:NonHumanIdentity {
    uuid: "00000000-0000-0000-0000-000000000200",
    tenant_id: "00000000-0000-0000-0000-000000000001",
    type: "ai_agent",
    status: "active",
    name: "deploy-bot",
    owner_id: alice.uuid,
    team_id: "Engineering",
    protocol: ["mcp", "a2a"],
    capabilities: ["read:github", "write:github", "read:aws"],
    is_governed: true,
    framework: "openai",
    deployment_environment: "production",
    risk_score: 0.35
})

CREATE (deployBot)-[:OWNED_BY {ownership_type: "primary"}]->(alice)
CREATE (deployBot)-[:HAS_ROLE {
    assigned_at: datetime(),
    assigned_by: admin.uuid,
    source: "direct",
    is_active: true
}]->(engRole)
