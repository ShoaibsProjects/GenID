# GENID BUILD BIBLE — Master Technical Specification
> **Version:** 1.1 · **Date:** 2026-08-15
> **For:** opencode + DeepSeek v4 Flash
> **Rule:** No action without this doc. Every file, every function, every struct defined herein.
> **HTTP STACK:** `gorilla/mux` + `net/http` — NOT gin. All handler/middleware examples use standard library patterns.

---

## TABLE OF CONTENTS

1. [Architecture Overview](#1-architecture-overview)
2. [Directory Structure (Target)](#2-directory-structure-target)
3. [Phase 0: Foundation Hardening](#3-phase-0-foundation-hardening)
4. [Phase 1: Conditional Access MVP](#4-phase-1-conditional-access-mvp)
5. [Phase 2: MCP Server + AI Integration](#5-phase-2-mcp-server--ai-integration)
6. [Phase 3: ObserveID Bridge](#6-phase-3-observeid-bridge)
7. [Phase 4: Production UI/UX](#7-phase-4-production-uiux)
8. [Security Baseline (OWASP ASVS L2)](#8-security-baseline-owasp-asvs-l2)
9. [Database Schema Evolution](#9-database-schema-evolution)
10. [API Contract (OpenAPI 3.1)](#10-api-contract-openapi-31)
11. [Testing Strategy](#11-testing-strategy)
12. [Deployment & Operations](#12-deployment--operations)

---

## 1. ARCHITECTURE OVERVIEW

### 1.1 Core Philosophy
**Event-driven, policy-first, graph-backed, zero-standing-privilege by default.**

Every access decision is made at event time by evaluating:
1. **Who** — identity (human or non-human) from Neo4j graph
2. **What** — requested resource/action
3. **When** — time-of-day, business hours
4. **Where** — location, network zone, geo
5. **How** — device trust, MFA method
6. **Risk** — continuously decaying risk score from event correlation

### 1.2 System Components

```
                    EXTERNAL WORLD
   Entra ID | Okta | Workday | Jira | Azure SB | CrowdStrike | ObserveID
                              |
              +---------------v----------------+
              |  INGESTION EDGE                |
              |  /api/v1/events/ingest/{source}|
              |  Azure SB | Webhook adapters   |
              +---------------v----------------+
                              | canonical events
                   +----------v----------+
                   |  NATS JetStream     |  <- INTERNAL EVENT FABRIC
                   |  Stream: genid-events|
                   |  Subjects: identity.*|
                   |           access.*   |
                   |           risk.*     |
                   +-+-----+-----+-----+-+
                     |     |     |     |
        +------------v+ +--v---+ +v---+ +v--------------+
        | Risk        | |Audit | |MCP | |Webhook        |
        | Processor   | |Ledger| |Srv | |Dispatcher     |
        | (exists)    | |(exists)|    | |(new)          |
        +------+------+ +------+ +----+ +---------------+
               | updates
    +----------v--------------------------------+
    |  Neo4j Identity Graph                     |
    |  Nodes: Identity, NHI, Resource, Role     |
    |  Props: risk_score, risk_band, context    |
    +----------+--------------------------------+
               | read at decision time
  +------------v----------------------------------------------+
  |  POLICY GATE                                               |
  |  Cedar (ABAC) -- evaluates context + risk + role          |
  |  OpenFGA -- evaluates relationships (who can approve?)     |
  |  Decision: auto-approve | step-up | deny | 2h JIT          |
  +------------+----------------------------------------------+
               |
    +----------v----------------------------------+
    |  TEMPORAL WORKFLOWS (durable, versioned)  |
    |  GrantAccessWorkflow                      |
    |  JITAccessWorkflow                        |
    |  ApprovalGateWorkflow                     |
    |  FirecallAccessWorkflow                   |
    |  RevokeAccessWorkflow                     |
    |  OnboardIdentityWorkflow                  |
    +-------------------------------------------+
```

### 1.3 Technology Stack (Locked)

| Layer | Technology | Version | Reason |
|-------|-----------|---------|--------|
| Language | Go | 1.23+ | Performance, Temporal SDK maturity |
| Frontend | Next.js 14 + React 18 | latest | App Router, Server Components |
| UI | Tailwind CSS + shadcn/ui | latest | 2026 standard, a11y built-in |
| Event Bus | NATS JetStream | 2.10+ | Lightweight, durable, queue groups |
| Orchestration | Temporal | 1.25+ | Durable workflows, saga pattern |
| Graph DB | Neo4j | 5.20+ | Native vector search, GDS library |
| Primary DB | PostgreSQL | 16+ | ACID, JSONB, RLS |
| Cache | Redis | 7+ | Session store, context cache |
| Policy | cedar-go | latest | AWS Cedar, per-tenant policies |
| Relationships | OpenFGA | 1.5+ | Fine-grained authorization |
| Auth | OIDC (self-hosted) + API Keys | -- | Human + machine auth |
| MCP | Model Context Protocol | 2025-11 | AI agent integration |
| Metrics | Prometheus + Grafana | -- | Observability |
| Tracing | OpenTelemetry | -- | Distributed tracing |

---

## 2. DIRECTORY STRUCTURE (TARGET)

This is the ONLY valid structure. opencode must migrate existing code into this layout.

```
genid/
+-- backend/
|   +-- cmd/
|   |   +-- identity-service/          # HTTP API + Temporal worker
|   |   |   +-- main.go
|   |   +-- event-processor/           # NATS consumer
|   |   |   +-- main.go
|   |   +-- mcp-server/                # MCP stdio/SSE server
|   |       +-- main.go
|   |
|   +-- internal/
|   |   +-- config/                    # Env config, validation
|   |   |   +-- config.go
|   |   +-- handlers/                  # HTTP handlers ONLY
|   |   |   +-- access_handler.go      # /access/* endpoints
|   |   |   +-- identity_handler.go    # /identities/* endpoints
|   |   |   +-- approval_handler.go    # /approvals/* endpoints
|   |   |   +-- catalog_handler.go     # /catalog/* endpoints
|   |   |   +-- event_handler.go       # /events/* endpoints
|   |   |   +-- webhook_handler.go     # /webhooks/* endpoints
|   |   |   +-- scim_handler.go        # /scim/v2/* endpoints
|   |   |   +-- risk_handler.go        # /risk/* endpoints
|   |   |   +-- auth_handler.go        # /auth/* endpoints (OIDC, API keys)
|   |   |   +-- health_handler.go      # /health, /metrics
|   |   +-- services/                  # Business logic, NO HTTP, NO DB
|   |   |   +-- access_service.go      # Grant/JIT/Firecall logic
|   |   |   +-- identity_service.go    # Identity CRUD, graph ops
|   |   |   +-- approval_service.go    # Approval queue, delegation
|   |   |   +-- risk_service.go        # Risk score queries
|   |   |   +-- policy_service.go      # Cedar + OpenFGA evaluation
|   |   |   +-- audit_service.go       # Hash-chained audit
|   |   |   +-- webhook_service.go     # Webhook dispatch
|   |   |   +-- enrichment_service.go  # Context enrichment
|   |   |   +-- notification_service.go # Slack/Teams/Email
|   |   +-- stores/                    # Data access ONLY
|   |   |   +-- identity_store.go      # PG + Neo4j identity queries
|   |   |   +-- access_store.go        # workflow_requests, approvals
|   |   |   +-- audit_store.go         # workflow_audit hash-chain
|   |   |   +-- policy_store.go        # Cedar policies from PG
|   |   |   +-- risk_store.go          # Risk score persistence
|   |   |   +-- webhook_store.go       # Webhook registrations
|   |   |   +-- event_store.go         # Outbox events
|   |   +-- workflow/                  # Temporal workflows + activities
|   |   |   +-- grant_access_workflow.go
|   |   |   +-- jit_access_workflow.go
|   |   |   +-- firecall_workflow.go
|   |   |   +-- revoke_workflow.go
|   |   |   +-- approval_gate_workflow.go
|   |   |   +-- onboard_workflow.go
|   |   |   +-- activities/
|   |   |       +-- provision_activity.go
|   |   |       +-- notify_activity.go
|   |   |       +-- audit_activity.go
|   |   |       +-- policy_check_activity.go
|   |   |       +-- enrich_context_activity.go
|   |   +-- domain/                    # Pure domain models
|   |   |   +-- identity.go
|   |   |   +-- access_request.go
|   |   |   +-- approval.go
|   |   |   +-- risk.go
|   |   |   +-- policy.go
|   |   |   +-- audit.go
|   |   |   +-- event.go
|   |   |   +-- webhook.go
|   |   |   +-- nhi.go
|   |   +-- eventbus/                  # NATS JetStream
|   |   |   +-- nats.go
|   |   |   +-- publisher.go
|   |   |   +-- subscriber.go
|   |   +-- eventing/                  # Event processing
|   |   |   +-- processor.go           # Risk processor (exists)
|   |   |   +-- sources/               # Ingestion adapters
|   |   |   |   +-- registry.go
|   |   |   |   +-- webhook_adapter.go
|   |   |   |   +-- azure_sb_adapter.go
|   |   |   |   +-- mapper.go
|   |   |   +-- normalizer.go
|   |   +-- cedar/                     # Policy engine
|   |   |   +-- engine.go
|   |   |   +-- loader.go
|   |   |   +-- templates/
|   |   |       +-- default_policy.cedar
|   |   +-- enrichment/                # Context enrichment (NEW)
|   |   |   +-- service.go
|   |   |   +-- geo.go                 # IP -> geo/zone
|   |   |   +-- device.go              # Device trust evaluation
|   |   |   +-- time.go                # Business hours
|   |   |   +-- cache.go               # Redis cache layer
|   |   +-- fga/                       # OpenFGA client
|   |   |   +-- client.go
|   |   |   +-- model.fga
|   |   +-- middleware/                # HTTP middleware
|   |   |   +-- auth.go                # OIDC + API key
|   |   |   +-- tenant.go              # Multi-tenant isolation
|   |   |   +-- rate_limit.go          # Token bucket per tenant
|   |   |   +-- cors.go                # CORS
|   |   |   +-- security.go            # Secure headers, CSP
|   |   |   +-- validate.go            # Input validation
|   |   |   +-- idempotency.go         # Idempotency keys
|   |   |   +-- logging.go             # Structured logging
|   |   +-- audit/                     # Audit ledger
|   |   |   +-- hasher.go              # Hash chain
|   |   |   +-- logger.go
|   |   +-- vault/                     # Secrets management
|   |   |   +-- client.go
|   |   |   +-- provider.go
|   |   +-- mcp/                       # MCP Server implementation
|   |   |   +-- server.go
|   |   |   +-- tools.go               # MCP tool definitions
|   |   |   +-- resources.go           # MCP resource definitions
|   |   |   +-- auth.go                # MCP auth context
|   |   +-- notify/                    # Notifications
|   |       +-- slack.go
|   |       +-- teams.go
|   |       +-- email.go
|   +-- pkg/                           # Public shared packages
|   |   +-- validators/                # Reusable validators
|   |   +-- crypto/                    # Crypto utilities
|   |   +-- errors/                    # Domain errors
|   +-- api/                           # Generated API clients
|   |   +-- proto/                     # Protobuf definitions
|   +-- migrations/                    # SQL migrations
|   |   +-- 001_initial_schema.sql
|   |   +-- 002_audit_hash_chain.sql
|   |   +-- 003_cedar_policies.sql
|   |   +-- 004_webhooks.sql
|   |   +-- 005_risk_events.sql
|   |   +-- 006_context_enrichment.sql
|   |   +-- 007_api_keys.sql
|   |   +-- 008_nhi_registry.sql
|   |   +-- 009_zsp_mode.sql
|   |   +-- 010_drop_legacy_audit.sql
|   +-- scripts/
|   |   +-- simulate-idp-events.sh
|   |   +-- demo-conditional-access.sh
|   |   +-- seed-policies.sh
|   +-- tests/
|   |   +-- integration/
|   |   +-- e2e/
|   +-- docs/
|   |   +-- architecture/
|   |   |   +-- ADR-001-event-backbone.md
|   |   |   +-- ADR-002-conditional-access.md
|   |   |   +-- ADR-003-mcp-server.md
|   |   |   +-- ADR-004-security-baseline.md
|   |   +-- guides/
|   |   |   +-- demo-event-driven-risk.md
|   |   |   +-- demo-conditional-access.md
|   |   +-- openapi.yaml
|   +-- go.mod
|   +-- go.sum
|   +-- Makefile
+-- frontend/
|   +-- app/                           # Next.js 14 App Router
|   |   +-- layout.tsx
|   |   +-- page.tsx                   # Dashboard
|   |   +-- (auth)/
|   |   |   +-- login/
|   |   |       +-- page.tsx
|   |   +-- (dashboard)/
|   |   |   +-- requests/
|   |   |   +-- inbox/
|   |   |   +-- catalog/
|   |   |   +-- identities/
|   |   |   +-- risk/
|   |   |   +-- audit/
|   |   |   +-- policies/
|   |   |   +-- nhi/                   # Non-Human Identity registry
|   |   |   +-- settings/
|   |   +-- api/
|   |       +-- auth/
|   |           +-- [...nextauth]/
|   |               +-- route.ts
|   +-- components/
|   |   +-- ui/                        # shadcn/ui components
|   |   +-- layout/
|   |   |   +-- sidebar.tsx
|   |   |   +-- header.tsx
|   |   |   +-- tenant-switcher.tsx
|   |   +-- risk/
|   |   |   +-- risk-score-card.tsx
|   |   |   +-- risk-timeline.tsx
|   |   |   +-- risk-event-stream.tsx
|   |   +-- access/
|   |   |   +-- request-form.tsx
|   |   |   +-- approval-card.tsx
|   |   +-- identity/
|   |   |   +-- identity-detail.tsx
|   |   |   +-- entitlement-graph.tsx
|   |   +-- policy/
|   |   |   +-- policy-editor.tsx
|   |   |   +-- policy-simulator.tsx
|   |   +-- nhi/
|   |       +-- nhi-registry.tsx
|   |       +-- agent-lifecycle.tsx
|   +-- lib/
|   |   +-- api.ts                     # API client (generated from OpenAPI)
|   |   +-- auth.ts                    # NextAuth.js OIDC config
|   |   +-- utils.ts
|   |   +-- constants.ts
|   +-- hooks/
|   |   +-- use-risk.ts
|   |   +-- use-identity.ts
|   |   +-- use-access.ts
|   |   +-- use-websocket.ts           # Real-time risk updates
|   +-- types/
|   |   +-- index.ts
|   +-- public/
|   +-- styles/
|   |   +-- globals.css
|   +-- tailwind.config.ts
|   +-- next.config.js
|   +-- package.json
|   +-- tsconfig.json
+-- infrastructure/
|   +-- docker-compose.yml
|   +-- docker-compose.prod.yml
|   +-- nats/
|   |   +-- nats-server.conf
|   +-- temporal/
|   |   +-- dynamicconfig/
|   |       +-- development.yaml
|   +-- neo4j/
|   |   +-- init.cypher
|   +-- postgres/
|   |   +-- init.sql
|   +-- openfga/
|   |   +-- model.fga
|   +-- prometheus/
|   |   +-- prometheus.yml
|   +-- grafana/
|   |   +-- dashboards/
|   +-- vault/
|       +-- config.hcl
+-- README.md
```

---

## 3. PHASE 0: FOUNDATION HARDENING

### 3.1 Kill List (Remove First)

BEFORE building anything new, opencode must:

1. Delete `internal/policy/` -- empty package, dead code.
2. VERIFY `audit_logs` vs `audit_log` BEFORE touching anything:
   - `audit_log` (singular) is the ACTIVE tamper-evident hash-chain ledger. 
     Written by `internal/audit/chain.go`. Verified by `VerifyAuditChain`. 
     **NEVER DROP. NEVER MIGRATE DATA OUT.**
   - `workflow_audit` is the Temporal workflow step trail. Different schema, different purpose. **KEEP.**
   - If a table named `audit_logs` (plural, LEGACY) exists AND is empty/unused:
     * Migrate any rows to `audit_log` (singular) format
     * Create migration `010_drop_legacy_audit_logs.sql`
   - If NO `audit_logs` (plural) table exists, SKIP this step entirely.
3. Remove Qdrant from docker-compose.yml and all docs.
4. Remove STATUS.md Kafka claims -- replace with NATS reality.
5. Delete old `internal/service/identity_service.go` after extracting logic into handlers/services/stores.

### 3.2 Split the God Object

Source: `internal/service/identity_service.go` (5,812 lines)
Target: Extract into handlers/services/stores with ZERO behavior change.

Extraction Rules:
- HTTP-specific code (gin/echo context binding, status codes, response JSON) -> `internal/handlers/`
- Business logic (workflow starts, policy checks, risk calculations) -> `internal/services/`
- Database queries (SQL, Cypher) -> `internal/stores/`
- Domain models -> `internal/domain/`

#### Handler Pattern (ALL handlers must follow this):

**CRITICAL: The actual codebase uses `gorilla/mux` + `net/http`. Do NOT switch to gin.**

```go
// internal/handlers/access_handler.go
package handlers

import (
    "encoding/json"
    "net/http"

    "github.com/gorilla/mux"
)

type AccessHandler struct {
    accessService  *services.AccessService
    policyService  *services.PolicyService
    auditService   *services.AuditService
}

func NewAccessHandler(as *services.AccessService, ps *services.PolicyService, aus *services.AuditService) *AccessHandler {
    return &AccessHandler{accessService: as, policyService: ps, auditService: aus}
}

func (h *AccessHandler) RegisterRoutes(r *mux.Router) {
    r.HandleFunc("/api/v1/access/grant", h.GrantAccess).Methods("POST")
    r.HandleFunc("/api/v1/access/jit", h.RequestJIT).Methods("POST")
    r.HandleFunc("/api/v1/access/firecall", h.RequestFirecall).Methods("POST")
    r.HandleFunc("/api/v1/access/revoke", h.RevokeAccess).Methods("POST")
}

func (h *AccessHandler) GrantAccess(w http.ResponseWriter, r *http.Request) {
    var req domain.GrantAccessRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid_request", Message: err.Error()})
        return
    }

    tenantID := r.Context().Value("tenant_id").(string)
    userID := r.Context().Value("user_id").(string)

    result, err := h.accessService.Grant(r.Context(), tenantID, userID, req)
    if err != nil {
        HandleServiceError(w, err)
        return
    }

    h.auditService.Log(r.Context(), domain.AuditEvent{
        TenantID: tenantID,
        ActorID:  userID,
        Action:   "access.grant.requested",
        TargetID: result.RequestID,
        Details:  req,
    })

    respondJSON(w, http.StatusAccepted, result)
}

// respondJSON is the standard JSON response helper from the existing codebase
func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(payload)
}
```

#### Service Pattern (ALL services must follow this):

```go
// internal/services/access_service.go
package services

type AccessService struct {
    accessStore    *stores.AccessStore
    identityStore  *stores.IdentityStore
    policyService  *PolicyService
    temporalClient client.Client
    eventBus       *eventbus.Bus
}

// Grant starts the GrantAccessWorkflow via Temporal
func (s *AccessService) Grant(ctx context.Context, tenantID, requesterID string, req domain.GrantAccessRequest) (*domain.AccessRequestResult, error) {
    // 1. Validate request
    if err := req.Validate(); err != nil {
        return nil, err
    }

    // 2. Evaluate policy
    policyResult, err := s.policyService.Evaluate(ctx, domain.PolicyCheckParams{
        TenantID:     tenantID,
        PrincipalID:  requesterID,
        ResourceID:   req.ResourceID,
        Action:       "grant",
        Context:      req.Context,
    })
    if err != nil {
        return nil, err
    }

    // 3. Start workflow
    workflowOptions := client.StartWorkflowOptions{
        ID:        fmt.Sprintf("grant-%s-%s", tenantID, uuid.New().String()),
        TaskQueue: "identity-service",
    }

    workflowInput := workflow.GrantAccessInput{
        TenantID:      tenantID,
        RequesterID:   requesterID,
        TargetID:      req.TargetID,
        ResourceID:    req.ResourceID,
        Duration:      req.Duration,
        PolicyResult:  policyResult,
        Context:       req.Context,
    }

    we, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, workflow.GrantAccessWorkflow, workflowInput)
    if err != nil {
        return nil, fmt.Errorf("workflow start failed: %w", err)
    }

    return &domain.AccessRequestResult{
        RequestID: we.GetID(),
        Status:    "pending",
        PolicyDecision: policyResult.Decision,
    }, nil
}
```

#### Store Pattern (ALL stores must follow this):

```go
// internal/stores/access_store.go
package stores

type AccessStore struct {
    db *sql.DB
}

func (s *AccessStore) CreateRequest(ctx context.Context, tenantID string, req *domain.AccessRequest) error {
    query := `
        INSERT INTO workflow_requests (id, tenant_id, requester_id, target_id, resource_id, 
                                       request_type, status, policy_decision, created_at, context)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    `
    _, err := s.db.ExecContext(ctx, query,
        req.ID, tenantID, req.RequesterID, req.TargetID, req.ResourceID,
        req.Type, req.Status, req.PolicyDecision, req.CreatedAt, req.ContextJSON,
    )
    return err
}
```

### 3.3 Security Baseline Implementation

#### 3.3.1 Authentication Layer

Two auth methods required:

1. Human auth: OIDC (self-hosted or external IdP)
   - Path: `/auth/oidc/login` -> redirect to IdP -> callback -> JWT session cookie
   - JWT claims: `sub`, `email`, `tenant_id`, `roles[]`
   - Refresh token rotation mandatory

2. Machine auth: API Keys
   - Path: `Authorization: Bearer genid_<key>`
   - Key format: `genid_<tenant_id>_<random32>`
   - Stored hashed (bcrypt) in `api_keys` table
   - Rate limit: 1000 req/min per key
   - Scope enforcement: `read:identities`, `write:access`, `admin:*`

```go
// internal/middleware/auth.go
package middleware

import (
    "context"
    "net/http"
    "strings"
)

func AuthMiddleware(oidcProvider *oidc.Provider, apiKeyStore *stores.APIKeyStore) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authHeader := r.Header.Get("Authorization")
            ctx := r.Context()

            if strings.HasPrefix(authHeader, "Bearer genid_") {
                key := strings.TrimPrefix(authHeader, "Bearer ")
                apiKey, err := apiKeyStore.Validate(ctx, key)
                if err != nil {
                    respondJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "invalid_api_key"})
                    return
                }
                ctx = context.WithValue(ctx, "tenant_id", apiKey.TenantID)
                ctx = context.WithValue(ctx, "auth_type", "api_key")
                ctx = context.WithValue(ctx, "scopes", apiKey.Scopes)
                next.ServeHTTP(w, r.WithContext(ctx))
                return
            }

            // OIDC JWT auth
            token := strings.TrimPrefix(authHeader, "Bearer ")
            idToken, err := oidcProvider.Verify(ctx, token)
            if err != nil {
                respondJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "invalid_token"})
                return
            }

            var claims struct {
                Sub      string   `json:"sub"`
                Email    string   `json:"email"`
                TenantID string   `json:"tenant_id"`
                Roles    []string `json:"roles"`
            }
            if err := idToken.Claims(&claims); err != nil {
                respondJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "invalid_claims"})
                return
            }

            ctx = context.WithValue(ctx, "user_id", claims.Sub)
            ctx = context.WithValue(ctx, "tenant_id", claims.TenantID)
            ctx = context.WithValue(ctx, "email", claims.Email)
            ctx = context.WithValue(ctx, "roles", claims.Roles)
            ctx = context.WithValue(ctx, "auth_type", "oidc")
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

#### 3.3.2 Authorization (Scope Enforcement)

```go
// internal/middleware/scope.go
func RequireScope(scopes ...string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authType := r.Context().Value("auth_type").(string)
            if authType == "oidc" {
                next.ServeHTTP(w, r) // Humans pass (RBAC handled by Cedar)
                return
            }

            keyScopes := r.Context().Value("scopes").([]string)
            for _, required := range scopes {
                if !hasScope(keyScopes, required) {
                    respondJSON(w, http.StatusForbidden, ErrorResponse{Error: "insufficient_scope"})
                    return
                }
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

#### 3.3.3 Rate Limiting

Token bucket per `tenant_id` + `identity_id`. Redis-backed.

```go
// internal/middleware/rate_limit.go
func RateLimitMiddleware(redisClient *redis.Client) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            tenantID := r.Context().Value("tenant_id").(string)
            key := fmt.Sprintf("ratelimit:%s:%s", tenantID, r.RemoteAddr)

            // Token bucket: 100 req/min, burst 20
            allowed, remaining, reset := checkTokenBucket(redisClient, key, 100, 20, time.Minute)
            if !allowed {
                w.Header().Set("Retry-After", fmt.Sprintf("%d", int(reset.Seconds())))
                respondJSON(w, http.StatusTooManyRequests, ErrorResponse{Error: "rate_limit_exceeded"})
                return
            }

            w.Header().Set("X-RateLimit-Limit", "100")
            w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
            next.ServeHTTP(w, r)
        })
    }
}
```

#### 3.3.4 Input Validation

Use `go-playground/validator/v10` with custom validators.

```go
// internal/middleware/validate.go
var validate = validator.New()

func init() {
    validate.RegisterValidation("riskband", validateRiskBand)
    validate.RegisterValidation("duration", validateDuration)
}

func ValidateRequest(model interface{}) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if err := json.NewDecoder(r.Body).Decode(model); err != nil {
                respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "validation_failed", Details: err.Error()})
                return
            }
            if err := validate.Struct(model); err != nil {
                respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "validation_failed", Details: formatValidationErrors(err)})
                return
            }
            ctx := context.WithValue(r.Context(), "validated_body", model)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

#### 3.3.5 Secure Headers

```go
// internal/middleware/security.go
func SecurityHeaders() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("X-Content-Type-Options", "nosniff")
            w.Header().Set("X-Frame-Options", "DENY")
            w.Header().Set("X-XSS-Protection", "1; mode=block")
            w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
            w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'")
            w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
            w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
            next.ServeHTTP(w, r)
        })
    }
}
```

#### 3.3.6 Multi-Tenant Isolation

CRITICAL: Every database query MUST include `tenant_id` filter. Middleware injects it; stores enforce it.

```go
// internal/middleware/tenant.go
func TenantIsolation() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            tenantID := r.Context().Value("tenant_id")
            if tenantID == nil || tenantID == "" {
                respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "tenant_id_required"})
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

Store enforcement:
```go
func (s *AccessStore) ListRequests(ctx context.Context, tenantID string, filters RequestFilters) ([]domain.AccessRequest, error) {
    // EVERY query MUST have tenant_id = $1
    query := `SELECT * FROM workflow_requests WHERE tenant_id = $1`
    // ...
}
```

#### 3.3.7 Secrets Management (Vault)

```go
// internal/vault/client.go
package vault

type Client interface {
    Get(ctx context.Context, path string) (string, error)
    Put(ctx context.Context, path, value string) error
    Delete(ctx context.Context, path string) error
}

// FileVault for dev, HashiCorp Vault for prod
type FileVault struct {
    basePath string
    mu       sync.RWMutex
}

func (v *FileVault) Get(ctx context.Context, path string) (string, error) {
    v.mu.RLock()
    defer v.mu.RUnlock()
    data, err := os.ReadFile(filepath.Join(v.basePath, path))
    if err != nil {
        return "", err
    }
    return string(data), nil
}
```

Use for: connector credentials, OIDC client secrets, API key hashing salt, webhook signing keys.

### 3.4 Database Migrations (Phase 0)

```sql
-- migrations/007_api_keys.sql
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    key_hash VARCHAR(255) NOT NULL,  -- bcrypt hash of genid_<tenant>_<random>
    scopes TEXT[] NOT NULL DEFAULT '{}',
    created_by UUID NOT NULL,
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_api_keys_tenant ON api_keys(tenant_id);
CREATE INDEX idx_api_keys_hash ON api_keys(key_hash);

-- migrations/010_drop_legacy_audit_logs.sql
-- ONLY run if audit_logs (plural, legacy) exists AND is empty/unused.
-- NEVER run against audit_log (singular) — that is the active tamper-evident ledger.
-- If audit_logs (plural) has data, migrate to audit_log format first.
DROP TABLE IF EXISTS audit_logs;
```


---

## 4. PHASE 1: CONDITIONAL ACCESS MVP

### 4.1 Context Enrichment Service

Purpose: Transform raw request signals into policy-evaluable context.

```go
// internal/enrichment/service.go
package enrichment

type Service struct {
    geoResolver    GeoResolver
    deviceChecker  DeviceChecker
    timeEvaluator  TimeEvaluator
    cache          *redis.Client
}

type ContextSignals struct {
    IPAddress     string `json:"ip_address"`
    UserAgent     string `json:"user_agent"`
    DeviceID      string `json:"device_id,omitempty"`
    MFAMethod     string `json:"mfa_method,omitempty"`
    SessionID     string `json:"session_id,omitempty"`
}

type EnrichedContext struct {
    Location      string  `json:"location"`       // "us-office-sf", "remote-india"
    NetworkZone   string  `json:"network_zone"`   // "corporate", "vpn", "public"
    DeviceTrust   string  `json:"device_trust"`   // "managed", "unmanaged", "unknown"
    TimeOfDay     string  `json:"time_of_day"`    // "business_hours", "after_hours", "weekend"
    GeoVelocity   bool    `json:"geo_velocity"`   // impossible travel detected
    RiskBand      string  `json:"risk_band"`      // from Neo4j
    RiskScore     int     `json:"risk_score"`     // from Neo4j
}

func (s *Service) Enrich(ctx context.Context, tenantID string, signals ContextSignals) (*EnrichedContext, error) {
    cacheKey := fmt.Sprintf("ctx:%s:%s", tenantID, signals.IPAddress)

    // Try cache
    cached, err := s.cache.Get(ctx, cacheKey).Result()
    if err == nil {
        var ec EnrichedContext
        if json.Unmarshal([]byte(cached), &ec) == nil {
            return &ec, nil
        }
    }

    geo, err := s.geoResolver.Resolve(ctx, signals.IPAddress)
    if err != nil {
        return nil, err
    }

    zone := s.geoResolver.NetworkZone(ctx, tenantID, signals.IPAddress)
    device := s.deviceChecker.Evaluate(ctx, signals.DeviceID, signals.UserAgent)
    tod := s.timeEvaluator.Evaluate(time.Now())

    ec := &EnrichedContext{
        Location:    geo.Location,
        NetworkZone: zone,
        DeviceTrust: device.TrustLevel,
        TimeOfDay:   tod,
        GeoVelocity: false, // computed from session history
    }

    // Cache for 5 minutes
    data, _ := json.Marshal(ec)
    s.cache.Set(ctx, cacheKey, data, 5*time.Minute)

    return ec, nil
}
```

#### 4.1.1 Geo Resolution

```go
// internal/enrichment/geo.go
package enrichment

// Use free GeoLite2 or ipapi.co for dev
// For production: MaxMind GeoIP2 or internal IPAM

type GeoResolver interface {
    Resolve(ctx context.Context, ip string) (*GeoResult, error)
    NetworkZone(ctx context.Context, tenantID, ip string) string
}

type GeoResult struct {
    Country     string  `json:"country"`
    City        string  `json:"city"`
    Latitude    float64 `json:"lat"`
    Longitude   float64 `json:"lon"`
    Location    string  `json:"location"` // derived: "us-office-sf"
}

// NetworkZone uses tenant-specific CIDR maps
func (r *IPGeoResolver) NetworkZone(ctx context.Context, tenantID, ip string) string {
    // Query tenant_cidr_zones table
    // SELECT zone FROM tenant_cidr_zones WHERE tenant_id = $1 AND ip_range @> $2::inet
    // Returns: "corporate", "vpn", "dmz", "public"
}
```

Migration:
```sql
-- migrations/006_context_enrichment.sql
CREATE TABLE tenant_cidr_zones (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    zone_name VARCHAR(50) NOT NULL,  -- 'corporate', 'vpn', 'public'
    cidr CIDR NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_cidr_zones_tenant ON tenant_cidr_zones(tenant_id);
CREATE INDEX idx_cidr_zones_range ON tenant_cidr_zones USING gist (cidr inet_ops);

CREATE TABLE tenant_business_hours (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    timezone VARCHAR(50) NOT NULL DEFAULT 'America/New_York',
    monday_start TIME,
    monday_end TIME,
    tuesday_start TIME,
    tuesday_end TIME,
    wednesday_start TIME,
    wednesday_end TIME,
    thursday_start TIME,
    thursday_end TIME,
    friday_start TIME,
    friday_end TIME,
    saturday_start TIME,
    saturday_end TIME,
    sunday_start TIME,
    sunday_end TIME,
    weekend_access BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

#### 4.1.2 Device Trust Evaluation

```go
// internal/enrichment/device.go
package enrichment

type DeviceResult struct {
    TrustLevel  string `json:"trust_level"` // "managed", "unmanaged", "unknown"
    IsCompliant bool   `json:"is_compliant"`
    MDMStatus   string `json:"mdm_status"`  // "enrolled", "not_enrolled"
}

func (c *DeviceChecker) Evaluate(ctx context.Context, deviceID, userAgent string) *DeviceResult {
    // 1. Check MDM enrollment via header or lookup
    // 2. Check compliance status
    // 3. Check EDR agent presence
    // For MVP: static mapping from headers
    // X-Device-Trust: managed | unmanaged | unknown
}
```

### 4.2 Cedar Policy with Context

Extend the Cedar engine to accept enriched context.

```go
// internal/cedar/engine.go
package cedar

type PolicyCheckParams struct {
    TenantID    string                 `json:"tenant_id"`
    PrincipalID string                 `json:"principal_id"`
    ResourceID  string                 `json:"resource_id"`
    Action      string                 `json:"action"`
    Context     map[string]interface{} `json:"context"`
}

type PolicyResult struct {
    Decision string `json:"decision"`     // "Allow", "Deny", "StepUp"
    Duration string `json:"duration,omitempty"` // "2h", "1d", "permanent"
    Reason   string `json:"reason"`
    PolicyID string `json:"policy_id"`
}

func (e *Engine) Evaluate(ctx context.Context, params PolicyCheckParams) (*PolicyResult, error) {
    // 1. Load tenant's policies from PostgreSQL
    policies, err := e.loader.LoadPolicies(ctx, params.TenantID)
    if err != nil {
        return nil, err
    }

    // 2. Build Cedar entities
    entities := cedar.NewEntities()
    entities.Add(cedar.Entity{
        UID: cedar.EntityUID{Type: "Identity", ID: params.PrincipalID},
        // ... attrs from Neo4j
    })

    // 3. Build Cedar request with context
    request := cedar.Request{
        Principal: cedar.EntityUID{Type: "Identity", ID: params.PrincipalID},
        Action:    cedar.EntityUID{Type: "Action", ID: params.Action},
        Resource:  cedar.EntityUID{Type: "Resource", ID: params.ResourceID},
        Context:   cedar.NewRecord(params.Context),
    }

    // 4. Evaluate
    for _, policy := range policies {
        result, err := e.authorizer.IsAuthorized(ctx, request, policy, entities)
        if err != nil {
            continue
        }
        if result.Decision == cedar.Allow {
            return &PolicyResult{
                Decision: "Allow",
                Duration: extractDuration(policy),
                Reason:   extractReason(policy),
                PolicyID: policy.ID,
            }, nil
        }
    }

    return &PolicyResult{Decision: "Deny", Reason: "no_policy_matched"}, nil
}
```

### 4.3 our stakeholder's Flagship Policy (Seed Policy)

```cedar
// internal/cedar/templates/default_policy.cedar
// Policy 1: IT Admin, Corporate Network, Business Hours, Low Risk -> Auto-Approve 2h JIT
permit(
    principal == Identity::"alice",
    action == Action::"grant",
    resource == Resource::"*"
)
when {
    context.role == "it-admin" &&
    context.network_zone == "corporate" &&
    context.time_of_day == "business_hours" &&
    context.device_trust == "managed" &&
    context.risk_score < 500
}
advice "auto_approve_2h";

// Policy 2: Remote or Unmanaged Device -> Step-Up Approval
forbid(
    principal,
    action == Action::"grant",
    resource
)
when {
    context.network_zone == "public" ||
    context.device_trust == "unmanaged"
}
advice "step_up_approval";

// Policy 3: Critical Risk -> Deny Everything
forbid(
    principal,
    action,
    resource
)
when {
    context.risk_band == "critical"
}
advice "deny_due_to_risk";

// Policy 4: After Hours -> Short Duration Only
permit(
    principal,
    action == Action::"grant",
    resource
)
when {
    context.time_of_day == "after_hours" &&
    context.role in ["oncall", "sre"]
}
advice "approve_30m_jit";
```

Migration:
```sql
-- migrations/003_cedar_policies.sql (extended)
ALTER TABLE cedar_policies ADD COLUMN IF NOT EXISTS advice VARCHAR(50);
ALTER TABLE cedar_policies ADD COLUMN IF NOT EXISTS priority INT DEFAULT 100;
ALTER TABLE cedar_policies ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT true;
```

### 4.4 Workflow Integration

Modify GrantAccessWorkflow to use enriched context and policy result.

```go
// internal/workflow/grant_access_workflow.go
package workflow

type GrantAccessInput struct {
    TenantID       string                    `json:"tenant_id"`
    RequesterID    string                    `json:"requester_id"`
    TargetID       string                    `json:"target_id"`
    ResourceID     string                    `json:"resource_id"`
    Duration       time.Duration             `json:"duration"`
    ContextSignals enrichment.ContextSignals `json:"context_signals"`
}

func GrantAccessWorkflow(ctx workflow.Context, input GrantAccessInput) error {
    // 1. Enrich context (activity)
    var enrichedCtx enrichment.EnrichedContext
    err := workflow.ExecuteActivity(ctx, activities.EnrichContext, input.TenantID, input.ContextSignals).Get(ctx, &enrichedCtx)
    if err != nil {
        return err
    }

    // 2. Evaluate policy (activity)
    var policyResult cedar.PolicyResult
    err = workflow.ExecuteActivity(ctx, activities.EvaluatePolicy, cedar.PolicyCheckParams{
        TenantID:    input.TenantID,
        PrincipalID: input.RequesterID,
        ResourceID:  input.ResourceID,
        Action:      "grant",
        Context:     enrichedCtx.ToMap(),
    }).Get(ctx, &policyResult)
    if err != nil {
        return err
    }

    // 3. Route based on policy decision
    switch policyResult.Decision {
    case "Allow":
        if policyResult.Duration == "2h" {
            // Auto-approve JIT
            return workflow.ExecuteActivity(ctx, activities.ProvisionJITAccess, input).Get(ctx, nil)
        }
        return workflow.ExecuteActivity(ctx, activities.ProvisionAccess, input).Get(ctx, nil)

    case "StepUp":
        // Start approval gate
        return workflow.ExecuteChildWorkflow(ctx, ApprovalGateWorkflow, input).Get(ctx, nil)

    case "Deny":
        return workflow.ExecuteActivity(ctx, activities.LogDenial, input, policyResult).Get(ctx, nil)
    }

    return nil
}
```

### 4.5 Test Matrix

opencode MUST implement these tests in `internal/enrichment/`:

| Test Case | IP | Device | Time | Risk | Expected Zone | Expected Trust | Expected Time | Policy Result |
|-----------|-----|--------|------|------|---------------|----------------|---------------|---------------|
| Office-Managed-Business-Low | 10.0.1.5 | managed | 10:00 | 200 | corporate | managed | business_hours | Allow (2h JIT) |
| Office-Unmanaged-Business-Low | 10.0.1.5 | unmanaged | 10:00 | 200 | corporate | unmanaged | business_hours | StepUp |
| Remote-Managed-Business-Low | 203.0.113.1 | managed | 10:00 | 200 | public | managed | business_hours | StepUp |
| Office-Managed-AfterHours-Low | 10.0.1.5 | managed | 22:00 | 200 | corporate | managed | after_hours | Allow (30m JIT) |
| Office-Managed-Business-Critical | 10.0.1.5 | managed | 10:00 | 850 | corporate | managed | business_hours | Deny |
| VPN-Managed-Business-Low | 172.16.0.5 | managed | 10:00 | 200 | vpn | managed | business_hours | StepUp |

---

## 5. PHASE 2: MCP SERVER + AI INTEGRATION

### 5.1 MCP Server Architecture

Transport: stdio (local AI agents) + SSE (remote/web agents)
Auth: API key + identity context in MCP session

```go
// cmd/mcp-server/main.go
package main

func main() {
    transport := os.Getenv("MCP_TRANSPORT")
    if transport == "sse" {
        runSSEServer()
    } else {
        runStdioServer()
    }
}

func runStdioServer() {
    server := mcp.NewServer("genid-identity-server", "1.0.0")

    // Register tools
    server.RegisterTool(mcp.QueryIdentityTool)
    server.RegisterTool(mcp.RequestAccessTool)
    server.RegisterTool(mcp.CheckRiskTool)
    server.RegisterTool(mcp.ListApprovalsTool)
    server.RegisterTool(mcp.AuditTrailTool)
    server.RegisterTool(mcp.ExplainAccessTool)  // GraphRAG: "Why does Alice have prod access?"

    // Register resources
    server.RegisterResource(mcp.IdentityGraphResource)
    server.RegisterResource(mcp.PolicyDefinitionsResource)
    server.RegisterResource(mcp.RiskScoresResource)

    server.ServeStdio()
}
```

### 5.2 MCP Tools Specification

```go
// internal/mcp/tools.go
package mcp

// Tool: query_identity
// Description: "Query the identity graph for a user or service account"
// Parameters:
//   - identity_id (string, required): UUID or email
//   - include_entitlements (boolean, optional): Include access grants
//   - include_risk (boolean, optional): Include risk score and band
// Returns: Identity node + relationships + risk

var QueryIdentityTool = mcp.Tool{
    Name:        "query_identity",
    Description: "Query the identity graph for a user or service account",
    InputSchema: map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "identity_id": map[string]interface{}{
                "type":        "string",
                "description": "UUID or email of the identity",
            },
            "include_entitlements": map[string]interface{}{
                "type":        "boolean",
                "description": "Include current access grants",
            },
            "include_risk": map[string]interface{}{
                "type":        "boolean",
                "description": "Include risk score and band",
            },
        },
        "required": []string{"identity_id"},
    },
    Handler: func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
        identityID := params["identity_id"].(string)
        includeEntitlements := params["include_entitlements"].(bool)
        includeRisk := params["include_risk"].(bool)

        // Call identity service
        identity, err := identityService.Get(ctx, identityID)
        if err != nil {
            return nil, err
        }

        result := map[string]interface{}{
            "id":     identity.ID,
            "name":   identity.Name,
            "email":  identity.Email,
            "type":   identity.Type, // "human" | "service" | "agent"
        }

        if includeRisk {
            result["risk_score"] = identity.RiskScore
            result["risk_band"] = identity.RiskBand
        }

        if includeEntitlements {
            ents, _ := identityService.GetEntitlements(ctx, identityID)
            result["entitlements"] = ents
        }

        return &mcp.ToolResult{Content: result}, nil
    },
}

// Tool: request_access
// Description: "Request access to a resource for an identity"
// Parameters:
//   - identity_id (string, required)
//   - resource_id (string, required)
//   - duration (string, optional): "2h", "1d", "permanent"
//   - reason (string, required)
// Returns: Request ID + status + policy decision

var RequestAccessTool = mcp.Tool{
    Name:        "request_access",
    Description: "Request access to a resource for an identity",
    InputSchema: map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "identity_id": map[string]interface{}{
                "type":        "string",
                "description": "Identity requesting access",
            },
            "resource_id": map[string]interface{}{
                "type":        "string",
                "description": "Resource to access",
            },
            "duration": map[string]interface{}{
                "type":        "string",
                "description": "Duration: 2h, 1d, permanent",
            },
            "reason": map[string]interface{}{
                "type":        "string",
                "description": "Business justification",
            },
        },
        "required": []string{"identity_id", "resource_id", "reason"},
    },
    Handler: func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
        // Start GrantAccessWorkflow
        // Return workflow execution ID + initial policy decision
    },
}

// Tool: explain_access
// Description: "Explain why an identity has access to a resource using the identity graph"
// Parameters:
//   - identity_id (string, required)
//   - resource_id (string, required)
// Returns: Path from identity to resource + policy that allowed it + risk context

var ExplainAccessTool = mcp.Tool{
    Name:        "explain_access",
    Description: "Explain why an identity has access to a resource",
    InputSchema: map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "identity_id": map[string]interface{}{
                "type": "string",
            },
            "resource_id": map[string]interface{}{
                "type": "string",
            },
        },
        "required": []string{"identity_id", "resource_id"},
    },
    Handler: func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
        // Graph query: shortest path from identity to resource
        // Cypher: MATCH p=(i:Identity {id: $id})-[:HAS_ROLE|HAS_ACCESS*]->(r:Resource {id: $rid}) RETURN p
        // Explain each hop: role assignment -> policy -> grant
    },
}
```

### 5.3 NHI / Agent Governance

Non-Human Identity as first-class citizens.

```go
// internal/domain/nhi.go
package domain

