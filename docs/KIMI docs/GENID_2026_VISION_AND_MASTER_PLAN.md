# GenID 2026 — Vision & Master Plan

> **Author:** Director/Architecture review · **Date:** 2026-08-15
> **Audience:** Shoaib (builder), vision sponsor (vision sponsor), ObserveID engineering
> **Status:** Active execution document. Every claim here was verified against the codebase on 2026-08-15 (`go build ./...` clean, `go test ./...` green).

---

## 1. The One-Paragraph Vision

**GenID is the event-driven, policy-first Identity Intelligence platform:** it watches identity events from every source (IdP, HR, ITSM, cloud), maintains a live identity graph with a continuously-decaying risk score per identity, and lets **policy — not schedulers, not ticket queues — decide** who gets access, for how long, and when it gets ripped away. Where SailPoint reacts quarterly and Saviynt reacts at request time, GenID reacts **at event time**.

our stakeholder's words, and they are correct: *"Intelligence before automation. React to change, not to clock."*

---

## 2. Where We Actually Are (verified, not aspirational)

### 2a. ObserveID today (the legacy estate — from our stakeholder's architecture diagram)

Kubernetes + Contour/Envoy ingress, monolithic .NET services (Auth, Access 360, Workflows, Guacamole PAM, Notifications, Logs, Obi-AI), **RabbitMQ** as the only broker, **MySQL NDB** as primary store, Universal Connector VMs for on-prem. No event-driven risk, no policy engine, no graph. Solid 2018-era IGA. This is what vision sponsor is trying to escape.

### 2b. GenID today (verified in code, 2026-08-15)

| Layer | State | Verdict |
|---|---|---|
| Temporal workflows (grant/JIT/firecall/revoke/approve) | Real, tested | ✅ Keep — this is the durable core |
| NATS JetStream event bus + outbox relay | Real, wired in `main.go`, queue-grouped risk consumer | ✅ Keep — the internal hub |
| Event-driven risk engine (decay, bands, auto session-kill, micro-review) | Real (`internal/eventing/processor.go`) | ✅ Keep — this is our stakeholder's exact ask |
| Cedar policy engine (real `cedar-go`, per-tenant, PG-loaded) | Real | ✅ Keep |
| Neo4j identity graph + blast radius | Real | ✅ Keep — the "relationship brain" |
| SCIM 2.0, OIDC provider, GraphQL | Real | ✅ Keep — integration surface |
| `identity_service.go` god object | 5,812 lines | ❌ Split (Phase 4) |
| Conditional access context (location/device/network) | **Zero** | ❌ Build (Phase 2) |
| External event ingestion (Azure SB, IdP webhooks) | **Zero** — only generic `POST /events/ingest` | ❌ Build (Phase 1) |
| Docs | STATUS.md claims Kafka is running (false) | ❌ Fix (Phase 0) |

**Bottom line:** the engine is 80% built. The missing 20% is exactly the part vision sponsor can *see*: events flowing in from the outside world and policy acting on them.

---

## 3. The 2026 Bar (what the leaders just shipped)

Grounded in current market state:

| Capability | Market position (2026) | GenID position |
|---|---|---|
| **NHI / AI-agent governance** | SailPoint "Adaptive Identity" (Mar 2026), Saviynt (2025) — first-class NHI | ✅ GenID has NHI registry + JIT passports — **ahead of the curve for our size** |
| **JIT / Zero Standing Privilege** | Saviynt live H1 2025; SailPoint only roadmap | ✅ GenID has 5-min JIT + kill switch — **at parity with leaders** |
| **ITDR / continuous risk** | Now table stakes (PingOne Protect, Entra ID Protection) | ✅ GenID event-driven risk exists — needs external event feeds |
| **Policy-based auto-access** | Entra Conditional Access is the benchmark; Lumos "AI-generated policies" | ⚠️ Cedar is real but has no context signals yet |
| **Real-time SoD prevention** | Saviynt claims 36% prevented at request point | ⚠️ GenID has SoD pages; wire into approval gate |
| **Natural-language admin (MCP/copilot)** | Saviynt MCP server shipped | ⚠️ GenID has `internal/ai/copilot.go` — extend |
| **Certifications** | Everyone; CyberArk pushing *continuous* compliance | ✅ GenID has certifications module |
| **Deployment TCO** | Lumos: "weeks not quarters, 80% lower TCO" is the new bar | ✅ GenID: one compose file, ~$0 dev — this is a **weapon**, keep it lean |

