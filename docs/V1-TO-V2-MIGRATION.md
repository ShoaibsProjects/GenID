# V1 → V2 Migration Analysis

> **What can we take from Version 1? What needs to be rebuilt?**

---

## Executive Summary

| Category | V1 Status | V2 Reuse | Action |
|----------|-----------|----------|--------|
| **Backend Code** | 33,047 lines Go | ~40% reusable | Refactor & extend |
| **Database Schema** | 666 lines SQL | ~60% reusable | Extend with event sourcing |
| **Frontend** | 15 pages (Next.js 14) | ~30% reusable | Redesign completely |
| **Infrastructure** | Docker Compose | ~50% reusable | Add multi-region, K8s |
| **Policies** | 5 Cedar files | 100% reusable | Expand policy library |
| **Tests** | Unit + Integration | ~70% reusable | Expand coverage |

---

## V1 Component Inventory

### Backend (Go) — 33,047 lines

| Component | File | Lines | V2 Status | Notes |
|-----------|------|-------|-----------|-------|
| **Identity Service** | `service/identity_service.go` | 4,064 | 🔶 Refactor | Core logic reusable, needs event sourcing |
| **Workflows** | `workflow/workflows.go` | 887 | ✅ Reuse | Temporal workflows solid, extend for V2 |
| **Activities** | `activities/activities.go` | 1,314 | 🔶 Refactor | Needs outbox pattern integration |
| **Cedar Engine** | `cedar/engine.go` | 483 | ✅ Reuse | Production-ready, extend for policy simulation |
| **OIDC Provider** | `oidc/handlers.go` | 653 | ✅ Reuse | Full OAuth2/OIDC, extend for SAML |
| **OIDC Clients** | `oidc/clients.go` | ~200 | ✅ Reuse | PostgreSQL-backed, solid |
| **OIDC Provider Core** | `oidc/provider.go` | ~400 | ✅ Reuse | RSA keys, JWT RS256, JWKS |
| **Connector Manager** | `connector/manager.go` | 727 | 🔶 Refactor | Needs plugin architecture |
| **Connectors** | `connector/*.go` | ~2,000 | 🔶 Refactor | LDAP, SCIM, Entra, CSV — extend |
| **GraphQL** | `graphql/*.go` | ~1,500 | 🔶 Refactor | Needs full parity with REST |
| **Middleware** | `middleware/*.go` | ~800 | ✅ Reuse | Auth, rate limit, validation — solid |
| **Audit** | `audit/audit.go` | ~300 | 🔶 Refactor | Needs event sourcing, OpenTelemetry |
| **Vault** | `vault/vault.go` | ~400 | 🔶 Refactor | AES-256-GCM, needs HashiCorp integration |
| **AI Copilot** | `ai/copilot.go` | ~500 | 🔶 Rebuild | Stub only, needs real GraphRAG |
| **Domain Models** | `domain/identity.go` | ~200 | ✅ Reuse | Core types, extend |
| **Telemetry** | `telemetry/metrics.go` | ~300 | ✅ Reuse | Prometheus metrics, extend |
| **Protobuf** | `pkg/proto/**/*.go` | ~500 | ✅ Reuse | gRPC definitions |

**Total Backend:** ~33,000 lines  
**Reusable:** ~13,000 lines (40%)  
**Needs Refactor:** ~15,000 lines (45%)  
**Needs Rebuild:** ~5,000 lines (15%)

---

### Database Schema — 666 lines SQL

