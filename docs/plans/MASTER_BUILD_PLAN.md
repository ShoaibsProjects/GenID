# GenID: Master Build Plan (The Agentic Fabric v2.0)

## 0. Core Mandate & Context
**Project:** GenID
**Goal:** Refactor the existing GenID codebase from a legacy IGA prototype into a FAANG-grade, Agentic Identity Fabric.
**Target Audience for this Document:** AI Coding Agent (e.g., OpenCode, Cursor).
**Instruction to AI:** Read this document entirely. Execute the tasks inch-by-inch in the exact order specified. Do not skip ahead. Adhere strictly to the "Tech Stack" and "Cut List" constraints. Ask for clarification if a task is ambiguous, but do not invent new architectural patterns.

### Tech Stack Constraints
*   **Backend Language:** Go 1.23
*   **Frontend:** Next.js 14 (App Router, Static Export)
*   **Database:** PostgreSQL 16 (Source of truth), Neo4j 5 (Graph analytics), Redis 7 (Cache)
*   **Event Bus:** NATS JetStream (Embedded, Go-native)
*   **Policy Engine:** AWS Cedar (in-process Go evaluation)
*   **Orchestration:** Temporal.io v1.25
*   **Deployment:** Docker Compose (Dev), Kubernetes (Prod), Cloudflare Pages (Frontend)

### The Strict Cut List (DO NOT IMPLEMENT OR INCLUDE)
*   **NO Kafka, Zookeeper, or Schema Registry.**
*   **NO Qdrant** (Use Neo4j native vector search instead).
*   **NO SPIFFE/SPIRE** for V1 (Use short-lived JIT JWTs instead).
*   **NO GraphRAG AI Chatbot** UI for V1 (Focus on REST API blast radius first).
*   **NO `X-User-Role` or `X-User-ID` header trust.** (Strict JWT/JWKS validation only).

---

## Phase 1: The Purge & Backend Hardening

### Task 1.1: Docker-Compose Cleanup
**Action:** Open `infrastructure/docker-compose.yml`.
**Delete:** Remove `kafka`, `kafka-zookeeper`, `schema-registry`, `kafka-init`, and `qdrant` services entirely.
**Add:** Add a `nats` service:
```yaml
  nats:
    image: nats:2.10-alpine
    command: ["-js"] # Enable JetStream
    ports:
      - "4222:4222" # Client connections
      - "8222:8222" # HTTP monitoring
    volumes:
      - nats-data:/data
    restart: unless-stopped
```
**Update:** Add `nats-data` to the `volumes` list at the bottom of the file.

### Task 1.2: API Gateway Security Fix (Critical)
**Action:** Open `backend/internal/middleware/auth.go` (or wherever `APIKeyAuth` is defined).
**Delete:** Remove all logic that reads `X-User-Role` or `X-User-ID` headers. Remove logic that treats Bearer tokens as API keys.
**Create:** Create `backend/internal/middleware/jwt_auth.go`.
*   Use `github.com/MicahParks/keyfunc/v3` and `github.com/golang-jwt/jwt/v5`.
*   Implement a `JWTAuth` struct that fetches and caches JWKS from `/.well-known/jwks.json` with background refreshing.
*   The middleware must intercept requests, look for `Authorization: Bearer <token>`, and validate it.
*   Extract custom claims: `tenant_id`, `sub` (user/agent ID), and `roles`.
*   Inject these securely into `context.Context` using custom context keys (e.g., `ContextKeyTenantID`).
*   Provide helper functions: `TenantIDFromContext(ctx) string`, `UserIDFromContext(ctx) string`, `RolesFromContext(ctx) []string`.
*   Still allow `X-API-Key` for internal Temporal workers, but inject a default "system" tenant context.
**Wire:** Update `backend/cmd/identity-service/main.go` to use `JWTAuth.Middleware` instead of `APIKeyAuth.Middleware` for all `/api/v1/*` routes. Ensure CORS middleware no longer allows `X-User-Role`.

### Task 1.3: NATS Event Bus Implementation
**Create:** Create `backend/internal/eventbus/nats.go`.
*   Implement a `NatsBus` struct that connects to NATS, creates a stream `genid-events`, and exposes a `Publish(ctx, event)` method.
**Update:** Open `backend/internal/outbox/processor.go`. After successfully applying an event to Neo4j, call `natsBus.Publish()`. Do not fail the outbox process if NATS publishing fails (NATS is a secondary notification, PG/Neo4j is source of truth).

