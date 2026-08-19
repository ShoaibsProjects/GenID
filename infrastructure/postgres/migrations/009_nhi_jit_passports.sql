-- 009_nhi_jit_passports.sql
-- JIT (just-in-time) passports for Non-Human Identities: short-lived,
-- single-scope credentials minted for a specific access grant, revoked
-- on completion and never transferable.
--
-- Renumbered from spec "008_nhi_registry": 008 was already taken by the
-- Phase 1 cedar policy migration. non_human_identities itself already
-- exists in init.sql; this migration only adds the passport table and
-- the parent_agent_id column for A2A delegation.

CREATE TABLE IF NOT EXISTS jit_passports (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id          UUID NOT NULL REFERENCES tenants(id),
    nhi_id             UUID NOT NULL REFERENCES non_human_identities(id),
    issuer_id          UUID,
    token_hash         TEXT NOT NULL UNIQUE,
    scope              TEXT NOT NULL DEFAULT 'access:grant',
    resource_id        UUID,
    grant_id           UUID,
    status             VARCHAR(30) NOT NULL DEFAULT 'active',
    issued_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at         TIMESTAMPTZ NOT NULL,
    revoked_at         TIMESTAMPTZ,
    consumed_at        TIMESTAMPTZ,
    created_by         UUID,
    parent_passport_id UUID,
    attributes         JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX idx_jit_passports_nhi     ON jit_passports(nhi_id, status);
CREATE INDEX idx_jit_passports_expiry  ON jit_passports(expires_at) WHERE status = 'active';
CREATE INDEX idx_jit_passports_tenant  ON jit_passports(tenant_id);
CREATE INDEX idx_jit_passports_grant   ON jit_passports(grant_id) WHERE grant_id IS NOT NULL;

-- A2A delegation: an agent may act on behalf of a parent agent.
ALTER TABLE non_human_identities
    ADD COLUMN IF NOT EXISTS parent_agent_id UUID REFERENCES non_human_identities(id);

CREATE INDEX IF NOT EXISTS idx_nhi_parent ON non_human_identities(parent_agent_id);