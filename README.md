<picture>
  <source media="(prefers-color-scheme: dark)" srcset="media/banner.svg">
  <img alt="ObserveID Reimagined — Identity Fabric" src="media/banner.svg" width="100%">
</picture>

<br/>

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go&logoColor=white&labelColor=1C1C24)](https://go.dev)
[![Next.js](https://img.shields.io/badge/Next.js-14-000000?style=flat&logo=next.js&logoColor=white&labelColor=1C1C24)](https://nextjs.org)
[![PostgreSQL](https://img.shields.io/badge/Neon/PG-16-4169E1?style=flat&logo=postgresql&logoColor=white&labelColor=1C1C24)](https://neon.tech)
[![Neo4j](https://img.shields.io/badge/Neo4j-5-4581C3?style=flat&logo=neo4j&logoColor=white&labelColor=1C1C24)](https://neo4j.com)
[![Temporal](https://img.shields.io/badge/Temporal-1.28-101010?style=flat&logo=temporal&logoColor=white&labelColor=1C1C24)](https://temporal.io)
[![Redis](https://img.shields.io/badge/Upstash-Redis-DC382D?style=flat&logo=redis&logoColor=white&labelColor=1C1C24)](https://upstash.com)

[![CI](https://github.com/ShoaibsProjects/observeid/actions/workflows/ci.yml/badge.svg)](https://github.com/ShoaibsProjects/observeid/actions/workflows/ci.yml)
[![Deploy](https://github.com/ShoaibsProjects/observeid/actions/workflows/deploy.yml/badge.svg)](https://github.com/ShoaibsProjects/observeid/actions/workflows/deploy.yml)
[![License](https://img.shields.io/badge/License-MIT-8B5CF6?style=flat&labelColor=1C1C24)]()

<br/>

---

### Event-Driven · AI-Native · Real-Time Governance

**Unify human and non-human identity under a single policy engine, graph database, and durable workflow system.**

---

<br/>
</div>

## Overview

ObserveID is an identity governance and administration (IGA) platform built for the 2026 security landscape. It treats every workload, agent, API key, and human as a first-class identity — tracked in Neo4j, orchestrated via Temporal, and governed by policy-as-code.

<pre align="center">
  SCIM Provisioning  ·  RBAC/ABAC/ReBAC  ·  AI Agent Security
  Real-Time Audit    ·  Event-Driven Sync  ·  GraphRAG Copilot
</pre>

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     CLOUDFLARE EDGE                              │
│  ┌─────────────────┐  ┌──────────────┐  ┌──────────────────┐   │
│  │  Pages (Next.js) │  │  DNS / WAF   │  │  Workers (proxy) │   │
│  └────────┬────────┘  └──────────────┘  └────────┬─────────┘   │
└───────────┼───────────────────────────────────────┼─────────────┘
            │ HTTPS                                 │
            ▼                                       ▼
┌─────────────────────────────────────────────────────────────────┐
│                     FLY.IO (Go Binary)                           │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  gorilla/mux  ·  pgx  ·  neo4j-driver  ·  go-redis       │   │
│  │  Temporal SDK  ·  gqlgen  ·  zerolog  ·  OpenTelemetry   │   │
│  └──────────────────────┬───────────────────────────────────┘   │
└─────────────────────────┼───────────────────────────────────────┘
                          │
          ┌───────────────┼───────────────────┐
          ▼               ▼                   ▼
┌──────────────┐ ┌──────────────┐ ┌──────────────────┐
│ Neon Postgres │ │ Neo4j AuraDB │ │ Upstash Redis    │
│ (512MB free)  │ │ (50K nodes)  │ │ (10K cmds/day)   │
└──────────────┘ └──────────────┘ └──────────────────┘
          │
          ▼
┌──────────────────┐
│ Temporal Cloud    │
│ (10K execs/mo)   │
└──────────────────┘
```

**Local development** runs the full stack via Docker Compose (11 containers). **Production** uses managed free-tier services — zero infrastructure to maintain.

## Key Features

| Area | Capability | Status |
|------|-----------|--------|
| **Identity Engine** | SCIM 2.0 CRUD, bulk import, soft-delete, PG + Neo4j dual-write | Done |
| **Connector Framework** | Entra ID (delta sync), LDAP/AD, CSV, SCIM — schema discovery, health monitoring | Done |
| **Policy Engine** | Cedar ABAC/RBAC/ReBAC with CI validation | Done |
| **Temporal Workflows** | Onboard, Offboard (fan-out), Grant, Revoke, JIT, SoD, Anomaly | Done |
| **AI Copilot** | GraphRAG over Neo4j — natural language identity queries | Done |
| **CAEP Stream** | Real-time session revocation broadcast | Done |
| **Credential Vault** | AES-256-GCM encrypted vault with UI | Done |
| **Audit Logs** | Immutable trail with detail viewer | Done |
| **HTTP QUERY** | RFC 10008 read-only endpoints (access check, copilot, connectors) | Done |
| **Design System** | 11 shared UI components, dark industrial theme | Done |

## Tech Stack

```
Go 1.26 · TypeScript · Next.js 14 · TailwindCSS

Backend:   gorilla/mux · pgx/v5 · neo4j-go-driver · go-redis/v9
           Temporal SDK · gqlgen · zerolog · OpenTelemetry

Frontend:  Next.js 14 (static export) · Plus Jakarta Sans · JetBrains Mono

Data:      PostgreSQL (Neon) · Neo4j 5 (AuraDB) · Redis (Upstash)

Cloud:     Fly.io (backend) · Cloudflare Pages (frontend)
           Temporal Cloud · Neon · AuraDB · Upstash

Dev:       Docker Compose (11 containers) · GitHub Actions CI/CD
```

## Quick Start

```bash
# 1. Start infrastructure (PG, Neo4j, Kafka, Temporal, Redis, etc.)
make up

# 2. Run DB migrations and seed data
make dev-db

# 3. Build and run backend (serves API on :8080)
cd backend && go run ./cmd/identity-service/

# 4. In a separate terminal — frontend dev server
cd frontend && npm install && npm run dev
```

The backend serves the API at `http://localhost:8080`. The frontend dev server runs at `http://localhost:3001` and proxies API calls to the backend.

## Deployment

Production is deployed via GitHub Actions on push to `main`:

| Service | Platform | Trigger |
|---------|----------|---------|
| Go backend | Fly.io | Push to `main` (Docker build) |
| Next.js frontend | Cloudflare Pages | Push to `main` (static export) |

```bash
# One-time setup
fly launch --name observeid-api --region iad
fly secrets set DATABASE_URL=... NEO4J_URI=... REDIS_ADDR=... TEMPORAL_HOST=...

# Subsequent deploys happen automatically via CI/CD
git push origin main
```

## Workflows

| Workflow | Description | Priority |
|----------|-------------|----------|
| `OffboardIdentityWorkflow` | Complete offboarding with parallel fan-out, CAEP broadcast, cascade agent revocation | Critical |
| `OnboardIdentityWorkflow` | Identity creation with role assignment and optional approval gates | High |
| `GrantAccessWorkflow` | Access provisioning with approval workflow and JIT auto-expiry | High |
| `RevokeAccessWorkflow` | Emergency access revocation with cache invalidation | Critical |
| `JustInTimeAccessWorkflow` | Time-bounded access with automatic expiration | Medium |
| `AgentAnomalyDetectionWorkflow` | Cron-based AI agent behavioral analysis | Medium |
| `DetectSoDViolationsWorkflow` | SoD violation scanning via Neo4j graph traversal | Medium |
| `CascadeRevokeWorkflow` | Cascade revocation across dependent access | Medium |

## API Endpoints

```
Health & Observability
  GET    /health                  Liveness probe
  GET    /healthz                 Full dependency check (PG + Neo4j + Redis + Temporal)
  GET    /ready                   Readiness probe
  GET    /metrics                 Prometheus metrics

SCIM 2.0
  GET    /scim/v2/Users           List users
  POST   /scim/v2/Users           Create user (triggers onboarding)
  GET    /scim/v2/Users/{id}      Get user
  PUT    /scim/v2/Users/{id}      Update user
  PATCH  /scim/v2/Users/{id}      Patch user
  DELETE /scim/v2/Users/{id}      Delete user (triggers offboarding)

Identity API
  GET    /api/v1/identities              List identities (Neo4j)
  POST   /api/v1/identities              Create identity (PG + Neo4j)
  GET    /api/v1/identities/{id}         Get identity with relationships
  PATCH  /api/v1/identities/{id}         Update identity
  DELETE /api/v1/identities/{id}         Soft-delete
  POST   /api/v1/identities/bulk         Bulk import with upsert
  GET    /api/v1/identities/{id}/entitlements    Access paths
  GET    /api/v1/identities/{id}/blast-radius    Blast radius analysis

Access Control
  QUERY  /api/v1/access/check            Real-time access evaluation
  POST   /api/v1/access/grant            Grant access (Temporal workflow)
  POST   /api/v1/access/revoke           Revoke access (Temporal workflow)
  POST   /api/v1/access/jit              Just-in-time access

Agent / NHI
  GET    /api/v1/agents                  List agents
  POST   /api/v1/agents                  Register agent
  GET    /api/v1/agents/{id}             Get agent
  POST   /api/v1/agents/{id}/kill-switch Emergency kill
  POST   /api/v1/agents/{id}/delegate    Delegate agent
  GET    /api/v1/agents/{id}/card        Agent card (A2A)

Connector API
  GET    /api/v1/connectors              List connectors
  POST   /api/v1/connectors              Create connector
  QUERY  /api/v1/connectors/test         Test connection
  GET    /api/v1/connectors/{id}         Get connector
  DELETE /api/v1/connectors/{id}         Delete connector
  POST   /api/v1/connectors/{id}/connect     Connect
  POST   /api/v1/connectors/{id}/disconnect  Disconnect
  QUERY  /api/v1/connectors/{id}/test        Test existing connector
  POST   /api/v1/connectors/{id}/sync        Full sync
  POST   /api/v1/connectors/{id}/sync-delta  Delta sync
  GET    /api/v1/connectors/{id}/schema      Schema discovery
  GET    /api/v1/connectors/{id}/health      Health monitoring
  POST   /api/v1/connectors/csv/upload       CSV upload

AI Copilot
  QUERY  /api/v1/copilot/query           Natural language identity query (GraphRAG)

CAEP
  GET    /api/v1/caep/events             List CAEP events
  POST   /api/v1/caep/broadcast          Broadcast session-revoked event

Vault
  GET    /api/v1/vault/secrets           List secrets
  POST   /api/v1/vault/secrets           Store secret (AES-256-GCM)
  GET    /api/v1/vault/secrets/{id}      Retrieve secret
  DELETE /api/v1/vault/secrets/{id}      Delete secret

Groups & Roles
  GET    /api/v1/groups                  List groups
  POST   /api/v1/groups                  Create group
  DELETE /api/v1/groups/{id}             Delete group
  POST   /api/v1/roles/assign            Assign role
  POST   /api/v1/roles/remove            Remove role

Audit
  GET    /api/v1/audit/logs              List audit logs
  GET    /api/v1/audit/logs/{id}         Get audit log detail
  GET    /api/v1/audit/stats             Audit statistics

GraphQL
  POST   /graphql                        GraphQL API (gqlgen)
```

## Project Structure

```
observeid/
├── proto/                        # Protobuf definitions
│   ├── event/v1/                 # Identity events
│   └── model/v1/                 # Data models
├── backend/
│   ├── cmd/identity-service/     # Entry point (main.go)
│   ├── internal/
│   │   ├── service/              # HTTP handlers + business logic
│   │   ├── connector/            # IGA connector framework
│   │   ├── workflow/             # Temporal workflow definitions
│   │   ├── activities/           # Temporal activity definitions
│   │   ├── domain/               # Core domain types
│   │   ├── vault/                # AES-256-GCM encrypted vault
│   │   ├── audit/                # Immutable audit logging
│   │   ├── graph/                # Neo4j query patterns
│   │   ├── ai/                   # GraphRAG copilot
│   │   ├── graphql/              # gqlgen resolvers
│   │   └── middleware/           # Auth, CORS, rate limit, validation
│   └── pkg/proto/                # Generated protobuf code
├── frontend/
│   ├── src/app/                  # Next.js App Router pages
│   ├── src/components/ui/        # Shared design system (11 components)
│   └── src/lib/                  # API client, utilities
├── infrastructure/               # Docker Compose + DB init scripts
├── deploy/
│   ├── terraform/                # Neon + Upstash provisioning
│   └── k8s/                      # Kubernetes manifests
├── docker/                       # Dockerfiles (backend + frontend)
├── .github/workflows/            # CI/CD (test, deploy, docker-publish)
├── fly.toml                      # Fly.io deployment config
└── Makefile                      # Build system
```

## Environment Variables

All configuration is via environment variables with local docker-compose fallbacks. See [`backend/.env.example`](backend/.env.example) for the full template.

| Variable | Description | Local Default | Cloud |
|----------|-------------|---------------|-------|
| `DATABASE_URL` | PostgreSQL connection string | `postgresql://observeid:observeid@localhost:5432/observeid?sslmode=disable` | Neon (`sslmode=require`) |
| `NEO4J_URI` | Neo4j Bolt URI | `bolt://localhost:7687` | `neo4j+s://xxx.databases.neo4j.io` |
| `NEO4J_USER` | Neo4j username | `neo4j` | `neo4j` |
| `NEO4J_PASSWORD` | Neo4j password | (local) | AuraDB password |
| `REDIS_ADDR` | Redis endpoint | `localhost:6379` | Upstash endpoint |
| `REDIS_PASSWORD` | Redis password | (empty) | Upstash token |
| `REDIS_TLS` | Enable TLS for Redis | `false` | `true` |
| `TEMPORAL_HOST` | Temporal frontend | `localhost:7233` | Temporal Cloud |
| `TEMPORAL_NAMESPACE` | Temporal namespace | `critical-offboarding` | Your namespace |
| `CORS_ORIGIN` | Allowed CORS origin | (empty) | `https://observeid.pages.dev` |
| `VAULT_MASTER_KEY` | AES-256 key (hex) | (generate with `openssl rand -hex 32`) | Fly.io secret |

## License

```
MIT License — ObserveID Reimagined, Inc.
Copyright 2026 ObserveID Reimagined
```
