# ObserveID 2026 — Identity Fabric

**Event-Driven, AI-Native Identity and Access Governance Platform**

ObserveID is a next-generation IAM/IGA platform architected for real-time identity governance, AI agent security, and durable execution. It unifies human and non-human identity under a single policy engine, graph database, and workflow system.

---

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    OBSERVEID PLATFORM                     │
├──────────────────────────────────────────────────────────┤
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌─────────────┐ │
│  │ Identity │ │ Access   │ │ Policy   │ │ AI Copilot  │ │
│  │ Service  │ │ Service  │ │ Engine   │ │ (GraphRAG)  │ │
│  │ (SCIM)   │ │ (RBAC/   │ │ (Cedar)  │ │ (Neo4j+LLM) │ │
│  │          │ │  ABAC)   │ │          │ │             │ │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └──────┬──────┘ │
│       │            │           │               │        │
│  ┌────┴────────────┴───────────┴───────────────┴──────┐ │
│  │            Event Bus (Kafka/Redpanda)               │ │
│  └────────────────────────────────────────────────────┘ │
│       │            │           │               │        │
│  ┌────┴────────────┴───────────┴───────────────┴──────┐ │
│  │           Temporal Workflow Engine                   │ │
│  │  Namespaces: critical_offboarding, provisioning,    │ │
│  │             reconciliation, analysis                 │ │
│  └────────────────────────────────────────────────────┘ │
│       │            │           │               │        │
│  ┌────┴────────────┴───────────┴───────────────┴──────┐ │
│  │  Data Layer: PostgreSQL | Neo4j | Qdrant | Redis   │ │
│  └────────────────────────────────────────────────────┘ │
│       │            │           │               │        │
│  ┌────┴────────────┴───────────┴───────────────┴──────┐ │
│  │  Security: SPIFFE/SPIRE | CAEP | FIDO2 | PQC       │ │
│  └────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```

## Tech Stack

| Layer | Technology | Purpose |
|-------|-----------|---------|
| Language | Go 1.23 | High-concurrency microservices |
| Workflows | Temporal.io | Durable execution, guaranteed delivery |
| Identity Graph | Neo4j 5 | Entitlements, roles, blast radius |
| Relational DB | PostgreSQL 16 | CRUD, config, sessions, outbox |
| Vector DB | Qdrant | Semantic search, GraphRAG |
| Policy Engine | Amazon Cedar | ABAC, RBAC, SoD enforcement |
| Event Bus | Redpanda/Kafka | Event-driven architecture |
| Immutable Audit | Amazon QLDB | Tamper-proof audit logs |
| Cache | Redis 7 | Sessions, revocation cache |
| Workload ID | SPIFFE/SPIRE | mTLS service identity |
| Frontend | Next.js 14 + Tailwind | Single Pane of Glass |
| Observability | OpenTelemetry + Grafana | Metrics, traces, logs |

## Project Structure

```
observeid/
├── proto/                      # Protobuf definitions
│   ├── event/v1/               # Identity events (UserCreated, AccessGranted, etc.)
│   └── model/v1/               # Data models (Identity, Entitlement, Role, etc.)
├── backend/                    # Go backend
│   ├── cmd/identity-service/   # Entry point
│   ├── internal/
│   │   ├── domain/             # Core domain types
│   │   ├── workflow/           # Temporal workflows (Onboard, Offboard, Grant, Revoke)
│   │   ├── activities/         # Temporal activities
│   │   ├── service/            # HTTP/gRPC service layer
│   │   ├── ai/                 # GraphRAG AI copilot pipeline
│   │   ├── graph/              # Neo4j query patterns
│   │   └── audit/              # Immutable audit logging
│   └── pkg/telemetry/          # OpenTelemetry + Prometheus metrics
├── frontend/                   # Next.js 14 TypeScript
│   └── src/
│       ├── app/                # Dashboard, Identities, Agents pages
│       ├── graphql/            # GraphQL schema
│       └── styles/             # Tailwind CSS
├── policies/                   # Amazon Cedar policies
│   ├── identity.cedarschema    # Strongly-typed policy schema
│   ├── rbac.cedar              # Role-based access control
│   ├── abac.cedar              # Attribute-based access control
│   ├── agent.cedar             # AI Agent / NHI policies
│   └── sod_emergency.cedar     # SoD and emergency access
├── infrastructure/             # Local dev environment
│   ├── docker-compose.yml      # All services (PostgreSQL, Neo4j, Kafka, Temporal, etc.)
│   ├── postgres/init.sql       # Schema + seed data
│   ├── neo4j/init.cypher       # Graph schema + seed data
│   └── temporal/config.yaml    # Temporal server config
├── deploy/k8s/                 # Kubernetes manifests
│   ├── identity-service.yaml   # Deployment, Service, HPA, PDB, NetworkPolicy
│   ├── spire.yaml              # SPIRE workload identity
│   └── spire-config.yaml       # SPIRE configuration
└── docker/                     # Dockerfiles
```

## Quick Start

```bash
# 1. Start infrastructure (PostgreSQL, Neo4j, Kafka, Temporal, Redis, Qdrant)
make up

