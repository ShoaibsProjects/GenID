<div align="center">

# GenID
### The Agentic Identity Fabric

[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?style=for-the-badge&logo=go)](https://go.dev/)
[![Next.js Version](https://img.shields.io/badge/Next.js-14-black?style=for-the-badge&logo=next.js)](https://nextjs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=for-the-badge&logo=postgresql)](https://www.postgresql.org/)
[![Neo4j](https://img.shields.io/badge/Neo4j-5-008CC1?style=for-the-badge&logo=neo4j)](https://neo4j.com/)
[![Temporal](https://img.shields.io/badge/Temporal-v1.25-4A2BFF?style=for-the-badge&logo=temporal)](https://temporal.io/)
[![License](https://img.shields.io/badge/License-MIT-blue?style=for-the-badge)](LICENSE)

**GenID is a graph-native, policy-as-code Identity & Access Management (IAM/IGA) platform built for the Agentic Era.** 
It secures humans, machines, and autonomous AI agents in real-time with mathematical certainty.

</div>

---

## Core Capabilities

- **109ms Kill Switch:** Automated, risk-tiered identity revocation using Temporal workflows and Redis JTI blocklists.
- **Continuous Authorization (Cedar):** AWS Cedar policy-as-code engine with hot-reloading and force-spanning scope.
- **Agentic Governance:** First-class Non-Human Identity (NHI) management with 5-minute JIT Ports (Zero-Standing Strategy).
- **Graph-Native Analytics:** Neo4j variable-length pathfinding for real-time Blast Radius analysis and SoD detection.
- **Tamper-Proof Audit Ledger:** Cryptologically chained SHA-256 audit logs to detect rogue admin modification.
- **Strict Multi-Tenancy:** PostgreSQL Row Level Security (RLS) enforced at the database layer across 28 tables.

## Architecture

GenID is built as a layered system with a strict multi-plane design:

1. **Edge & Ingestion:** Cloudflare WAF, SCIM 2.0 Gateway, Next.js 14 UI.
2. **Policy & Authorization:** Go API Gateway, AWS Cedar Engine, OIDC JWT Token Minting.
3. **Agentic Governance:** NHI Registry, Blast Radius Analytics, Kill Switch.
4. **Compliance & Audit:** Access Certifications, Tamper-Proof Audit Chain, CAEP-dependent Warnings.
5. **State & Orchestration:** PostgreSQL (RLS), Neo4j (Graph), Redis (Cache Rate), Temporal & NATS.

<details>
<summary> View Architecture Diagram</summary>

```
                           ┌──────────────────────┐
                           │   Cloudflare Pages   │
                           │    (Next.js 14)      │
                           └──────────┬───────────┘
                                      │ HTTPS :8080
                                      ▼
                    ┌─────────────────────────────────┐
                    │        Go API Gateway            │
                    │  ┌──────────┐ ┌──────────────┐  │
                    │  │ JWTAuth   │ │ RateLimiter  │  │
                    │  ├──────────┤ ├──────────────┤  │
                    │  │ API Key   │ │ ContentType  │  │
                    │  ├──────────┤ ├──────────────┤  │
                    │  │ SCIM 2.0 │ │WorkflowGuard│  │
                    │  └──────────┘ └──────────────┘  │
                    └───────┬─────────┬───────┬───────┘
                            │         │       │
                   ┌────────┼─────────┼───────┼────────┐
                   ▼        ▼         ▼       ▼        ▼
           ┌──────────┐ ┌───────┐ ┌───────┐ ┌─────────┐
           │PostgreSQL│ │ Neo4j │ │ Redis │ │ Temporal│
           │   (RLS)  │ │Graph  │ │ Cache │ │Workflows│
           └──────────┘ └───────┘ └───────┘ └────┬────┘
                                                  │
                                          ┌───────┴───────┐
                                          │  NATS JetStream │
                                          │  Event Bus      │
                                          └─────────────────┘
```

</details>

## Tech Stack

| Category | Technology |
|----------|------------|
| **Backend** | Go 1.25, gorilla/mux, AWS Cedar, Temporal SDK |
| **Frontend** | Next.js 14, React 18, Tailwind CSS |
| **Data Line** | PostgreSQL 16, Neo4j 5 Community, Redis 7 |
| **Infra** | Docker, NATS JetStream, Cloudflare Tunnel |

## Quickstart (Local Dev)

1. **Clone the repository:**
   ```bash
   git clone https://github.com/ShoaibsProject/observeid-V2.git
   cd GenID
   ```

2. **Start infrastructure:**
   ```bash
   cd infrastructure
   docker compose up -d
   ```

3. **Run the backend:**
   ```bash
   cd ../backend
   cp .env.example .env
   go run cmd/identity-service/main.go
   ```

4. **Run the frontend:**
   ```bash
   cd ../frontend
   npm install && npm run dev
   ```

API → `http://localhost:8080` | UI → `http://localhost:3001`

---

<div align="center">
Built with determination by <a href="https://github.com/ShoaibsProject">Shoaib Akthar</a>
</div>