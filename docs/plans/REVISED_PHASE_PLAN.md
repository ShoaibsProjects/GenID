# GenID IAM — Revised Phase Plan (audited 2026-08-15)

> Audit result: all prior phases verified against the live stack (13 containers,
> 24 identities, 6 connectors, 77 hash-chained audit rows, Temporal 1.25).
> Three Phase-2 defects were found and fixed with unit tests (see below).

## Audit findings (fixed)

| # | Defect | Root cause | Fix |
|---|--------|-----------|-----|
| 1 | Firecall rows never persisted after the first | `UNIQUE(tenant_id, type, COALESCE(idempotency_key,''))` made every key-less request collide → `ON CONFLICT DO NOTHING` skipped insert, orphaned Temporal workflows | Migration `003`: partial unique index `WHERE idempotency_key IS NOT NULL`; `CreateRequest` now returns `(created bool, id)` and replays return the existing request (`idempotent_replay: true`) |
| 2 | Firecall status stuck at `pending` | `SET status=$1` (varchar) + `CASE WHEN $1 IN (...)` (text) → Postgres `42P08` inconsistent parameter types | Use the `status` column in the CASE instead of `$1` |
| 3 | Audit hash-chain broken | live `audit_log` predated `prev_hash`/`hash` columns | Migration `003` adds both columns + `idx_audit_hash` |

**Test coverage added:** `internal/workflow/store_test.go` (7 tests, pgxmock) +
`internal/workflow/firecall_workflow_test.go` (4 tests, Temporal testsuite).
Full suite: 8 packages green, `go vet` clean.

## Revised roadmap (based on what is ACTUALLY live)

### Phase 0 — Platform Foundations ✅ DONE
- Connector sync (HR, SCIM, OIDC), outbox + queue-group fan-out, Postgres vault,
  risk engine (formula + bands), 13-container compose stack.

### Phase 1 — Core IGA ✅ DONE
- HR-sync JML pipeline (Onboard/Offboard/CascadeRevoke), access grant/revoke/JIT,
  certification campaigns, CAEP events, risk-recalc cron, SoD detection, 18 UI pages.

### Phase 2 — Identity Automation (IN PROGRESS)
**2.1 Done**
- ✅ `workflow_requests` / `workflow_approvals` / `workflow_audit` tables
  (migration 002 + fix 003)
- ✅ `workflow.Store` CRUD + 11 unit tests
- ✅ Firecall break-glass end-to-end: auto-grant → expiry/early-revoke →
  mandatory 7d post-review → auto-escalate (pulled forward from old Phase 3)
- ✅ `/requests` UI (list, filter, audit trail) — rebuilt with `React.createElement`
  to dodge the SWC parser bug
- ✅ Idempotency-safe submission (`Idempotency-Key` header → replay returns same request)

**2.2 DONE — Approval routing engine**
- ✅ Routing rules: `EvaluateRouting` (approval_router.go) — risk band + resource
  escalation (database/infra/payment/pii/production +1, secrets +2);
  minimal→auto-approve, low→1 owner, elevated/high→2 (owner + security_admin),
  critical→3 (+ciso); sequential + parallel chains; 9 unit tests
- ✅ `workflow_approvals` lifecycle: `CreateApprovalChain` (idempotent batch),
  `DecideApproval` (immutable), `ApprovalsPendingFor`, `GetApproval`,
  `ListPendingApprovals` (inbox) — store + pgxmock tests
- ✅ `ApprovalGateWorkflow` (approval_gate.go): child workflow, one level open at
  a time (parallel = same-level all open), per-level timer from `due_at`,
  `ApprovalDecision` signal, auto-deny on timeout, audit
  `approval.completed/denied/timed_out`; 5 unit tests
- ✅ Wire `GrantAccessWorkflow` through the store + gate: request row created
  (status `pending_approval`), risk band computed, gate runs as child
  `approval-gate-<requestID>`, denial → `denied`, gate failure → `failed`,
  approval → `approved` → provision → `executed`; idempotent replay
- ✅ API: `POST /approvals/{id}/decide` (persists + signals gate; gate
  reconciles store on signal race), `GET /approvals/queue` (inbox)
- ✅ E2E validated: high-risk grant → chain pending → decide → `executed`,
  full audit trail; test data cleaned

**2.3 DONE — Approval inbox UI**
- ✅ `/inbox` page (approve/deny with comment, live poll) — nav entry added,
  built with `React.createElement` (SWC workaround); API client fns added to `@/lib/api`
- ✅ `failure_reason` persisted on deny (e.g. "denied at level 1 by <id>: denied by approver")
- ✅ Inbox deterministic ordering (`level ASC`)

**2.4 DONE — Approval ecosystem**
- ✅ Webhook notifications: `internal/notify` (Slack/Teams incoming webhooks),
  events `approval.required/decided/timed_out`; wired into gate workflow via
  `NotifyApproval` activity; degrades to log-only if no webhooks configured
- ✅ JIT expiry persistence: request row created with `expires_at`,
  status `pending` → `executed` → `completed`/`revoked`; audit
  `jit.granted`/`jit.expired`/`jit.revoked`; idempotent submission
- ✅ Approval delegation: `POST /approvals/{id}/delegate` reassigns pending
  approvals; store `DelegateApproval`; audit `approval.delegated`; inbox
  immediately reflects new approver
- ✅ Self-service catalog: `GET /catalog/roles` (requestable roles:
  `approval_required && is_active && !is_auto_assigned`),
  `POST /access/request-role` routes through GrantAccessWorkflow +
  approval gate; reuses existing provisioning; idempotent + risk band

### Phase 3 — Break-Glass & PAM (partially pulled into 2.1)
- [x] Firecall + post-review (DONE)
- [ ] PAM session recording + dual-control
- [ ] Service account governance (rotation, ownership)
- [ ] Cloud role assumption (AWS STS, Azure PIM)
- [ ] Compliance evidence pack generation

### Phase 4 — Zero Standing Privilege
- [ ] ZSP mode / CIEM integration
- [ ] Event-driven auto-remediation (risk>700 → auto-revoke, half-built in RiskAlertWorkflow)
- [ ] Custom workflow DSL (YAML → Temporal)
- [ ] Drift detection + least-privilege recommender

### Phase 5 — CIEM + SSPM
- [ ] AWS/Azure/GCP entitlement sync
- [ ] SaaS posture (Prisma Cloud / SSPM)
- [ ] Cross-tenant governance

## Guardrails carried forward
- **No GitHub pushes.** Local workspace only.
- **SWC parser bug in Next.js:** new component files with multi-line JSX often
  fail `Unexpected token`. Workaround: `React.createElement` or simple JSX.
- **Demo URL:** http://localhost:3001 (frontend → identity-service:8080).
- **Dev login:** `POST /api/v1/dev/login` `admin@genid.io` / `dev-login`.