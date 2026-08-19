# Testing & Verification Guide

End-to-end verification of the live stack. Run the whole thing against **http://localhost:3001**.

## 1. Smoke checks (2 min)

| Check | Expected |
|-------|----------|
| `docker compose ps` | 13 containers, all `Up` / `healthy` |
| `curl -s localhost:8080/health` | `{"status":"ok","service":"genid-identity","version":"1.0.0"}` |
| `curl -s localhost:8080/ready` | `{"status":"ready","checks":{...}}` (no 503) |
| `curl -s localhost:8080/healthz` | `{"status":"healthy","checks":{...}}` |
| http://localhost:3001 | auto-login lands on Dashboard |
| Dashboard | identities=6, avg=0 (fresh), risk bands visible |

## 2. Login flow

```bash
curl -s -X POST localhost:8080/api/v1/dev/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin@genid.io","password":"dev-login"}'
```
→ `access_token` (5 min TTL). Use it as `Authorization: Bearer <token>` below.

## 3. Provisioning (JML)

```bash
# Create via SCIM (async offboarding / sync create)
curl -s -X POST localhost:8080/scim/v2/Users \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],
       "userName":"jane@example.com","displayName":"Jane","active":true}'
```
→ `201 {"id":...}`. Then:
```bash
curl -s localhost:8080/api/v1/identities | jq 'map(select(.email|test("jane")))'
```
→ Jane present (Neo4j syncs in ~100ms). Offboard via `DELETE /scim/v2/Users/{id}` → `202 {"workflow_id":"offboard-<id>"}`; watch it complete in Temporal (http://localhost:8234).

## 4. Risk engine (the full arc)

**Target**: fresh identity. **Method**: Event Simulator in the UI, or direct:

```bash
curl -s -X POST localhost:8080/api/v1/events/ingest -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"event_type":"auth.failed_login","identity_id":"<uuid>","severity":"high","metadata":{"attempts":3}}'
```
Send `auth.failed_login` (high) ×3, then `credential_leaked` (critical).

| After | Expect (via `GET /api/v1/risk/score/<uuid>`) |
|-------|-------------|
| 3× failed_login | dynamic ≈ +300 → **elevated** (300–599), micro-review `elevated_risk` |
| + credential_leaked | +300 → **critical** (800+), sessions terminated, review `critical_risk` due +3d |

Verify band actions in Neo4j:
```cypher
MATCH (i:Identity)-[:HAS_REVIEW]->(r:Review) RETURN i.email, r.trigger_type, r.status;
MATCH (i:Identity)-[:HAS_SESSION]->(s:Session) WHERE s.status <> 'active' RETURN i.email, count(s);
```

## 5. Access control

```bash
curl -s -X POST localhost:8080/api/v1/access/check \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"identity_id":"<uuid>","resource_id":"r1","resource_type":"resource","action":"read"}'
```
→ `{"allowed":true/false,"reason":...,"evaluated":"neo4j","latency_ms":N}`.

## 6. Audit integrity

```bash
curl -s localhost:8080/api/v1/audit/verify -H "Authorization: Bearer $TOKEN"
```
→ chain verifies. Tamper with a row (UPDATE hash = 'x') → reverify fails.

## 7. GraphQL

```bash
curl -s -X POST localhost:8080/graphql -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ identities { id email type status } }"}'
```

## 8. OIDC (IDP test console)

UI → **IDP** (`/idp`): create client → run authorize → token → userinfo end-to-end. Verify JWKS at `http://localhost:8080/.well-known/jwks.json` and discovery at `/.well-known/openid-configuration`.

## Reset between runs

See [demo guide — reset](demo.md#reset-demo-state).

## Backend unit tests

```bash
cd backend && go test ./...   # requires running Postgres/Redis/Neo4j for integration paths
```
