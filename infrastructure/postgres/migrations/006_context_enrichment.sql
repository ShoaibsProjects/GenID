-- 006_context_enrichment.sql
-- Phase 1 (Conditional Access MVP): context enrichment backing tables.
--
-- tenant_cidr_zones    : IP → network zone ("corporate" | "vpn" | "public")
--                        matched by longest-prefix CIDR containment.
-- tenant_business_hours: per-tenant work hours used to classify
--                        "business_hours" | "after_hours" | "weekend".
--                        A row with NULL day columns means "use default
--                        09:00-17:00" for that day.

BEGIN;

CREATE TABLE IF NOT EXISTS tenant_cidr_zones (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    zone_name   VARCHAR(50) NOT NULL,  -- 'corporate', 'vpn', 'public'
    cidr        CIDR NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cidr_zones_tenant ON tenant_cidr_zones(tenant_id);
CREATE INDEX IF NOT EXISTS idx_cidr_zones_range ON tenant_cidr_zones USING gist (cidr inet_ops);

CREATE TABLE IF NOT EXISTS tenant_business_hours (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL REFERENCES tenants(id),
    timezone       VARCHAR(50) NOT NULL DEFAULT 'America/New_York',
    monday_start   TIME, monday_end   TIME,
    tuesday_start  TIME, tuesday_end  TIME,
    wednesday_start TIME, wednesday_end TIME,
    thursday_start TIME, thursday_end TIME,
    friday_start   TIME, friday_end   TIME,
    saturday_start TIME, saturday_end TIME,
    sunday_start   TIME, sunday_end   TIME,
    weekend_access BOOLEAN DEFAULT FALSE,
    created_at     TIMESTAMPTZ DEFAULT NOW()
);

-- Seed data for the demo tenant (ObserveID internal).
INSERT INTO tenant_cidr_zones (tenant_id, zone_name, cidr, description) VALUES
    ('00000000-0000-0000-0000-000000000001', 'corporate', '10.0.0.0/8',  'Office networks'),
    ('00000000-0000-0000-0000-000000000001', 'vpn',       '172.16.0.0/12', 'VPN range'),
    ('00000000-0000-0000-0000-000000000001', 'public',    '0.0.0.0/0',   'Internet')
ON CONFLICT DO NOTHING;

INSERT INTO tenant_business_hours
    (tenant_id, timezone,
     monday_start, monday_end, tuesday_start, tuesday_end,
     wednesday_start, wednesday_end, thursday_start, thursday_end,
     friday_start, friday_end, weekend_access) VALUES
    ('00000000-0000-0000-0000-000000000001', 'America/New_York',
     '09:00', '17:00', '09:00', '17:00', '09:00', '17:00',
     '09:00', '17:00', '09:00', '17:00', FALSE)
ON CONFLICT DO NOTHING;

COMMIT;