**Strategic read:** GenID's differentiation is not feature count — it's the **event-driven intelligence loop** (events → graph → risk → policy → auto-action) at a fraction of the cost. SailPoint/Saviynt bolt this onto 15-year-old codebases; GenID was born with it.

---

## 4. Target Architecture (2026 production, cost-disciplined)

```
                    EXTERNAL EVENT SOURCES
   Entra ID · Okta · Workday · Jira · Azure Service Bus · CrowdStrike
                              │
              ┌───────────────▼────────────────┐
              │  INGESTION EDGE (new, Phase 1) │
              │  Source adapters → normalize → │
              │  canonical identity events     │
              └───────────────┬────────────────┘
                              │
                   ┌──────────▼──────────┐
                   │  NATS JetStream     │  ← internal hub (NOT Neo4j, NOT Kafka-for-everything)
                   │  genid-events.>     │
                   └──┬───────┬───────┬──┘
                      │       │       │
              ┌───────▼──┐ ┌──▼─────┐ ┌▼─────────────┐
              │ Risk     │ │ Audit  │ │ Webhook/     │
              │ Processor│ │ Ledger │ │ ObserveID    │
              │ (exists) │ │(exists)│ │ bridge (P2)  │
              └──┬───────┘ └────────┘ └──────────────┘
                 │ updates
        ┌────────▼─────────────────────────┐
        │  Neo4j identity graph            │
        │  risk_score · risk_band · edges  │
        └────────┬─────────────────────────┘
                 │ read at decision time
   ACCESS REQUEST → ┌──────────────────────────────┐
                    │ POLICY GATE                  │
                    │ Cedar (ABAC context) +       │
                    │ OpenFGA (relationships) +    │
                    │ risk band → auto-approve /   │
                    │ step-up / deny / 2h JIT      │
                    └──────────────┬───────────────┘
                                   │
                    ┌──────────────▼───────────────┐
                    │ Temporal workflows (exists)  │
                    │ grant · JIT · firecall ·     │
                    │ revoke · certify             │
                    └──────────────────────────────┘
```

**Cost discipline rules (production-realistic, not enterprise-bloated):**

1. **NATS JetStream over Kafka** for the internal fabric. JetStream gives durability, replay, queue groups at ~15MB RAM vs a Kafka+ZK/KRaft cluster. Bridges to Azure Service Bus / Kafka live at the *edge* only, where the customer's world demands it. This is the answer to "which hub are we using?": **NATS inside, any broker at the border.**
2. **Modular monolith, 3 deployables** (gateway, temporal-worker, event-processor). No microservices tax until a customer scale forces it.
3. **No Qdrant** — Neo4j 5 native vector search covers GraphRAG. One less datastore.
4. **Open source leverage per vision sponsor:** cedar-go (policy), OpenFGA (relationships), Temporal (orchestration), NATS (fabric). We write glue and domain logic, not infrastructure.
5. **Cloudflare free tier** at the edge. Postgres+Neo4j+Redis+NATS+Temporal in compose for dev; managed equivalents (Neon/ Aura / Upstash / Cloud NATS / Temporal Cloud) only when revenue exists.

---

## 5. Kill List (what we remove or stop doing)

