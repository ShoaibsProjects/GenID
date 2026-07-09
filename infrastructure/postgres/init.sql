-- ─── ObserveID PostgreSQL Schema ─────────────────────────────
-- Core tables for the Identity Fabric

-- Extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "btree_gin";

-- ─── Identity Types ─────────────────────────────────────────
CREATE TYPE identity_type AS ENUM (
    'human', 'service_account', 'ai_agent', 'robot', 'iot_device', 'rpa_bot', 'api_key'
);

CREATE TYPE identity_status AS ENUM (
    'active', 'inactive', 'suspended', 'terminated', 'revoked', 'pending_review'
);

CREATE TYPE identity_source AS ENUM (
    'hris', 'scim', 'manual', 'agent_registration', 'discovery', 'ldap', 'saml'
);

-- ─── Tenant ─────────────────────────────────────────────────
CREATE TABLE tenants (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(255) NOT NULL,
    slug        VARCHAR(100) UNIQUE NOT NULL,
    tier        VARCHAR(50) NOT NULL DEFAULT 'starter',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    settings    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tenants_slug ON tenants(slug);

-- ─── Identity (Human) ───────────────────────────────────────
CREATE TABLE identities (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    type            identity_type NOT NULL DEFAULT 'human',
    status          identity_status NOT NULL DEFAULT 'active',
    email           VARCHAR(320) NOT NULL,
    display_name    VARCHAR(255) NOT NULL,
    department      VARCHAR(255),
    employee_id     VARCHAR(100),
    manager_id      UUID REFERENCES identities(id),
    source          identity_source NOT NULL DEFAULT 'manual',
    risk_score      DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    risk_factors    TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    assurance_level VARCHAR(10) NOT NULL DEFAULT 'aal1',
    attributes      JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed_at TIMESTAMPTZ,
    last_reviewed_at TIMESTAMPTZ,
    UNIQUE(tenant_id, email)
);

CREATE INDEX idx_identities_tenant      ON identities(tenant_id);
CREATE INDEX idx_identities_status      ON identities(status);
CREATE INDEX idx_identities_type        ON identities(type);
CREATE INDEX idx_identities_department  ON identities(department);
CREATE INDEX idx_identities_employee    ON identities(employee_id);
CREATE INDEX idx_identities_manager     ON identities(manager_id);
CREATE INDEX idx_identities_risk_score  ON identities(risk_score DESC);
CREATE INDEX idx_identities_lookup      ON identities(tenant_id, email, status);
-- Full-text search index
CREATE INDEX idx_identities_search ON identities USING GIN(
    to_tsvector('english', coalesce(display_name, '') || ' ' || coalesce(email, ''))
);

-- ─── Non-Human Identity (NHI / Agents) ─────────────────────
CREATE TABLE non_human_identities (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id            UUID NOT NULL REFERENCES tenants(id),
    name                 VARCHAR(255) NOT NULL,
    type                 identity_type NOT NULL DEFAULT 'service_account',
    status               identity_status NOT NULL DEFAULT 'active',
    agent_card_id        VARCHAR(255),
    protocols            TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    owner_id             UUID REFERENCES identities(id),
    team_id              VARCHAR(255),
    created_by           UUID REFERENCES identities(id),
    deployment_environment VARCHAR(50) DEFAULT 'production',
    framework            VARCHAR(100),
    capabilities         TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    is_governed          BOOLEAN NOT NULL DEFAULT FALSE,
    secrets_age_days     INTEGER NOT NULL DEFAULT 0,
    last_rotated_at      TIMESTAMPTZ,
    expires_at           TIMESTAMPTZ,
    risk_score           DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    attributes           JSONB NOT NULL DEFAULT '{}',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_nhi_tenant      ON non_human_identities(tenant_id);
CREATE INDEX idx_nhi_owner       ON non_human_identities(owner_id);
CREATE INDEX idx_nhi_status      ON non_human_identities(status);
CREATE INDEX idx_nhi_type        ON non_human_identities(type);
CREATE INDEX idx_nhi_ungoverned  ON non_human_identities(is_governed) WHERE is_governed = FALSE;
CREATE INDEX idx_nhi_expiring    ON non_human_identities(expires_at) WHERE expires_at IS NOT NULL;

-- ─── Roles ──────────────────────────────────────────────────
CREATE TABLE roles (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id          UUID NOT NULL REFERENCES tenants(id),
    name               VARCHAR(255) NOT NULL,
    description        TEXT,
    role_type          VARCHAR(50) NOT NULL DEFAULT 'business',
    is_auto_assigned   BOOLEAN NOT NULL DEFAULT FALSE,
    approval_required  BOOLEAN NOT NULL DEFAULT FALSE,
    max_duration_hours INTEGER,
    created_by_rolmining BOOLEAN NOT NULL DEFAULT FALSE,
    confidence_score   DOUBLE PRECISION,
    is_active          BOOLEAN NOT NULL DEFAULT TRUE,
    attributes         JSONB NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_roles_tenant ON roles(tenant_id);

-- ─── Entitlements / Permissions ────────────────────────────
CREATE TABLE entitlements (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id            UUID NOT NULL REFERENCES tenants(id),
    app_name             VARCHAR(255) NOT NULL,
    permission_level     VARCHAR(255) NOT NULL,
    entitlement_type     VARCHAR(50) NOT NULL DEFAULT 'read',
    risk_classification  VARCHAR(50) NOT NULL DEFAULT 'medium',
    is_toxic             BOOLEAN NOT NULL DEFAULT FALSE,
    is_rubberband        BOOLEAN NOT NULL DEFAULT FALSE,
    last_used_at         TIMESTAMPTZ,
    usage_count_90d      INTEGER NOT NULL DEFAULT 0,
    attributes           JSONB NOT NULL DEFAULT '{}',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, app_name, permission_level)
);

CREATE INDEX idx_entitlements_tenant  ON entitlements(tenant_id);
CREATE INDEX idx_entitlements_app     ON entitlements(app_name);
CREATE INDEX idx_entitlements_toxic   ON entitlements(is_toxic) WHERE is_toxic = TRUE;

-- ─── Resources (Applications/Services) ─────────────────────
CREATE TABLE resources (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id          UUID NOT NULL REFERENCES tenants(id),
    name               VARCHAR(255) NOT NULL,
    resource_type      VARCHAR(100) NOT NULL,
    criticality        VARCHAR(10) NOT NULL DEFAULT 'p3',
    data_classification VARCHAR(50) NOT NULL DEFAULT 'internal',
    owner_team         VARCHAR(255),
    connection_type    VARCHAR(50),
    health_status      VARCHAR(50) NOT NULL DEFAULT 'unknown',
    attributes         JSONB NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_resources_tenant        ON resources(tenant_id);
CREATE INDEX idx_resources_criticality   ON resources(criticality);
CREATE INDEX idx_resources_classification ON resources(data_classification);

-- ─── Identity-Role Assignments ─────────────────────────────
CREATE TABLE identity_roles (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id          UUID NOT NULL REFERENCES tenants(id),
    identity_id        UUID NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    role_id            UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_by        UUID REFERENCES identities(id),
    assigned_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at         TIMESTAMPTZ,
    approved_by        UUID REFERENCES identities(id),
    approval_ticket    VARCHAR(255),
    source             VARCHAR(50) NOT NULL DEFAULT 'direct',
    is_active          BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE(tenant_id, identity_id, role_id, source)
);

CREATE INDEX idx_idroles_identity      ON identity_roles(identity_id);
CREATE INDEX idx_idroles_role          ON identity_roles(role_id);
CREATE INDEX idx_idroles_active        ON identity_roles(is_active) WHERE is_active = TRUE;

-- ─── Role-Entitlement Assignments ──────────────────────────
CREATE TABLE role_entitlements (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id        UUID NOT NULL REFERENCES tenants(id),
    role_id          UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    entitlement_id   UUID NOT NULL REFERENCES entitlements(id) ON DELETE CASCADE,
    condition        TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, role_id, entitlement_id)
);

-- ─── Direct Entitlements (Toxic if not via Role) ──────────
CREATE TABLE direct_entitlements (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id),
    identity_id       UUID NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    entitlement_id    UUID NOT NULL REFERENCES entitlements(id) ON DELETE CASCADE,
    assigned_by       UUID REFERENCES identities(id),
    assigned_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_exception      BOOLEAN NOT NULL DEFAULT FALSE,
    exception_approved_by UUID REFERENCES identities(id),
    exception_expires_at  TIMESTAMPTZ,
    reason            TEXT,
    UNIQUE(tenant_id, identity_id, entitlement_id)
);

CREATE INDEX idx_direct_toxic ON direct_entitlements(is_exception) WHERE is_exception = FALSE;

-- ─── NHI Role Entitlements (Delegation Chains) ────────────
CREATE TABLE delegation_chains (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id           UUID NOT NULL REFERENCES tenants(id),
    parent_identity_id  UUID NOT NULL REFERENCES non_human_identities(id) ON DELETE CASCADE,
    child_identity_id   UUID NOT NULL REFERENCES non_human_identities(id) ON DELETE CASCADE,
    scope_narrowing     TEXT[],
    max_depth_remaining INTEGER NOT NULL DEFAULT 1,
    delegated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at          TIMESTAMPTZ,
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE(parent_identity_id, child_identity_id)
);

-- ─── Sessions ──────────────────────────────────────────────
CREATE TABLE sessions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    identity_id     UUID NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    auth_method     VARCHAR(100) NOT NULL,
    assurance_level VARCHAR(10) NOT NULL DEFAULT 'aal1',
    device_id       VARCHAR(255),
    ip_address      INET,
    user_agent      TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_identity ON sessions(identity_id);
CREATE INDEX idx_sessions_active   ON sessions(is_active) WHERE is_active = TRUE;

-- ─── Outbox (Transactional Event Publication) ─────────────
CREATE TABLE outbox (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aggregate_id    VARCHAR(255) NOT NULL,
    aggregate_type  VARCHAR(100) NOT NULL,
    event_type      VARCHAR(255) NOT NULL,
    payload         JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ
);

CREATE INDEX idx_outbox_pending ON outbox(published_at) WHERE published_at IS NULL;
CREATE INDEX idx_outbox_aggregate ON outbox(aggregate_id);

-- ─── Audit Log ─────────────────────────────────────────────
CREATE TABLE audit_log (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    event_type      VARCHAR(255) NOT NULL,
    actor_id        VARCHAR(255),
    actor_type      VARCHAR(50),
    target_id       VARCHAR(255),
    target_type     VARCHAR(50),
    action          VARCHAR(255) NOT NULL,
    resource        VARCHAR(255),
    details         JSONB NOT NULL DEFAULT '{}',
    ip_address      INET,
    user_agent      TEXT,
    correlation_id  VARCHAR(255),
    trace_id        VARCHAR(255),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_tenant       ON audit_log(tenant_id);
CREATE INDEX idx_audit_event_type   ON audit_log(event_type);
CREATE INDEX idx_audit_actor        ON audit_log(actor_id);
CREATE INDEX idx_audit_created_at   ON audit_log(created_at DESC);
CREATE INDEX idx_audit_correlation  ON audit_log(correlation_id);
-- Partition by month for retention

-- ─── CAEP Events ───────────────────────────────────────────
CREATE TABLE caep_events (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id),
    event_type        VARCHAR(255) NOT NULL,
    event_jti         VARCHAR(255) UNIQUE NOT NULL,
    identity_id       UUID,
    session_id        UUID,
    initiating_entity VARCHAR(50) NOT NULL,
    reason_admin      TEXT,
    reason_user       TEXT,
    event_payload     JSONB NOT NULL DEFAULT '{}',
    delivery_status   VARCHAR(50) NOT NULL DEFAULT 'pending',
    delivered_to      TEXT[],
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at      TIMESTAMPTZ
);

CREATE INDEX idx_caep_type       ON caep_events(event_type);
CREATE INDEX idx_caep_identity   ON caep_events(identity_id);
CREATE INDEX idx_caep_status     ON caep_events(delivery_status);

-- ─── Cedar Policies ───────────────────────────────────────
CREATE TABLE cedar_policies (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    policy_id       VARCHAR(255) NOT NULL,
    effect          VARCHAR(10) NOT NULL CHECK (effect IN ('permit', 'forbid')),
    policy_source   TEXT NOT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    version         INTEGER NOT NULL DEFAULT 1,
    created_by      UUID REFERENCES identities(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, policy_id, version)
);

CREATE INDEX idx_cedar_tenant ON cedar_policies(tenant_id);

-- ─── Agent Cards ───────────────────────────────────────────
CREATE TABLE agent_cards (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    nh_identity_id  UUID NOT NULL UNIQUE REFERENCES non_human_identities(id) ON DELETE CASCADE,
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    card_type       VARCHAR(50) NOT NULL DEFAULT 'a2a',
    card_document   JSONB NOT NULL,
    signature       TEXT NOT NULL,
    signature_scheme VARCHAR(50) NOT NULL DEFAULT 'ml-dsa-44',
    is_valid        BOOLEAN NOT NULL DEFAULT TRUE,
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ,
    UNIQUE(nh_identity_id, card_type)
);

-- ─── Access Reviews / Certifications ──────────────────────
CREATE TABLE certification_campaigns (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    name            VARCHAR(255) NOT NULL,
    campaign_type   VARCHAR(50) NOT NULL,  -- quarterly, triggered, emergency
    status          VARCHAR(50) NOT NULL DEFAULT 'draft',
    starts_at       TIMESTAMPTZ NOT NULL,
    ends_at         TIMESTAMPTZ NOT NULL,
    created_by      UUID REFERENCES identities(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE certification_entries (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id     UUID NOT NULL REFERENCES certification_campaigns(id) ON DELETE CASCADE,
    identity_id     UUID NOT NULL REFERENCES identities(id),
    certifier_id    UUID REFERENCES identities(id),
    status          VARCHAR(50) NOT NULL DEFAULT 'pending',
    decision        VARCHAR(50),  -- certified, revoked, modified
    notes           TEXT,
    decided_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── SoD Rules ─────────────────────────────────────────────
CREATE TABLE sod_rules (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id               UUID NOT NULL REFERENCES tenants(id),
    name                    VARCHAR(255) NOT NULL,
    description             TEXT,
    conflicting_entitlements UUID[][] NOT NULL,
    severity                VARCHAR(50) NOT NULL DEFAULT 'high',
    is_active               BOOLEAN NOT NULL DEFAULT TRUE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

-- ─── Emergency Access (Break-Glass) ───────────────────────
CREATE TABLE emergency_access (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    identity_id     UUID NOT NULL REFERENCES identities(id),
    resource_id     UUID REFERENCES resources(id),
    reason          TEXT NOT NULL,
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    granted_by      UUID REFERENCES identities(id),
    granted_at      TIMESTAMPTZ,
    reviewed_by     UUID REFERENCES identities(id),
    reviewed_at     TIMESTAMPTZ,
    review_notes    TEXT,
    is_expired      BOOLEAN NOT NULL DEFAULT FALSE
);

-- ─── Connectors ────────────────────────────────────────────
CREATE TABLE connectors (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    name            VARCHAR(255) NOT NULL,
    connector_type  VARCHAR(100) NOT NULL,
    status          VARCHAR(50) NOT NULL DEFAULT 'disconnected',
    config          JSONB NOT NULL DEFAULT '{}',
    last_sync_at    TIMESTAMPTZ,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

-- ─── Connector-Synced Identities ─────────────────────────
-- Stores identities synced from external directories via connectors.
CREATE TABLE connector_identities (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    connector_id    UUID NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
    external_id     VARCHAR(500) NOT NULL,
    username        VARCHAR(255),
    email           VARCHAR(320),
    display_name    VARCHAR(255),
    first_name      VARCHAR(255),
    last_name       VARCHAR(255),
    department      VARCHAR(255),
    title           VARCHAR(255),
    employee_id     VARCHAR(100),
    manager_id      VARCHAR(500),
    phone           VARCHAR(100),
    mobile          VARCHAR(100),
    street_address  VARCHAR(255),
    city            VARCHAR(100),
    state           VARCHAR(100),
    zip_code        VARCHAR(20),
    country         VARCHAR(100),
    cost_center     VARCHAR(100),
    division        VARCHAR(100),
    company         VARCHAR(255),
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    locked          BOOLEAN NOT NULL DEFAULT FALSE,
    groups          TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    roles           TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    raw_attributes  JSONB NOT NULL DEFAULT '{}',
    first_synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(connector_id, external_id)
);

CREATE INDEX idx_conn_id_tenant     ON connector_identities(tenant_id);
CREATE INDEX idx_conn_id_connector  ON connector_identities(connector_id);
CREATE INDEX idx_conn_id_email      ON connector_identities(email);
CREATE INDEX idx_conn_id_enabled    ON connector_identities(enabled);
CREATE INDEX idx_conn_id_dept       ON connector_identities(department);
CREATE INDEX idx_conn_id_lookup     ON connector_identities(connector_id, external_id);

-- ─── Auto-Update Timestamps ───────────────────────────────
CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_identities_updated   BEFORE UPDATE ON identities           FOR EACH ROW EXECUTE FUNCTION update_timestamp();
CREATE TRIGGER trg_nhi_updated          BEFORE UPDATE ON non_human_identities FOR EACH ROW EXECUTE FUNCTION update_timestamp();
CREATE TRIGGER trg_roles_updated        BEFORE UPDATE ON roles                FOR EACH ROW EXECUTE FUNCTION update_timestamp();
CREATE TRIGGER trg_resources_updated    BEFORE UPDATE ON resources            FOR EACH ROW EXECUTE FUNCTION update_timestamp();
CREATE TRIGGER trg_connectors_updated   BEFORE UPDATE ON connectors           FOR EACH ROW EXECUTE FUNCTION update_timestamp();

-- ─── Seed Data: Default Tenant ─────────────────────────────
INSERT INTO tenants (id, name, slug, tier) VALUES
    ('00000000-0000-0000-0000-000000000001', 'ObserveID Internal', 'observeid', 'enterprise');

-- ─── Seed Data: Admin Identity ─────────────────────────────
INSERT INTO identities (id, tenant_id, type, status, email, display_name, employee_id, source, assurance_level)
VALUES (
    '00000000-0000-0000-0000-000000000002',
    '00000000-0000-0000-0000-000000000001',
    'human', 'active',
    'admin@observeid.io', 'System Admin', 'ADMIN-001',
    'manual', 'aal2'
);

-- ─── Seed Data: Base Roles ─────────────────────────────────
INSERT INTO roles (id, tenant_id, name, description, role_type) VALUES
    ('00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000001', 'Administrator', 'Full system access', 'technical'),
    ('00000000-0000-0000-0000-000000000011', '00000000-0000-0000-0000-000000000001', 'Security Reviewer', 'Access review permissions', 'business'),
    ('00000000-0000-0000-0000-000000000012', '00000000-0000-0000-0000-000000000001', 'Auditor', 'Read-only audit access', 'business'),
    ('00000000-0000-0000-0000-000000000013', '00000000-0000-0000-0000-000000000001', 'Engineer', 'Standard engineering access', 'technical'),
    ('00000000-0000-0000-0000-000000000014', '00000000-0000-0000-0000-000000000001', 'Contractor', 'Limited contractor access', 'business');

-- Assign admin role
INSERT INTO identity_roles (tenant_id, identity_id, role_id, assigned_by, source) VALUES
    ('00000000-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-000000000002',
     '00000000-0000-0000-0000-000000000010',
     '00000000-0000-0000-0000-000000000002', 'direct');