| Table | V1 Status | V2 Action | Notes |
|-------|-----------|-----------|-------|
| `tenants` | ✅ Complete | ✅ Reuse | Multi-tenant foundation |
| `identities` | ✅ Complete | 🔶 Extend | Add event sourcing columns |
| `non_human_identities` | ✅ Complete | ✅ Reuse | AI agents, service accounts |
| `roles` | ✅ Complete | 🔶 Extend | Add hierarchical roles |
| `identity_roles` | ✅ Complete | 🔶 Extend | Add temporal validity |
| `entitlements` | ✅ Complete | 🔶 Extend | Add risk scoring, JIT |
| `resources` | ✅ Complete | ✅ Reuse | Resource catalog |
| `cedar_policies` | ✅ Complete | 🔶 Extend | Add versioning, simulation |
| `audit_log` | ✅ Complete | 🔶 Rebuild | Event sourcing backbone |
| `connectors` | ✅ Complete | 🔶 Extend | Plugin architecture |
| `connector_identities` | ✅ Complete | ✅ Reuse | Synced identities |
| `connector_groups` | ✅ Complete | ✅ Reuse | Synced groups |
| `connector_entitlements` | ✅ Complete | ✅ Reuse | Synced entitlements |
| `oidc_clients` | ✅ Complete | ✅ Reuse | OAuth2 clients |
| `oidc_auth_codes` | ✅ Complete | ✅ Reuse | Authorization codes |
| `oidc_refresh_tokens` | ✅ Complete | ✅ Reuse | Refresh tokens |
| `oidc_device_codes` | ✅ Complete | ✅ Reuse | Device flow |
| `vault_secrets` | ✅ Complete | 🔶 Extend | Add key rotation |
| `certification_campaigns` | ✅ Complete | 🔶 Extend | AI-driven reviews |
| `sod_policies` | ✅ Complete | ✅ Reuse | SoD detection |
| `outbox_events` | ❌ Missing | 🆕 Add | Outbox pattern (researched) |

**V2 Schema Additions Needed:**
- `events` (event sourcing backbone)
- `event_store` (append-only log)
- `snapshots` (state materialization)
- `identity_sessions` (session management)
- `mfa_devices` (passwordless)
- `webauthn_credentials` (FIDO2)
- `access_requests` (JIT workflow)
- `policy_versions` (GitOps)
- `risk_events` (continuous auth)
- `agent_cards` (AI agent identity)
- `delegation_chains` (AI agent delegation)

---

### Frontend (Next.js 14) — 15 Pages

| Page | V1 Status | V2 Action | Notes |
|------|-----------|-----------|-------|
| `dashboard` | ✅ Complete | 🔶 Redesign | Linear/Vercel aesthetic |
| `identities` | ✅ Complete | 🔶 Redesign | Graph visualization |
| `groups` | ✅ Complete | 🔶 Redesign | Entitlement graph |
| `access` | ✅ Complete | 🔶 Redesign | Real-time graph |
| `policies` | ✅ Complete | 🔶 Redesign | Policy simulation UI |
| `audit` | ✅ Complete | 🔶 Redesign | Trace visualization |
| `connectors` | ✅ Complete | 🔶 Redesign | Plugin marketplace |
| `certifications` | ✅ Complete | 🔶 Redesign | AI-driven reviews |
| `sod` | ✅ Complete | 🔶 Redesign | SoD remediation |
| `vault` | ✅ Complete | 🔶 Redesign | Secrets management |
| `agents` | ✅ Complete | 🔶 Redesign | AI agent lifecycle |
| `csv` | ✅ Complete | ❌ Remove | Merge into connectors |
| `idp` | ✅ Complete | 🔶 Redesign | OIDC/OAuth2 management |
| `settings` | ✅ Complete | 🔶 Redesign | Tenant configuration |
| `__tests__` | ✅ Complete | ✅ Reuse | E2E tests |

**V2 UI Philosophy:**
- Not traditional admin portal
- Feels like **Linear, Vercel, Notion, Stripe Dashboard, Cursor**
- Graph-first visualization
- Real-time updates (WebSocket)
- AI copilot integrated everywhere
- Dark mode by default
- Keyboard-first navigation

---

### Infrastructure

