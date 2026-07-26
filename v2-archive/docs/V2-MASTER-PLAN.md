# Fortune Identity Cloud — V2 Master Plan

> **⚠️ START HERE FOR VERSION 2**
> 
> This is the master plan for building **Fortune Identity Cloud** — not another IAM platform, but an **Identity Runtime** for 2032.
> 
> **Quick Navigation:**
> - 🏢 **[FORTUNE-BRAND.md](./FORTUNE-BRAND.md)** — Brand identity, naming rationale, messaging
> - 🗺️ **[FORTUNE-MINDMAP.md](./FORTUNE-MINDMAP.md)** — Complete architecture tree, visual blueprint
> - 📖 **[V2-VISION.md](./V2-VISION.md)** — The 2032 vision, architecture components, scale targets
> - 🤖 **[V2-AI-WORKFLOW.md](./V2-AI-WORKFLOW.md)** — AI team workflow, model selection, prompts
> - 🔄 **[V1-TO-V2-MIGRATION.md](./V1-TO-V2-MIGRATION.md)** — What to keep, refactor, rebuild from V1
> - 📦 **[OUTBOX-PATTERN-RESEARCH.md](./OUTBOX-PATTERN-RESEARCH.md)** — Deep-dive on event sourcing consistency
> - 📋 **[V2-MASTER-PLAN.md](./V2-MASTER-PLAN.md)** — This document — execution plan
> 
> ---
> 
> **Context:** We completed V1 (ObserveID) with OIDC/OAuth2 provider (10/10 tests), Cedar engine (10 unit tests), Temporal workflows, and basic IAM features. V2 transforms this into **Fortune Identity Cloud** — a production-grade identity platform that scales to 100M+ identities.
> 
> **Next Step:** Begin **Phase 1: Foundation** (Event Sourcing + Outbox Pattern) — see below.
> 
> ---

> **Fortune Identity Cloud — Version 2**
> 
> Not another IAM platform. An identity operating system for 2032.
> 
> **Tagline**: "Identity. Reimagined."

---

## 📋 Document Index

| Document | Purpose |
|----------|---------|
| **V2-VISION.md** | The 2032 vision, architecture components, scale targets |
| **V2-AI-WORKFLOW.md** | AI team workflow, model selection, prompts |
| **V1-TO-V2-MIGRATION.md** | What to keep, refactor, rebuild from V1 |
| **OUTBOX-PATTERN-RESEARCH.md** | Deep-dive on event sourcing consistency |
| **V2-MASTER-PLAN.md** | This document — execution plan |

---

## 🎯 The Goal

Build the **Identity Cloud** — a platform that:
- Scales to **100M+ identities** and **billions of auth decisions**
- Supports **workforce, customer, B2B, workload, machine, AI agent** identities
- Provides **continuous authorization** (not just login-time)
- Feels like **Linear, Vercel, Stripe Dashboard** (not 2015 admin portals)
- Has a **developer platform** (SDK, CLI, Terraform, K8s operator)
- Includes an **MCP Server** for AI-native identity operations
- Could realistically become a **$5B company**

---

## 🏗️ Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                     Identity Cloud                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │  Identity    │  │  Policy      │  │  Risk        │          │
│  │  Graph       │  │  Engine      │  │  Engine      │          │
│  │  (Neo4j)     │  │  (Cedar)     │  │  (Real-time) │          │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │
│         │                  │                  │                  │
│  ┌──────┴──────────────────┴──────────────────┴───────┐        │
│  │              Event Sourcing Backbone                │        │
│  │         (PostgreSQL + Outbox + CDC)                 │        │
│  └──────┬──────────────────┬──────────────────┬───────┘        │
│         │                  │                  │                  │
│  ┌──────┴───────┐  ┌──────┴───────┐  ┌──────┴───────┐          │
│  │  Workflow    │  │  AI          │  │  Observ-     │          │
│  │  Engine      │  │  Layer       │  │  ability     │          │
│  │  (Temporal)  │  │  (GraphRAG)  │  │  (OTel)      │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
│                                                                  │
│  ┌─────────────────────────────────────────────────────┐        │
│  │              Developer Platform                      │        │
│  │   REST • GraphQL • SDK • CLI • Terraform • K8s      │        │
│  └─────────────────────────────────────────────────────┘        │
│                                                                  │
│  ┌─────────────────────────────────────────────────────┐        │
│  │              Identity Mesh                           │        │
│  │   OAuth2 • OIDC • SAML • SCIM • MCP • WebAuthn      │        │
│  └─────────────────────────────────────────────────────┘        │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## 🚀 Implementation Phases

