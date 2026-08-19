# API Overview

Base URL: `http://localhost:8080` (or the frontend proxy `http://localhost:3001` which forwards `/api`, `/scim`, `/graphql`, `/health` to the service).

## Authentication

### 1. JWT (primary)

`POST /api/v1/dev/login` in dev mode returns an admin JWT. All protected endpoints accept:

```
Authorization: Bearer <access_token>
```

In production, obtain tokens through the [OIDC provider](oidc.md) (`/authorize` + `/token`).

### 2. API keys (internal)

Set via `API_KEYS=name:key,...`. Accepted as:

```
X-API-Key: <key>
Authorization: Bearer <key>
```

Internal worker keys bypass JWT validation.

### 3. Master key (sensitive ops)

Operations guarded by the workflow guard (grant/revoke access, kill switch, LCM, vault writes, SCIM writes, role changes, bulk import) additionally require:

```
X-Master-Key: <MASTER_KEY env>
```

## Middleware chain

Applied to every request, in order:

| # | Middleware | Effect |
|---|-----------|--------|
| 1 | Security headers | `nosniff`, `DENY` framing, strict-referrer, HSTS (TLS) |
| 2 | CORS | Configurable origin; allows `Content-Type, Authorization, X-API-Key, X-Master-Key, X-Requested-With` |
| 3 | OpenTelemetry | Traces every request |
| 4 | Rate limit | **100 req/s, burst 200**, per-IP; `429` + `Retry-After: 1` |
| 5 | Request validation | JSON content-type required for POST/PUT/PATCH/QUERY (except OIDC form endpoints); 10 MB body cap |
| 6 | JWT auth | Validates bearer tokens via JWKS; sets `tenant_id`, `user_id`, `roles`, `jwt_claims` in context |
| 7 | Audit logging | Writes `http_request` to the SHA-256 tamper-evident chain |

### JWT-exempt (anonymous) paths

`/health`, `/ready`, `/healthz`, `/metrics`, all static UI routes (`/`, `/dashboard`, ...), `/api/v1/dev/login`, `/api/v1/connectors/stats`, `/api/v1/audit/stats`, the OIDC discovery/JWKS/authorize/token/userinfo/introspect/revoke/device endpoints, and `/ui/*`.

## Conventions

- JSON request/response bodies throughout (`Content-Type: application/json`).
- Errors: `{"error": "<message>"}` with appropriate status.
- 202 Accepted for async operations that start workflows (provisioning, offboarding, JIT, certifications).
- Query params: `?limit=`, `?offset=`, `?expand=attributes`, `?filter=`.
- List responses: `{"data": [...], "total": N, "limit": L, "offset": O}` (some modules return `total` + array directly — see each endpoint).

## Module index

