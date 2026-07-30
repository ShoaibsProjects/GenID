# Contributing to GenID

Welcome to the Identity Fabric Engine. This document covers the conventions, workflow, and quality bar for contributing.

## Philosophy

- **Safety first** — no code gets merged that has not been verified to compile and pass tests
- **Micro-tasking** — break work into small, single-file prompts that can be verified independently
- **Graph-native** — the identity graph (Neo4j) is the source of truth for relationships; PostgreSQL is the source of truth for CRUD
- **Security compliance and principle** — every endpoint should pass the FAANG security matrix before ship

## Development Workflow

### Clone & Setup

```bash
git clone https://github.com/ShoaibsProject/observeid-V2
cd observeid-V2

# Install dependencies
cd backend && go mod download && cd ..
cd frontend && npm install && cd ..

# Start infrastructure (Docker not required for minimal dev)
make dev-db         # PostgreSQL + Neo4j + Redis + Temporal + NATS
# or connect to your running services via .env
```

### Running Locally

```bash
# Build and run from project root (required: .env must be present)
cd backend && go build -o /tmp/identity-svc ./cmd/identity-service/
cd ..
./tmp/identity-svc

# Or equivalently dive:
make backend-run
```

> **IMPORTANT:** The binary MUST be launched from the **project root** directory. The `.env` file is loaded via `os.ReadFile(".env")` — this means its path is always relative to the current working directory.

### Before You Commit

```bash
# 1. Lint (if available)
make lint      # or: golangci-lint run ./...

# 2. Run tests
make test      # or: go test ./... && npx jest

# 3. Type-check frontend
cd frontend && npx tsc --noEmit

# 4. Secret scan (pre-commit hook covers this)
gitleaks detect --verbose
```

The pre-commit hook (under `.githooks/pre-commit`) runs `gitleaks` before every local commit. No secrets should ever pass this gate.

## Architecture Conventions

### Backend (Go 1.25+)

**Package structure:**

```
backend/
├── cmd/                       # Service entrypoints
│   └── identity-service/      # Main API gateway
├── internal/
│   ├── activities/             # Temporal workflows activities (idempotent, atomic)
│   ├── audit/                  # SHA-256 hash chain audit ledger
│   ├── cedar/                  # Cedar for Authorization
│   ├── connector/              # Connector framework (manifest, JSONPath, engine)
│   ├── domain/                 # Domain types and validation
│   ├── eventbus/               # NATS JetStream publisher/subscriber
│   ├── graphql/                # GraphQL resolvers and schema (gqlgen-generated)
│   ├── middleware/             # Auth, rate limiting, request validation, permissions
│   ├── oidc/                   # OIDC-compliant provider (JWKS, token management)
│   ├── outbox/                 # Outbox pattern for durable event dispatch
│   ├── risk/                   # Dynamic risk score engine
│   ├── service/                # HTTP handler methods on IdentityService struct
│   ├── vault/                  # AES-256-GCM encrypted credential vault
│   └── workflow/               # Temporal workflow definitions
├── manifests/                  # Connector YAML manifest files
├── pkg/
│   ├── proto/                  # Protocol buffer definitions + generated code
│   └── telemetry/              # Prometheus metrics, OTLP tracing helpers
├── go.mod
└── go.sum
```

**Coding standards:**

1. **Always use parameterized queries.** Never interpolate user-controlled strings into SQL or Cypher. Use `$1`, `$id`, `$uuid`.
2. **Neo4j MERGE writes are async** — Postgre comes first (source of truth), Neo4j shadows it.
3. **Temporal activities must be idempotent** — every activity should handle retries.
4. **Activity-service dependencies** — `ActService` gets `pgPool`, `neo4j`, `redis` via constructor injection.
5. **Error handling** — return errors, don't panic. Log with structured fields (`dst levels`).
6. **Tenant isolation** — always call `set_config('app.current_tenant', $1, true)` in database transactions.
7. **Graph updates** — after modifying code that creates or mutates the graph, run `graphify update .` to keep the knowledge graph current.

### Frontend (Next.js 14 + Tailwind CSS)

