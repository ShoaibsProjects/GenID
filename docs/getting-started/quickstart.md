# Quickstart

Get the full GenID stack running locally in a few minutes.

## Prerequisites

- Docker Desktop (or Docker Engine) with Docker Compose v2
- Go 1.25+ (only needed if you run the backend natively instead of in Docker)
- Node 20+ (only needed to rebuild the frontend)
- ~8 GB free memory (the stack runs 13 containers)

## 1. Clone and start infrastructure

```bash
git clone https://github.com/ShoaibsProject/observeid-V2.git
cd GenID
cd infrastructure
docker compose up -d
```

This starts 13 containers:

| Container | Purpose | Port |
|-----------|---------|------|
| `genid-postgres` | Source of truth (RLS) | 5434 → 5432 |
| `genid-neo4j` | Graph / risk store | 7474, 7687 |
| `genid-redis` | Cache, JIT grants, locks | 6379 |
| `genid-nats` | JetStream event bus | 4222, 8222 |
| `genid-openfga` | (Reserved) fine-grained authz | 8090, 8091 |
| `genid-temporal` | Workflow orchestration | 7233, 8233 |
| `genid-temporal-admin` | Namespace bootstrap | — |
| `genid-temporal-ui` | Temporal web UI | 8234 |
| `genid-otel` | OpenTelemetry collector | 4317, 4318 |
| `genid-grafana` | Metrics dashboards | 3000 (host) |
| `genid-identity-service` | Go API + outbox + NATS producer | 8080 |
| `genid-event-processor` | NATS consumer → risk scorer | — |
| `genid-frontend` | Next.js static export via nginx | 3001 → 3000 |

Wait for health:

```bash
docker compose ps
```

All containers should show `Up ... (healthy)` or `Up`. If the identity service crash-loops, see [Operations → Troubleshooting](../operational/docker.md#troubleshooting).

> **Known gotcha:** if the Postgres volume predates the `outbox_events` table (init.sql only runs on fresh volumes), the outbox processor will log `relation "outbox_events" does not exist`. Create the table manually:
>
> ```sql
> CREATE TABLE IF NOT EXISTS outbox_events (
>     id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
>     event_type VARCHAR(50) NOT NULL,
>     aggregate_type VARCHAR(50) NOT NULL,
>     aggregate_id VARCHAR(255) NOT NULL,
>     payload JSONB NOT NULL,
>     metadata JSONB,
>     created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
>     processed BOOLEAN NOT NULL DEFAULT FALSE,
>     processed_at TIMESTAMPTZ,
>     retry_count INTEGER NOT NULL DEFAULT 0,
>     error_message TEXT,
>     expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days'
> );
> CREATE INDEX idx_outbox_unprocessed ON outbox_events (processed, created_at) WHERE processed = FALSE;
> CREATE INDEX idx_outbox_aggregate ON outbox_events (aggregate_type, aggregate_id);
> CREATE INDEX idx_outbox_retry ON outbox_events (processed, retry_count, created_at) WHERE processed = FALSE AND retry_count > 0;
> CREATE INDEX idx_outbox_expires ON outbox_events (expires_at) WHERE processed = FALSE;
> ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id);
> ```
>
> then `docker compose restart identity-service event-processor`.

## 2. Verify the API

```bash
# Health (liveness)
curl http://localhost:8080/health

# Dev login (demo mode) → returns an admin JWT
curl -X POST http://localhost:8080/api/v1/dev/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin@genid.io","password":"dev-login"}'
```

## 3. Open the UI

Browse to **http://localhost:3001**.

The frontend auto-logs-in via `devLogin()` (admin@genid.io / dev-login, dev mode only). If it does not, click the **Settings** page and confirm the API base is `http://localhost:8080` (or reload — the dev-login bootstrap runs once on mount).

## 4. Try the core flow (5 minutes)

1. **Risk Intelligence** (`/risk`) — fleet-wide scoreboard. `demo-dave` (if seeded) is pre-loaded as over-privileged.
2. **Event Simulator** (`/events`) — pick identity `demo-bob`, send `failed_login` × 3, watch the gauge hit **elevated** (300).
3. Send `credential_leaked` (300) twice → score crosses **critical** (800+).
4. Open the processor logs to see the auto-response:

```bash
docker logs genid-event-processor --tail 50
```

You should see `CRITICAL: terminating all sessions` and a micro-review being created. Verify on `/risk` (the review appears in the identity drill-down).

## Running natively (optional)

If you prefer running the backend on your host:

```bash
cd backend
cp .env.example .env
go run cmd/identity-service/main.go
```

The binary probes `frontend/out`, `../frontend/out`, `./frontend/out` and serves the static export itself, so you can drop the nginx frontend container.

## Known gotcha: `outbox_events` missing on old volumes

If you have a Postgres volume from a build before the outbox table existed, the backend logs `relation "outbox_events" does not exist` (SQLSTATE 42P01). `init.sql` only runs on a **fresh** volume, so you must create the table manually. Full fix (create table + indexes, restart services) in [Operations → Troubleshooting](../operational/docker.md#relation-outbox_events-does-not-exist-sqlstate-42p01).

## Known gotcha: 502 Bad Gateway after backend restart

nginx caches the backend container IP at startup; if the IP changes on restart, the UI returns 502 until reload:

```bash
docker exec genid-frontend nginx -s reload
```

---

## Next

- [Configuration reference](configuration.md)
- [Architecture overview](../architecture/overview.md)
- [Demo guide](../guides/demo.md)