| Module | Section |
|--------|---------|
| Health & readiness | [Infrastructure](#infrastructure) |
| Dev login | [Auth & dev](#auth--dev-login) |
| Identities | [Identities](#identities) |
| NHI / Agents | [Agents](#agents--non-human-identities) |
| Access control | [Access](#access-control) |
| Risk intelligence | [Risk](#risk-intelligence) |
| Events | [Events](#events) |
| Roles & entitlements | [Roles / Groups / Entitlements](#roles--groups--entitlements) |
| Connectors | [Connectors](#connectors) |
| LCM & certifications | [LCM & certifications](#lcm--certifications) |
| Vault | [Vault](#vault) |
| Audit | [Audit](#audit) |
| OIDC | [oidc.md](oidc.md) |
| SCIM | [scim.md](scim.md) |
| CAEP | [CAEP](#caep) |
| GraphQL | [GraphQL](#graphql) |

## Infrastructure

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness: `{"status":"ok","service":"genid-identity","version":"1.0.0"}` |
| GET | `/ready` | Checks redis, postgres, neo4j → `{"status":"ready","checks":{...}}` (503 if any down) |
| GET | `/healthz` | Full check incl. temporal (3s timeout) → `{"status":"healthy","checks":{...}}` |
| GET | `/metrics` | Prometheus metrics |

## Auth & dev login

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/dev/login` | `{"username","password"}` → `{"access_token","expires_in":300,"token_type":"Bearer","user_id"}`. Admin JWT minted for any existing **active** identity in demo mode. Disabled when `DEV_LOGIN_ENABLED=false`. |

## Identities

| Method | Path | Guard | Description |
|--------|------|-------|-------------|
| GET | `/api/v1/identities` | — | List identities (`?expand=attributes`, `?limit`, `?offset`, `?filter`) |
| POST | `/api/v1/identities` | — | Create `{"email","display_name","type","department","employee_id","manager_id","source","attributes"}` |
| POST | `/api/v1/identities/bulk` | `bulk_import` | Bulk create identities |
| GET | `/api/v1/identities/{id}` | — | Get identity detail |
| PATCH | `/api/v1/identities/{id}` | — | Update identity |
| DELETE | `/api/v1/identities/{id}` | — | Delete identity |
| GET | `/api/v1/identities/{id}/entitlements` | — | Entitlements held by identity |
| GET | `/api/v1/identities/{id}/blast-radius` | — | Graph blast-radius analysis |
| POST | `/api/v1/identities/{id}/risk/recalculate` | — | Recompute legacy V1 risk; `?tenant_id=` (defaults to default tenant) → `{"identity_id","risk_score","risk_factors"}` |
| POST | `/api/v1/identities/csv/preview` | — | Preview CSV import |
| POST | `/api/v1/identities/csv/import` | — | Import identities from CSV |
| GET | `/api/v1/identities/csv/export` | — | Export identities to CSV |

## Agents / Non-Human Identities

| Method | Path | Guard | Description |
|--------|------|-------|-------------|
| GET | `/api/v1/agents` | — | List NHIs / agents |
| POST | `/api/v1/agents` | — | Register agent (name, type, owner, protocols, capabilities) |
| GET | `/api/v1/agents/{id}` | — | Get agent |
| POST | `/api/v1/agents/{id}/kill-switch` | `agent_kill_switch` | Emergency revoke (Temporal `RevokeAccessWorkflow`, cascades to delegated children) |
| POST | `/api/v1/agents/{id}/delegate` | `delegate_agent` | Create delegation chain |
| GET | `/api/v1/agents/{id}/card` | — | Agent card document |

## Access control

| Method | Path | Guard | Description |
|--------|------|-------|-------------|
| QUERY/POST | `/api/v1/access/check` | — | `{"identity_id","resource_id","resource_type","action"}` → `{"allowed":bool,"reason":str,"evaluated":"neo4j","latency_ms":int}` |
| POST | `/api/v1/access/grant` | `grant_access` | `GrantAccessInput` → starts `GrantAccessWorkflow` (SoD + Cedar check, optional approval signal, JIT expiry) |
| POST | `/api/v1/access/revoke` | `revoke_access` | `RevokeAccessInput` → `RevokeAccessWorkflow` (emergency: disable + rotate + CAEP broadcast) |
| POST | `/api/v1/access/jit` | — | `JustInTimeInput` → `JustInTimeAccessWorkflow` (defaults: read / 60 min) |
| GET | `/api/v1/access/sessions` | — | Active JIT sessions |

**GrantAccessInput**: `{identity_id, resource_id, resource_type, role_id, requested_by, duration_hours, reason, tenant_id, requires_approval}`

**JustInTimeInput**: `{identity_id, resource_id, resource_type, action, reason, requested_by, duration_mins, tenant_id}`

## Risk intelligence

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/risk/dashboard` | `{total_identities, critical_count, high_count, elevated_count, low_count, minimal_count, average_score, top_risk_identities:[...]}` |
| GET | `/api/v1/risk/score/{identityId}` | `{identityId, displayName, riskScore, staticScore, dynamicScore, peerScore, riskBand, factors, eventCount, lastEvent, lastSource, calculatedAt}` |
| GET | `/api/v1/risk/peer/{identityId}` | `{identity_id, peer_deviation_score, factors:[...]}` |
| POST | `/api/v1/risk/calculate/{identityId}` | Recompute + persist combined score → `{identity_id, final_score, static_score, dynamic_score, peer_score, risk_band, static_factors, peer_factors}` |
| GET | `/api/v1/risk/sessions/{identityId}` | `{identity_id, sessions:[{sessionId, source, ipAddress, createdAt}]}` |
| GET | `/api/v1/risk/reviews/{identityId}` | `{identity_id, reviews:[{reviewId, triggerType, riskScore, riskBand, description, createdAt, dueDate}]}` |

## Events

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/events/ingest` | `{"event_type" (required), "identity_id", "source", "severity", "metadata": {...}}` → `202 {"status":"accepted","eventId":"<uuid>"}`. Publishes to NATS; processor applies risk delta. |

See the [Event Catalog](../events/catalog.md) for all `event_type` values and weights.

## Roles / Groups / Entitlements

| Method | Path | Guard | Description |
|--------|------|-------|-------------|
| GET | `/api/v1/groups` | — | List roles/groups |
| POST | `/api/v1/groups` | `create_group` | Create group `{"name","description","role_type","attributes"}` |
| GET | `/api/v1/groups/{id}` | — | Get group |
| DELETE | `/api/v1/groups/{id}` | `delete_group` | Delete group |
| POST | `/api/v1/groups/{id}/entitlements` | — | Link entitlement to role `{"entitlement_id","condition","expires_at"}` → `{"status":"linked"}` |
| DELETE | `/api/v1/groups/{id}/entitlements/{entitlement_id}` | — | Unlink → `{"status":"unlinked"}` |
| POST | `/api/v1/roles/assign` | `assign_role` | Assign role to identity |
| POST | `/api/v1/roles/remove` | `remove_role` | Remove role from identity |
| GET | `/api/v1/entitlements` | — | List entitlements |
| POST | `/api/v1/entitlements` | — | Create entitlement `{"app_name","permission_level","entitlement_type","risk_classification","is_toxic"}` |

## Connectors

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/connectors` | List connectors |
| POST | `/api/v1/connectors` | Create `ConnectorConfig{name,type,status,endpoint,auth_type,...}` (types: `entra_id, ldap, active_directory, scim, okta, aws_iam, gcp_iam, generic, csv`) |
| QUERY/POST | `/api/v1/connectors/test` | Test connection |
| GET | `/api/v1/connectors/stats` | Connector stats (JWT-exempt) |
| GET | `/api/v1/connectors/{id}` | Get connector |
| DELETE | `/api/v1/connectors/{id}` | Delete connector |
| POST | `/api/v1/connectors/{id}/connect` | Connect |
| POST | `/api/v1/connectors/{id}/disconnect` | Disconnect |
| QUERY/POST | `/api/v1/connectors/{id}/test` | Test existing |
| POST | `/api/v1/connectors/{id}/sync` | Sync (full) |
| POST | `/api/v1/connectors/{id}/sync-delta` | Delta sync |
| GET | `/api/v1/connectors/{id}/users` | Connector users |
| GET | `/api/v1/connectors/{id}/identities` | Linked identities |
| GET | `/api/v1/connectors/{id}/schema` | Schema |
| GET | `/api/v1/connectors/{id}/health` | Health |
| GET | `/api/v1/connectors/{id}/groups` | Groups |
| GET | `/api/v1/connectors/{id}/entitlements` | Entitlements |
| GET | `/api/v1/connectors/{id}/resources` | Resources |
| POST | `/api/v1/connectors/{id}/full-sync` | Full sync |
| POST | `/api/v1/connectors/{id}/sync-groups` | Sync groups |
| POST | `/api/v1/connectors/{id}/sync-entitlements` | Sync entitlements |
| POST | `/api/v1/connectors/{id}/sync-resources` | Sync resources |
| POST | `/api/v1/connectors/csv/upload` | CSV upload |

LCM action enum: `create_user, update_user, delete_user, enable_user, disable_user, create_group, update_group, delete_group, add_to_group, remove_from_group, assign_role, revoke_role, full_sync`. Status: `pending, in_progress, success, failed, skipped`.

## LCM & certifications

| Method | Path | Guard | Description |
|--------|------|-------|-------------|
| POST | `/api/v1/lcm` | `execute_lcm` | Execute lifecycle action `{"action","connector_id","payload",...}` |
| GET | `/api/v1/lcm/history` | — | LCM history |
| POST | `/api/v1/certifications/generate` | `execute_lcm` | Start `AccessCertificationWorkflow` (campaign defaults: quarterly) |
| GET | `/api/v1/certifications` | — | List campaigns |
| POST | `/api/v1/certifications/entries/{id}/decide` | `execute_lcm` | `{"decision":"certified|revoked|modified","notes"}` |

## Vault

| Method | Path | Guard | Description |
|--------|------|-------|-------------|
| GET | `/api/v1/vault/secrets` | — | List secrets (ciphertext masked `"[encrypted]"`) |
| POST | `/api/v1/vault/secrets` | `vault_store_secret` | `{"name","secret_type","reference","plaintext"}` → `{"id"}` (AES-256-GCM) |
| GET | `/api/v1/vault/secrets/{id}` | — | Retrieve decrypted plaintext |
| DELETE | `/api/v1/vault/secrets/{id}` | `vault_delete_secret` | Delete secret |

Secret types: `connector_password, client_secret, api_key, tls_cert`.

## Audit

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/audit/logs` | List with `Filter{level,method,path,status,source_ip,since,until}` + limit/offset; reverse-chronological |
| GET | `/api/v1/audit/logs/{id}` | Single entry |
| GET | `/api/v1/audit/stats` | `{total, by_level, capacity, usage_pct}` (JWT-exempt) |
| GET | `/api/v1/audit/verify` | Verify the SHA-256 tamper-evident chain |

## CAEP

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/caep/events` | List CAEP events |
| POST | `/api/v1/caep/broadcast` | Broadcast CAEP event (HMAC-signed webhook if configured) |

## GraphQL

| Method | Path | Description |
|--------|------|-------------|
| POST | `/graphql` | GraphQL API (gqlgen). Queries: `Identities`, `Identity`, `Agents`, `Agent`, `Connectors`, `Connector`, `ConnectorUsers/Groups/Health`, `AuditLogs`, `Health`, `Ready`; mutations for identity & connector CRUD. Enums: `IdentityType` (HUMAN, SERVICE_ACCOUNT, AI_AGENT, ROBOT, IOT_DEVICE, RPA_BOT, API_KEY), `IdentityStatus` (ACTIVE, INACTIVE, SUSPENDED, TERMINATED, REVOKED, PENDING_REVIEW). |

## Error codes

| Status | Meaning |
|--------|---------|
| 400 | Bad request / validation (`{"error":"..."}`) |
| 401 | Missing/invalid credentials |
| 403 | Forbidden (workflow guard / RBAC) |
| 404 | Not found |
| 429 | Rate limited (`Retry-After: 1`) |
| 500 | Internal error |
| 503 | Not ready (dependency down) |

## Related

- [SCIM 2.0](scim.md)
- [OIDC / OAuth 2.0](oidc.md)
- [Auth middleware details](../architecture/overview.md)
