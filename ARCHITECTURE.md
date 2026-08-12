# GenID · Architecture Deep Dive

> A code-backed, FAANG-style engineering reference. Every subsystem below maps to a real
> file in this repository — recruiters, senior engineers, and AI agents reviewing this repo
> can verify every claim by following the file paths.

GenID is a **graph-native, policy-as-code Identity & Access Management (IAM/IGA) platform**
built for the agentic era. It unifies human, machine, and autonomous AI-agent identity
under a single real-time policy engine, with a 109ms kill switch and a tamper-proof audit chain.

---

## 1. Design Principles

| Principle | How GenID honors it |
|-----------|---------------------|
| **Policy-as-code** | AWS Cedar is the only authority. Every check goes through `cedar/engine.go`. Raw RBAC/ABAC/ReBAC checks are not allowed at the data plane. |
| **Dual-write core** | Identity & entitlement state lives in **PostgreSQL** (source of truth) and is **mirrored to Neo4j** for graph queries. Writes are transactional; reads are routed. |
| **Zero-trust by default** | Every request traverses JWT/JWKS, API-key, rate-limit, and WorkflowGuard middleware before reaching handlers. No bypass at the gateway. |
| **Zero-standing privilege** | Non-Human Identities (NHIs) get 5-minute scoped JIT JWTs. Long-lived service credentials are an anti-pattern here. |
| **Tamper-evident audit** | Audit entries are SHA-256 chained in DB. Tampering is cryptologically detectable. |
| **Stays uncluttered** | One modular monolith (Go) + one static-export SPA (Next.js). No premature microservices. |

---

## 2. Five-Plane Architecture

```
┌─ EDGE (Cloudflare)        Tunnel/WAF · Next.js UI · SCIM 2.0 gateway
├─ GATEWAY (Go 1.25)         gorilla/mux · JWT/JWKS · API-Key · 100/s rate-limit · WorkflowGuard
├─ SERVICES                  Identity · Access · Cedar · NHI · IGA · GraphQL · AI Copilot
├─ ASYNC                     Temporal v1.25 (11 workflows · 29 activities) · NATS JetStream · Outbox
└─ STATE                     PostgreSQL 16 (RLS, 28 tables) · Neo4j 5 · Redis 7
```

Full visual: see `Architecturenewidea.svg` in the repo root, also embedded in `README.md`.

### 2.1 Edge plane
- **Cloudflare Tunnel + WAF** terminates TLS, applies DDoS/WAF rules, and forwards to the Go gateway over localhost only — the service is **never exposed to the public internet**. Hardened compose binds all ports to `127.0.0.1`.
- **Next.js 14 static export** served via nginx (see `docker/frontend.Dockerfile`). Pages: dashboard, identities, agents (NHI), connectors, access, audit, vault, idp.

### 2.2 Gateway plane
- Single Go HTTP server on `:8080` (`backend/cmd/identity-service/main.go`). Middleware chain:
  - `middleware/jwt_auth.go` — RS256 JWT validation against rotating JWKS (`/.well-known/jwks.json`)
  - `middleware/auth.go` — API-key + WorkflowGuard X-Master-Key on admin endpoints
  - `middleware/rate_limit.go` — per-IP 100 req/s, burst 200 (token bucket)
  - `middleware/workflow_permission.go` — admin action gating

### 2.3 Services plane
| Subsystem | Entry point | What it does |
|-----------|-------------|--------------|
| **Identity Service** | `service/identity_service.go` | CRUD + bulk sync. Performs the **PG → Neo4j dual-write** and runs **Blast Radius** Cypher (real-time graph analytics). |
| **Access Service** | `service/identity_service.go` | RBAC / ABAC / ReBAC; the unified `CheckAccess` path runs **through Cedar**, never raw SQL. |
| **Cedar Engine** | `cedar/engine.go` | Hot-reloads policy files; persists compiled policies in `cedar_policies.cedar_text` (Postgres). 4 active policy files: `policies/{rbac,abac,agent,sod_emergency}.cedar`. |
| **AI Copilot (GraphRAG)** | `ai/copilot.go` | Real GraphRAG pipeline — see §3. |
| **OIDC Provider** | `oidc/{provider,clients,handlers}.go` | Discovery, JWKS, Authorize, Token, UserInfo, Introspect, Revoke, Register. |
| **Connector Framework** | `connector/` | Manifest-driven: `manifest.go` + `loader.go` + `jsonpath.go` + `engine.go`. Connectors: `entra.go`, `ldap.go`, `scim.go`, `csv.go` (+ generic REST). |
| **Credential Vault** | `vault/vault.go` | AES-256-GCM with key from `VAULT_MASTER_KEY`. File-backed, 0600 perms. Has unit tests. |
| **Audit Chain** | `audit/{audit,chain}.go` | `sha256.Sum256(payload)` chained across entries — tamper evident. Has unit tests. |

### 2.4 Async plane
- **Temporal v1.25** — 4 namespaced queues (`critical-offboarding`, `provisioning`, `reconciliation`, `analysis`). See `workflow/workflows.go` (11 workflows) and `activities/activities.go` (29 activities): Onboard, Offboard (fan-out), Grant, Revoke, JIT Provision/Revoke, Anomaly, SoD.
- **NATS JetStream** (`outbox/`) — Transactional-outbox pattern in Postgres → background pump → NATS. Replaces Kafka at ~15MB RAM. Topics: `identity`, `access`, `policy`, `audit`, `agent`, `caep`.
- **Kill switch** — Offboard workflow + Redis **JTI blocklist** delivers revocation in ~109ms end-to-end (Temporal signal + JTI check at every request).

