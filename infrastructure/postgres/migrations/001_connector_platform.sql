-- ─── Migration 001: Connector Platform Foundations ───────────────
-- Part of the BRUTAL_ENGINEERING.md Phase 0 plan.
--
-- Adds the per-connector watermarks, scheduling, governance columns, vault
-- reference, and RLS on the connector cache tables needed to graduate to
-- enterprise-grade connector operations (real delta, per-connector cron,
-- vaulted credentials, governed resources) at 50k+ identities.
--
-- IDEMPOTENT: safe to run repeatedly (IF NOT EXISTS / DO blocks).
-- ADDITIVE: no existing column/table is dropped or renamed.

-- ─── 1. Per-sync watermarks ──────────────────────────────────────
-- Tracks how far each object_class (user|group|entitlement|resource) of a
-- connector has been synced, so SyncDelta can request only changes from
-- the source. The watermark JSONB is source-defined (delta-token, modified
-- timestamp, paging cursor, ...) and opaque to the platform.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'connector_sync_state'
    ) THEN
        CREATE TABLE connector_sync_state (
            id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
            connector_id    UUID NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
            object_class    VARCHAR(50) NOT NULL,
            watermark       JSONB NOT NULL DEFAULT '{}'::jsonb,
            last_synced_at  TIMESTAMPTZ,
            last_count      INT,
            last_error      TEXT,
            created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            UNIQUE (connector_id, object_class)
        );
        CREATE INDEX idx_connector_sync_state_connector
            ON connector_sync_state (connector_id);

        COMMENT ON TABLE connector_sync_state IS
            'Per-connector, per-object-class sync watermark for delta sync.';
    END IF;
END $$;

-- ─── 1.5. vault_secrets table (must exist before connectors.vault_secret_id FK) ──
-- The vault is currently file-backed (see backend/internal/vault/vault.go),
-- so vault_secrets may not be in the database yet. Add it idempotently so
-- the FK on connectors.vault_secret_id resolves. When the vault gets a
-- Postgres-backed storage backend (Phase 0.3), it will use this table.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'vault_secrets'
    ) THEN
        CREATE TABLE vault_secrets (
            id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
            tenant_id   UUID NOT NULL,
            name        VARCHAR(255) NOT NULL,
            secret_type VARCHAR(50) NOT NULL,
            reference   VARCHAR(255),
            ciphertext  BYTEA NOT NULL,
            nonce       BYTEA NOT NULL,
            version     INT NOT NULL DEFAULT 1,
            created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            UNIQUE (tenant_id, name)
        );
        CREATE INDEX idx_vault_secrets_tenant ON vault_secrets (tenant_id);
        CREATE INDEX idx_vault_secrets_reference ON vault_secrets (reference);

        ALTER TABLE vault_secrets ENABLE ROW LEVEL SECURITY;
        ALTER TABLE vault_secrets FORCE ROW LEVEL SECURITY;

        CREATE POLICY vault_secrets_tenant_isolation ON vault_secrets
            USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
            WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

        COMMENT ON TABLE vault_secrets IS
            'AES-256-GCM encrypted secret storage backed by Postgres. ciphertext + nonce are the sealed plaintext; the master key never leaves the application env (VAULT_MASTER_KEY).';
    END IF;
END $$;

-- ─── 2. Connector columns: schedule, governance, vault ──────────
-- schedule_cron: per-connector cron expression (default */20 * * * *).
-- ha_state: 'active' connects the connector health to risk scoring.
-- owner_identity_id: governance owner for certification campaigns.
-- risk_weight: contribution to dependents risk when connector degrades.
ALTER TABLE connectors
    ADD COLUMN IF NOT EXISTS schedule_cron VARCHAR(64) DEFAULT '*/20 * * * *';

ALTER TABLE connectors
    ADD COLUMN IF NOT EXISTS owner_identity_id UUID REFERENCES identities(id);

ALTER TABLE connectors
    ADD COLUMN IF NOT EXISTS risk_weight INT NOT NULL DEFAULT 50
    CHECK (risk_weight BETWEEN 0 AND 100);

ALTER TABLE connectors
    ADD COLUMN IF NOT EXISTS connector_governance_status VARCHAR(20)
    NOT NULL DEFAULT 'active';

