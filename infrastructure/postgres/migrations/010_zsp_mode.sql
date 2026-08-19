-- 010_zsp_mode.sql
-- Zero Standing Privilege (ZSP) mode for tenants. When enabled, all
-- access grants are forced through JIT (no permanent/standing access),
-- and override requests require an explicit approval step.

-- Renumbered from spec "009_zsp": 009 was taken by NHI passports.

ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS zsp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS zsp_max_jit_duration INTERVAL NOT NULL DEFAULT '2 hours',
    ADD COLUMN IF NOT EXISTS zsp_override_requires_approval BOOLEAN NOT NULL DEFAULT TRUE;

CREATE INDEX IF NOT EXISTS idx_tenants_zsp ON tenants(zsp_enabled) WHERE zsp_enabled = TRUE;
