<picture>
  <source media="(prefers-color-scheme: dark)" srcset="media/banner.svg">
  <img alt="ObserveID — Identity Fabric Engine" src="media/banner.svg" width="100%">
</picture>

<br/>

<div align="center">

[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go&logoColor=white&labelColor=1C1C24)](https://go.dev)
[![Next.js 14](https://img.shields.io/badge/Next.js-14-000000?style=flat&logo=next.js&logoColor=white&labelColor=1C1C24)](https://nextjs.org)
[![Neo4j 5](https://img.shields.io/badge/Neo4j-5-4581C3?style=flat&logo=neo4j&logoColor=white&labelColor=1C1C24)](https://neo4j.com)
[![Temporal](https://img.shields.io/badge/Temporal-1.28-101010?style=flat&logo=temporal&logoColor=white&labelColor=1C1C24)](https://temporal.io)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-4169E1?style=flat&logo=postgresql&logoColor=white&labelColor=1C1C24)](https://postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?style=flat&logo=redis&logoColor=white&labelColor=1C1C24)](https://redis.io)

[![CI](https://github.com/ShoaibsProjects/observeid/actions/workflows/ci.yml/badge.svg)](https://github.com/ShoaibsProjects/observeid/actions/workflows/ci.yml)
[![Deploy](https://github.com/ShoaibsProjects/observeid/actions/workflows/deploy.yml/badge.svg)](https://github.com/ShoaibsProjects/observeid/actions/workflows/deploy.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-8B5CF6?style=flat&labelColor=1C1C24)](LICENSE)
[![Tests](https://img.shields.io/badge/tests-112%20passed-22C55E?style=flat&labelColor=1C1C24)]()

<br/>

### **Identity Governance for the AI Era**

**Unify humans, service accounts, AI agents, and IoT devices under a single graph-based policy engine — evaluated in milliseconds, orchestrated by durable workflows.**

<br/>

---

</div>

## What is ObserveID?

ObserveID is an **open-source Identity Governance and Administration (IGA) platform** built on four architectural decisions no other IAM product makes:

| Decision | Why it matters |
|----------|---------------|
| **Neo4j as the identity graph** | Relationships are first-class citizens — not an afterthought mapped onto SQL JOINs. You can traverse who-has-access-to-what in a single Cypher query instead of 12 SQL joins across 7 bridge tables. |
| **Temporal for durable execution** | Access grants, offboarding, and SoD scans are long-running workflows, not synchronous HTTP calls. They survive crashes, retry automatically, and fan out to parallel revocation paths. |
| **Cedar for policy-as-code** | ABAC/RBAC/ReBAC rules are version-controlled, testable, and evaluated at access-check time. No more opaque admin consoles where policy drift goes undetected for quarters. |
| **Full-text search via PostgreSQL** | Identity lists with 17 sortable/filterable columns, GIN tsvector indexes, and parameterized queries. Not "eventually consistent" — ACID strong. |

**Every identity type is a first-class entity.** Humans, service accounts, API keys, AI agents, RPA bots, and IoT devices all share the same graph, policy engine, and lifecycle workflows.

---

## Why ObserveID over Okta, SailPoint, or Entra?

| Dimension | Okta / SailPoint / Entra | ObserveID |
|-----------|-------------------------|-----------|
| **Identity model** | Human-first. NHIs are an afterthought. | Humans + NHIs share the same graph, policies, and workflows. |
| **Relationship model** | Flat SQL tables with bridge tables for RBAC. | Neo4j graph with `*1..N` traversal — roles, entitlements, resources, and delegation chains in a single query. |
| **Policy engine** | Proprietary rules engines. Invisible to CI/CD. | Cedar ABAC policies stored as code in PostgreSQL — versioned, testable, evaluable in CI. |
| **Access evaluation** | Synchronous API calls. No concept of durability. | Multi-tier: Redis cache → Neo4j graph traversal → Cedar policy evaluation. Fail-open configurable per layer. |
| **Workflow reliability** | Best-effort HTTP calls. Failures = broken state. | Temporal durable workflows with retry, signal handling, and child workflows. Offboarding fans out to 11 parallel activities. |
| **NHI governance** | Separate console or non-existent. | Agent cards (SPIFFE-style), kill switches, delegation chains with max-depth, anomaly detection via Temporal cron. |
| **Deployment** | SaaS-only. You don't own your identity data. | Self-hosted (Docker Compose on a laptop) or free-tier cloud (Neon, AuraDB, Upstash, Fly.io). Zero infrastructure cost. |
| **API design** | REST with inconsistent semantics. | REST + GraphQL + SCIM 2.0. QUERY method (RFC 10008) for safe, idempotent read operations with request bodies. |
| **Audit trail** | Separate product or add-on. | Built-in middleware logs every request with method, path, status, latency, source IP. Queryable with 7 filter dimensions. |

> **The core insight:** Existing IAM products optimize for selling to CIOs. ObserveID optimizes for the engineering team that has to integrate, extend, and audit it.

---

## How Fast Is It?

Access checks traverse **three data stores** and return a decision. Here's what that looks like in practice:

```
IdentityGraph (Neo4j)     PolicyEngine (Cedar/PG)    Cache (Redis)
─────────────────────     ──────────────────────     ────────────
MATCH path *1..3          SELECT effect FROM          GET policy:decision
15-30ms cold              2-5ms indexed query         <1ms cache hit
5-8ms warm                                          30s TTL auto-expiry
```

| Scenario | Latency | Layers Hit |
|----------|---------|-----------|
| Cached decision (Redis hit) | **< 1ms** | Redis only |
| Neo4j path + Cached policy | **8-12ms** | Redis + Neo4j |
| Neo4j path + Cedar evaluation | **15-30ms** | Redis + Neo4j + PostgreSQL |
| Full cold path (no caches) | **50-100ms** | All three stores |

**Scale characteristics:**
- PostgreSQL: 50 connections (self-hosted), GIN full-text index on identity search
- Neo4j: Session-per-request pooled via driver, 3-hop traversal limit (configurable)
- Redis: Policy decision cache (30s TTL), revocation sticky cache (5min TTL)
- Temporal Worker: 500 concurrent activities, 500 concurrent workflow tasks
- HTTP server: gorilla/mux with ReadTimeout 15s, WriteTimeout 30s, graceful shutdown
- Rate limiter: Per-IP token bucket, 100 req/s burst 200

---

## Architecture

```
                              ┌──────────────────┐
                              │   Cloudflare      │
                              │   Pages + Proxy   │
                              └────────┬─────────┘
                                       │ HTTPS
                                       ▼
                         ┌─────────────────────────┐
                         │      Fly.io (Go)        │
                         │         :8080           │
                         │  ┌───────────────────┐  │
                         │  │ gorilla/mux       │  │  ← 30+ HTTP handlers
                         │  │ otelhttp (OTLP)   │  │  ← OpenTelemetry tracing
                         │  │ gqlgen (GraphQL)  │  │  ← Typed GraphQL API
                         │  │ audit.LoggingMW   │  │  ← Every request logged
                         │  │ middleware chain   │  │  ← Auth → RateLimit → Validate
                         │  └───────┬───────────┘  │
                         └──────────┼──────────────┘
                                    │
          ┌─────────────────────────┼─────────────────────────┐
          │                         │                         │
          ▼                         ▼                         ▼
┌──────────────────┐   ┌──────────────────┐   ┌──────────────────┐
│   PostgreSQL     │   │     Neo4j 5      │   │     Redis 7      │
│   (Source of     │   │   (Identity      │   │   (Cache Layer)  │
│    Truth)        │   │    Graph)        │   │                  │
│                  │   │                  │   │ Policy decisions │
│ 29 tables        │   │ Identity nodes   │   │ Revocation cache │
│ 8 enum types     │   │ Resource nodes   │   │ Rate limit state │
│ GIN tsvector     │   │ Role nodes       │   │ JIT grant TTL    │
│ 50 conns (local) │   │ HAS_ROLE edges   │   │                  │
│ ACID writes      │   │ *1..N traversal  │   │ 30s/5min TTLs    │
└──────────────────┘   └──────────────────┘   └──────────────────┘
          │
          ▼
┌──────────────────┐
│  Temporal Cloud  │   ← Durable execution layer
│  (Workflow       │
│   Engine)        │   9 workflows registered:
│                  │   Onboard, Offboard, GrantAccess,
│ Worker: 500/500  │   RevokeAccess, JITAccess,
│ activities/WF    │   CascadeRevoke, SoDDetection,
│                  │   AgentAnomalyDetection
└──────────────────┘
```

**Read path:** Handlers query PostgreSQL (ACID) for listing/filtering/searching. Neo4j for graph traversal and relationship queries. Redis for caching with automatic expiry.

**Write path:** Handlers fire Temporal workflows. Workflows execute activities that write to both PostgreSQL and Neo4j (dual-write with graceful degradation). Redis caches are invalidated on writes.

---

## Quick Start

**60 seconds to a running identity platform:**

```bash
# 1. Clone and start infrastructure
git clone https://github.com/ShoaibsProjects/observeid.git
cd observeid
make up          # PostgreSQL, Neo4j, Redis, Temporal, Zookeeper — 6 containers

# 2. Seed the database
make dev-db      # Creates 29 tables, seeds 15 HR identities, 8 resources, 3 Cedar policies

# 3. Start the backend
cd backend && go run ./cmd/identity-service/
# → Ready at http://localhost:8080
# → Health: http://localhost:8080/healthz

# 4. (Optional) Start the frontend in another terminal
cd frontend && npm install && npm run dev
# → http://localhost:3001
```

**Verify it's running:**

```bash
# Check health — all 4 dependencies alive
curl http://localhost:8080/healthz | jq

# List identities — 15 seeded from HR CSV
curl "http://localhost:8080/api/v1/identities?limit=5" | jq

# Check access — multi-tier evaluation
curl -X QUERY http://localhost:8080/api/v1/access/check \
  -H "Content-Type: application/json" \
  -d '{"identity_id":"46d0ca3a-...","resource_id":"res-aws-prod","action":"read"}' | jq
```

**Technical requirements:** Go 1.25+, Node.js 18+, Docker, Make. Cloud deployment uses free tiers of Neon, AuraDB, Upstash, Temporal Cloud, Fly.io — zero cost to run.

---

## Features

### Identity Management — Complete
| Feature | Implementation |
|---------|---------------|
| Identity types | 7 types: human, service_account, ai_agent, robot, iot_device, rpa_bot, api_key |
| Lifecycle states | 6 states: active, inactive, suspended, terminated, revoked, pending_review |
| Identity listing | PostgreSQL query with full-text search (GIN tsvector), filter by status/type/dept/source, sort by any column, pagination (limit/offset) |
| Identity detail | 17 fields, entitlement view, blast radius (4-hop graph traversal), risk factors |
| Bulk import | Up to 5,000 identities per batch via JSON API |
| SCIM 2.0 | User CRUD with Temporal workflow integration for offboarding |

### Connectors — 5 Implemented, 4 Defined
| Connector | Status | Capabilities |
|-----------|--------|-------------|
| Microsoft Entra ID | **Implemented** — 1,366 lines | Full sync, delta sync, users, groups, entitlements (directory roles, app roles, licenses, OAuth2 grants), resources |
| LDAP / Active Directory | **Implemented** | Search, bind, user attributes, group membership, UAC parsing |
| SCIM 2.0 | **Implemented** | Bearer token auth, CRUD, schema discovery |
| CSV Import | **Implemented** | File upload with validation, 14+ field parsing |
| Okta | Defined | Schema-ready |
| AWS IAM / GCP IAM | Defined | Schema-ready |

### Access Control — Multi-Tier
| Layer | Mechanism | Fallback |
|-------|-----------|----------|
| 1. Redis revocation cache | 5-minute sticky deny for recently revoked identities | Fail-open (proceed to next layer) |
| 2. Redis decision cache | 30-second cache of previous access decisions | Miss → Neo4j query |
| 3. Neo4j graph traversal | `MATCH path = (i)-[:HAS_ROLE\|HAS_DIRECT_ACCESS\|HAS_TEMPORARY_ACCESS*1..3]->(res)` | No path → deny |
| 4. Cedar policy evaluation | Pattern-matched permit/forbid rules from PostgreSQL | Error → default allow by path |

### Cedar Policy Engine
- **Pattern format:** `effect(identity_pattern, action_pattern, resource_pattern)` with `*` wildcards
- **Identity matching:** Type or department against policy identity pattern
- **Resource matching:** Resource ID, type, or classification against policy resource pattern
- **Priority:** Forbid always wins. At least one permit required for access.
- **Storage:** PostgreSQL `cedar_policies` table — versioned, active/inactive toggle

### Durable Workflows (Temporal)
| Workflow | Type | Activities |
|----------|------|-----------|
| `OffboardIdentityWorkflow` | On-demand | Audit trail → lock → query entitlements → 11 parallel revocations → CAEP broadcast → cascade agent delegation → audit finalize |
| `GrantAccessWorkflow` | On-demand | Policy check → SoD check → approval gate (with signal) → provision (PG + Neo4j + Redis) → JIT timer + auto-revoke |
| `RevokeAccessWorkflow` | On-demand | Force revoke → cache invalidation → CAEP broadcast |
| `JustInTimeAccessWorkflow` | On-demand | Cedar check → grant temp access → Redis TTL → auto-revoke child workflow |
| `CascadeRevokeWorkflow` | On-demand | Fan-out across dependent access with error aggregation |
| `AgentAnomalyDetectionWorkflow` | Cron | Behavioral scan → kill-switch trigger for critical anomalies |
| `DetectSoDViolationsWorkflow` | Cron | Toxic pair + transitive + rubberband SoD scans via Neo4j |

### Security
- **AES-256-GCM** vault for secrets — random nonces, crypto/rand, persistent to disk with 0600 permissions
- **WorkflowGuard** middleware — 12 operations require master permission (X-Master-Key or X-User-Role)
- **API key auth** — configurable via `API_KEYS` env var, Bearer token or X-API-Key header
- **HTTP security headers** — nosniff, XFO, X-XSS-Protection, Permissions-Policy, HSTS
- **Pre-commit gitleaks hook** — 60+ commits scanned, zero secret leaks committed

### Observability
- **OpenTelemetry** — OTLP gRPC trace exporter, Prometheus metrics on `/metrics`
- **14 metric counters** — AccessCheckTotal, AccessCheckLatency, CedarDenyRate, WorkflowExecutions
- **Grafana** — included in Docker Compose, accessible at `:3000`
- **Audit log** — in-memory ring buffer (10,000 entries), 7 filter dimensions (level, method, status, path, source IP, time range)

### Frontend — 14 Pages
| Page | Size | Features |
|------|------|----------|
| Dashboard | 257 lines | Live metrics (15s refresh), 4 service health indicators, architecture summary |
| Identities | 779 lines | Full-text search, 4 filter dimensions, 8 sortable columns, pagination, detail slide-out with entitlements + blast radius |
| Connectors | 706 lines | Stats bar (8 metrics), health badges, 5 detail tabs (accounts, groups, entitlements, resources, schema), dynamic add form per connector type |
| Audit | 747 lines | 7 filter dimensions (level, method, status, path, source IP, time range), stats bar, pagination, live/pause polling |
| Access | 433 lines | Check access, JIT access, grant/revoke — all with Cedar evaluation feedback |
| Policies | 148 lines | Cedar policies CRUD, filter by effect, policy format reference, evaluation rules |
| Vault | 194 lines | Secret store/retrieve/delete with AES-256-GCM, 7 secret types |
| Groups | 137 lines | Roles and groups with search, create, delete |
| Agents | 99 lines | Non-human identity management |
| Certifications | 67 lines | Access review campaigns with progress bars |
| SoD | 63 lines | Separation of duties rules with risk levels |
| Settings | 115 lines | System configuration |

---

## API Reference

The full API is documented inline. Key design principles:

- **QUERY method (RFC 10008)** for safe, idempotent read operations with request bodies — used for access checks, copilot queries, and connector tests
- **202 Accepted** for workflow-triggered operations — the server returns immediately, Temporal handles durability
- **SCIM 2.0** compliance for interoperable identity provisioning
- **GraphQL** endpoint at `/graphql` for typed queries and mutations
- **Content-Type validation** on all POST/PUT/PATCH/QUERY requests

### Selected Endpoints

```
# Access Control — the core flow
QUERY  /api/v1/access/check     → Multi-tier: Redis → Neo4j → Cedar
POST   /api/v1/access/grant     → Fires GrantAccessWorkflow, returns 202
POST   /api/v1/access/revoke    → Fires RevokeAccessWorkflow, returns 202
POST   /api/v1/access/jit       → Time-bounded access with auto-expiry

# Identity — PostgreSQL powered
GET    /api/v1/identities?search=david&status=active&sort_by=risk_score&sort_dir=desc&limit=25
POST   /api/v1/identities/bulk  → 5,000 record batch import

# Connectors — 5 sync types
POST   /api/v1/connectors/{id}/full-sync      → Users + Groups + Entitlements + Resources
POST   /api/v1/connectors/{id}/sync-delta     → Incremental (Entra ID)
GET    /api/v1/connectors/stats               → Aggregate metrics across all connectors

# Vault — AES-256-GCM encrypted
POST   /api/v1/vault/secrets       → Store (encrypted at rest)
GET    /api/v1/vault/secrets/{id}  → Retrieve (decrypted on demand)

# Audit — 7 filter dimensions
GET    /api/v1/audit/logs?method=POST&status=403&level=error&since=2026-01-01T00:00:00Z
```

---

## Project Structure

```
observeid/
├── backend/
│   ├── cmd/identity-service/main.go      # Entry point, 830 lines — all route + middleware wiring
│   ├── internal/
│   │   ├── service/identity_service.go   # 3,174 lines — 50+ HTTP handlers, Cedar eval, CSV upload
│   │   ├── connector/
│   │   │   ├── entra.go                  # 1,366 lines — Microsoft Graph API, OAuth2, delta sync
│   │   │   ├── ldap.go                   # LDAP search, UAC parsing, group membership
│   │   │   ├── scim.go                   # SCIM 2.0 connector with bearer token auth
│   │   │   ├── csv.go                    # CSV file connector with validation
│   │   │   ├── manager.go               # 690 lines — connector lifecycle, sync orchestration
│   │   │   ├── provisioning.go          # LCM provisioning engine
│   │   │   └── types.go                 # 235 lines — all connector types/interfaces
│   │   ├── workflow/workflows.go        # 887 lines — 9 Temporal workflows
│   │   ├── activities/activities.go     # 1,307 lines — 25+ activities with full error propagation
│   │   ├── vault/vault.go              # AES-256-GCM encryption, key derivation, secret lifecycle
│   │   ├── audit/audit.go              # In-memory ring buffer, 7-filter query, HTTP middleware
│   │   ├── middleware/                  # Auth, rate limit, validation, workflow permissions
│   │   ├── domain/identity.go          # 11 domain types — Identity, NHI, Role, Entitlement, Resource
│   │   ├── graphql/                    # gqlgen resolvers (50+ queries + mutations)
│   │   └── ai/copilot.go              # GraphRAG copilot over Neo4j
│   └── pkg/telemetry/metrics.go       # 14 Prometheus metric definitions
├── frontend/src/app/
│   ├── dashboard/       # Live metrics dashboard
│   ├── identities/      # Identity admin console (779 lines)
│   ├── connectors/      # Directory management (706 lines)
│   ├── audit/           # Access logs with 7D filtering (747 lines)
│   ├── access/          # Access control console (433 lines)
│   ├── policies/        # Cedar policy management
│   ├── vault/           # Encrypted secret management
│   ├── groups/          # Roles and groups
│   ├── agents/          # Non-human identity management
│   └── components/ui/   # 11 shared components (Button, Card, Badge, Modal, etc.)
├── infrastructure/postgres/init.sql    # 579 lines — 29 tables, 8 enum types, 8 indexes, 10 triggers
├── infrastructure/neo4j/init.cypher    # Seed constraints + indexes
├── samples/hr-identities.csv           # 15 realistic identities across 5 departments
├── docker-compose.yml                  # 6 containers (PG, Neo4j, Redis, Temporal, Zookeeper, OTel)
├── fly.toml                            # Fly.io deployment (512MB shared CPU, port 8080)
└── .github/workflows/                  # CI/CD — test backend → test frontend → deploy
```

---

## What's an MVP vs What's Being Built?

### Production-Ready Today

- Identity lifecycle: CRUD, bulk import, soft-delete, dual-write (PG + Neo4j)
- Access control: Multi-tier check with Neo4j path traversal + Cedar policy evaluation
- Policy engine: Pattern-matched permit/forbid rules with wildcard support
- Durable workflows: 9 Temporal workflows with retry, signal handling, parallel fan-out
- Connectors: Entra ID (full + delta), LDAP/AD, SCIM, CSV
- Vault: AES-256-GCM encryption with decryption-on-demand
- Audit: 7-dimension filterable log with live/paused polling
- Frontend: 14 pages, 112 Jest tests, 12 test suites all passing
- CI/CD: GitHub Actions → Fly.io + Cloudflare Pages

### In Active Development

| Feature | Current State | Target |
|---------|--------------|--------|
| Real Cedar evaluation | Pattern-matched with `*` wildcards. Forbid/permit with priority. | Full Cedar WASM runtime for Cedar policy language support |
| SCIM 2.0 server | Stub handlers returning static data. | Complete SCIM server with workflow integration |
| Cross-store transactions | Dual-write PG + Neo4j. No outbox pattern. | Outbox + change data capture for guaranteed consistency |
| Auth federation | API key only. Configurable per deployment. | OIDC/OAuth2 + SAML. JWT validation middleware. |
| Connection pooling | PG: 50 local / 5 cloud. Neo4j: session-per-request. | Production-tuned pools with circuit breakers. |
| GraphQL parity | Subset of REST endpoints available via GQL. | Full REST-to-GQL parity for all 50+ endpoints. |

---

## Environment Variables

All configuration is via environment variables. See [`.env.example`](backend/.env.example) for the template.

| Variable | Purpose | Default |
|----------|---------|---------|
| `DATABASE_URL` | PostgreSQL connection | `postgresql://observeid:observeid@localhost:5432/observeid` |
| `NEO4J_URI` | Neo4j Bolt endpoint | `bolt://localhost:7687` |
| `NEO4J_USER` / `NEO4J_PASSWORD` | Neo4j credentials | `neo4j` / (local) |
| `REDIS_ADDR` / `REDIS_PASSWORD` | Redis connection | `localhost:6379` / (empty) |
| `TEMPORAL_HOST` / `TEMPORAL_NAMESPACE` | Temporal | `localhost:7233` / `critical-offboarding` |
| `VAULT_MASTER_KEY` | AES-256 encryption key | Generate with `openssl rand -hex 32` |
| `MASTER_KEY` | Workflow guard master key | Empty → guard disabled (dev mode) |
| `API_KEYS` | API key authentication | `name:key,name:key` format. Empty → auth disabled. |
| `CORS_ORIGIN` | Allowed CORS origin | Empty (no CORS restrictions in dev) |

---

## Graphify — Codebase Knowledge Graph

This project ships with a [Graphify](https://github.com/Graphify-Labs/graphify) knowledge graph that maps all 86 source files into a queryable code graph:

```
graphify-out/
├── graph.json         # 1,079 nodes, 2,406 edges, 63 communities
├── graph.html         # Interactive force-directed visualization
└── GRAPH_REPORT.md    # God nodes, surprising connections, architecture summary
```

Built with tree-sitter AST (zero API cost, fully local). Query it:

```bash
graphify path "GrantAccessWorkflow" "ProvisionAccess"
# → 4 hops: Workflow → Duration → AcquireLock → ActivityService → ProvisionAccess

graphify explain "IdentityService"
# → 87 connections: Manager, Vault, Store, 30+ HTTP handlers

graphify path "ListIdentities" "evaluateCedarPolicy"
# → 2 hops: through Context
```

---

## License

MIT License — ObserveID Reimagined, Inc. Copyright 2026.

```
Built with Go · TypeScript · PostgreSQL · Neo4j · Redis · Temporal · Docker · Love
```