```
frontend/
├── src/
│   ├── app/                 # Next.js App Router pages
│   │   ├── access/          # Access and session management
│   │   ├── agents/          # AI agent management
│   │   ├── audit/           # Audit log viewer
│   │   ├── certifications/  # Access certification campaigns
│   │   ├── connectors/      # Directory connector management + mapping
│   │   ├── csv/             # CSV import/export
│   │   ├── dashboard/       # Main dashboard
│   │   ├── groups/          # Role groups
│   │   ├── identities/      # Identity management
│   │   ├── idp/             # IDP configuration
│   │   ├── policies/        # Ced coordinator policy editor
│   │   ├── settings/         # Tenant settings
│   │   ├── sod/               # Separation of duties
│   │   └── vault/            # Encrypted credentials
│   ├── components/
│   │   └── ui/               # Shared UI components (Card, Button, Input, etc.)
│   ├── graphql/              # GraphQL schema
│   └── lib/                  # Shared utilities and API client
├── tailwind.config.ts
└── package.json
```

**Design system:**

- Dark mode by default (`bg-zinc-900`, `text-white`, `border-zinc-800`)
- Hover effects with amber accent (`hover:border-amber-500/30`, `hover:shadow-[...amber...]`)
- Border lines: `border-zinc-800` (primary), `border-zinc-800/50` (secondary)
- Font: `font-black` for huge headings, `text-sm font-semibold` for section headings
- Glass background: `bg-zinc-900/50 backdrop-blur-sm`
- Cards: `rounded-xl border border-zinc-800 bg-zinc-900/50`

### Infra (Docker TLS)

```
infrastructure/
├── docker-compose.yml        # Full stack (PG, Neo4j, Redis, NATS, Temporal, OTel)
├── postgres/
│   └── init.sql              # Full database schema + RLS + migrations
├── neo4j/
│   └── init.cypher           # Graph schema + seed data
└── otel-collector-config.yaml
```

## Dependencies & Build

| Layer | Runtime | Toolchain |
|-------|---------|-----------|
| Backend | Go 1.25+ | `go build`, `go test`, `govulncheck`, `golangci-lint` |
| Frontend | `node 18+` | `next`, `typescript` |
| Protocols | `make proto` | `buf` or `protoc` |
| Secrets | `.githooks` | `gitleaks` |

## Testing Strategy

```bash
# Backend unit tests
make test-backend     # go test -v -race ./...

# Frontend tests
make test-frontend    # npm test

# Integration smoke tests (requires running services)
curl -s http://localhost:8080/health
curl -s http://localhost:8080/api/v1/access/check ...

# Load test (if you need to verify rate limiter)
# Use k6 or Postman collections
```

All PRs must pass CI. The CI pipeline runs: 1) `go test`, 2) `npm lint && tsc`, 3) `gitleaks detect`.

## Commit Messages

We use [conventional commits](https://www.conventionalcommits.org/):

```
feat(<scope or layer>): one line description of change

- Bullet point detail
- Another detail
```

Common to scopes: `risk-engine`, `connectors`, `jit`, `audit`, `graph`, `frontend`, `auth`, `vault`.

## Identity Graph Conventions

### Edge Types

| Edge | From | To | Purpose |
|------|------|----|---------|
| `HAS_ROLE` | Identity | Role | Direct role assignment |
| `GRANTS` | Role | Entitlement | Role-level entitlement |
| `ACCESSES` | Entitlement | Resource | Entitlement-level resource access |
| `HAS_DIRECT_ACCESS` | Identity | Resource | Direct (not role-based) access |
| `HAS_ENTITLEMENT` | Identity | Entitlement | Direct entitlement (not role-based) |
| `HAS_TEMPORARY_ACCESS` | Identity | Resource | JIT (Just-In-Time) time-bounded access |
| `PROXY_OF` | Identity | Identity | Identity proxy delegation |
| `OWNED_BY` | NonHumanIdentity | Identity | Ownership of service accounts/agents |
| `DELEGATED_FROM` | Identity | Identity | Delegation chains |

### Hybrid Query Pattern

The canonical access check follows this pattern:

```cypher
MATCH (i {uuid: $identityId}) WHERE i:Identity OR i:NonHumanIdentity
OPTIONAL MATCH pathRole      = (i)-[:HAS_ROLE]->(:Role)-[:GRANTS]->(:Entitlement)-[:ACCESSES]->(res:Resource {uuid: $resourceId})
OPTIONAL MATCH pathDirectEnt = (i)-[:HAS_ENTITLEMENT]->(:Entitlement)-[:ACCESSES]->(res:Resource {uuid: $resourceId})
OPTIONAL MATCH pathDirect    = (i)-[:HAS_DIRECT_ACCESS]->(res:Resource {uuid: $resourceId})
OPTIONAL MATCH pathTemp      = (i)-[:HAS_TEMPORARY_ACCESS]->(res:Resource {uuid: $resourceId})
```

---

## Questions?

Open an issue or dig at the project maintainers. Start with `make help` for a quick overview of all available commands.