### Phase 1: Foundation (Weeks 1-4)
**Theme:** Event Sourcing + Consistency

| Week | Deliverable | V1 Reuse | V2 New |
|------|-------------|----------|--------|
| 1 | Event sourcing backbone | — | `events` table, event publisher |
| 2 | Outbox pattern | — | Outbox processor, CDC |
| 3 | CQRS read models | Identity service queries | Separate read/write |
| 4 | Integration tests | Existing tests | Event replay tests |

**Key Decisions:**
- Event store: PostgreSQL (start) → Kafka (scale)
- CDC: Debezium (future) → Polling (now)
- Read models: PostgreSQL + Neo4j (dual)

**Success Criteria:**
- [ ] All identity mutations produce events
- [ ] Outbox processor syncs to Neo4j
- [ ] Event replay works
- [ ] No dual-write inconsistencies

---

### Phase 2: Identity Expansion (Weeks 5-8)
**Theme:** Full Identity Spectrum

| Week | Deliverable | V1 Reuse | V2 New |
|------|-------------|----------|--------|
| 5 | Workload identity | NHI table | K8s service accounts |
| 6 | Machine identity | NHI table | IoT, edge devices |
| 7 | AI agent identity | NHI table | MCP, A2A protocols |
| 8 | Identity lifecycle | Workflows | Full lifecycle automation |

**Key Decisions:**
- Identity types: Extensible enum (not hardcoded)
- Agent identity: MCP-first design
- Lifecycle: Temporal workflows for each type

**Success Criteria:**
- [ ] 10 identity types supported
- [ ] AI agents can register via MCP
- [ ] Workload identity auto-discovery
- [ ] Lifecycle workflows for all types

---

### Phase 3: Authorization (Weeks 9-12)
**Theme:** Continuous + Risk-Adaptive

| Week | Deliverable | V1 Reuse | V2 New |
|------|-------------|----------|--------|
| 9 | Risk engine | Basic scoring | Real-time risk API |
| 10 | Continuous auth | Cedar engine | Every API call evaluated |
| 11 | Policy simulation | — | What-if analysis UI |
| 12 | Delegated admin | — | Hierarchy, constraints |

**Key Decisions:**
- Risk scoring: Behavioral + contextual + threat intel
- Continuous auth: Middleware-level evaluation
- Policy simulation: Dry-run mode in Cedar

**Success Criteria:**
- [ ] Risk scoring <10ms latency
- [ ] Continuous auth on all endpoints
- [ ] Policy simulation works
- [ ] Delegated admin hierarchy

---

### Phase 4: Developer Platform (Weeks 13-16)
**Theme:** SDK + CLI + Terraform

| Week | Deliverable | V1 Reuse | V2 New |
|------|-------------|----------|--------|
| 13 | Go SDK | — | `observeid-go` package |
| 14 | TypeScript SDK | — | `@observeid/sdk` npm |
| 15 | CLI | — | `observeid` binary |
| 16 | Terraform provider | — | `terraform-provider-observeid` |

**Key Decisions:**
- SDKs: Auto-generated from OpenAPI
- CLI: Cobra (Go)
- Terraform: Official provider framework

**Success Criteria:**
- [ ] Go SDK published
- [ ] TypeScript SDK on npm
- [ ] CLI works end-to-end
- [ ] Terraform provider passes acceptance tests

---

### Phase 5: UI Redesign (Weeks 17-20)
**Theme:** Linear/Vercel Aesthetic

| Week | Deliverable | V1 Reuse | V2 New |
|------|-------------|----------|--------|
| 17 | Design system | — | Components, tokens |
| 18 | Identity graph UI | Basic page | Real-time graph viz |
| 19 | Policy editor | Basic page | Monaco editor, simulation |
| 20 | AI copilot UI | Stub | Integrated everywhere |

**Key Decisions:**
- Framework: Next.js 15 (App Router)
- Design: Shadcn/ui + custom
- Graph: D3.js or Cytoscape.js
- Real-time: WebSocket + SSE

**Success Criteria:**
- [ ] Feels like Linear/Vercel
- [ ] Graph visualization works
- [ ] AI copilot integrated
- [ ] Real-time updates

---