| Component | V1 Status | V2 Action | Notes |
|-----------|-----------|-----------|-------|
| **Docker Compose** | ✅ Complete | 🔶 Extend | Add Kafka, Elasticsearch |
| **PostgreSQL** | ✅ Complete | 🔶 Extend | Add logical replication |
| **Neo4j** | ✅ Complete | 🔶 Extend | Add clustering |
| **Redis** | ✅ Complete | ✅ Reuse | Add Redis Streams |
| **Temporal** | ✅ Complete | ✅ Reuse | Extend workflows |
| **Kubernetes** | ❌ Missing | 🆕 Add | Production deployment |
| **Terraform** | ❌ Missing | 🆕 Add | IaC for all infra |
| **Helm Charts** | ❌ Missing | 🆕 Add | K8s packaging |
| **CI/CD** | ✅ Complete | 🔶 Extend | Add multi-region deploy |
| **Monitoring** | ✅ Partial | 🔶 Extend | Add Grafana, Jaeger |

---

### Policies (Cedar) — 5 Files

| Policy | V1 Status | V2 Action | Notes |
|--------|-----------|-----------|-------|
| `rbac.cedar` | ✅ Complete | ✅ Reuse | Role-based access |
| `abac.cedar` | ✅ Complete | ✅ Reuse | Attribute-based access |
| `agent.cedar` | ✅ Complete | ✅ Reuse | AI agent policies |
| `sod_emergency.cedar` | ✅ Complete | ✅ Reuse | Emergency access |
| `identity.cedarschema` | ✅ Complete | 🔶 Extend | Add new identity types |

**V2 Policy Additions:**
- `continuous_auth.cedar` (risk-adaptive)
- `jit_access.cedar` (just-in-time)
- `delegation.cedar` (AI agent delegation)
- `data_classification.cedar` (data governance)
- `compliance.cedar` (SOC2, HIPAA, GDPR)
- `zero_trust.cedar` (network-level)

---

## What to Keep from V1

### ✅ Keep As-Is (No Changes)

1. **Cedar Engine** (`cedar/engine.go`)
   - Production-ready
   - cedar-go v1.8.0 integration
   - Hot reload, Prometheus metrics
   - 10 unit tests passing

2. **OIDC Provider** (`oidc/*.go`)
   - Full OAuth2/OIDC implementation
   - 10/10 E2E tests passing
   - RSA keys, JWT RS256, JWKS
   - Device authorization flow

3. **Temporal Workflows** (`workflow/workflows.go`)
   - Offboarding, onboarding, JIT access
   - Retry policies, compensating transactions
   - Solid foundation

4. **Middleware** (`middleware/*.go`)
   - Auth, rate limiting, validation
   - Workflow guards
   - Request validation

5. **Telemetry** (`telemetry/metrics.go`)
   - Prometheus metrics
   - OpenTelemetry integration
   - Extend for V2 metrics

6. **Protobuf Definitions** (`pkg/proto/**/*.go`)
   - gRPC service definitions
   - Identity event schemas
   - Extend for V2 events

---

### 🔶 Refactor & Extend

1. **Identity Service** (`service/identity_service.go`)
   - **Keep:** Core CRUD logic, Neo4j queries
   - **Add:** Event sourcing, outbox pattern, CQRS
   - **Refactor:** Break into smaller services (identity, role, entitlement)

2. **Activities** (`activities/activities.go`)
   - **Keep:** Temporal activity implementations
   - **Add:** Outbox integration, idempotency keys
   - **Refactor:** Extract Neo4j sync to outbox processor

3. **Connector Manager** (`connector/manager.go`)
   - **Keep:** Core connector logic
   - **Add:** Plugin architecture, marketplace
   - **Refactor:** Interface-based design for extensibility

4. **GraphQL** (`graphql/*.go`)
   - **Keep:** Schema definitions
   - **Add:** Full REST parity, subscriptions
   - **Refactor:** Add real-time updates

