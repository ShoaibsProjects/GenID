# Architecture Overview

GenID is a layered, event-driven identity platform. This document describes the system's planes, components, and data flow. For the risk scoring internals see [Risk Engine](risk-engine.md).

## High-level diagram

```
                     ┌──────────────────────────┐
                     │   Next.js 14 UI (nginx)  │  :3001  (static export)
                     └────────────┬─────────────┘
                                  │ /api, /scim, /graphql, /health
                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Go API Gateway (:8080)                        │
│  ┌──────────────┐ ┌───────────────┐ ┌─────────────────────────┐ │
│  │ Security     │ │ CORS          │ │ OpenTelemetry tracing   │ │
│  ├──────────────┤ ├───────────────┤ ├─────────────────────────┤ │
│  │ Rate limiter │ │ Request       │ │ JWT Auth (JWKS)         │ │
│  │ (100rps/200) │ │ validation    │ │ + API keys              │ │
│  ├──────────────┤ ├───────────────┤ ├─────────────────────────┤ │
│  │ Audit        │ │ Workflow      │ │                         │ │
│  │ (SHA-256     │ │ Master-Key    │ │                         │ │
│  │  chain)      │ │ guard         │ │                         │ │
│  └──────────────┘ └───────────────┘ └─────────────────────────┘ │
│   REST (/api/v1)   SCIM 2.0 (/scim/v2)   OIDC/OAuth 2.0   GraphQL │
└───────────────────────────┬─────────────────────────────────────┘
                            │
   ┌────────────────────────┼───────────────────────────┐
   ▼                        ▼                           ▼
┌────────────┐      ┌───────────────┐           ┌──────────────┐
│ PostgreSQL │◄─────│  NATS JetStream │◄─────────│   Temporal  │
│ 16 (RLS)   │ outbox│  genid-events   │          │  workflows  │
└────────────┘      └───────┬───────┘           └──────────────┘
      │  ▲                  │ consume                     │
      │  │ sync             ▼                            triggers
      │  └─────────┌──────────────────┐                  │
      │            │  Event Processor │◄─────────────────┘
      │            │  (risk scorer)   │
      │            └────────┬─────────┘
      ▼                     ▼
┌────────────┐       ┌──────────────────┐
│   Redis 7  │       │  Neo4j 5 (graph) │
│ cache, JIT,│       │  risk scores,    │
│ locks, JTI │       │  access paths    │
└────────────┘       └──────────────────┘
```

## Architectural planes

### 1. Edge & Ingestion
- **Cloudflare** — WAF, CDN (production).
- **Frontend** — Next.js 14 static export served by nginx; proxies API traffic.
- **SCIM 2.0 Gateway** — RFC 7644 user provisioning (`/scim/v2`).
- **Event Ingest API** — `POST /api/v1/events/ingest` pushes security events onto the bus.

### 2. Policy & Authorization
- **Go API Gateway** — gorilla/mux router with a strict middleware chain.
- **AWS Cedar** — policy-as-code engine (`CheckAccessPolicy` activity); hot-reloading, force-spanning scope.
- **OIDC/JWT** — full OAuth 2.0 + OIDC provider (authorize/token/userinfo/introspect/revoke/device flow), RS256 JWKS.

### 3. Agentic Governance
- **NHI Registry** — first-class `non_human_identities`, agent cards, delegation chains.
- **Kill Switch** — risk-tiered revocation; cascade revoke for delegated agents.
- **Blast Radius** — Neo4j variable-length path traversal.

### 4. Compliance & Audit
- **Access Certifications** — campaign generation + entry decision.
- **Tamper-Proof Audit** — SHA-256 chained ledger (`prev_hash`, `hash`).
- **CAEP** — shared signals broadcast (HMAC-signed webhooks).

### 5. State & Orchestration
- **PostgreSQL** — source of truth; RLS on 28 tables; transactional outbox.
- **Neo4j** — graph store for access paths, relationships, and live risk scores.
- **Redis** — rate limiting, JIT grants, identity locks (fencing), JTI blocklists, access-check caching.
- **Temporal** — durable workflow execution (JML, JIT, approval, revocation, cron).
- **NATS JetStream** — durable event bus (`genid-events` stream, 100k msgs / 24h).

## Components

| Component | Binary | Role |
|-----------|--------|------|
| `identity-service` | `cmd/identity-service/main.go` | REST + SCIM + OIDC + GraphQL, outbox writer, NATS producer, Temporal worker |
| `event-processor` | `cmd/event-processor/main.go` | NATS consumer; applies risk deltas to Neo4j; fires band actions |
| `frontend` | `frontend/` (Next.js) | Admin console + demo surfaces (risk, event simulator) |

## Two binaries, one graph

The **identity-service** owns the API and the **event-processor** owns risk scoring. Both write to Neo4j; Postgres is written only by the identity-service (and Temporal activities). There is deliberately no shared in-memory state between them — everything crosses the event bus or the graph.

## Consistency model

- **Transactional outbox**: identity mutations write to Postgres + `outbox_events` in one transaction. The outbox processor applies them to Neo4j, then republishes onto NATS (see [Data model → outbox](../data-model/postgres.md#outbox_events)).
- **Direct dual-write**: some paths (identity create via GraphQL, role assignment, NHI registration, delegation) write Postgres *and* Neo4j in the same handler. Risk-relevant state is always reconcilable from Postgres via `RiskRecalculationCronWorkflow`.
- **Event sourcing for risk**: risk scores live only in Neo4j, mutated by events (with decay) — they are not re-derived from Postgres on every read.

## Data flow: the risk event loop (our stakeholder's use case)

```
User fails login 3×          Entitlement unchanged
        │
        ▼
POST /api/v1/events/ingest  (identity-service)
        │  {event_type: "auth.failed_login", identity_id, severity}
        ▼
NATS JetStream  genid-events.auth.failed_login
        │
        ▼
Event Processor (event-processor)
        │  weight 100 × severity multiplier → delta
        │  decay past score (-5/hr) → clamp [0,1000]
        ▼
Neo4j  SET risk_score, risk_dynamic, risk_band, risk_event_count
        │
        ├── publish genid-events.identity.events.risk.updated
        │
        └── band action (e.g. score ≥ 800):
              • TerminateAllSessions → Session.status = 'terminated'
              • TriggerReview → Review node (critical_risk, due +3d)
```

Score moves even though no entitlement changed — purely event-driven, no batch job.

## Security model

- **JWT auth** validates RS256 tokens against the local JWKS; API keys bypass JWT for internal workers.
- **Workflow guard** requires `X-Master-Key` for sensitive mutations.
- **RLS** isolates tenants at the database layer (`SET app.current_tenant`).
- **Audit chain** hashes immutable fields so tampering is detectable.
- **Vault** encrypts connector secrets with AES-256-GCM before persistence.

See [Operational → Security](../operational/security.md).

## Related

- [Risk Engine deep-dive](risk-engine.md)
- [Event Catalog](../events/catalog.md)
- [Workflows](../workflows/temporal.md)
- [Complete architecture & feature map](COMPLETE_ARCHITECTURE.md)
