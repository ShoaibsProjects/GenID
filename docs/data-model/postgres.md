# PostgreSQL Data Model

PostgreSQL is the **source of truth**. Source: `infrastructure/postgres/init.sql`.

## Enums

| Type | Values |
|------|--------|
| `identity_type` | `human, service_account, ai_agent, robot, iot_device, rpa_bot, api_key` |
| `identity_status` | `active, inactive, suspended, terminated, revoked, pending_review` |
| `identity_source` | `hris, scim, manual, agent_registration, discovery, ldap, saml` |

## Core: tenants & identities

### tenants
`id UUID PK, name, slug UNIQUE, tier (default 'starter'), is_active, settings JSONB, created_at, updated_at`

### identities
| Column | Type | Notes |
|--------|------|-------|
| `id` | UUID PK | |
| `tenant_id` | UUID FK → tenants | |
| `type` | identity_type | default `human` |
| `status` | identity_status | default `active` |
| `email` | VARCHAR(320) | UNIQUE(tenant_id, email) |
| `display_name` | VARCHAR(255) | |
| `department` | VARCHAR(255) | used by peer-risk |
| `employee_id` | VARCHAR(100) | |
| `manager_id` | UUID self-FK | |
| `source` | identity_source | |
| `risk_score` | DOUBLE PRECISION | default 0.0 |
| `risk_factors` | TEXT[] | |
| `assurance_level` | VARCHAR(10) | default `aal1` |
| `attributes` | JSONB | |
| `last_accessed_at`, `last_reviewed_at` | TIMESTAMPTZ | |

Indexes: tenant, status, type, department, employee, manager, `risk_score DESC`, composite `(tenant_id,email,status)`, GIN full-text.

### non_human_identities
`id, tenant_id FK, name UNIQUE(tenant_id,name), type, status, agent_card_id, protocols TEXT[], owner_id → identities, team_id, created_by → identities, deployment_environment, framework, capabilities TEXT[], is_governed, secrets_age_days, last_rotated_at, expires_at, risk_score, attributes`

## RBAC

### roles
`id, tenant_id, name UNIQUE(tenant_id,name), description, role_type (default 'business'), is_auto_assigned, approval_required, max_duration_hours, created_by_rolmining, confidence_score, is_active, attributes`

### entitlements
`id, tenant_id, app_name, permission_level, entitlement_type (default 'read'), risk_classification (default 'medium'), is_toxic, is_rubberband, last_used_at, usage_count_90d, attributes` — UNIQUE(tenant_id, app_name, permission_level)

### resources
`id, tenant_id, name UNIQUE(tenant_id,name), resource_type, criticality (default 'p3'), data_classification (default 'internal'), owner_team, connection_type, health_status, attributes`

### identity_roles
`id, tenant_id, identity_id FK CASCADE, role_id FK CASCADE, assigned_by → identities, assigned_at, expires_at, approved_by, approval_ticket, source (default 'direct'), is_active` — UNIQUE(tenant_id, identity_id, role_id, source)

### role_entitlements
`id, tenant_id, role_id FK CASCADE, entitlement_id FK CASCADE, condition, created_at` — UNIQUE(tenant_id, role_id, entitlement_id)

### direct_entitlements
`id, tenant_id, identity_id FK CASCADE, entitlement_id FK CASCADE, assigned_by, assigned_at, is_exception, exception_approved_by, exception_expires_at, reason` — UNIQUE(tenant_id, identity_id, entitlement_id); partial index on toxic non-exceptions

### delegation_chains
`id, tenant_id, parent_identity_id FK CASCADE → non_human_identities, child_identity_id FK CASCADE → non_human_identities, scope_narrowing TEXT[], max_depth_remaining (default 1), delegated_at, expires_at, is_active` — UNIQUE(parent, child)

## Sessions

### sessions
`id, identity_id FK CASCADE, tenant_id, auth_method, assurance_level (aal1), device_id, ip_address INET, user_agent, is_active, created_at, expires_at NOT NULL, last_activity_at`

## OIDC

### oidc_clients
`id, tenant_id, name, client_id UNIQUE, client_secret, redirect_uris TEXT[], grant_types TEXT[], scopes TEXT[], is_public, response_types, token_endpoint_auth_method, jwks_uri, sector_identifier_uri, request_uris, contacts, logo/policy/tos URIs, backchannel/frontchannel logout URIs + *_session_required, attributes`

### oidc_auth_codes
`id, code UNIQUE, client_id FK, user_id FK → identities, redirect_uri, scope TEXT[], code_challenge, code_challenge_method (S256), nonce, expires_at, consumed_at, created_at`