### Phase 6: Observability & Scale (Weeks 21-24)
**Theme:** Traces + Multi-Region

| Week | Deliverable | V1 Reuse | V2 New |
|------|-------------|----------|--------|
| 21 | Identity traces | OTel basics | Full trace per decision |
| 22 | Multi-region | — | US, EU, APAC |
| 23 | Performance | — | <50ms p99 at 100K rps |
| 24 | Load testing | — | 100M identities |

**Key Decisions:**
- Tracing: OpenTelemetry + Jaeger
- Multi-region: Active-active with CRDTs
- Performance: Connection pooling, caching

**Success Criteria:**
- [ ] Every auth decision has trace
- [ ] Multi-region deployment works
- [ ] p99 <50ms at scale
- [ ] 100M identities loaded

---

## 🔧 Technology Stack

### Backend
| Component | V1 | V2 |
|-----------|----|----|
| Language | Go 1.25 | Go 1.26 |
| Framework | Gorilla Mux | Chi or Fiber |
| Database | PostgreSQL 16 | PostgreSQL 17 + Citus |
| Graph | Neo4j 5 | Neo4j 5 (cluster) |
| Cache | Redis 7 | Redis 7 Cluster |
| Queue | — | Kafka 3.7 |
| Workflow | Temporal 1.24 | Temporal 1.25 |
| Policy | Cedar (cedar-go) | Cedar + custom |
| Search | — | Elasticsearch 8 |
| Vector DB | — | Qdrant |

### Frontend
| Component | V1 | V2 |
|-----------|----|----|
| Framework | Next.js 14 | Next.js 15 |
| UI Library | Tailwind | Tailwind + Shadcn |
| State | React hooks | Zustand |
| Graph Viz | — | D3.js / Cytoscape |
| Editor | — | Monaco |
| Real-time | — | WebSocket + SSE |

### Infrastructure
| Component | V1 | V2 |
|-----------|----|----|
| Container | Docker | Docker + K8s |
| Orchestration | Docker Compose | Kubernetes 1.30 |
| IaC | — | Terraform |
| CI/CD | GitHub Actions | GitHub Actions + ArgoCD |
| Monitoring | Prometheus | Prometheus + Grafana |
| Tracing | OTel | OTel + Jaeger |
| Logging | Zerolog | OTel + Loki |

### Developer Tools
| Component | V1 | V2 |
|-----------|----|----|
| API | REST + GraphQL | REST + GraphQL + gRPC |
| SDK | — | Go, TypeScript, Python |
| CLI | — | Cobra (Go) |
| Terraform | — | Official provider |
| K8s Operator | — | Operator SDK |
| VS Code | — | Extension |

---

## 📊 Scale Targets

| Metric | V1 | V2 Target |
|--------|----|-----------|
| Identities | 10K | 100M+ |
| Auth decisions/sec | 1K | 100K+ |
| Graph nodes | 100K | 1B+ |
| Graph edges | 500K | 10B+ |
| Latency (p99) | 200ms | <50ms |
| Availability | 99.9% | 99.99% |
| Regions | 1 | 3+ (US, EU, APAC) |
| Tenants | 1 | 10K+ |
| Connectors | 5 | 500+ |
| API coverage | 70% | 100% |
| Test coverage | 60% | 90% |

---

## 🎨 UI/UX Philosophy

### Not an Admin Portal. An Identity Operating System.

**Inspiration:**
- Linear (speed, keyboard-first)
- Vercel (clean, minimal)
- Stripe Dashboard (data density)
- Notion (flexible views)
- Cursor (AI-native)

**Principles:**
1. **Graph-first** — Everything is a node, everything is a relationship
2. **Real-time** — WebSocket updates, no refresh needed
3. **AI-native** — Copilot integrated everywhere
4. **Keyboard-first** — Power users never touch the mouse
5. **Dark mode** — Default, not an afterthought
6. **Composable** — Every component is reusable

**Key Screens:**
- **Identity Graph** — Interactive force-directed graph
- **Policy Editor** — Monaco + simulation + AI suggestions
- **Access Timeline** — Who had access when, why
- **Risk Dashboard** — Real-time risk heatmap
- **AI Copilot** — Natural language → identity operations

---

## 🤖 AI Integration

### MCP Server
Built-in Model Context Protocol server for AI-native identity operations:

```
Cursor / Claude / GPT
        ↓
      MCP
        ↓
   ObserveID
        ↓
• Get user
• Provision user
• Revoke access
• Analyze permissions
• Generate access review
• Detect privilege escalation
• Run policy simulation
```

