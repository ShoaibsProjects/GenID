<div align="center">

<img src="media/banner.svg" width="900" alt="GenID — The Agentic Identity Fabric" />

# GenID · The Agentic Identity Fabric

### A graph-native, policy-as-code Identity & Access Management (IAM/IGA) platform for the AI era.

[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?style=for-the-badge&logo=go)](https://go.dev/)
[![Next.js Version](https://img.shields.io/badge/Next.js-14-black?style=for-the-badge&logo=next.js)](https://nextjs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=for-the-badge&logo=postgresql)](https://www.postgresql.org/)
[![Neo4j](https://img.shields.io/badge/Neo4j-5-008CC1?style=for-the-badge&logo=neo4j)](https://neo4j.com/)
[![Cedar](https://img.shields.io/badge/Policy-AWS_Cedar-FF9900?style=for-the-badge&logo=amazonaws)](https://www.cedarpolicy.com/)
[![Temporal](https://img.shields.io/badge/Temporal-v1.25-4A2BFF?style=for-the-badge&logo=temporal)](https://temporal.io/)
[![NATS](https://img.shields.io/badge/Events-NATS_JetStream-8CB4C6?style=for-the-badge&logo=nats)](https://nats.io/)
[![MIT License](https://img.shields.io/badge/License-MIT-blue?style=for-the-badge)](LICENSE)

[![The Journey](https://img.shields.io/badge/Docs-The_Journey-8B5CF6?style=for-the-badge)](docs/JOURNEY.md)
[![Security](https://img.shields.io/badge/Security-12_Layer_Audit-EF4444?style=for-the-badge)](SECURITY.md)
[![Contributing](https://img.shields.io/badge/Contributing-Guidelines-22C55E?style=for-the-badge)](CONTRIBUTING.md)

> **GenID secures humans, machines, and autonomous AI agents in real time** — with a 109ms
> kill switch, a mathematically-tight audit chain, and zero-standing privileges for non-human identities.

</div>

---

## 📊 By the Numbers

| Metric | Value |
|--------|-------|
| **Commits** | 98+ and counting |
| **Go backend** | ~31K lines (monolithic, modular) |
| **TypeScript frontend** | ~7.2K lines (Next.js 14) |
| **RLS-protected tables** | 28 |
| **Identity kill switch** | **109 ms** (Temporal + Redis JTI) |
| **JIT agent sessions** | 5-minute scoped JWTs · zero-standing default |

---

## 🏗️ Architecture

Five layered planes — Edge → Gateway → Services → Async → Data — built around a
**PG + Neo4j dual-write** core and an **AWS Cedar policy engine** at the heart of every decision.

<img src="Architecturenewidea.svg" width="880" alt="GenID architecture diagram" />

### The planes at a glance

1. **Edge & Ingestion** — Cloudflare Tunnel/WAF, Next.js 14 UI, SCIM 2.0 gateway.
2. **Policy & Authorization** — Go API Gateway, AWS Cedar, OIDC JWT minting (JWKS).
3. **Agentic Governance** — NHI registry, Blast-Radius analytics, 109ms Kill Switch.
4. **Compliance & Audit** — Access certifications, tamper-proof SHA-256 audit chain, CAEP signals.
5. **State & Orchestration** — PostgreSQL 16 (RLS), Neo4j 5, Redis 7, Temporal + NATS JetStream.

---

## ⚡ Core Capabilities

- **🛡️ 109ms Kill Switch** — Risk-tiered identity revocation via Temporal workflows + Redis JTI blocklists.
- **📜 Continuous Authorization (Cedar)** — AWS Cedar policy-as-code with hot-reload and force-spanning scope.
- **🤖 Agentic Governance (NHI)** — First-class Non-Human Identity management with 5-minute JIT Ports.
- **🕸️ Graph-Native Analytics** — Neo4j variable-length pathfinding for real-time Blast Radius & SoD detection.
- **🔗 Tamper-Proof Audit Ledger** — Cryptologically chained SHA-256 logs to catch rogue admin modification.
- **🔐 Strict Multi-Tenancy** — PostgreSQL Row-Level Security enforced at the DB layer across 28 tables.
- **📡 Event-Driven** — Transactional outbox → NATS JetStream (replaces Kafka at 15MB RAM).
- **🔌 Universal Connectors** — Manifest-driven framework: Entra ID · LDAP · SCIM 2.0 · CSV · generic REST.

---

## 📁 Repository Map (for reviewers & AI agents)

```
backend/
  cmd/identity-service/        Main Go service (gateway, middleware, routes)
  internal/
    connector/                 Manifest-driven connector framework (entra, epic, jsonpath, loader)
    service/                   Identity, Access, Cedar, NHI, Audit services
    cedar/                     AWS Cedar policy engine integration
    vault/                     AES-256-GCM credential vault
infrastructure/
  postgres/init.sql            RLS, 28 tables, audit chain
  docker-compose.yml           PG · Neo4j · Redis · Temporal · NATS
frontend/
  src/app/                     Next.js 14 UI (dashboard, identities, agents, connectors, access)
docs/
  JOURNEY.md                   From cold repo to FAANG-grade identity fabric — the full story
```

---

## 🛠️ Tech Stack

| Category | Technology |
|----------|------------|
| **Backend** | Go 1.25, gorilla/mux, AWS Cedar, Temporal SDK, neo4j-go-driver, pgx/v5 |
| **Frontend** | Next.js 14, React 18, Tailwind CSS |
| **Data Line** | PostgreSQL 16 (RLS), Neo4j 5, Redis 7 |
| **Infra** | Docker Compose, NATS JetStream, Cloudflare Tunnel, Prometheus metrics |

---

## 🚀 Quickstart (Local Dev)

```bash
git clone https://github.com/ShoaibsProjects/GenID.git
cd GenID

# 1. Infrastructure (PG · Neo4j · Redis · Temporal · NATS)
cd infrastructure && docker compose up -d

# 2. Backend
cd ../backend && cp .env.example .env && go run cmd/identity-service/main.go

# 3. Frontend
cd ../frontend && npm install && npm run dev
```

API → `http://localhost:8080` · UI → `http://localhost:3001`

---

## 📚 Documentation

- **[The Journey](docs/JOURNEY.md)** — 6 phases, 98 commits: how GenID was engineered end-to-end.
- **[Security Audit](SECURITY.md)** — 12-layer FAANG-grade audit + remediation queue.
- **[Contributing](CONTRIBUTING.md)** — standards, workflow, and review checklist.

---

<div align="center">
  <sub>Built with determination by <a href="https://github.com/ShoaibsProjects">Shoaib Akthar</a> · GenID © 2026</sub>
</div>
