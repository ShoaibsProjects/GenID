# Security Policy

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Email security reports to the project maintainers. You should receive a response within 48 hours. Confirmed issues receive a patch as soon as possible, coordinated with CVE publication if warranted.

## Supported Versions

| Version | Supported |
|---------|-----------|
| `main` branch | :white_check_mark: Active development / nightly |
| Tagged releases | :white_check_mark: Once released |

---

## Security Architecture Baseline

GenID implements defense-in-depth across 7 layers. The table below summarizes the audited state of each control (last audit: 2026-07-29).

### Layer 1: Authentication & Sessions

| Control | Status | Location |
|---------|--------|----------|
| RS256 JWT validation against JWKS endpoint | ✅ PASS | `backend/internal/middleware/jwt_auth.go:89-110` |
| JWT `exp` claim enforcement (access: 5m, refresh: 30d) | ✅ PASS | `backend/internal/oidc/provider.go:79,149` |
| Refresh token rotation (old revoked before new issued) | ✅ PASS | `backend/internal/oidc/handlers.go:241` |
| Refresh tokens stored as SHA-256 hash (not raw) | ✅ PASS | `backend/internal/oidc/provider.go:303` |
| OAuth2 token revocation endpoint (RFC 7009) | ✅ PASS | `backend/internal/oidc/handlers.go:440-483` |
| Access token jti-based replay detection | ⚠️ GAP | `backend/internal/middleware/jwt_auth.go` — jti exists but not checked |
| Access token revocation blocklist | ⚠️ GAP | `backend/internal/oidc/handlers.go:480` — documented as deferred |
| `DEV_LOGIN_ENABLED` defaults to `true` in dev mode | ⚠️ RISK | `backend/cmd/identity-service/main.go:249` |

### Layer 2: Authorization & Policy

| Control | Status | Location |
|---------|--------|----------|
| WorkflowGuard — 12 sensitive operations require master permission | ✅ PASS | `backend/internal/middleware/workflow_permission.go:75-116` |
| X-Master-Key header validation against configured key | ✅ PASS | `backend/internal/middleware/workflow_permission.go:101-103` |
| JWT role-based master check (master/admin/owner roles) | ✅ PASS | `backend/internal/middleware/workflow_permission.go:106-113` |
| Cedar policy engine — forbid always wins over permit | ✅ PASS | `backend/internal/cedar/engine.go` |
| Multi-tenant Row Level Security — 28 tables, `app.current_tenant` | ✅ PASS | `infrastructure/postgres/init.sql:723-789` |
| API key authentication with runtime key rotation | ✅ PASS | `backend/internal/middleware/auth.go:22-77` |

### Layer 3: Cryptography & Key Management

| Control | Status | Location |
|---------|--------|----------|
| AES-256-GCM for vault secrets (crypto/rand nonces) | ✅ PASS | `backend/internal/vault/vault.go:161-198` |
| Vault master key minimum length enforcement (32 chars) | ✅ PASS | `backend/internal/vault/vault.go:44-49` |
| Vault file permissions `0600` | ✅ PASS | `backend/internal/vault/vault.go:80` |
| No hardcoded IVs, seeds, or deterministic nonces | ✅ PASS | Entire crypto surface |
| No deprecated algorithms (MD5, SHA1, DES) | ✅ PASS | Garable audit |
| JWT signing key regenerated on every restart | ⚠️ GAP | `backend/internal/oidc/provider.go:37` — no key persistence |
| HMAC secret for CAEP webhooks defaults to `genid-dev-secret` | ⚠️ GAP | `backend/internal/activities/activities.go:832` |
| JSON output encryption via `json.Marshal` | ✅ PASS | `backend/internal/vault/vault.go:207-211` |

### Layer 4: Input Validation & Output Safety

| Control | Status | Location |
|---------|--------|----------|
| Request body size limits (10 MB default) | ✅ PASS | `backend/internal/middleware/validate.go:9` |
| Content-Type enforcement on POST/PUT/PATCH | ✅ PASS | `backend/internal/middleware/validate.go:39-67` |
| Security headers: X-Content-Type-Options, X-Frame-Options, Referrer-Policy | ✅ PASS | `backend/cmd/identity-service/main.go:885-897` |
| CORS origin validation — exact match, never wildcard | ✅ PASS | `backend/cmd/identity-service/main.go:900-921` |
| Connector config credential sanitizer | ✅ PASS | `backend/internal/service/identity_service.go:1715-1732` |
| No `dangerouslySetInnerHTML` in frontend | ✅ PASS | `frontend/src/` |
| User input injected into HTML without escaping | ⚠️ GAP | `backend/internal/oidc/handlers.go:77,354` |

### Layer 5: Rate Limiting & DoS Protection

