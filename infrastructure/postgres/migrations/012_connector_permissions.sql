-- ─── Migration 012: Connector Permission Catalog ─────────────────
-- Adds the connector_permissions table: the "Managed Attributes" catalog of
-- permission items a connected application DEFINES (appRoles, OAuth2 scopes,
-- application permissions). Distinct from connector_entitlements, which holds
-- the ASSIGNMENTS of those items to principals. Modeled on SailPoint's
-- separation between the entitlement catalog and account links.
--
-- IDEMPOTENT: safe to run repeatedly (IF NOT EXISTS / DO blocks).

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'connector_permissions'
    ) THEN
        CREATE TABLE connector_permissions (
            id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
            tenant_id       UUID NOT NULL,
            connector_id    UUID NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
            permission_id   VARCHAR(255) NOT NULL,
            name            VARCHAR(512) NOT NULL,
            permission_type VARCHAR(50) NOT NULL DEFAULT 'app_role',
            app_id          VARCHAR(255),
            app_name        VARCHAR(512),
            description     TEXT,
            is_admin        BOOLEAN NOT NULL DEFAULT FALSE,
            raw_attributes  JSONB NOT NULL DEFAULT '{}'::jsonb,
            first_synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            last_synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            UNIQUE (connector_id, permission_id)
        );
        CREATE INDEX idx_connector_permissions_tenant
            ON connector_permissions (tenant_id);
        CREATE INDEX idx_connector_permissions_app
            ON connector_permissions (app_id);
        CREATE INDEX idx_connector_permissions_admin
            ON connector_permissions (is_admin)
            WHERE is_admin = TRUE;

        ALTER TABLE connector_permissions ENABLE ROW LEVEL SECURITY;
        ALTER TABLE connector_permissions FORCE ROW LEVEL SECURITY;

        CREATE POLICY connector_permissions_tenant_isolation ON connector_permissions
            USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
            WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

        COMMENT ON TABLE connector_permissions IS
            'Permission catalog (Managed Attributes) a connector source defines: app roles, OAuth2 scopes, and application permissions, independent of who holds them.';
    END IF;
END $$;

INSERT INTO schema_migrations (id, description)
VALUES ('012_connector_permissions', 'connector_permissions catalog table with RLS tenant isolation')
ON CONFLICT (id) DO UPDATE
    SET description = EXCLUDED.description;