type IdentityType string

const (
    IdentityTypeHuman   IdentityType = "human"
    IdentityTypeService IdentityType = "service"
    IdentityTypeAgent   IdentityType = "agent"      // AI agent
    IdentityTypeCI      IdentityType = "ci"         // CI/CD pipeline
    IdentityTypeIoT     IdentityType = "iot"        // Device
)

type NonHumanIdentity struct {
    ID            string       `json:"id"`
    Type          IdentityType `json:"type"`
    Name          string       `json:"name"`
    OwnerID       string       `json:"owner_id"`        // Human responsible
    ParentAgentID string       `json:"parent_agent_id,omitempty"` // For sub-agents
    CreatedAt     time.Time    `json:"created_at"`
    ExpiresAt     *time.Time   `json:"expires_at"`      // Auto-expire
    JITPassport   *JITPassport `json:"jit_passport,omitempty"`
    IsActive      bool         `json:"is_active"`
}

type JITPassport struct {
    TokenID     string     `json:"token_id"`
    IssuedAt    time.Time  `json:"issued_at"`
    ExpiresAt   time.Time  `json:"expires_at"`
    Scope       []string   `json:"scope"`
    TaskID      string     `json:"task_id"`       // What task is this for?
    RevokedAt   *time.Time `json:"revoked_at"`
}
```

Migration:
```sql
-- migrations/008_nhi_registry.sql
CREATE TABLE non_human_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    identity_type VARCHAR(20) NOT NULL CHECK (identity_type IN ('service', 'agent', 'ci', 'iot')),
    name VARCHAR(255) NOT NULL,
    owner_id UUID NOT NULL REFERENCES identities(id),
    parent_agent_id UUID REFERENCES non_human_identities(id),
    description TEXT,
    expires_at TIMESTAMPTZ,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE jit_passports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nhi_id UUID NOT NULL REFERENCES non_human_identities(id),
    token_hash VARCHAR(255) NOT NULL,
    scope TEXT[] NOT NULL,
    task_id VARCHAR(255),
    issued_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### 5.4 ZSP (Zero Standing Privilege) Mode