# 2. Run database migrations
make dev-db

# 3. Build backend
make backend

# 4. Start frontend (separate terminal)
make frontend-dev
```

## Key Workflows

| Workflow | Description | Priority |
|----------|-------------|----------|
| `OffboardIdentityWorkflow` | Complete identity offboarding with parallel fan-out, CAEP broadcast, QLDB audit, and cascade agent revocation | Critical |
| `OnboardIdentityWorkflow` | Identity creation with role assignment and optional approval gates | High |
| `GrantAccessWorkflow` | Access provisioning with optional approval workflow and JIT auto-expiry | High |
| `RevokeAccessWorkflow` | Emergency access revocation with sticky cache invalidation | Critical |
| `JustInTimeAccessWorkflow` | Time-bounded access with automatic expiration | Medium |
| `AgentAnomalyDetectionWorkflow` | Cron-based AI agent behavioral analysis | Medium |
| `DetectSoDViolationsWorkflow` | Hourly SoD violation scanning via Neo4j graph traversal | Medium |

## Cedar Policies

Policies are authored in Amazon's Cedar language and validated at CI time:

```cedar
// RBAC: Administrators have full access
permit(
    principal in Role::"Administrator",
    action,
    resource
);

// ABAC: Contractors cannot access PII data
forbid(
    principal in Role::"Contractor",
    action,
    resource
) when {
    resource.data_classification in ["pii", "phi", "pci"]
};

// Agent: Kill switch — deny all for revoked agents
forbid(
    principal is Agent,
    action,
    resource
) when {
    principal.is_revoked == true
};
```

## API Endpoints

### SCIM 2.0
- `GET /scim/v2/Users` — List users
- `POST /scim/v2/Users` — Create user
- `GET /scim/v2/Users/{id}` — Get user
- `DELETE /scim/v2/Users/{id}` — Delete user (triggers offboarding)

### Identity API
- `GET /api/v1/identities` — List identities
- `GET /api/v1/identities/{id}` — Identity 360 view
- `GET /api/v1/identities/{id}/entitlements` — Access paths
- `GET /api/v1/identities/{id}/blast-radius` — Blast radius analysis

### Agent / NHI API
- `GET /api/v1/agents` — List agents
- `POST /api/v1/agents` — Register agent
- `POST /api/v1/agents/{id}/kill-switch` — Emergency kill switch
- `POST /api/v1/agents/{id}/delegate` — Agent delegation

### AI Copilot
- `POST /api/v1/copilot/query` — Natural language identity query

### CAEP
- `POST /api/v1/caep/broadcast` — Broadcast session-revoked event

## License

Confidential — ObserveID, Inc. Internal Engineering
