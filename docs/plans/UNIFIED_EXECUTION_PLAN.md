# GenID — UNIFIED EXECUTION PLAN (Assembled 2026-08-15)

> **Source of truth:** Merges completed work (Phases 2.1–2.4) + KIMI vision (Phases 0–5) + our stakeholder's asks.
> **Status:** Every completed item verified `go test ./...` green, `go vet` clean, E2E validated.
> **Next action:** Phase 1 (Ingestion Edge) — the vision sponsor POC.

---

## 1. WHAT'S DONE (Verified, Not Aspirational)

| Phase | Deliverable | Verification |
|-------|-------------|--------------|
| **2.1** | Workflow tables (`workflow_requests`, `workflow_approvals`, `workflow_audit`), Store CRUD (11 tests), Firecall E2E, `/requests` UI, Idempotency | ✅ `go test` green, E2E: firecall auto-grant → expiry → post-review |
| **2.2** | Approval Router (risk-band + resource escalation), ApprovalGateWorkflow (Temporal child, sequential/parallel, timeout, reconciliation), GrantAccess wiring, APIs (`/approvals/{id}/decide`, `/approvals/queue`) | ✅ 14 new tests, E2E: 2-level secrets chain approve → `executed` |
| **2.3** | `/inbox` UI (approve/deny + comment), `failure_reason` on deny, deterministic ordering | ✅ HTTP 200, E2E: deny at level 1 → `denied` + reason |
| **2.4** | **Notifications** (Slack/Teams webhooks, `approval.required/decided/timed_out`), **JIT expiry persistence** (`expires_at`, status transitions), **Delegation** (`POST /approvals/{id}/delegate`), **Self-service Catalog** (`/catalog/roles`, `POST /access/request-role` → GrantAccessWorkflow) | ✅ All 4 E2E validated, catalog page serves |

**Core engine intact:** 6 Temporal workflows, Cedar policy (stubbed but wired), Neo4j graph, hash-chained audit, outbox table.

---

## 2. WHAT THE STAKEHOLDER ACTUALLY WANTS (The Gap)

| Ask | Current State | Required |
|-----|---------------|----------|
| **"Kafka hub"** | Empty `outbox_events` table, NATS JetStream in compose | **NATS inside** (internal fabric), **Debezium CDC → Kafka at edge** for ObserveID consumption |
| **"Which hub?"** | Confusion — Neo4j is NOT a hub | **ADR-001:** NATS JetStream = internal hub; brokers at border only |
| **"Location-based access"** | ZERO context in policy | **Phase 2:** Context Enrichment Service (CIDR→zone, device trust, time) + Cedar ABAC |
| **"Plug into ObserveID"** | REST only, no events, no SCIM, no webhooks | **Phase 3:** Webhook registration, API keys, SCIM 2.0, ObserveID connector |
| **"Connect identities later"** | CSV/HR connectors only | **Phase 3:** ObserveID connector type (bidirectional sync) |

---

## 3. UNIFIED PHASE PLAN (Reconciled)

### ✅ PHASE 0 — Truth & Alignment (DONE)
- ADR-001 written (NATS inside, brokers at edge)
- STATUS.md infra table fixed to match compose reality

### ✅ PHASES 2.1–2.4 — Workflow & Approval Ecosystem (DONE)
*See Section 1. All verified, tested, UI serving.*

### 🔄 PHASE 1 — INGESTION EDGE (vision sponsor POC) — **NEXT (2–3 days)**
**Goal:** Event → NATS → risk +300 → band change → session kill, live on laptop.

| Task | Owner | Spec |
|------|-------|------|
| `internal/eventing/sources/adapter.go` — interface + registry | Kimi | `type SourceAdapter interface { Start(ctx) error; Stop() error; Events() <-chan CanonicalEvent }` |
| Webhook adapter (generic, JSONPath mapping for Okta/Entra/Jira) | Kimi | `POST /events/ingest` → normalize → publish to NATS `genid.events.identity` |
| Azure Service Bus adapter (ObserveID has ASB) | Kimi | AMQP 1.0 consumer → same normalization |
| `simulate-idp-events.sh` — fires "alice: 3 failed logins" end-to-end | Kimi | `curl -X POST /events/ingest -d '{"source":"entra","type":"failed_login","identity":"alice","count":3}'` |
| Risk processor consumes → updates Neo4j `risk_score`/`risk_band` | Kimi | Already exists in `internal/eventing/processor.go` — verify wiring |
| Demo runbook: `docker compose up -d && ./scripts/simulate-idp-events.sh` → watch `/risk` UI | Shoaib | Exit: vision sponsor sees risk move +300, band change, session kill |

**Deliverable:** Single script that runs the full loop on a laptop.

### 🔄 PHASE 2 — CONDITIONAL ACCESS (1 week)
**Goal:** Office=auto JIT, remote=step-up, policy matrix passes.

