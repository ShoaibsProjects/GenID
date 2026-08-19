# Security Model

GenID's security posture across authentication, authorization, data protection, and integrity.

## Authentication

| Mechanism | Detail |
|-----------|--------|
| JWT (RS256) | Signed via embedded OIDC provider; verified from `/.well-known/jwks.json` |
| API keys | `API_KEYS=name:key,...`; `X-API-Key` or bearer fallback for internal workers |
| Master key | `MASTER_KEY` env; required via `X-Master-Key` for workflow-guarded ops (grant/revoke access, kill switch, LCM, vault writes, SCIM writes, role changes, bulk import) |
| OIDC | Full provider: authorize/token/userinfo/introspect/revoke + device flow + PKCE (S256); refresh tokens hashed at rest, rotated on use |
| Dev login | `DEV_LOGIN_ENABLED` (default true in dev); disable in production |

## Authorization

- **RBAC**: `roles` claim on JWT; middleware enforces per-route guards (e.g. `grant_access`, `agent_kill_switch`, `vault_store_secret`).
- **Policy engine**: Amazon Cedar (`cedar_policies` table, `CheckAccessPolicy` activity). Decision caching in Redis (30s TTL).
- **SoD**: `CheckSoDConflicts` — toxic entitlement pairs (`CONFLICTS_WITH`), transitive roles, rubberband entitlements; hard deny when risk > 0.7.
- **Tenant isolation**: Postgres **RLS** on 28 tables keyed by `app.current_tenant` session variable, set per-request from the JWT.
- **Emergency access**: temporary grants tracked in `emergency_access` with mandatory expiry + review.

## Data protection

- **At rest**: vault secrets AES-256-GCM (`VAULT_MASTER_KEY`); refresh tokens SHA-256 hashed; audit chain SHA-256 linked hashes; agent cards signature scheme `ml-dsa-44`.
- **In transit**: JWT-RS256; CAEP webhooks HMAC-SHA256 signed; OIDC PKCE; HTTPS assumed in production (HSTS header set under TLS).

## Request hardening (middleware)

- Security headers (`nosniff`, framing DENY, strict-referrer, HSTS).
- CORS restricted to configured origins.
- **Rate limiting**: 100 req/s, burst 200, per-IP → `429` + `Retry-After: 1` (protects `/api/v1/dev/login`, token endpoints).
- Request validation: JSON content-type enforcement, 10 MB body cap, OpenTelemetry tracing on every request.

## Integrity & tamper evidence

- `audit_log` entries are SHA-256 chained (`prev_hash` → `hash`); `GET /api/v1/audit/verify` recomputes and detects any modification.
- Agent cards signed with ML-DSA-44; delegation chains depth-limited (`max_depth_remaining`, default 1) with scope narrowing.
- Kill-switch `CascadeRevokeWorkflow` revokes SPIFFE SVID → OAuth tokens → API keys → rotates credentials.

## Secrets

| Secret | Where | Rotation |
|--------|-------|----------|
| `MASTER_KEY` | env | manual |
| `VAULT_MASTER_KEY` | env (`openssl rand -hex 32`) | manual |
| `API_KEYS` | env | manual |
| client secrets | `oidc_clients` | via client mgmt |
| vault secrets | PG (AES-256-GCM) | via vault API |

## Known production hardening items

> Local demo mode is intentionally permissive. Before production:
> 1. Set `DEV_LOGIN_ENABLED=false`.
> 2. Move `MASTER_KEY` / `VAULT_MASTER_KEY` / `API_KEYS` to a secret manager.
> 3. Terminate TLS at a reverse proxy (HSTS only activates under TLS).
> 4. Enable production OIDC client auth (client_secret_basic) + enforce PKCE.
> 5. Restrict CORS origins; rotate all seed credentials.

## Related

- [Auth middleware](../api/overview.md#authentication)
- [Audit & chain of custody](../api/overview.md#audit)
- [RBAC data model](../data-model/postgres.md#rbac)
