# Configuration Reference

All runtime configuration for the backend is via environment variables. The containerized stack (docker-compose) sets these for you; run natively with defaults or override as needed.

## Backend (`identity-service`)

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgresql://observeid:observeid@localhost:5432/observeid?sslmode=disable` | PostgreSQL connection. In compose: `postgres:5432/observeid`. |
| `NEO4J_URI` | `bolt://localhost:7687` | Neo4j Bolt endpoint. In compose: `bolt://neo4j:7687`. |
| `NEO4J_USER` | `neo4j` | Neo4j user. |
| `NEO4J_PASSWORD` | *(empty)* | Neo4j password. In compose: `observeid123`. |
| `REDIS_ADDR` | `localhost:6379` | Redis address. In compose: `redis:6379`. |
| `REDIS_PASSWORD` | *(empty)* | Redis password (empty for local/Upstash token for cloud). |
| `REDIS_TLS` | `false` | `true` for Upstash / cloud Redis over TLS. |
| `TEMPORAL_HOST` | `localhost:7233` | Temporal gRPC address. In compose: `temporal:7233`. |
| `TEMPORAL_NAMESPACE` | `critical-offboarding` | Default Temporal namespace; doubles as the app task queue. |
| `CORS_ORIGIN` | *(empty)* | Allowed CORS origin. Empty = `*`. Set to the frontend URL in production. |
| `QDRANT_ADDR` | `localhost:6333` | Qdrant (reserved; unused by current risk flow). |
| `MASTER_KEY` | *(empty)* | Master key required by the workflow guard (`X-Master-Key`) for sensitive ops (grant/revoke access, kill-switch, LCM, vault writes, etc.). |
| `NATS_URL` | `nats://localhost:4222` | NATS JetStream URL. In compose: `nats://nats:4222`. |
| `JWKS_URL` | `http://localhost:8080/.well-known/jwks.json` | JWKS endpoint used by the JWT auth middleware to validate tokens. |
| `API_KEYS` | *(empty)* | Internal API keys, format `name:key,name:key`. These bypass JWT auth (used by Temporal workers / internal calls). Also accepted via `X-API-Key` or `Authorization: Bearer`. |
| `JWT_SIGNING_KEY` | *(generated)* | Signing key for minted tokens. |
| `DEV_LOGIN_ENABLED` | `true` | When `false`, `POST /api/v1/dev/login` is disabled. Auto-login no-ops in production. |
| `RISK_RECALC_CRON_SCHEDULE` | `*/15 * * * *` | Cron expression for the periodic risk-recalculation workflow. |
| `VAULT_MASTER_KEY` | *(required for vault)* | AES-256-GCM master key for `vault/secrets`. Must be ≥ 32 chars; generate with `openssl rand -hex 32`. |
| `VAULT_PATH` | `/tmp/genid-vault.json` | File where encrypted secrets are persisted. |
| `FRONTEND_DIR` | auto-probe | If set, the Go binary serves the static export from this dir instead of probing `frontend/out`. |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | *(empty)* | Optional TLS termination for the Go server. |
| `CAEP_WEBHOOK_URL` | *(empty)* | If set, `BroadcastCAEPEvent` POSTs HMAC-SHA256-signed CAEP/SET events to this URL. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | *(otel-collector)* | OpenTelemetry trace export endpoint. |

## Service ports

| Service | Host port | In-container | Notes |
|---------|-----------|--------------|-------|
| identity-service | 8080 | 8080 | REST + OIDC + GraphQL |
| frontend (nginx) | 3001 | 3000 | Static export, proxies `/api`, `/scim`, `/graphql`, `/health` |
| postgres | 5434 | 5432 | `observeid` / `observeid` |
| neo4j | 7474 / 7687 | 7474 / 7687 | Browser UI + Bolt; `neo4j` / `observeid123` |
| redis | 6379 | 6379 | No auth locally |
| nats | 4222 / 8222 | 4222 / 8222 | Client + monitoring |
| temporal | 7233 / 8233 | 7233 / 8233 | gRPC + HTTP API |
| temporal-ui | 8234 | 8080 | Web UI |
| openfga | 8090 / 8091 | 8080 / 8081 | HTTP + gRPC |
| grafana | 3000 | 3000 | `admin` / `observeid123` |
| otel-collector | 4317 / 4318 | 4317 / 4318 | OTLP gRPC/HTTP |

> Note: host port 3000 is used by **Grafana**; the frontend maps host `3001`. The nginx container listens on 3000 *inside* its network.

## Default seed data

Created by `infrastructure/postgres/init.sql` on a fresh volume:

- Tenant: `observeid` (UUID `00000000-0000-0000-0000-000000000001`)
- Admin identity: `admin@genid.io`
- Five base roles with admin assignment

Neo4j seeds (`infrastructure/neo4j/init.cypher`) add roles, entitlements, resources, and relationships; demo identities (`demo-*`) may be seeded separately for the risk demo.

## Temporal namespaces

`temporal-admin` auto-creates on startup:

| Namespace | Retention |
|-----------|-----------|
| `critical-offboarding` | 720 h |
| `provisioning` | 336 h |
| `reconciliation` | 168 h |
| `analysis` | 168 h |