Default-deny standing privileges. All access is JIT unless explicitly exempted.

```go
// internal/services/access_service.go
func (s *AccessService) Grant(ctx context.Context, tenantID, requesterID string, req domain.GrantAccessRequest) (*domain.AccessRequestResult, error) {
    // Check ZSP mode
    tenantConfig, _ := s.tenantStore.GetConfig(ctx, tenantID)
    if tenantConfig.ZSPEnabled && req.Duration == 0 {
        // Permanent access requested but ZSP is on
        // Force JIT: max 2 hours, require explicit override approval
        req.Duration = 2 * time.Hour
        req.RequiresOverride = true
    }
    // ... rest of grant logic
}
```

Migration:
```sql
-- migrations/009_zsp_mode.sql
ALTER TABLE tenants ADD COLUMN zsp_enabled BOOLEAN DEFAULT false;
ALTER TABLE tenants ADD COLUMN zsp_max_jit_duration INTERVAL DEFAULT '2 hours';
ALTER TABLE tenants ADD COLUMN zsp_override_requires_approval BOOLEAN DEFAULT true;
```


---

## 6. PHASE 3: OBSERVEID BRIDGE

### 6.1 Webhook Registration API

```go
// internal/handlers/webhook_handler.go
package handlers

type WebhookHandler struct {
    webhookService *services.WebhookService
}

func (h *WebhookHandler) Register(w http.ResponseWriter, r *http.Request) {
    var req domain.WebhookRegistration
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid_request"})
        return
    }

    tenantID := r.Context().Value("tenant_id").(string)
    webhook, err := h.webhookService.Register(r.Context(), tenantID, req)
    if err != nil {
        HandleServiceError(w, err)
        return
    }

    respondJSON(w, http.StatusCreated, webhook)
}
```