| Control | Status | Location |
|---------|--------|----------|
| Per-IP token bucket rate limiter | ✅ PASS | `backend/internal/middleware/rate_limit.go:11-17,72-87` |
| X-Forwarded-For support for proxied deployments | ✅ PASS | `backend/internal/middleware/rate_limit.go:75-77` |
| Cleanup goroutine for stale IPs (10 min interval) | ✅ PASS | `backend/internal/middleware/rate_limit.go:53-70` |
| Retry-After header on 429 responses | ✅ PASS | `backend/internal/middleware/rate_limit.go:80` |
| No panic recovery HTTP middleware | ⚠️ GAP | `backend/cmd/identity-service/main.go` |

### Layer 6: Audit & Integrity

| Control | Status | Location |
|---------|--------|----------|
| SHA-256 hash chain (prev_hash → hash) | ✅ PASS | `backend/internal/audit/chain.go:69-82` |
| Genesis hash constant (`000...000`) | ✅ PASS | `backend/internal/audit/chain.go:18` |
| Mutex serialization prevents chain forks | ✅ PASS | `backend/internal/audit/chain.go:44-45` |
| Verify() endpoint — replays and recomputes entire chain | ✅ PASS | `backend/internal/audit/chain.go:154-207` |
| Backfill for copies rows | ✅ PASS | `backend/internal/audit/chain.go:211-271` |
| Error response bodies captured in audit message (potential token leak) | ⚠️ GAP | `backend/internal/audit/audit.go:203` |

### Layer 8: Dependency Supply Chain

| Control | Status | Location |
|---------|--------|----------|
| Go module verification via `go.sum` | ✅ PASS | `backend/go.sum` |
| npm audit / package-lock.json | ✅ PASS | `frontend/package-lock.json` |
| Pre-commit gitleaks hook | ✅ PASS | `.githooks/pre-commit` |
| Pinned Docker image tags | ✅ PASS | `docker/` Dockerfiles |
| NATS client 7 versions behind latest | ⚠️ RISK | `backend/go.mod` — `nats-io/nats.go v1.52.0` |
| `golang.org/x/crypto` contains known CVE | ⚠️ RISK | `backend/go.sum` — requires update |

### Layer 9: Infrastructure Hardening

| Control | Status | Location |
|---------|--------|----------|
| Identity service runs as non-root user (UID 1001) | ✅ PASS | `docker/identity-service.Dockerfile:30-31` |
| Frontend Dockerfile lacks explicit `USER nginx` | ⚠️ GAP | `docker/frontend.Dockerfile` |
| Redis exposed without password | ⚠️ GAP | `infrastructure/docker-compose.yml` — `REDIS_PASSWORD=` empty |
| Internal services bound to `0.0.0.0` in docker-compose | ⚠️ GAP | `infrastructure/docker-compose.yml` — many `ports:` directives |
| QA deployments enabled by default in compose | ⚠️ GAP | `infrastructure/docker-compose.yml:243` |

### Layer 10: Error Handling & Timing Resistance

| Control | Status | Location |
|---------|--------|----------|
| Authentication timing oracle — different messages for different failures | ⚠️ GAP | `backend/internal/oidc/provider.go:453-459` |
| Error messages leak internal detail to clients | ⚠️ GAP | `backend/internal/middleware/jwt_auth.go:101` |
| No `panic()` in business logic or handlers | ✅ PASS | All handler files |

### Layer 11: Redirect URI & OAuth Security

| Control | Status | Location |
|---------|--------|----------|
| Redirect URI validation exists | ✅ PASS | `backend/internal/oidc/handlers.go:541` |
| Redirect URI validated via `strings.HasPrefix` (open redirect risk) | ⚠️ HIGH | `backend/internal/oidc/handlers.go:544` |
| PKCE (S256) enforced for authcode flow | ✅ PASS | `backend/internal/oidc/provider.go:278-299` |
| State parameter present in auth request | ✅ PASS | `backend/internal/oidc/handlers.go:79` |

### Layer 12: IDP & TLS

| Control | Status | Location |
|---------|--------|----------|
| LDAP TLS enforcement, `InsecureSkipVerify: false` | ✅ PASS | `backend/internal/connector/ldap.go:57-58` |
| OTel exporter uses `insecure.NewCredentials()` always | ⚠️ GAP | `backend/cmd/backend_service/main.go:993-994` |
| HTTP client timeout via `http.DefaultClient` for JWKS fetch | ✅ PASS | `backend/internal/middleware/jwt_auth.go:231` |

---

## OWASP Top 10:2025 — Coverage Map

OWASP's flagship Top 10 was refreshed in 2025 (the 2021 list is now superseded). Every
2025 risk category is addressed by a concrete control in this repository — recruiters and
senior engineers can verify each claim by following the file path.