### Task 1.4: CAEP Broadcast Completion
**Action:** Open `backend/internal/activities/activities.go`. Find `BroadcastCAEPEvent`.
**Rewrite:** Instead of just writing `delivery_status='pending'` to PG, implement an HTTP POST loop.
1. Read the event payload.
2. HMAC-SHA256 sign the payload.
3. POST it to a configured webhook URL (read from env `CAEP_WEBHOOK_URL` for testing).
4. Update `delivery_status` to `delivered` or `failed` based on the HTTP response.

---

## Phase 2: Frontend Cleanup & Navigation

The frontend currently has 14 pages, some with mock data. We need a unified, fancy UI.

### Task 2.1: Global Layout & Sidebar
**Action:** Create or update the global layout component in `frontend/src/app/layout.tsx`.
**UI:** Dark mode default (Tailwind `zinc-900` background). Left sidebar navigation.
**Sidebar Items:**
1.  **Dashboard** (`/dashboard`)
2.  **Identities** (`/identities`)
3.  **AI Agents (NHI)** (`/agents`)
4.  **Access Control** (`/access`)
5.  **Connectors** (`/connectors`)
6.  **Compliance** (`/certifications`)
7.  **Audit Logs** (`/audit`)
8.  **Settings** (`/settings`)

### Task 2.2: Page Directives
*   **`/dashboard`:** Fetch from `/api/v1/connectors/stats` and `/api/v1/audit/stats`. Display 4 top cards: Total Identities, Total Agents, Active JIT Sessions, Critical Revocations. Add a 15s auto-refresh.
*   **`/agents` (The Moat):** Clean up the 700-line file. Display a table of Non-Human Identities. Add a "Register Agent" button that opens a modal. Add a prominent red "Kill Switch" button per agent that fires `POST /api/v1/agents/{id}/kill-switch`.
*   **`/access`:** Remove permanent grant UI. Focus on a "Request JIT Access" form (User, Resource, Duration, Justification). Display a table of "Active JIT Sessions".
*   **`/certifications`:** Remove hardcoded mock data. Display "Pending Reviews" (empty state is fine for now, but UI must be clean).
*   **`/sod` and `/csv`:** Hide these routes from the sidebar. They can remain as admin tools but are not primary nav items.

---

## Phase 3: Agentic Identity & NHI Logic

### Task 3.1: JIT JWT Minting
**Action:** Open `backend/internal/identity_service.go`. Find `JustInTimeAccess` handler.
**Logic:**
1. Receive request (Agent ID, Resource ID, Action).
2. Call Cedar Engine to verify if agent is allowed.
3. If allowed, mint a short-lived JWT (5-minute TTL) with scoped claims (`resource_id`, `action`).
4. Store the JWT `jti` (JWT ID) in Redis with a TTL.
5. Return JWT to the caller.

### Task 3.2: The Kill Switch
**Action:** Verify `AgentKillSwitch` handler in `backend/internal/identity_service.go`.
**Logic:** When called, it must:
1. Update Neo4j `NonHumanIdentity` status to `revoked`.
2. Add the agent's active JWT `jti` to a Redis blocklist (or delete the `jti` key to force instant failure).
3. Trigger Temporal `CascadeRevokeWorkflow` to kill child agents.

---

## Phase 4: Enterprise Data & IGA

### Task 4.1: PostgreSQL RLS (Row Level Security)
**Action:** Open `infrastructure/postgres/init.sql`.
**SQL:** Add `ALTER TABLE identities ENABLE ROW LEVEL SECURITY;` for all tenant-scoped tables.
**SQL:** Create a policy: `CREATE POLICY tenant_isolation ON identities USING (tenant_id = current_setting('app.current_tenant')::uuid);`
**Go:** Update the PG connection pool (`backend/internal/service/postgres.go` or similar) to execute `SET app.current_tenant = '<uuid>';` at the start of every transaction based on the `context.Context` tenant ID.

### Task 4.2: Entity Resolution (Identity Stitching)
**Action:** In the Connector Framework sync logic (`backend/internal/connector/manager.go`), before inserting a new Identity, check for existing identities with the same `email` or `employee_id` but different `source`.
**Logic:** If found, do not create a new PG row. Instead, create a `Persona` node in Neo4j and link both identities to it via a `RESOLVES_TO` relationship.

### Task 4.3: Access Certifications (IGA)
**Action:** Create `backend/internal/workflow/certifications.go`.
**Logic:**
1. Create a Temporal workflow `AccessCertificationWorkflow`.
2. It should query PG for all users with access to critical resources.
3. Create a `certification_campaigns` entry in PG.
4. (For V1, just log the campaign creation. Email sending can be a stub).
**Frontend:** Update `/certifications` page to fetch from a new `/api/v1/certifications` endpoint to display these campaigns.