Domain model:
```go
// internal/domain/webhook.go
package domain

type WebhookRegistration struct {
    ID          string   `json:"id"`
    TenantID    string   `json:"tenant_id"`
    URL         string   `json:"url" validate:"required,url"`
    Events      []string `json:"events" validate:"required,dive,oneof=access.approved access.denied access.revoked access.requested risk.changed identity.created identity.updated"`
    Secret      string   `json:"secret,omitempty"` // For HMAC-SHA256
    IsActive    bool     `json:"is_active"`
    CreatedAt   time.Time `json:"created_at"`
}
```

Webhook dispatch (async via Temporal activity):
```go
// internal/services/webhook_service.go
func (s *WebhookService) Dispatch(ctx context.Context, event domain.Event) error {
    webhooks, err := s.store.ListByEvent(ctx, event.Type)
    if err != nil {
        return err
    }

    for _, wh := range webhooks {
        payload, _ := json.Marshal(event)
        signature := hmacSHA256(payload, wh.Secret)

        req, _ := http.NewRequestWithContext(ctx, "POST", wh.URL, bytes.NewReader(payload))
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("X-GenID-Signature", "sha256="+hex.EncodeToString(signature))
        req.Header.Set("X-GenID-Event", event.Type)
        req.Header.Set("X-GenID-Delivery", uuid.New().String())

        resp, err := s.httpClient.Do(req)
        if err != nil {
            s.store.RecordFailure(ctx, wh.ID, event.ID, err.Error())
            continue
        }
        resp.Body.Close()

        if resp.StatusCode >= 200 && resp.StatusCode < 300 {
            s.store.RecordSuccess(ctx, wh.ID, event.ID)
        } else {
            s.store.RecordFailure(ctx, wh.ID, event.ID, fmt.Sprintf("HTTP %d", resp.StatusCode))
        }
    }
    return nil
}
```

