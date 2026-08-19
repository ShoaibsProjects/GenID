-- 003_fix_workflow_dedup_and_audit_chain.sql
-- Fixes two defects found during Phase 2 audit:
--
-- 1) workflow_requests_idem_uniq was built on (tenant_id, type,
--    COALESCE(idempotency_key, '')) which made every request WITHOUT an
--    idempotency key collide with the first one (COALESCE -> ''), so
--    CreateRequest's ON CONFLICT DO NOTHING silently skipped real requests
--    and left orphaned Temporal workflows behind.
--    Fix: partial unique index that only applies when a key is actually
--    supplied by the client.
--
-- 2) The live audit_log table predates the hash-chain columns, so
--    audit.Chain.Append fails on every insert with
--    `column "hash" does not exist`.
--    Fix: add prev_hash + hash columns if missing.

BEGIN;

-- ─── Fix 1: idempotency dedup should only apply when a key exists ───────
DROP INDEX IF EXISTS workflow_requests_idem_uniq;

CREATE UNIQUE INDEX workflow_requests_idem_uniq
    ON workflow_requests (tenant_id, type, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';

-- ─── Fix 2: audit_log tamper-evident hash chain columns ──────────────────
ALTER TABLE audit_log
    ADD COLUMN IF NOT EXISTS prev_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS hash     VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_audit_hash ON audit_log(hash);

COMMIT;