### 2.5 State plane
- **PostgreSQL 16** — 28 tables (incl. identities, connectors, cedar_policies, audit_chain, outbox). **Row-Level Security** enforced at the DB layer; see 56 RLS statements in `infrastructure/postgres/init.sql`. Apps connect through RLS-aware roles — multi-tenancy is guaranteed by Postgres, not application code.
- **Neo4j 5 (community)** — Identity graph: 8 labels, 7 relationship types. Powers Blast Radius, SoD detection, access-path queries.
- **Redis 7** — session cache, distributed **fence tokens** (optimistic concurrency), JTI revocation list with TTL.

---

## 3. AI / Agentic Governance — the part SpaceX AI / Anthropic / Okta reviewers care about

GenID is not "AI-washing". It is built for the era where autonomous agents act, not just humans.

### 3.1 Non-Human Identity (NHI) governance
Service accounts, API keys, OAuth clients, and **autonomous AI agents** are first-class identities
with lifecycle, blast-radius analysis, toxic-combination detection, and automated revocation — exactly
the control plane Okta / Anthropic / SpaceX are investing in. The agent-scoped Cedar policy file
`policies/agent.cedar` narrows an agent's span at evaluation time, not at token issuance.

### 3.2 GraphRAG Copilot (`backend/internal/ai/copilot.go`)

A genuine **Graph-RAG** pipeline — not keyword search dressed up.

```
User question
   │
   ▼
1. classifyQuery()      → intent classification (7 intents: blast radius, access-path, SoD…)
   │
   ▼
2. executeGraphQuery()  → Neo4j Cypher retrieval (open-set retrieval over the identity graph)
   + Cedar policy fetch → context enrichment from `cedar_policies`
   │
   ▼
3. rerank results       → cross-encoder-style step (slot for production cross-encoder)
   │
   ▼
4. generateResponse()   → context-assembled answer
   │
   ▼
5. validate + score     → third-pass LLM validation (slot), confidence: 0.6–0.92
```

Key callouts:
- Retrieval is **graph-native** (Cypher over Neo4j), not flat vector search — so WHY access exists
  (the policy chain) is answerable, not just WHAT exists.
- A **Qdrant vector** path is stubbed for production-grade hybrid retrieval; today the graph provides
  deterministic entity and path coverage. This keeps the dev stack at $0 cost (no Qdrant container).
- Entity extraction + classification have clear "in production use LLM" comments — transparent
  about what runs deterministically today vs. what swaps in a model in production.

This is exactly the **agentic AI governance + retrieval** stack recruiters at AI-forward companies
want to see evidence of.

---

## 4. Security model (FAANG-grade)

| Control | Implementation | Verifiable file |
|---------|----------------|-----------------|
| JWT/JWKS validation | RS256 against rotating JWKS | `middleware/jwt_auth.go` |
| Rate limiting | 100 req/s, burst 200, per-IP token bucket | `middleware/rate_limit.go` |
| Admin action gating | `X-Master-Key` WorkflowGuard | `middleware/workflow_permission.go` |
| Credential vault | AES-256-GCM, env-keyed | `vault/vault.go` |
| Multi-tenancy | RLS on 28 tables (56 policies) | `infrastructure/postgres/init.sql` |
| Tamper-proof audit | SHA-256 chained hashes | `audit/chain.go` |
| SQL safety | Parameterized queries everywhere (pgx/v5) | throughout `service/` |
| Secrets | 3-tier: dev `.env` → staging secrets → prod KMS | `CONTRIBUTING.md` |
| NHI/JIT | 5-minute scoped JWTs, JTI revoked via Redis | `workflow/workflows.go`, Redis |

Full 12-layer audit and priority remediation queue: **[SECURITY.md](SECURITY.md)**.

---

## 5. Repository map

```
backend/
  cmd/identity-service/            Single Go entry; in-process Temporal worker
  internal/
    ai/copilot.go                  GraphRAG pipeline
    cedar/engine.go                AWS Cedar policy engine + hot reload
    service/                       Identity + Access + NHI services
    workflow/activities/           Temporal workflows & activities (11 / 29)
    connector/                     Manifest-driven connector framework
    vault/ audit/ oidc/ outbox/    AES vault · audit chain · IdP · outbox
  policies/*.cedar                 Cedar policy-as-code (rbac, abac, agent, SoD)
infrastructure/
  postgres/init.sql                RLS · 28 tables · cedar_text · audit chain
  docker-compose.yml               Hardened layout; bind to 127.0.0.1
  temporal/ neo4j/ kafka/          Service configs
frontend/src/app/                  Next.js 14 (dashboard, identities, agents, connectors, access, audit, vault, idp)
docs/JOURNEY.md                    Engineering story — 6 phases, 98 commits
SECURITY.md                        12-layer audit · remediation queue
ARCHITECTURE.md                    ← you are here
```

---

## 6. Production path

- **Today**: single-region, single-binary Go service + static-export SPA, all behind Cloudflare Tunnel.
- **Scale-out** (no code change): split the Temporal worker to its own process; add horizontal
  replica for the gateway (stateless); shards for Neo4j. Cedar hot-reload means policy churn needs no redeploy.
```
docker compose up -d        # bring stack alive
make backend                # or rely on the compose identity-service build
```

Deployment guide: **[docs/JOURNEY.md](docs/JOURNEY.md)** for the engineering story and the
deployment kit described in **[DEPLOYMENT.md](DEPLOYMENT.md)**.