| Task | Owner | Spec |
|------|-------|------|
| Extend `PolicyCheckParams.Context`: `location`, `network_zone`, `device_trust`, `time_of_day`, `risk_band` | Kimi | All workflows (GrantAccess, JIT, Firecall) pass enriched context |
| Context Enrichment Service v1: CIDR→zone map + business-hours table (static config) | Kimi | Redis cache (TTL 5m), called from workflow via activity |
| Seed Cedar policy: `IT admin + corporate zone + business hours + risk<500 → auto-approve 2h JIT`; remote/unmanaged → step-up approval | Kimi | Real `cedar-go` eval, no pattern converter |
| Test matrix: office/remote × managed/unmanaged × low/high risk | Shoaib | 8 scenarios, all assert correct decision |

### 🔄 PHASE 3 — OBSERVEID BRIDGE (1 week)
**Goal:** ObserveID consumes GenID via events + API; bidirectional identity sync.

| Task | Owner | Spec |
|------|-------|------|
| Webhook registration API: `POST /webhooks` — ObserveID registers callbacks for `access.approved/denied/revoked`, `risk.changed` | Kimi | HMAC-signed payloads, retry with backoff |
| API-key auth + rate limit for machine consumers | Kimi | `Authorization: Bearer gk_...`, 1000 req/min default |
| Cursor pagination + ETags on `/requests`, `/approvals/queue` | Kimi | `?cursor=&limit=`, `If-None-Match` |
| SCIM 2.0 endpoint: `/scim/v2/Users`, `/Groups` (ObserveID provisions into GenID) | Kimi | RFC 7644 compliant, per-tenant |
| ObserveID connector type (pull identities from ObserveID into graph) | Kimi | New connector kind, maps ObserveID schema → Neo4j nodes |

### 🔄 PHASE 4 — HARDENING (1 week)
| Task | Owner |
|------|-------|
| Split `identity_service.go` (5,812 lines) → `handlers/` `services/` `stores/` | Kimi |
| Tenant isolation enforcement (RLS exists; verify every query path) | Kimi |
| Vault wired for connector secrets; Prometheus `/metrics` exposed | Kimi |

### 🔄 PHASE 5 — INTELLIGENCE LAYER (2+ weeks, post-POC)
| Task | Owner |
|------|-------|
| OpenFGA wiring for relationship checks ("who else can reach this resource?") | Kimi |
| Real-time SoD check inside approval gate (benchmark: Saviynt 36% prevention) | Kimi |
| GraphRAG copilot: natural-language "why does alice have prod access?" | Kimi |
| Role mining from graph clusters (SailPoint Harbor Pilot equivalent, v1 heuristic) | Kimi |

---

## 4. ARCHITECTURE DECISIONS (Locked)

| Decision | Rationale |
|----------|-----------|
| **NATS JetStream** = internal event fabric | 15MB RAM, durability, replay, queue groups; no ZK/KRaft ops |
| **Kafka only at edge** (Debezium CDC on `outbox_events`) | ObserveID expects Kafka; we don't run it internally |
| **Neo4j** = identity graph + risk + vector search | No Qdrant; native vectors cover GraphRAG |
| **Cedar-go** = policy engine | Real ABAC, per-tenant policies loaded from PG |
| **Temporal** = all orchestration | Durable, testable, versionable; no custom scheduler |
| **Modular monolith, 3 deployables** | gateway, temporal-worker, event-processor; no microservices tax |

---

## 5. KILL LIST (Enforced)

| Remove | Status |
|--------|--------|
| Kafka claims in docs | ✅ ADR-001 supersedes |
| `audit_logs` legacy table | ⏳ Migrate → drop |
| Qdrant references | ✅ None in compose |
| RabbitMQ patterns | ✅ None in GenID |
| `internal/vault` unused | ⏳ Wire in Phase 4 |
| Docs-first commits | ✅ Enforced |

---

## 6. DIVISION OF LABOR

| Role | Responsibility |
|------|----------------|
| **Kimi (this agent)** | Phases 1–4 code, tests, docs, demo scripts |
| **Shoaib** | `docker compose up -d`, run demo script, click `/risk` and `/inbox`, report friction |
| **vision sponsor** | Validate demo matches mental model; supply real ASB connection string when ready |

---

## 7. IMMEDIATE NEXT STEPS (Today)

1. **Kimi:** Create `internal/eventing/sources/adapter.go` + webhook adapter + ASB adapter
2. **Kimi:** Wire NATS publish in `main.go` (verify `eventing` package starts consumer)
3. **Kimi:** Update `simulate-idp-events.sh` to hit real endpoint
4. **Shoaib:** Run `docker compose up -d` → execute script → confirm risk moves in `/risk` UI
5. **Both:** Demo to vision sponsor by end of week.

---

## 8. FILES TO CREATE/MODIFY (Phase 1)

```
internal/eventing/
├── sources/
│   ├── adapter.go           # interface + registry
│   ├── webhook_adapter.go   # generic JSONPath → CanonicalEvent
│   └── asb_adapter.go       # Azure Service Bus consumer
├── processor.go             # verify NATS subscription + risk update
├── canonical.go             # CanonicalEvent struct (already exists?)
scripts/
└── simulate-idp-events.sh   # end-to-end demo
docs/
└── STATUS.md                # update infra table
```

---

**This is the plan. No more drift. Phase 1 starts now.**