5. **Audit** (`audit/audit.go`)
   - **Keep:** Audit log structure
   - **Add:** Event sourcing, OpenTelemetry traces
   - **Refactor:** Replay capability

6. **Vault** (`vault/vault.go`)
   - **Keep:** AES-256-GCM encryption
   - **Add:** HashiCorp Vault integration, key rotation
   - **Refactor:** Secrets lifecycle management

---

### 🔶 Rebuild

1. **AI Copilot** (`ai/copilot.go`)
   - **Current:** Stub with 9 intent classifications
   - **V2:** Real GraphRAG with Qdrant, LLM integration
   - **Scope:** 3-4 weeks

2. **Frontend** (all pages)
   - **Current:** Functional but basic
   - **V2:** Linear/Vercel aesthetic, graph-first, real-time
   - **Scope:** 4-6 weeks

3. **Database Schema** (core tables)
   - **Current:** Relational + graph dual-write
   - **V2:** Event sourcing backbone, outbox pattern
   - **Scope:** 2-3 weeks

---

### ❌ Remove

1. **CSV Import Page** (`frontend/src/app/csv/`)
   - Merge into connectors
   - Not a standalone feature

2. **Dual-Write Pattern** (scattered in code)
   - Replace with outbox pattern
   - Remove direct Neo4j writes from service layer

---

## What's Missing in V1 (V2 Additions)

### 🔴 Critical Gaps

| Feature | V1 Status | V2 Priority | Effort |
|---------|-----------|-------------|--------|
| **Event Sourcing** | ❌ Missing | P0 | 2-3 weeks |
| **Outbox Pattern** | ❌ Missing | P0 | 1-2 weeks |
| **Passwordless/WebAuthn** | ❌ Missing | P0 | 2 weeks |
| **Continuous Authorization** | ❌ Missing | P0 | 3 weeks |
| **Multi-Region** | ❌ Missing | P1 | 4 weeks |
| **Kubernetes Deployment** | ❌ Missing | P1 | 2 weeks |
| **Real GraphRAG** | ❌ Missing | P1 | 3-4 weeks |
| **MCP Server** | ❌ Missing | P1 | 1 week |
| **Policy Simulation** | ❌ Missing | P2 | 2 weeks |
| **Identity Observability** | ❌ Missing | P2 | 2 weeks |

### 🟡 Partial Implementations

| Feature | V1 Status | V2 Action |
|---------|-----------|-----------|
| **SAML IdP** | ❌ Missing | Build from scratch |
| **Access Reviews** | ✅ Stub | Complete with AI |
| **SoD Detection** | ✅ Detection works | Add remediation UI |
| **Risk Engine** | ✅ Basic scoring | Add continuous auth |
| **JIT Access** | ✅ Workflow exists | Add request UI |
| **Delegated Admin** | ❌ Missing | Build hierarchy |

---

## V2 Architecture Decisions

### 1. Event Sourcing Backbone

**V1:** Direct writes to PostgreSQL + Neo4j (dual-write)  
**V2:** Event sourcing with outbox pattern

```
V1: Service → PostgreSQL + Neo4j (inconsistent)
V2: Service → Event Store → Read Models (PostgreSQL + Neo4j)
```

**Impact:**
- All V1 service methods need refactoring
- Add `events` table, event publisher, event handlers
- CQRS: separate read/write models

### 2. Identity Types

**V1:** Human, Service Account, AI Agent (basic)  
**V2:** Full identity spectrum

```
V2 Identity Types:
- Workforce (Human)
- Customer (CIAM)
- B2B Federation
- Workload (K8s, VM)
- Machine (IoT, Edge)
- AI Agent (MCP, A2A)
- Service Account
- API Key
- Robot (RPA)
```

**Impact:**
- Extend `identities` table
- Add identity-specific lifecycle workflows
- New Neo4j node types

### 3. Authorization Model

**V1:** RBAC + ABAC + Cedar (basic)  
**V2:** Continuous authorization + risk-adaptive