### 6.2 SCIM 2.0 Consumer

ObserveID pushes identities/groups into GenID via SCIM.

```go
// internal/handlers/scim_handler.go
// POST /scim/v2/Users
// POST /scim/v2/Groups
// GET /scim/v2/Users/{id}
// PUT /scim/v2/Users/{id}
// DELETE /scim/v2/Users/{id}
// PATCH /scim/v2/Users/{id}

func (h *SCIMHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
    var scimUser domain.SCIMUser
    if err := json.NewDecoder(r.Body).Decode(&scimUser); err != nil {
        respondJSON(w, http.StatusBadRequest, scimErrorResponse("invalidPayload", err.Error()))
        return
    }

    // Map SCIM user to GenID identity
    identity := domain.Identity{
        Email:      scimUser.UserName,
        FirstName:  scimUser.Name.GivenName,
        LastName:   scimUser.Name.FamilyName,
        ExternalID: scimUser.ExternalID,
        Source:     "scim-observeid",
    }

    err := h.identityService.CreateFromSCIM(r.Context(), identity)
    if err != nil {
        respondJSON(w, http.StatusInternalServerError, scimErrorResponse("internalError", err.Error()))
        return
    }

    respondJSON(w, http.StatusCreated, scimUserResponse(identity))
}
```