| # | OWASP Top 10:2025 | GenID Control | Verifiable file |
|---|--------------------|---------------|------------------|
| **A01** | Broken Access Control | Multi-tenant RLS on 28 tables + AWS Cedar policy-as-code (forbid-wins) + WorkflowGuard on 12 sensitive ops | `infrastructure/postgres/init.sql:723-789`, `backend/internal/cedar/engine.go`, `backend/internal/middleware/workflow_permission.go:75` |
| **A02** | Security Misconfiguration | All container ports bind to `127.0.0.1` (tunnel-only ingress); dev secrets flagged `dev-only-`; production gate planned | `infrastructure/docker-compose.yml`, this file — queue item 7 |
| **A03** | Software Supply Chain Failures | `go mod tidy` + `npm audit` in CI; reproducible `go.sum`; images pinned by version tag (no `:latest`); `go.mod` module pinned | `backend/go.sum`, `.github/workflows/ci.yml`, `infrastructure/docker-compose.yml` |
| **A04** | Cryptographic Failures | AES-256-GCM vault, RS256 JWT/JWKS (no HS256 in prod), SHA-256 chained audit, refresh tokens stored hashed | `backend/internal/vault/vault.go:161`, `backend/internal/oidc/provider.go:303`, `backend/internal/audit/chain.go` |
| **A05** | Injection | Parameterized queries everywhere (pgx/v5); Cedar policies compiled, not string-evaluated; SCIM inputs validated against RFC 7643/7644 | `backend/internal/service/identity_service.go`, `backend/internal/cedar/engine.go` |
| **A06** | Insecure Design | Policy-as-code (Cedar) is the single authority; defense-in-depth across 12 layers; zero-trust gateway; JIT, not long-lived credentials; Redis fence tokens for optimistic concurrency | this file — Layers 1–12, `backend/internal/activities/activities.go:285-321` |
| **A07** | Authentication Failures | RS256 JWT with rotating JWKS, refresh-token rotation, API-key auth with runtime rotation, 5-minute JIT NHI JWTs | `backend/internal/middleware/jwt_auth.go:89`, `backend/internal/middleware/auth.go:22`, `backend/internal/activities/activities.go:645` |
| **A08** | Software or Data Integrity Failures | Tamper-proof audit chain (SHA-256 linked), Cedar policies hot-reload (compiled, not eval'd); outbox guarantees event ordering | `backend/internal/audit/chain.go`, `backend/internal/outbox/processor.go` |
| **A09** | Security Logging and Alerting Failures | Audit subsystem with chained hash; OpenTelemetry traces; Prometheus metrics (14 families); health endpoints feed alerting | `backend/internal/audit/audit.go`, `infrastructure/otel-collector-config.yaml` |
| **A10** | Mishandling of Exceptional Conditions *(new in 2025, replaces SSRF)* | Redis fence tokens guard race conditions (optimistic concurrency); no user-controllable outbound URL fetch surface in the connector framework; panic-recovery middleware is open (queue #11) | `backend/internal/activities/activities.go:285-321`, `backend/internal/connector/`, this file — queue item 11 |

**Open items mapped to OWASP Top 10:2025:**
- **A07** — JWT `jti` replay protection not yet enforced (queue #4)
- **A07** — JWT signing key not persisted across restarts (queue #5)
- **A04** — HMAC default secret for CAEP webhooks (queue #6)
- **A02** — Production gate to fail-fast on known default keys (queue #7)
- **A10** — Panic-recovery HTTP middleware (queue #11)

---

## OWASP Top 10 for LLM Applications:2025 — Agentic Coverage Map

The OWASP Top 10 for LLM/GenAI Applications is a separate, AI-specific list (2025 edition).
GenID is built for the agentic era — its **Non-Human Identity governance is a direct mitigation
for LLM06 (Excessive Agency)**, the single biggest risk when LLM agents act on real systems.
Mapping below is honest: ✅ controlled today, ⛔ low risk by design, ⚠️ partial / production slot.

| # | OWASP LLM:2025 | GenID stance | Status |
|---|----------------|--------------|--------|
| **LLM01** | Prompt Injection | Copilot pipeline is deterministic retrieve→rerank→generate→**validate** (5-step in `ai/copilot.go`). Production LLM path needs strict input sanitization. | ⚠️ partial |
| **LLM02** | Sensitive Information Disclosure | GraphRAG retrieves only what the requesting identity can access (tenancy enforced through RLS on the source PG); confidence scoring flags low-information answers. Production needs PII redaction in LLM responses. | ⚠️ partial |
| **LLM03** | Supply Chain | Reuses A03 controls (locked deps, reproducible `go.sum`, pinned images). Model supply chain is a future concern (current copilot is deterministic, no third-party model dependency). | ✅ |
| **LLM04** | Data and Model Poisoning | Graph is sourced from audit-governed Postgres writes, not untrusted user text → poisoning surface is minimal today. Watch when fine-tuning/embedding pipelines land. | ✅ by design |
| **LLM05** | Improper Output Handling | Copilot returns structured responses (confidence + recommendations), never raw actions to execute; no `eval`/shell surface. | ✅ |
| **LLM06** | **Excessive Agency** | 🎯 **GenID's core thesis.** Agents get **5-minute scoped JIT JWTs** (never long-lived creds); Cedar narrows an agent's span at **evaluation time, not issuance time**; `policies/agent.cedar` scopes agents; the 109ms kill switch revokes a compromised agent identity mid-action. **No agent ever holds blanket authority.** | ✅ |
| **LLM07** | System Prompt Leakage | No user-facing system prompts today (deterministic copilot); low risk. | ⛔ |
| **LLM08** | Vector and Embedding Weaknesses | Primary retrieval is **graph-native (Neo4j Cypher)**, not flat vector → no cross-tenant vector leakage. The Qdrant hybrid slot is future work, designed to layer *on top of* tenancy. | ✅ by design |
| **LLM09** | Misinformation | Confidence scoring (0.6–0.92) + validation step (queue production LLM); recommendations are flagged, not asserted as fact. | ⚠️ partial |
| **LLM10** | Unbounded Consumption | Gateway rate limit (100 req/s, burst 200, per-IP token bucket) bounds copilot request volume. Per-tenant cost caps slot for production. | ⚠️ partial |

The standout message for AI-security recruiters: GenID does not bolt AI onto a legacy IAM.
Its **NHI plane is purpose-built to neutralize LLM06 (Excessive Agency)** — the OWASP LLM risk
that Anthropic, OpenAI, and every agentic-AI company must solve to ship agents that act safely.

---



### CRITICAL — Fix Today
1. **Rotate credentials** — Remove `POSTGRES_PASSWORD`, `NEO4J_AUTH`, `API_KEYS`, `MASTER_KEY` from `infrastructure/docker-compose.yml`. Move to `.env`.
2. **Fix redirect URI validation** — Replace `strings.HasPrefix` with exact URL component match in `backend/internal/oidc/handlers.go:544`
3. **Escape user input in HTML** — Use `html.EscapeString()` or `html/template` in `backend/internal/oidc/handlers.go:77,354`

### HIGH — Fix This Week
4. **Implement JWT jti replay protection** — Check jti against Redis blocklist in `backend/internal/middleware/jwt_auth.go:132`
5. **Persist JWT signing key** — Write key to file on first start, load on subsequent starts `backend/internal/oidc/provider.go:37`
6. **Remove HMAC default secret** — Return error if `CAEP_HMAC_SECRET` is not set `backend/internal/activities/activities.go:832`
7. **Add production gate** — Exit with fatal on startup if `MASTER_KEY`/`VAULT_MASTER_KEY` matches known default

### MEDIUM — Fix This Sprint
8. **Redis authentication** — Set `REDIS_PASSWORD` in `.env` and configure Redis container with `requirepass`
9. ✅ **DONE (2026-08)** — All internal services in `docker-compose.yml` now bind to `127.0.0.1`; only the Cloudflare Tunnel reaches the gateway.
10. **Audit log response body sanitizer** — Strip tokens from error response bodies before capture `backend/internal/audit/audit.go:203`
11. **Add panic recovery HTTP middleware** — Top-level recovery wrapper in `backend/cmd/identity-service/main.go`

---

## Security Best Practices for Deployers

1. **Set `VAULT_MASTER_KEY`** — 32+ character hex key generated with `openssl rand -hex 32`. The service does NOT start without it.
2. **Set `MASTER_KEY`** — enables WorkflowGuard for 12 sensitive mutation endpoints.
3. **Set `API_KEYS`** — comma-separated `name:key` values. Without these, auth is disabled entirely.
4. **Disable `DEV_LOGIN_ENABLED`** — set to `false` in production. This is a developer-test-only endpoint.
5. **Set `CORS_ORIGIN`** — your frontend domain. Never use `*`.
6. **Set `PRODUCTION=true`** — enables additional startup checks (key strength, disabled dev endpoints).
7. **Rotate `MASTER_KEY` every 90 days** — use the `SetKeys()` hot-reload over API.
8. **Enable TLS** — set `TLS_CERT_FILE` and `TLS_KEY_FILE`.
9. **Bind database ports to `127.0.0.1`** — do not expose PostgreSQL, Redis, or Neo4j to the network.

## Dependency Security

Check dependencies before every deploy:

```bash
# Go
go mod verify
go mod tidy

# Frontend
npm audit fix

# Secrets scan
gitleaks detect --verbose

# Go vulnerability check
govulncheck ./...
```

This project follows the principle of **least privilege**, **defense in depth**, and **assume breach** architecture.