```
V2 Authorization:
- Static: RBAC, ABAC, ReBAC (Cedar)
- Dynamic: Risk scoring, behavioral analytics
- Continuous: Every API call evaluated
- Contextual: Device, location, time, behavior
```

**Impact:**
- Extend Cedar engine with risk functions
- Add real-time risk scoring
- New authorization middleware

### 4. Developer Platform

**V1:** REST API + GraphQL (partial)  
**V2:** Full developer experience

```
V2 Developer Platform:
- REST API (full coverage)
- GraphQL API (with subscriptions)
- SDK (Go, Python, TypeScript, Java)
- CLI
- Terraform Provider
- Kubernetes Operator
- VS Code Extension
- Plugin Marketplace
```

**Impact:**
- New packages for SDKs
- Terraform provider project
- K8s operator project
- Plugin architecture

---

## Migration Strategy

### Phase 1: Foundation (Weeks 1-4)
**Goal:** Event sourcing backbone, outbox pattern

**Keep from V1:**
- All existing functionality (don't break it)
- Cedar engine, OIDC provider, Temporal workflows

**Add:**
- `events` table (event store)
- Outbox processor
- Event publisher
- CQRS read models

**Refactor:**
- Identity service → event-sourced
- Activities → outbox integration

### Phase 2: Identity Expansion (Weeks 5-8)
**Goal:** Full identity type support

**Keep from V1:**
- Identity CRUD
- Neo4j graph queries

**Add:**
- Workload identity
- Machine identity
- AI agent identity (MCP)
- Identity lifecycle workflows

**Refactor:**
- Identity types → extensible
- Neo4j schema → new node types

### Phase 3: Authorization (Weeks 9-12)
**Goal:** Continuous authorization, risk-adaptive

**Keep from V1:**
- Cedar engine
- Policy evaluation

**Add:**
- Risk engine
- Continuous authorization
- Policy simulation
- Delegated administration

**Refactor:**
- Authorization middleware → risk-aware
- Cedar engine → risk functions

### Phase 4: Developer Platform (Weeks 13-16)
**Goal:** SDK, CLI, Terraform, K8s operator

**Keep from V1:**
- REST API
- GraphQL API

**Add:**
- SDK (Go, Python, TypeScript)
- CLI
- Terraform provider
- Kubernetes operator
- Plugin architecture

### Phase 5: UI Redesign (Weeks 17-20)
**Goal:** Linear/Vercel aesthetic, graph-first

**Keep from V1:**
- Page structure (routes)
- API integration

**Rebuild:**
- All UI components
- Graph visualization
- Real-time updates (WebSocket)
- AI copilot integration

### Phase 6: Observability & Scale (Weeks 21-24)
**Goal:** Identity traces, multi-region, performance

**Keep from V1:**
- Prometheus metrics
- OpenTelemetry

**Add:**
- Identity traces
- Multi-region deployment
- Performance optimization
- Load testing (100M identities)

---

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| **Breaking existing functionality** | High | Feature flags, gradual rollout |
| **Data migration complexity** | Medium | Dual-write during transition |
| **Performance regression** | Medium | Load testing at each phase |
| **Scope creep** | High | Strict phase boundaries |
| **Team bandwidth** | Medium | Prioritize P0 features |

---

## Success Metrics

| Metric | V1 Baseline | V2 Target |
|--------|-------------|-----------|
| **Identities** | 10K | 100M |
| **Auth decisions/sec** | 1K | 100K |
| **Latency (p99)** | 200ms | <50ms |
| **Availability** | 99.9% | 99.99% |
| **Test coverage** | 60% | 90% |
| **API coverage** | 70% | 100% |
| **Identity types** | 3 | 10 |
| **Connectors** | 5 | 50 |

---

**Last Updated:** 2026-07-22  
**Status:** Analysis Complete — Ready for V2 Implementation Planning