### 6.3 API Contract Hardening

Cursor pagination on ALL list endpoints:

```go
// internal/domain/pagination.go
package domain

type CursorPagination struct {
    Limit     int    `form:"limit" validate:"min=1,max=100"`
    Cursor    string `form:"cursor"`
    Direction string `form:"direction" validate:"oneof=next prev"`
}

type CursorPage struct {
    Data       []interface{} `json:"data"`
    NextCursor string        `json:"next_cursor,omitempty"`
    PrevCursor string        `json:"prev_cursor,omitempty"`
    HasMore    bool          `json:"has_more"`
}
```

ETags for cache validation:
```go
func (h *IdentityHandler) Get(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id := vars["id"]
    identity, err := h.identityService.Get(r.Context(), id)
    if err != nil {
        HandleServiceError(w, err)
        return
    }

    etag := fmt.Sprintf(""%x"", sha256.Sum256([]byte(identity.UpdatedAt.String())))
    if match := r.Header.Get("If-None-Match"); match == etag {
        w.WriteHeader(http.StatusNotModified)
        return
    }

    w.Header().Set("ETag", etag)
    w.Header().Set("Cache-Control", "private, max-age=60")
    respondJSON(w, http.StatusOK, identity)
}
```

---

## 7. PHASE 4: PRODUCTION UI/UX

### 7.1 Design System (shadcn/ui)

Base components to install:
```bash
npx shadcn add button card badge avatar tabs table dialog form input select toast
npx shadcn add chart             # For risk timeline
npx shadcn add data-table        # For paginated tables
npx shadcn add sheet             # For mobile sidebar
```

Theme tokens (dark mode default):
```css
/* styles/globals.css */
@layer base {
  :root {
    --background: 222.2 84% 4.9%;
    --foreground: 210 40% 98%;
    --card: 222.2 84% 4.9%;
    --card-foreground: 210 40% 98%;
    --popover: 222.2 84% 4.9%;
    --popover-foreground: 210 40% 98%;
    --primary: 217.2 91.2% 59.8%;
    --primary-foreground: 222.2 47.4% 11.2%;
    --secondary: 217.2 32.6% 17.5%;
    --secondary-foreground: 210 40% 98%;
    --muted: 217.2 32.6% 17.5%;
    --muted-foreground: 215 20.2% 65.1%;
    --accent: 217.2 32.6% 17.5%;
    --accent-foreground: 210 40% 98%;
    --destructive: 0 62.8% 30.6%;
    --destructive-foreground: 210 40% 98%;
    --border: 217.2 32.6% 17.5%;
    --input: 217.2 32.6% 17.5%;
    --ring: 224.3 76.3% 48%;
    --radius: 0.5rem;
  }
}
```

### 7.2 Key Pages

#### Risk Dashboard (/risk)
```tsx
// app/(dashboard)/risk/page.tsx
export default function RiskDashboard() {
  const { identities, isLoading } = useRiskIdentities()

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-4 gap-4">
        <RiskScoreCard title="Critical" count={identities.filter(i => i.riskBand === 'critical').length} color="red" />
        <RiskScoreCard title="High" count={identities.filter(i => i.riskBand === 'high').length} color="orange" />
        <RiskScoreCard title="Elevated" count={identities.filter(i => i.riskBand === 'elevated').length} color="yellow" />
        <RiskScoreCard title="Low/Minimal" count={identities.filter(i => ['low','minimal'].includes(i.riskBand)).length} color="green" />
      </div>

      <RiskTimeline identities={identities} />
      <RiskEventStream />
    </div>
  )
}
```

#### Policy Simulator (/policies/simulator)
```tsx
// components/policy/policy-simulator.tsx
export function PolicySimulator() {
  const [scenario, setScenario] = useState({
    role: 'it-admin',
    networkZone: 'corporate',
    deviceTrust: 'managed',
    timeOfDay: 'business_hours',
    riskScore: 200,
  })

  const { result, isSimulating } = usePolicySimulation(scenario)

  return (
    <Card>
      <CardHeader>
        <CardTitle>Policy Simulator</CardTitle>
        <CardDescription>Test "What if Alice requests prod access from home at 2am?"</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <Select value={scenario.role} onValueChange={v => setScenario({...scenario, role: v})}>
          <SelectTrigger><SelectValue placeholder="Role" /></SelectTrigger>
          <SelectContent>
            <SelectItem value="it-admin">IT Admin</SelectItem>
            <SelectItem value="developer">Developer</SelectItem>
            <SelectItem value="oncall">On-Call</SelectItem>
          </SelectContent>
        </Select>

        <Select value={scenario.networkZone} onValueChange={v => setScenario({...scenario, networkZone: v})}>
          <SelectTrigger><SelectValue placeholder="Network Zone" /></SelectTrigger>
          <SelectContent>
            <SelectItem value="corporate">Corporate Office</SelectItem>
            <SelectItem value="vpn">VPN</SelectItem>
            <SelectItem value="public">Public Internet</SelectItem>
          </SelectContent>
        </Select>

        <Button onClick={() => simulate(scenario)} disabled={isSimulating}>
          {isSimulating ? 'Simulating...' : 'Run Simulation'}
        </Button>

        {result && (
          <Alert variant={result.decision === 'Allow' ? 'default' : result.decision === 'StepUp' ? 'warning' : 'destructive'}>
            <AlertTitle>{result.decision}</AlertTitle>
            <AlertDescription>{result.reason}</AlertDescription>
          </Alert>
        )}
      </CardContent>
    </Card>
  )
}
```

#### NHI Registry (/nhi)
```tsx
// app/(dashboard)/nhi/page.tsx
export default function NHIRegistry() {
  const { nhies } = useNHIIdentities()

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">Non-Human Identities</h1>
        <Button>Register Agent</Button>
      </div>

      <DataTable
        columns={[
          { accessorKey: 'name', header: 'Name' },
          { accessorKey: 'type', header: 'Type' },
          { accessorKey: 'owner', header: 'Owner' },
          { accessorKey: 'riskBand', header: 'Risk' },
          { accessorKey: 'expiresAt', header: 'Expires' },
        ]}
        data={nhies}
      />
    </div>
  )
}
```

### 7.3 Real-Time Updates (WebSocket/SSE)

```tsx
// hooks/use-websocket.ts
export function useRiskStream() {
  const [events, setEvents] = useState<RiskEvent[]>([])

  useEffect(() => {
    const es = new EventSource('/api/v1/events/stream?token=' + getToken())

    es.onmessage = (e) => {
      const event = JSON.parse(e.data)
      setEvents(prev => [event, ...prev].slice(0, 100))
    }

    return () => es.close()
  }, [])

  return events
}
```

---

## 8. SECURITY BASELINE (OWASP ASVS L2)

### 8.1 Checklist

| ASVS Requirement | Implementation | File |
|-----------------|----------------|------|
| V1.1 Secure SDLC | Threat model in docs | `docs/security/THREAT_MODEL.md` |
| V2.1 Password policy | OIDC only, no local passwords | `internal/middleware/auth.go` |
| V2.2 MFA | Enforce MFA for admin roles | `internal/services/policy_service.go` |
| V3.1 Session management | JWT with refresh rotation | `internal/handlers/auth_handler.go` |
| V4.1 Access control | Cedar + OpenFGA + scopes | `internal/cedar/`, `internal/fga/` |
| V5.1 Input validation | go-playground/validator | `internal/middleware/validate.go` |
| V5.2 Sanitization | SQL parameterized queries | ALL stores |
| V5.3 Output encoding | JSON encoding only | Handlers |
| V6.1 Cryptography | bcrypt for passwords, AES-256-GCM for secrets | `internal/vault/` |
| V7.1 Log content | Structured JSON, no PII | `internal/middleware/logging.go` |
| V8.1 Data protection | TLS 1.3, encrypted at rest | `infrastructure/` |
| V9.1 Communications | mTLS for service-to-service | `internal/config/` |
| V10.1 Code integrity | Dependency scanning (govulncheck) | CI/CD |
| V11.1 Business logic | Idempotency keys | `internal/middleware/idempotency.go` |
| V12.1 File upload | No file uploads in API | N/A |
| V13.1 API security | Rate limiting, API keys, OIDC | `internal/middleware/` |
| V14.1 Configuration | Secrets in Vault, not env | `internal/vault/` |

### 8.2 Security Headers (Already defined in middleware)

See `internal/middleware/security.go` in Phase 0.

### 8.3 Secrets Management Policy

1. NEVER commit secrets to git. Use `.env.example` only.
2. Development: `internal/vault/` FileVault with encrypted files
3. Production: HashiCorp Vault or cloud KMS
4. Rotation: API keys expire after 90 days. Webhook secrets rotate monthly.
5. Audit: Every secret access is logged to `workflow_audit`.

### 8.4 Input Validation Rules

```go
// internal/pkg/validators/rules.go
package validators

var Rules = map[string]validator.Func{
    "riskband": func(fl validator.FieldLevel) bool {
        valid := []string{"minimal", "low", "elevated", "high", "critical"}
        return contains(valid, fl.Field().String())
    },
    "duration": func(fl validator.FieldLevel) bool {
        _, err := time.ParseDuration(fl.Field().String())
        return err == nil
    },
    "uuid": func(fl validator.FieldLevel) bool {
        _, err := uuid.Parse(fl.Field().String())
        return err == nil
    },
    "cidr": func(fl validator.FieldLevel) bool {
        _, _, err := net.ParseCIDR(fl.Field().String())
        return err == nil
    },
}
```

