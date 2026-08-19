-- 007_api_keys.sql
-- DB-backed API key authentication (Phase 0 security baseline).
--
-- Key format on the wire: genid_<id>_<secret>
--   - <id>     : the table's UUID (public, indexable, used for lookup)
--   - <secret> : never stored in plaintext; only its bcrypt hash lives in
--                key_hash. bcrypt salt is embedded in the hash.
--
-- scopes double as authorization roles injected into request context
-- (context key "roles"); expiry is enforced at auth time (expired or
-- disabled rows are rejected).

BEGIN;

CREATE TABLE IF NOT EXISTS api_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    key_hash    TEXT        NOT NULL,             -- bcrypt(secret)
    scopes      TEXT[]      NOT NULL DEFAULT '{}',
    tenant_id   UUID        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    expires_at  TIMESTAMPTZ,
    enabled     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

-- Enforce bcrypt-only hashes at the DB layer.
ALTER TABLE api_keys ADD CONSTRAINT api_keys_key_hash_prefix_check
    CHECK (key_hash LIKE '$2%');

CREATE INDEX IF NOT EXISTS api_keys_tenant_idx ON api_keys (tenant_id);

COMMIT;