### GraphRAG Copilot
Real LLM + vector database for intelligent queries:

```
User: "Who has access to production databases?"
  ↓
Copilot:
1. Parse intent (graph query)
2. Query Neo4j (path traversal)
3. Apply Cedar policies (filter)
4. Return results + explanation
```

### AI-Driven Features
- **Access recommendations** — "You should grant X access to Y"
- **Policy suggestions** — "Consider adding this policy"
- **Anomaly explanation** — "This user's behavior changed because..."
- **Certification assistance** — "These 5 users should be reviewed"

---

## 🔐 Security Model

### Zero Trust Architecture
- **Never trust, always verify** — Every request authenticated + authorized
- **Least privilege** — Minimum permissions by default
- **Assume breach** — Continuous verification, not just login

### Identity Security
- **Passwordless by default** — WebAuthn, passkeys
- **MFA adaptive** — Based on risk score
- **Session binding** — Device, location, behavior
- **Continuous auth** — Re-evaluate on every action

### Data Security
- **Encryption at rest** — AES-256-GCM
- **Encryption in transit** — TLS 1.3
- **Key rotation** — Automatic, zero-downtime
- **Audit trail** — Every action logged, immutable

---

## 📈 Success Metrics

### Technical
- [ ] 100M identities loaded
- [ ] 100K auth decisions/sec
- [ ] <50ms p99 latency
- [ ] 99.99% availability
- [ ] 90% test coverage

### Product
- [ ] 10 identity types
- [ ] 50 connectors
- [ ] 100 Cedar policies
- [ ] MCP server working
- [ ] AI copilot integrated

### Business
- [ ] $5B valuation potential
- [ ] Target customer interest
- [ ] Open source traction
- [ ] Community contributions
- [ ] Enterprise pilot

---

## 🎯 The Hiring Signal

This project should demonstrate:

### For Google
- Distributed systems at scale
- Clean architecture
- Production-grade code
- Performance optimization

### For Apple
- Beautiful UI/UX
- Attention to detail
- Privacy-first design
- Developer experience

### For Okta
- Deep identity expertise
- OAuth2/OIDC/SAML mastery
- Federation protocols
- Enterprise features

### For Target
- MCP integration
- AI-native identity
- Continuous authorization
- Risk-adaptive access

### For Netflix
- Event sourcing
- Multi-region deployment
- Observability
- Chaos engineering

---

## 🚨 Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Scope creep | High | High | Strict phase boundaries |
| Performance regression | High | Medium | Load testing each phase |
| Data migration issues | High | Medium | Dual-write during transition |
| Team bandwidth | High | High | Prioritize P0 features |
| Technology changes | Medium | Medium | Abstract interfaces |
| Security vulnerabilities | Critical | Low | Security review each phase |

---

## 📅 Timeline

| Phase | Duration | Start | End |
|-------|----------|-------|-----|
| Phase 1: Foundation | 4 weeks | Week 1 | Week 4 |
| Phase 2: Identity | 4 weeks | Week 5 | Week 8 |
| Phase 3: Authorization | 4 weeks | Week 9 | Week 12 |
| Phase 4: Developer | 4 weeks | Week 13 | Week 16 |
| Phase 5: UI | 4 weeks | Week 17 | Week 20 |
| Phase 6: Scale | 4 weeks | Week 21 | Week 24 |
| **Total** | **24 weeks** | **~6 months** | |

---

## 🎓 What This Demonstrates

### Principal Engineer Level
- System design at scale
- Tradeoff analysis
- Technology selection
- Migration strategy

### Distinguished Engineer Level
- Original architecture
- Industry influence
- Technical vision
- Cross-team impact

### The Story
> "I designed and built an Identity Cloud — a distributed identity platform that scales to 100M+ identities, supports 10 identity types (including AI agents), provides continuous authorization, and includes a full developer platform with SDK, CLI, Terraform provider, and Kubernetes operator. It uses event sourcing for consistency, Cedar for policy-as-code, Neo4j for the identity graph, and includes an MCP server for AI-native identity operations."

That's not "another IAM project."

That's the kind of project that makes recruiters stop scrolling.

---

**Last Updated:** 2026-07-22  
**Status:** Plan Complete — Ready for Phase 1 Implementation  
**Next Step:** Begin Phase 1 (Event Sourcing Foundation)