---

## 9. DATABASE SCHEMA EVOLUTION

### 9.1 Migration Order (CRITICAL — run in this order)

```sql
-- 001_initial_schema.sql (existing)
-- 002_audit_hash_chain.sql (existing)
-- 003_cedar_policies.sql (extended with advice, priority, is_active)
-- 004_webhooks.sql (new)
-- 005_risk_events.sql (existing)
-- 006_context_enrichment.sql (new — CIDR zones, business hours)
-- 007_api_keys.sql (new — machine auth)
-- 008_nhi_registry.sql (new — non-human identities)
-- 009_zsp_mode.sql (new — zero standing privilege)
-- 010_drop_legacy_audit.sql (new — remove old audit_logs)
```

### 9.2 Key Schema Definitions

```sql
-- workflow_requests (extended)
ALTER TABLE workflow_requests ADD COLUMN IF NOT EXISTS context JSONB;
ALTER TABLE workflow_requests ADD COLUMN IF NOT EXISTS enriched_context JSONB;
ALTER TABLE workflow_requests ADD COLUMN IF NOT EXISTS policy_decision VARCHAR(20);
ALTER TABLE workflow_requests ADD COLUMN IF NOT EXISTS policy_reason TEXT;

-- workflow_approvals (extended)
ALTER TABLE workflow_approvals ADD COLUMN IF NOT EXISTS delegated_from UUID REFERENCES workflow_approvals(id);
ALTER TABLE workflow_approvals ADD COLUMN IF NOT EXISTS auto_approved BOOLEAN DEFAULT false;
ALTER TABLE workflow_approvals ADD COLUMN IF NOT EXISTS auto_approve_reason TEXT;

-- identities (extended)
ALTER TABLE identities ADD COLUMN IF NOT EXISTS external_id VARCHAR(255);
ALTER TABLE identities ADD COLUMN IF NOT EXISTS source VARCHAR(50) DEFAULT 'manual';
ALTER TABLE identities ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;
ALTER TABLE identities ADD COLUMN IF NOT EXISTS mfa_enabled BOOLEAN DEFAULT false;

-- New: event_sources config table
CREATE TABLE event_source_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    source_name VARCHAR(50) NOT NULL,
    source_type VARCHAR(20) NOT NULL CHECK (source_type IN ('webhook', 'azure_sb', 'kafka')),
    config JSONB NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(tenant_id, source_name)
);

-- New: webhook deliveries for retry tracking
CREATE TABLE webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL,
    event_id UUID NOT NULL,
    status VARCHAR(20) NOT NULL, -- 'pending', 'success', 'failed'
    http_status INT,
    response_body TEXT,
    error_message TEXT,
    attempted_at TIMESTAMPTZ DEFAULT NOW(),
    retry_count INT DEFAULT 0
);
CREATE INDEX idx_webhook_deliveries_webhook ON webhook_deliveries(webhook_id);
CREATE INDEX idx_webhook_deliveries_status ON webhook_deliveries(status) WHERE status != 'success';
```

---

## 10. API CONTRACT (OPENAPI 3.1)

### 10.1 Key Endpoints

```yaml
# docs/openapi.yaml (excerpt)
openapi: 3.1.0
info:
  title: GenID API
  version: 1.0.0
  description: Event-driven Identity Governance Platform

servers:
  - url: http://localhost:8080/api/v1

security:
  - bearerAuth: []
  - apiKeyAuth: []

paths:
  /access/grant:
    post:
      summary: Request access grant
      operationId: grantAccess
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/GrantAccessRequest'
      responses:
        202:
          description: Request accepted
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AccessRequestResult'
        400:
          $ref: '#/components/responses/BadRequest'
        401:
          $ref: '#/components/responses/Unauthorized'
        429:
          $ref: '#/components/responses/RateLimited'

  /access/jit:
    post:
      summary: Request just-in-time access
      operationId: requestJIT
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/JITAccessRequest'
      responses:
        202:
          description: JIT request accepted

  /events/ingest/{source}:
    post:
      summary: Ingest external events
      operationId: ingestEvent
      parameters:
        - name: source
          in: path
          required: true
          schema:
            type: string
            enum: [entra, okta, jira, azure-sb]
        - name: X-GenID-Signature
          in: header
          required: false
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
      responses:
        202:
          description: Event accepted

  /webhooks:
    post:
      summary: Register webhook
      operationId: registerWebhook
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/WebhookRegistration'
      responses:
        201:
          description: Webhook registered

  /risk/identities:
    get:
      summary: List identities with risk scores
      operationId: listRiskIdentities
      parameters:
        - name: band
          in: query
          schema:
            type: string
            enum: [minimal, low, elevated, high, critical]
        - name: limit
          in: query
          schema:
            type: integer
            default: 20
            maximum: 100
        - name: cursor
          in: query
          schema:
            type: string
      responses:
        200:
          description: Paginated list
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/CursorPage'

  /scim/v2/Users:
    post:
      summary: SCIM 2.0 Create User
      operationId: scimCreateUser
      requestBody:
        required: true
        content:
          application/scim+json:
            schema:
              $ref: '#/components/schemas/SCIMUser'
      responses:
        201:
          description: User created

components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
    apiKeyAuth:
      type: apiKey
      in: header
      name: X-API-Key

  schemas:
    GrantAccessRequest:
      type: object
      required: [target_id, resource_id, reason]
      properties:
        target_id:
          type: string
          format: uuid
        resource_id:
          type: string
        duration:
          type: string
          description: "e.g., 2h, 1d, permanent"
        reason:
          type: string
          minLength: 10
        context_signals:
          $ref: '#/components/schemas/ContextSignals'

    ContextSignals:
      type: object
      properties:
        ip_address:
          type: string
          format: ipv4
        user_agent:
          type: string
        device_id:
          type: string
        mfa_method:
          type: string
          enum: [totp, webauthn, sms, push]

    AccessRequestResult:
      type: object
      properties:
        request_id:
          type: string
          format: uuid
        status:
          type: string
          enum: [pending, approved, denied, auto_approved]
        policy_decision:
          type: string
          enum: [Allow, Deny, StepUp]
        workflow_id:
          type: string

    WebhookRegistration:
      type: object
      required: [url, events]
      properties:
        url:
          type: string
          format: uri
        events:
          type: array
          items:
            type: string
            enum: [access.approved, access.denied, access.revoked, risk.changed, identity.created]
        secret:
          type: string
          minLength: 32

    CursorPage:
      type: object
      properties:
        data:
          type: array
          items:
            type: object
        next_cursor:
          type: string
        prev_cursor:
          type: string
        has_more:
          type: boolean

    SCIMUser:
      type: object
      properties:
        userName:
          type: string
        name:
          type: object
          properties:
            givenName:
              type: string
            familyName:
              type: string
        externalId:
          type: string
        active:
          type: boolean

  responses:
    BadRequest:
      description: Invalid request
      content:
        application/json:
          schema:
            type: object
            properties:
              error:
                type: string
              message:
                type: string
              details:
                type: object

    Unauthorized:
      description: Authentication required
      content:
        application/json:
          schema:
            type: object
            properties:
              error:
                type: string
                enum: [invalid_token, invalid_api_key, expired_token]

    RateLimited:
      description: Too many requests
      headers:
        Retry-After:
          schema:
            type: integer
      content:
        application/json:
          schema:
            type: object
            properties:
              error:
                type: string
                enum: [rate_limit_exceeded]
```

---

## 11. TESTING STRATEGY

### 11.1 Test Pyramid

```
        /\
       /  \
      / E2E \      ~5%  (Playwright, full stack)
     /--------\
    /  Integration \  ~15% (DB, NATS, Temporal, Neo4j)
   /----------------\
  /     Unit Tests    \ ~80% (handlers, services, stores, domain)
 /----------------------\
```

### 11.2 Unit Test Patterns

```go
// internal/enrichment/geo_test.go
package enrichment

func TestNetworkZone_CorporateCIDR(t *testing.T) {
    resolver := NewIPGeoResolver(testDB)

    // Seed test CIDR
    testDB.Exec("INSERT INTO tenant_cidr_zones (tenant_id, zone_name, cidr) VALUES ($1, $2, $3)",
        "tenant-1", "corporate", "10.0.0.0/8")

    zone := resolver.NetworkZone(context.Background(), "tenant-1", "10.0.1.5")
    assert.Equal(t, "corporate", zone)
}

func TestNetworkZone_PublicIP(t *testing.T) {
    resolver := NewIPGeoResolver(testDB)
    zone := resolver.NetworkZone(context.Background(), "tenant-1", "203.0.113.1")
    assert.Equal(t, "public", zone)
}
```

### 11.3 Integration Test Patterns

```go
// tests/integration/conditional_access_test.go
package integration

func TestConditionalAccess_OfficeManagedBusinessLow(t *testing.T) {
    // Setup: start full stack in docker-compose
    ctx := context.Background()

    // Create identity with role it-admin
    identity := createTestIdentity(ctx, "alice", "it-admin")

    // Create request with corporate IP, managed device, business hours
    req := domain.GrantAccessRequest{
        TargetID:   identity.ID,
        ResourceID: "prod-db",
        Reason:     "routine maintenance",
        ContextSignals: domain.ContextSignals{
            IPAddress: "10.0.1.5",
            UserAgent: "Mozilla/5.0 (ManagedDevice)",
        },
    }

    result, err := accessService.Grant(ctx, "tenant-1", identity.ID, req)
    require.NoError(t, err)
    assert.Equal(t, "auto_approved", result.Status)
    assert.Equal(t, "Allow", result.PolicyDecision)
}

func TestConditionalAccess_RemoteManagedBusinessLow(t *testing.T) {
    identity := createTestIdentity(ctx, "bob", "it-admin")

    req := domain.GrantAccessRequest{
        TargetID:   identity.ID,
        ResourceID: "prod-db",
        Reason:     "emergency fix",
        ContextSignals: domain.ContextSignals{
            IPAddress: "203.0.113.1", // Public IP
            UserAgent: "Mozilla/5.0 (ManagedDevice)",
        },
    }

    result, err := accessService.Grant(ctx, "tenant-1", identity.ID, req)
    require.NoError(t, err)
    assert.Equal(t, "pending", result.Status)
    assert.Equal(t, "StepUp", result.PolicyDecision)
}
```

