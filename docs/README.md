# ObserveID — Documentation

> **From ObserveID concept to Fortune Identity Cloud reality**

---

## 🚀 Start Here

### Fortune Identity Cloud (V2)

| Document | Purpose | Status |
|----------|---------|--------|
| **[V2-MASTER-PLAN.md](./V2-MASTER-PLAN.md)** | ⭐ Master execution plan, phases, timeline | ✅ Complete |
| **[FORTUNE-BRAND.md](./FORTUNE-BRAND.md)** | Brand identity, naming rationale, messaging | ✅ Complete |
| **[FORTUNE-MINDMAP.md](./FORTUNE-MINDMAP.md)** | Complete architecture tree, visual blueprint | ✅ Complete |
| **[V2-VISION.md](./V2-VISION.md)** | 2032 vision, architecture components | ✅ Complete |
| **[V2-AI-WORKFLOW.md](./V2-AI-WORKFLOW.md)** | AI team workflow, model selection | ✅ Complete |
| **[V1-TO-V2-MIGRATION.md](./V1-TO-V2-MIGRATION.md)** | What to keep/refactor/rebuild from V1 | ✅ Complete |

### Deep-Dive Research

| Document | Purpose | Status |
|----------|---------|--------|
| **[OUTBOX-PATTERN-RESEARCH.md](./OUTBOX-PATTERN-RESEARCH.md)** | Event sourcing consistency (P0 #3) | ✅ Complete |

---

## 📊 Project Status

### Version 1 (ObserveID) — Complete ✅

| Feature | Status | Tests |
|---------|--------|-------|
| OIDC/OAuth2 Provider | ✅ Complete | 10/10 E2E |
| Cedar Policy Engine | ✅ Complete | 10/10 unit |
| Temporal Workflows | ✅ Complete | Integration |
| SCIM 2.0 Server | ✅ Complete | — |
| Identity Graph (Neo4j) | ✅ Complete | — |
| RBAC + ABAC | ✅ Complete | — |
| AI Copilot (stub) | ⚠️ Partial | — |
| Frontend (15 pages) | ✅ Complete | — |

### Version 2 (Fortune Identity Cloud) — Planning Complete 📋

**Next:** Phase 1 (Event Sourcing Foundation) — 4 weeks

---

## 🎯 The Vision

### From ObserveID to Fortune

**ObserveID** was the proof of concept — a solid foundation demonstrating modern IAM principles.

**Fortune Identity Cloud** is the evolution:
- Not just observing identity, but **governing** it
- Not just authentication, but **continuous authorization**
- Not just humans, but **AI agents, workloads, machines**
- Not just policies, but **event sourcing, risk scoring, AI-native**
- Not just a product, but a **platform, marketplace, ecosystem**

### The Goal

An **Identity Runtime** — the kind of platform that Apple, Google, Microsoft, AWS, Okta, Anthropic, or OpenAI would build today.

**Scale:**
- 100M+ identities
- Billions of auth decisions
- Multi-region deployment
- 10 identity types (workforce, customer, B2B, workload, machine, AI agent)

**Capabilities:**
- Continuous authorization
- Risk-adaptive access
- AI-native identity (MCP)
- Event sourcing
- Zero trust
- Passwordless/WebAuthn
- Policy simulation
- Real-time entitlement graph
- Developer platform (SDK, CLI, Terraform, K8s operator)

---

## 🏢 Brand Identity

```
Fortune Identity Cloud

Tagline: "Identity. Reimagined."

Colors:
- Primary: Deep Navy (#0A1929)
- Accent: Electric Blue (#00D4FF)
- Success: Emerald (#10B981)
- Warning: Amber (#F59E0B)
- Error: Rose (#F43F5E)

Typography:
- Headings: Inter (Bold)
- Body: Inter (Regular)
- Code: JetBrains Mono
```

See **[FORTUNE-BRAND.md](./FORTUNE-BRAND.md)** for complete brand guidelines.

---

## 🗺️ Architecture Overview

```
Fortune Identity Cloud
│
├── Identity Core (10 identity types)
├── Authentication (OAuth2, OIDC, SAML, SCIM, MCP, WebAuthn)
├── Authorization (Cedar, Continuous Auth, Risk-Adaptive)
├── Governance (Access Reviews, SoD, Compliance, PAM)
├── AI & Intelligence (GraphRAG, MCP Server, ML Models)
├── Developer Platform (SDK, CLI, Terraform, K8s Operator)
├── Infrastructure (Event Sourcing, Multi-Region, Observability)
└── Security (Zero Trust, Encryption, Threat Detection)
```

See **[FORTUNE-MINDMAP.md](./FORTUNE-MINDMAP.md)** for the complete architecture tree.

---

## 📁 Repository Structure

```
observeid/
├── backend/                 # Go backend (33K+ lines)
│   ├── cmd/                 # Entry point
│   ├── internal/
│   │   ├── activities/      # Temporal activities
│   │   ├── ai/              # AI copilot (stub)
│   │   ├── audit/           # Audit logging
│   │   ├── cedar/           # Cedar policy engine ✅
│   │   ├── connector/       # SCIM, LDAP, Entra, CSV
│   │   ├── domain/          # Domain models
│   │   ├── graphql/         # GraphQL API
│   │   ├── middleware/      # Auth, rate limit, validation
│   │   ├── oidc/            # OAuth2/OIDC provider ✅
│   │   ├── service/         # Identity service
│   │   ├── vault/           # Secrets vault
│   │   └── workflow/        # Temporal workflows
│   └── pkg/
│       ├── proto/           # Protobuf definitions
│       └── telemetry/       # Prometheus metrics
│
├── frontend/                # Next.js 14 frontend
│   └── src/app/             # 15 pages
│
├── infrastructure/          # Docker, PostgreSQL, Neo4j
│
├── policies/                # Cedar policies
│   ├── rbac.cedar
│   ├── abac.cedar
│   ├── agent.cedar
│   └── identity.cedarschema
│
└── docs/                    # This directory
    ├── README.md            # You are here
    ├── V2-MASTER-PLAN.md    # ⭐ START HERE
    ├── FORTUNE-BRAND.md     # Brand identity
    ├── FORTUNE-MINDMAP.md   # Architecture tree
    ├── V2-VISION.md         # 2032 vision
    ├── V2-AI-WORKFLOW.md    # AI workflow
    ├── V1-TO-V2-MIGRATION.md # Migration analysis
    └── OUTBOX-PATTERN-RESEARCH.md # Event sourcing
```

---

## 🧠 AI Workflow

Use the right AI for the right task:

| Model | Role | Usage |
|-------|------|-------|
| **GPT-5.5** | Principal Architect | Architecture, design reviews |
| **Qwen3-Coder** | Senior Engineer | Implementation (70-80%) |
| **Kimi K2** | Staff Reviewer | Code review, finding gaps |
| **Claude Opus** | Design Critic | Brutally honest reviews |

See **[V2-AI-WORKFLOW.md](./V2-AI-WORKFLOW.md)** for detailed prompts and workflow.

---

## 🎯 Next Steps

### Immediate (This Session)
1. ✅ Complete V1 (OIDC + Cedar done)
2. ✅ Plan V2 (all docs complete)
3. ✅ Define Fortune brand identity
4. ✅ Create architecture mindmap
5. 🔲 Begin Phase 1: Event Sourcing Foundation

### Phase 1: Foundation (Weeks 1-4)
- [ ] Event sourcing backbone (`events` table)
- [ ] Outbox pattern implementation
- [ ] CQRS read models
- [ ] Integration tests

### Phase 2-6: See V2-MASTER-PLAN.md

---

## 📞 Resources

### External
- [Cedar Policy Language](https://www.cedarpolicy.com/)
- [Temporal Documentation](https://docs.temporal.io/)
- [Neo4j Documentation](https://neo4j.com/docs/)
- [OAuth 2.0 / OIDC](https://openid.net/connect/)

### Internal
- Backend API: `http://localhost:8080`
- Frontend: `http://localhost:3000`
- Neo4j Browser: `http://localhost:7474`
- Temporal UI: `http://localhost:8233`

---

**Last Updated:** 2026-07-22  
**Maintainer:** Shoaib Akthar  
**Project:** ObserveID → Fortune Identity Cloud