-- Vault integration.  NULL for legacy connectors that still keep secrets
-- inline in `config` JSONB; non-NULL means secrets are stored encrypted
-- in the vault_secrets table and swapped in at runtime.
ALTER TABLE connectors
    ADD COLUMN IF NOT EXISTS vault_secret_id UUID REFERENCES vault_secrets(id);

-- last_sync_duration_ms: rolling latency metric for ops dashboards.
ALTER TABLE connectors
    ADD COLUMN IF NOT EXISTS last_sync_duration_ms INT;

-- ─── 3. RLS on connector cache tables ────────────────────────────
-- The plan-level Postgres RLS posture claimed "28 tables". The connector
-- cache tables (connector_identities, connector_groups, connector_entitlements,
-- connector_resources) currently inherit no policy, so cross-tenant reads
-- are possible if the application skipped the tenant filter. Add RLS now,
-- keyed on the same `app.current_tenant` session variable the rest of
-- the schema uses.

ALTER TABLE connectors ENABLE ROW LEVEL SECURITY;
ALTER TABLE connectors FORCE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'connectors' AND policyname = 'connectors_tenant_isolation'
    ) THEN
        CREATE POLICY connectors_tenant_isolation ON connectors
            USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
            WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);
    END IF;
END $$;

ALTER TABLE connector_identities ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_identities FORCE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'connector_identities' AND policyname = 'connector_identities_tenant_isolation'
    ) THEN
        CREATE POLICY connector_identities_tenant_isolation ON connector_identities
            USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
            WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);
    END IF;
END $$;

ALTER TABLE connector_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_groups FORCE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'connector_groups' AND policyname = 'connector_groups_tenant_isolation'
    ) THEN
        CREATE POLICY connector_groups_tenant_isolation ON connector_groups
            USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
            WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);
    END IF;
END $$;

ALTER TABLE connector_entitlements ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_entitlements FORCE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'connector_entitlements' AND policyname = 'connector_entitlements_tenant_isolation'
    ) THEN
        CREATE POLICY connector_entitlements_tenant_isolation ON connector_entitlements
            USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
            WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);
    END IF;
END $$;

ALTER TABLE connector_resources ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_resources FORCE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'connector_resources' AND policyname = 'connector_resources_tenant_isolation'
    ) THEN
        CREATE POLICY connector_resources_tenant_isolation ON connector_resources
            USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
            WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);
    END IF;
END $$;

ALTER TABLE connector_sync_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_sync_state FORCE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'connector_sync_state' AND policyname = 'connector_sync_state_tenant_isolation'
    ) THEN
        CREATE POLICY connector_sync_state_tenant_isolation ON connector_sync_state
            USING (
                connector_id IN (
                    SELECT c.id FROM connectors c
                    WHERE c.tenant_id = current_setting('app.current_tenant', true)::uuid
                )
            )
            WITH CHECK (
                connector_id IN (
                    SELECT c.id FROM connectors c
                    WHERE c.tenant_id = current_setting('app.current_tenant', true)::uuid
                )
            );
    END IF;
END $$;

-- ─── 4. Helpful index for governance queries ─────────────────────
CREATE INDEX IF NOT EXISTS idx_connectors_owner
    ON connectors (owner_identity_id)
    WHERE owner_identity_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_connectors_governance
    ON connectors (connector_governance_status)
    WHERE connector_governance_status <> 'active';

-- ─── 6. Record the migration ────────────────────────────────────
-- A lightweight migration ledger. Reads from the audit chain correctly
-- surface changes to the schema as well.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'schema_migrations'
    ) THEN
        CREATE TABLE schema_migrations (
            id          VARCHAR(100) PRIMARY KEY,
            applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            description TEXT
        );
        ALTER TABLE schema_migrations ENABLE ROW LEVEL SECURITY;
        CREATE POLICY schema_migrations_admin ON schema_migrations
            USING (true) WITH CHECK (true);
    END IF;
END $$;

INSERT INTO schema_migrations (id, description)
VALUES ('001_connector_platform', 'Connector platform foundations: connector_sync_state, schedule_cron, owner, risk_weight, vault_secret_id, RLS on connector cache tables, vault_secrets base, schema_migrations ledger')
ON CONFLICT (id) DO UPDATE
    SET description = EXCLUDED.description;