### oidc_refresh_tokens
`id, token_hash UNIQUE, client_id FK, user_id FK, scope, expires_at, revoked_at, created_at, last_used_at`

### oidc_device_codes
`id, device_code UNIQUE, user_code UNIQUE, client_id FK, scope, expires_at, authorized_at, user_id, created_at`

## Audit, CAEP, outbox

### audit_log
Tamper-evident chain: `id, tenant_id, event_type, actor_id, actor_type, target_id, target_type, action, resource, details JSONB, ip_address INET, user_agent, correlation_id, trace_id, prev_hash VARCHAR(64), hash VARCHAR(64)`

`hash` = SHA-256 over the immutable fields; `prev_hash` links each entry to the prior one → tampering is detectable. Verified by `GET /api/v1/audit/verify`.

### caep_events
`id, tenant_id, event_type, event_jti UNIQUE, identity_id, session_id, initiating_entity, reason_admin, reason_user, event_payload JSONB, delivery_status (pending), delivered_to TEXT[], created_at, delivered_at`

### outbox_events
`id UUID PK, event_type VARCHAR(50), aggregate_type, aggregate_id, payload JSONB NOT NULL, metadata JSONB, created_at, processed BOOLEAN, processed_at, retry_count, error_message, expires_at (+7d)`

Event types: `identity.created/updated/deleted`, `role.assigned/revoked`, `entitlement.provisioned/revoked`. Inserted atomically with the main write; polled by the outbox processor; partial indexes on unprocessed/retry/expires.

## Policies, agent cards, SoD, certifications, emergency access

### cedar_policies
`id, tenant_id, policy_id, effect CHECK (permit|forbid), policy_source, cedar_text, is_active, version, created_by` — UNIQUE(tenant_id, policy_id, version)

### agent_cards
`id, nh_identity_id UNIQUE FK → non_human_identities CASCADE, tenant_id, card_type (a2a), card_document JSONB NOT NULL, signature NOT NULL, signature_scheme (ml-dsa-44), is_valid, issued_at, expires_at, revoked_at`

### sod_rules
`id, tenant_id, name UNIQUE(tenant_id,name), description, conflicting_entitlements UUID[][] NOT NULL, severity (high), is_active, created_at`

### certification_campaigns
`id, tenant_id, name, campaign_type (quarterly|triggered|emergency), status (draft), starts_at, ends_at, created_by, created_at`

### certification_entries
`id, campaign_id FK CASCADE, identity_id FK, certifier_id, status (pending), decision (certified|revoked|modified), notes, decided_at, created_at`

### emergency_access
`id, tenant_id, identity_id FK, resource_id FK → resources, reason NOT NULL, requested_at, expires_at NOT NULL, granted_by, granted_at, reviewed_by, reviewed_at, review_notes, is_expired`

## Connector sync tables

### connectors
`id, tenant_id, name UNIQUE(tenant_id,name), connector_type, status (disconnected), config JSONB, last_sync_at, last_error`

### connector_identities
Wide table for discovered users: `id, tenant_id, connector_id FK CASCADE, external_id, username, email, display_name, first/last_name, department, title, employee_id, manager_id, phone, mobile, street_address, city, state, zip_code, country, cost_center, division, company, enabled, locked, groups TEXT[], roles TEXT[], raw_attributes JSONB, sync timestamps` — UNIQUE(connector_id, external_id)

### connector_groups
`id, tenant_id, connector_id FK, external_id, name, description, group_type, scope, member_ids TEXT[], raw_attributes, sync timestamps` — UNIQUE(connector_id, external_id)

### connector_entitlements
`id, tenant_id, connector_id FK, identity_external_id, entitlement_type, source_id, source_name, source_type, app_id, app_name, assigned_at, is_active, raw_attributes` — unique index (connector_id, identity_external_id, source_id, entitlement_type)

### connector_resources
`id, tenant_id, connector_id FK, external_id, resource_type, name, description, enabled, owner_ids TEXT[], raw_attributes` — UNIQUE(connector_id, external_id)

## Row-Level Security (RLS)

Enabled on **28 tables**, each with policy:

```sql
USING (tenant_id = current_setting('app.current_tenant')::uuid)
```

The Go service sets the tenant context per request: `SET app.current_tenant = '<tenantID>'`. This guarantees tenant isolation at the database layer even if application-layer checks were bypassed.

## Triggers & seeds

- `update_timestamp()` trigger on 11 tables (`BEFORE UPDATE`).
- Seeds (fresh volume only): tenant `observeid`, admin identity, 5 base roles + admin assignment.

## Related

- [Neo4j graph model](neo4j.md)
- [Risk properties](../architecture/risk-engine.md)