| Remove / Stop | Why |
|---|---|
| **Kafka claims in STATUS.md/docs** | False. Compose runs NATS. Doc drift destroyed credibility with vision sponsor once already. |
| **`audit_logs` legacy table** | Dual audit is confusing; `workflow_audit` hash-chain is the real ledger. Migrate + drop. |
| **Qdrant anywhere in docs/compose** | Neo4j native vectors replace it. |
| **RabbitMQ thinking** (ObserveID side) | In GenID there is no RabbitMQ; do not reintroduce the pattern. |
| **`internal/vault` unused wrapper** | Wire it or delete it. Decision: wire it in Phase 4 (secrets for connector creds). |
| **`internal/cedar` pattern-converter fallback** | Once policies are all real Cedar text, delete `convertPatternToCedar`. |
| **Docs-first commits** | No more README/badge/journey commits until Phase 1 demo runs on a laptop. |

---

## 6. Execution Plan (phased, inch by inch)

### Phase 0 — Truth & Alignment (0.5 day) ✅ *this session*
- [x] ADR-001: Event backbone decision (NATS inside, brokers at edge) — answers our stakeholder's "which hub?"
- [x] Fix STATUS.md infra table to match compose reality

### Phase 1 — The vision sponsor POC (2–3 days) — *his Monday demo, made real*
1. `internal/eventing/sources/`: adapter interface + registry
2. Webhook adapter (generic, JSONPath-mapped — covers Okta/Entra/Jira events)
3. Azure Service Bus adapter (he said ASB is installed at ObserveID)
4. `scripts/simulate-idp-events.sh` — fires "alice: 3 failed logins" end-to-end
5. Demo runbook: event → NATS → risk +300 → band change → visible in `/risk` UI → (critical) sessions killed
- **Exit criteria:** vision sponsor watches a failed-login storm move a risk score and trigger action, live, from a laptop.

### Phase 2 — Conditional Access (1 week) — *his flagship policy example*
- Extend Cedar `AuthRequest.Context`: `location`, `network_zone`, `device_trust`, `time_of_day`, `risk_band`
- Context enrichment v1: CIDR→zone map + business-hours table (static config, no MDM yet)
- Seed policy: *IT admin + corporate zone + business hours + risk<500 → auto-approve 2h JIT*; remote/unmanaged → step-up approval
- Test matrix: office/remote × managed/unmanaged × low/high risk

### Phase 3 — ObserveID Bridge (1 week)
- Webhook registration API (`POST /webhooks`) — ObserveID gets `access.approved/denied/revoked`, `risk.changed` callbacks
- API-key auth + rate limit for machine consumers
- Cursor pagination + ETags on `/requests`, `/approvals/queue`
- ObserveID connector type (pull identities from ObserveID into the graph)

### Phase 4 — Hardening (1 week)
- Split `identity_service.go` → `handlers/` `services/` `stores/` (mechanical, test-protected)
- Tenant isolation enforcement in middleware (RLS exists; verify every query path)
- Vault wired for connector secrets; Prometheus `/metrics`

### Phase 5 — Intelligence Layer (2+ weeks, post-POC)
- OpenFGA wiring for relationship checks ("who else can reach this resource?")
- Real-time SoD check inside the approval gate (Saviynt's 36%-prevention claim is the benchmark)
- GraphRAG copilot: natural-language "why does alice have prod access?" over the graph
- Role mining from graph clusters (SailPoint Harbor Pilot equivalent, v1 heuristic)

---

## 7. What vision sponsor Gets to Say After Phase 1–2

> "Same platform, new behavior. Events come in from the IdP, risk moves in real time, policy auto-grants two-hour access to a low-risk admin in the office and steps up anyone remote. No scheduler. No ticket. That's the intelligence layer — and it cost us NATS, not a Kafka cluster."

That is a SailPoint-2026-roadmap story, running on a laptop, in a product that already exists.

---

## 8. Division of Labor

- **Kimi (this agent):** Phases 0–4 code, tests, docs, demo scripts. I build, you review.
- **Shoaib (laptop):** `docker compose up -d`, run the demo script, click through `/risk` and `/inbox`, report friction.
- **vision sponsor:** Validate the demo script matches his mental model; supply one real Azure Service Bus connection string when ready to leave simulation.

---

*Next review: after Phase 1 demo runs green on Shoaib's laptop.*
