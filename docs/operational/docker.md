# Docker Compose Operations

The stack runs as **13 containers** via `infrastructure/docker-compose.yml`.

## Topology

| Service | Image | Ports | Healthcheck |
|---------|-------|-------|-------------|
| `identity-service` | `golang:1.25-alpine` (mounts `../backend`) | 8080 | `wget -qO- localhost:8080/health` |
| `event-processor` | `golang:1.25-alpine` (mounts `../backend`) | — | `wget -qO- localhost:8080/health` (separate process, same binary) |
| `frontend` | `nginx:alpine` (static export `../frontend/out`) | 3001 | curl :80 |
| `postgres` | `postgres:16-alpine` | 5434:5432 | pg_isready |
| `neo4j` | `neo4j:5` | 7474:7474, 7687:7687 | curl bolt (:7474) |
| `redis` | `redis:7-alpine` | 6379 | redis-cli ping |
| `nats` | `nats:2.10-alpine` | 4222, 8222 | `nats-server -T` |
| `temporal` | `temporalio/auto-setup:1.25` | 7233, 8233, 8234 | tctl |
| `temporal-admin-tools` | `temporalio/admin-tools:1.25` | — | — |
| `openfga` | `openfga/openfga:latest` | 8080→`8081`, 8090, 8091 | /healthz |
| `grafana` | `grafana/grafana:11` | 3000 | /api/health |
| `otel-collector` | `otel/opentelemetry-collector-contrib` | 4317, 4318 | — |
| `temporal-ui` | `temporalio/ui` | 8080→`8083`, 8084 | — |

## Common commands

```bash
docker compose up -d                 # start all
docker compose ps                    # status
docker compose logs -f identity-service
docker compose logs -f event-processor
docker compose restart identity-service event-processor
docker compose down                  # stop (keeps volumes)
docker compose down -v               # wipe volumes (re-seed everything)
```

## Credentials (local dev)

| Service | Credential |
|---------|------------|
| Postgres | `observeid:observeid@localhost:5434/observeid` |
| Neo4j | `neo4j:observeid123` @ bolt://localhost:7687 |
| Grafana | `admin:observeid123` |
| Temporal UI | http://localhost:8234 |

## Troubleshooting

### `relation "outbox_events" does not exist` (SQLSTATE 42P01)

Cause: Postgres volume predates the outbox table; `init.sql` only runs on a **fresh** volume.

Fix (in-container psql):

```bash
docker exec -it genid-postgres psql -U observeid -d observeid <<'SQL'
CREATE TABLE IF NOT EXISTS outbox_events (
  id UUID PRIMARY KEY,
  event_type VARCHAR(50) NOT NULL,
  aggregate_type VARCHAR(50) NOT NULL,
  aggregate_id VARCHAR(255),
  payload JSONB NOT NULL,
  metadata JSONB,
  created_at TIMESTAMPTZ DEFAULT now(),
  processed BOOLEAN DEFAULT FALSE,
  processed_at TIMESTAMPTZ,
  retry_count INT DEFAULT 0,
  error_message TEXT,
  expires_at TIMESTAMPTZ,
  tenant_id UUID
);
CREATE INDEX IF NOT EXISTS idx_outbox_events_unprocessed ON outbox_events (created_at) WHERE processed = FALSE;
CREATE INDEX IF NOT EXISTS idx_outbox_events_processed ON outbox_events (processed, created_at);
CREATE INDEX IF NOT EXISTS idx_outbox_events_retry ON outbox_events (retry_count);
CREATE INDEX IF NOT EXISTS idx_outbox_events_expires ON outbox_events (expires_at) WHERE expires_at IS NOT NULL;
SQL
docker compose restart identity-service event-processor
```

### 502 Bad Gateway on the UI after backend restart

Cause: nginx caches the backend container's IP at startup; on restart the IP can change.

Fix:
```bash
docker exec genid-frontend nginx -s reload
```

### Risk scores not updating after events

Check `event-processor` logs for NATS/delivery errors, then:
```bash
docker compose restart event-processor
```

### Fresh-volume seeding

`docker compose down -v && docker compose up -d` re-runs Postgres `init.sql` and Neo4j `init.cypher` from scratch (tenant, admin, base roles, demo graph).

## Related

- [Quickstart](../getting-started/quickstart.md)
- [Configuration reference](../getting-started/configuration.md)
- [Security model](security.md)