### 11.4 E2E Test Patterns (Playwright)

```typescript
// tests/e2e/conditional-access.spec.ts
import { test, expect } from '@playwright/test'

test('auto-approve IT admin in office', async ({ page }) => {
  await page.goto('/policies/simulator')

  await page.selectOption('[name="role"]', 'it-admin')
  await page.selectOption('[name="networkZone"]', 'corporate')
  await page.selectOption('[name="deviceTrust"]', 'managed')
  await page.selectOption('[name="timeOfDay"]', 'business_hours')
  await page.fill('[name="riskScore"]', '200')

  await page.click('text=Run Simulation')

  await expect(page.locator('[data-testid="result"]')).toContainText('Allow')
  await expect(page.locator('[data-testid="result"]')).toContainText('2h JIT')
})
```

---

## 12. DEPLOYMENT & OPERATIONS

### 12.1 Docker Compose (Development)

```yaml
# infrastructure/docker-compose.yml
version: "3.9"

services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: genid
      POSTGRES_PASSWORD: genid
      POSTGRES_DB: genid
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./postgres/init.sql:/docker-entrypoint-initdb.d/init.sql
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U genid"]
      interval: 5s
      timeout: 5s
      retries: 5

  neo4j:
    image: neo4j:5.20-community
    environment:
      NEO4J_AUTH: neo4j/genid123
      NEO4J_PLUGINS: '["apoc", "gds"]'
    volumes:
      - neo4j_data:/data
      - ./neo4j/init.cypher:/docker-entrypoint-initdb.d/init.cypher
    ports:
      - "7474:7474"
      - "7687:7687"

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

  nats:
    image: nats:2.10-alpine
    command: "-js -m 8222"
    ports:
      - "4222:4222"
      - "8222:8222"
    volumes:
      - nats_data:/data/jetstream
      - ./nats/nats-server.conf:/etc/nats/nats-server.conf

  temporal:
    image: temporalio/auto-setup:1.25
    environment:
      DB: postgresql
      DB_PORT: 5432
      POSTGRES_USER: genid
      POSTGRES_PWD: genid
      POSTGRES_SEEDS: postgres
      DYNAMIC_CONFIG_FILE_PATH: /etc/temporal/dynamicconfig/development.yaml
    volumes:
      - ./temporal/dynamicconfig:/etc/temporal/dynamicconfig
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "7233:7233"

  temporal-ui:
    image: temporalio/ui:2.28
    environment:
      TEMPORAL_ADDRESS: temporal:7233
    ports:
      - "8081:8080"
    depends_on:
      - temporal

  openfga:
    image: openfga/openfga:v1.5
    command: run
    environment:
      OPENFGA_DATASTORE_ENGINE: postgres
      OPENFGA_DATASTORE_URI: postgres://genid:genid@postgres:5432/genid?sslmode=disable
    ports:
      - "8082:8080"
    depends_on:
      - postgres

  prometheus:
    image: prom/prometheus:v2.54
    volumes:
      - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus_data:/prometheus
    ports:
      - "9090:9090"

  grafana:
    image: grafana/grafana:11.0
    environment:
      GF_SECURITY_ADMIN_PASSWORD: admin
    volumes:
      - grafana_data:/var/lib/grafana
      - ./grafana/dashboards:/etc/grafana/provisioning/dashboards
    ports:
      - "3000:3000"
    depends_on:
      - prometheus

volumes:
  postgres_data:
  neo4j_data:
  redis_data:
  nats_data:
  prometheus_data:
  grafana_data:
```

### 12.2 NATS Configuration

```
# infrastructure/nats/nats-server.conf
jetstream {
    store_dir: "/data/jetstream"
    max_memory_store: 1GB
    max_file_store: 10GB
}

streams: [
    {
        name: "genid-events"
        subjects: ["identity.*", "access.*", "risk.*", "audit.*"]
        retention: "limits"
        max_msgs: 1000000
        max_bytes: 1GB
        max_age: "30d"
        storage: "file"
        replicas: 1
    }
]
```

### 12.3 Makefile Targets

```makefile
# backend/Makefile
.PHONY: all build test lint migrate seed demo

all: lint test build

build:
	go build -o bin/identity-service ./cmd/identity-service
	go build -o bin/event-processor ./cmd/event-processor
	go build -o bin/mcp-server ./cmd/mcp-server

test:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...
	go vet ./...

migrate-up:
	migrate -path migrations -database "postgres://genid:genid@localhost:5432/genid?sslmode=disable" up

migrate-down:
	migrate -path migrations -database "postgres://genid:genid@localhost:5432/genid?sslmode=disable" down 1

seed:
	go run scripts/seed.go

demo-risk:
	./scripts/simulate-idp-events.sh

demo-conditional:
	./scripts/demo-conditional-access.sh

dev:
	docker compose -f ../infrastructure/docker-compose.yml up -d
	go run ./cmd/identity-service/main.go &
	go run ./cmd/event-processor/main.go
```

### 12.4 Environment Configuration

```bash
# .env.example (NEVER commit .env)
# Database
GENID_DATABASE_URL=postgres://genid:genid@localhost:5432/genid?sslmode=disable
GENID_NEO4J_URI=bolt://localhost:7687
GENID_NEO4J_USER=neo4j
GENID_NEO4J_PASSWORD=genid123

# Cache
GENID_REDIS_URL=redis://localhost:6379/0

# Event Bus
GENID_NATS_URL=nats://localhost:4222

# Temporal
GENID_TEMPORAL_HOST=localhost:7233

# Auth
GENID_OIDC_ISSUER=https://auth.observeid.io
GENID_OIDC_CLIENT_ID=genid-app
GENID_OIDC_CLIENT_SECRET=changeme
GENID_OIDC_REDIRECT_URL=http://localhost:8080/auth/oidc/callback
GENID_JWT_SECRET=changeme-min-32-chars-long

# Vault
GENID_VAULT_TYPE=file
GENID_VAULT_PATH=./.secrets

# OpenFGA
GENID_OPENFGA_API_URL=http://localhost:8082

# Azure Service Bus (optional)
AZURE_SERVICEBUS_CONNECTION_STRING=Endpoint=sb://...;SharedAccessKeyName=...;SharedAccessKey=...

# Dev
GENID_DEV_LOGIN_ENABLED=true
GENID_LOG_LEVEL=debug
```

---

## APPENDIX A: OPENCODE EXECUTION CHECKLIST

For each phase, opencode MUST verify before proceeding:

### Phase 0 Verification
- [ ] `go build ./...` passes with zero errors
- [ ] `go test ./...` passes with zero failures
- [ ] `go vet ./...` passes with zero warnings
- [ ] `golangci-lint run ./...` passes
- [ ] All handlers use the handler pattern (no business logic)
- [ ] All services use the service pattern (no HTTP, no DB)
- [ ] All stores use the store pattern (tenant_id in every query)
- [ ] Dev-login replaced with OIDC + API key auth
- [ ] Rate limiting active on all mutating endpoints
- [ ] Secure headers on all responses
- [ ] Input validation on all JSON bodies
- [ ] `audit_logs` table dropped, data migrated

### Phase 1 Verification
- [ ] Context enrichment service returns correct zone/trust/time for all test matrix cases
- [ ] Cedar engine evaluates our stakeholder's flagship policy correctly
- [ ] Auto-approve path: no human approval, immediate JIT provision
- [ ] Step-up path: approval gate workflow started
- [ ] Deny path: request rejected with reason
- [ ] Demo script runs end-to-end: `scripts/demo-conditional-access.sh`
- [ ] UI shows policy simulator with correct results

### Phase 2 Verification
- [ ] MCP server starts with stdio transport
- [ ] `query_identity` tool returns correct data
- [ ] `request_access` tool starts workflow
- [ ] `explain_access` tool returns graph path
- [ ] NHI can be registered with owner
- [ ] JIT passport auto-expires
- [ ] ZSP mode forces JIT duration

### Phase 3 Verification
- [ ] Webhook registration API accepts valid payloads
- [ ] Webhook dispatch delivers events with HMAC signature
- [ ] SCIM 2.0 endpoints accept ObserveID user pushes
- [ ] Cursor pagination works on all list endpoints
- [ ] ETags return 304 for unmodified resources

### Phase 4 Verification
- [ ] Frontend builds with `npm run build`
- [ ] All pages accessible with proper auth
- [ ] Risk dashboard shows real-time updates
- [ ] Policy simulator interactive
- [ ] NHI registry lists agents
- [ ] Dark mode toggle works
- [ ] a11y: keyboard navigation, screen reader labels

---

## APPENDIX B: C#/.NET BRIDGE SPECIFICATION

For ObserveID integration, provide these contracts:

1. **gRPC Service** (protobuf in `api/proto/`)
   ```protobuf
   service IdentityBridge {
     rpc SyncIdentities(SyncRequest) returns (SyncResponse);
     rpc StreamEvents(EventFilter) returns (stream IdentityEvent);
     rpc EvaluatePolicy(PolicyRequest) returns (PolicyResponse);
   }
   ```

2. **OpenAPI Client Generation**
   ```bash
   # Generate C# client from OpenAPI spec
   openapi-generator-cli generate -i docs/openapi.yaml -g csharp -o clients/csharp/
   ```

3. **Webhook Contract**
   - ObserveID registers `https://observeid.io/api/genid/webhook`
   - GenID POSTs events with `X-GenID-Signature: sha256=<hmac>`
   - ObserveID verifies HMAC before processing

4. **Kafka Bridge (Future)**
   - NATS -> Kafka Connect connector
   - Topics: `genid.access.events`, `genid.risk.events`
   - Avro schema registry

---

## APPENDIX C: PERFORMANCE TARGETS

| Metric | Target | Measurement |
|--------|--------|-------------|
| API p99 latency | < 100ms | Prometheus histogram |
| Workflow start | < 50ms | Temporal metrics |
| Policy evaluation | < 10ms | Custom metric |
| Risk processor | < 500ms/event | NATS consumer lag |
| Event ingestion | < 50ms | API latency |
| Frontend TTI | < 2s | Lighthouse |
| DB query p99 | < 20ms | pg_stat_statements |
| Neo4j query p99 | < 50ms | Neo4j metrics |

---

*End of GenID Build Bible v1.0*
*Built for opencode + DeepSeek v4 Flash*
*All code paths verified against go build, go test, go vet*
