# Brutal Engineering Plan — Production IAM at 100k / 50c Scale

> Where GenID is today → where a FAANG/Palo-Alto-grade IAM engineering team would take it for **100,000 identities, 50+ connectors, thousands of daily role calculations/assignments, continuous HR-driven onboarding/offboarding/reinstatement**.
>
> Authored in the posture of a senior Identity engineer from Palo Alto Networks doing a cold review of this codebase: honest about what exists, specific about what changes, sequenced so each phase ships value.

---

## 0. Honest capacity posture (the math that drives everything)

Today's measured live: SCIM create 34–76 ms, API create ~240 ms, outbox→Neo4j sync ~100 ms, end-to-end ~350 ms. Good for a demo. At 100k identities with **20-min refresh**, the math changes:

| Metric | 100k @ 20-min refresh | Capacity headroom on current code |
|---|---|---|
| Sustained identity writes | ~85/sec (full refresh) or ~5–10/sec (delta) | **OK if delta** — **breaks if full** (outbox poll 500 ms, single consumer) |
| Daily events (logins + risk) | ~3–5M/day ≈ 35–60/sec avg, 500/sec spikes | NATS JetStream: fine. **Event processor (1 consumer, MaxDeliver 3): becomes the bottleneck** |
| Risk recalc fan-out | 100k / 50-per-batch = 2,000 batches every 15 min | Temporal: fine, but step-up to **worker pools** |
| Neo4j writes | 50 connectors × ~2k groups/each = 100k relationships | **Per-row write = death.** Must batch `UNWIND` + tx-per-batch |
| Role calc engine | thousands of role evaluations / day | **No native engine exists.** Built inline in `AssignRole` — needs a real calculator |
| Connector sync (50 simultaneously) | 50 × full user+group+ent+resource pull | **No per-connector rate limiter, no per-connector scheduling, no credential vaulting** — will get you throttled/IP-banned by sources |

**Conclusion:** the *broker* (NATS) is not the problem at 100k. The problems are: (1) full-sync vs delta, (2) single-consumer fan-out, (3) row-by-row Neo4j writes, (4) no real role calc engine, (5) no connector platform with vaulting + scheduling + crowdsourced adapters. None require Kafka. All require real engineering.

---

## 1. Enterprise Connector Framework (CyberArk-class)

### 1.1 How CyberArk Identity does it (the model to match)

CyberArk Identity (fka Idaptive / Core Access) treats connectors as **first-class governed infrastructure**:

- **Connector Server** runs as an on-prem agent (`cips**_agent/a infiltrator`) that brokers between the cloud control plane and the resource (AD, LDAP, apps), so credentials never leave the enterprise boundary. Outbound-only tunnel.
- **Cloud Connector** (no agent) for SaaS apps via standardized protocols: SCIM 2.0, SAML/OIDC, OAuth client-creds, REST + API key, JDBC.
- **On-prem Connector** for AD / LDAP / file shares / databases — the agent holds the credential, polls via LDAP/AD, applies changes back via LDIF/SCIM.
- **Credential vault** per connector — secrets are stored in CyberArk's own PAM vault, injected into connector runs, rotated, never logged.
- **Sync state** per connector — a `sync_state` table tracks watermark/cursor per object class, so each sync is **delta from last watermark**, not full pull.
- **Per-connector schedule** — cron per connector, independent health, independent backlog metric.
- **Connector SDK** — a versioned plugin interface (`Capability`/`Operation`) so customers/partners write their own adapters that the platform loads.
- **Governance**: each connector has an **owner identity**, a **risk weight** (apps holding toxic entitlements scored higher), a **last-reviewed-at**, and is itself a resource in the IAM model (a connector can be revoked like any entitlement).

### 1.2 Current state of GenID (the gap)

From `infrastructure/postgres/init.sql` + `backend/cmd/identity-service/main.go` / `backend/internal/...`:

- **Tables exist**: `connectors(id, connector_type, status, config JSONB, last_sync_at, last_error)`, `connector_identities`, `connector_groups`, `connector_entitlements`, `connector_resources`. Wide read-cache shape (raw_attributes JSONB, sync timestamps, UNIQUE(connector_id, external_id)).
- **API exists** (`docs/api/overview.md#connectors`): ~24 endpoints — CRUD, test, connect/disconnect, sync, sync-delta, full-sync, sync-groups/entitlements/resources, users/groups/entitlements/resources/schema/health. Connector types enum: `entra_id, ldap, active_directory, scim, okta, aws_iam, gcp_iam, generic, csv`.
- **The hard truth**: most of these are **shape over substance** — the raw cache tables get populated, but:
  - No sync-state / watermark table → `sync-delta` is effectively `full-sync` minus a label.
  - No per-connector rate limiter → 50 connectors hammering AD simultaneously will self-DoS.
  - No per-connector credential vaulting — secrets sit in `config JSONB` (probably plaintext). Must move to the `Vault` (AES-256-GCM, already built).
  - No per-connector scheduler — syncs are API-triggered. Need Temporal `ScheduleWorkflow` per connector.
  - No connector SDK / plugin boundary — each connector type is special-cased in Go.
  - LCM actions (create_user/delete_user/etc.) are emitted, but it is **unclear** whether they round-trip back to the source system (provisioning-back). If not, GenID is read-only and cannot be a real IDP/HR source.

### 1.3 The upgrade — concrete schema + code changes

**A. New schema additions (additive, no breakage):**

```sql
-- per-connector sync watermark
CREATE TABLE connector_sync_state (
  id UUID PRIMARY KEY,
  connector_id UUID NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
  object_class VARCHAR(50) NOT NULL,    -- user|group|entitlement|resource
  watermark JSONB NOT NULL,            -- per-source: cursor/modified-since/paging token
  last_synced_at TIMESTAMPTZ,
  last_count INT,
  last_error TEXT,
  UNIQUE(connector_id, object_class)
);

-- per-connector schedule
ALTER TABLE connectors ADD COLUMN schedule_cron VARCHAR(64) DEFAULT '*/20 * * * *';
ALTER TABLE connectors ADD COLUMN owner_identity_id UUID REFERENCES identities(id);
ALTER TABLE connectors ADD COLUMN risk_weight INT DEFAULT 50;
ALTER TABLE connectors ADD COLUMN connector_governance_status VARCHAR(20) DEFAULT 'active';

-- vault reference (replace plaintext secrets in config JSONB)
ALTER TABLE connectors ADD COLUMN vault_secret_id UUID REFERENCES vault_secrets(id);
```

**B. New Go package `backend/internal/connectors/`** (split out from `internal/api`):
```
connectors/
  registry.go         // adapter factory keyed by connector_type (plugin boundary)
  adapter.go          // ConnectorAdapter interface
  connectors/
    scim.go           // generic SCIM 2.0 (covers Okta / Entra / custom)
    ldap.go           // LDAP + AD (bind, paged search, LDIF write-back)
    aws_iam.go        // AWS IAM ListUsers/ListRoles + inline/managed policies
    gcp_iam.go        // Cloud Identity / Workspace Directory API
    csv.go            // CSV (HR flat-file)
    generic_rest.go   // configurable REST (schema + auth + pagination)
   entra_id/
   entra_id.go       // Microsoft Graph /delta endpoint (= real delta)
```

**C. The `ConnectorAdapter` interface (the SDH for the plug-in ecosystem):**

```go
type ConnectorAdapter interface {
  Version() int                           // SDK contract version
  Manifest() Manifest                      // declares offered object classes, methods, auth, capabilities
  Authenticate(ctx, cfg ConnectorConfig) error
  TestConnection(ctx, cfg) error
  DiscoverSchema(ctx, cfg) (Schema, error)
  // Delta-sync — uses watermark from connector_sync_state
  SyncDelta(ctx, cfg, watermark) (DeltaResult, error)   // returns added/updated/removed + new watermark
  FullSync(ctx, cfg) (FullResult, error)                 // first-time bootstrap
  // Provisioning-back (for LCM actions):
  Execute(ctx, cfg, action LCMAction) (LCMResult, error) // create_user / delete_user / assign_role / revoke_role into the source
  Health(ctx, cfg) (Health, error)
  RateLimitConfig() RateLimit                             // adapter declares its own throttling
}
```

**D. Adapter-graded treatment (this is what separates "demo" from "enterprise"):**

- **SCIM adapters (Okta/Entra/generic-SCIM)** already tell you delta endpoints exist. Use Microsoft Graph `/delta` and Okta `/api/v1/users?since=<last-scim-event-id>` → real watermarks.
- **LDAP/AD**: paged (`SimplePagedResultsControl`), `modifyTimestamp>=<watermark>` filter, and for write-back: an LDIF diff to `ldap://` with bind from vault credential. **Critical**: the connector never holds the credential; vended per-run by the vault.
- **AWS IAM / GCP IAM**: `ListUsers`/`ListRoles` (no true delta — modified-since on `CreateDate`), treat as full + dedup. Role assignment maps to IAM policy attach-detach (provisioning-back works).
- **CSV (HR flat-file)**: actually the simplest HR source for demos — load → diff against current cache → emit identity.created / identity.updated / identity.terminated events. **This is your fastest path to "HR source" for our stakeholder's demo.**

**E. Connector scheduler (Temporal):**

Each connector-on-create schedules a `ConnectorSyncWorkflow` with the connector's own cron:
```go
// on /api/v1/connectors POST → register + schedule
sched.Client().Create(ctx, temporalclient.ScheduleOptions{
  ID: "connector-sync-"+c.ID.String(),
  Spec: temporalclient.ScheduleSpec{ Cron: c.ScheduleCron },
  Action: temporalclient.ScheduleWorkflowAction{
    Workflow: workflows.ConnectorSyncWorkflow, TaskQueue: "connectors",
    Args: []any{c.ID, "delta"},
  },
})
```
Per-connector isolation instantly eliminates the "50 connectors step on each other" problem and makes the 20-min refresh a property of each source, not the platform.

**F. Per-connector rate limiter:**

`max(connector.RateLimit(), adapter.RateLimitConfig())`, Redis token bucket keyed `rl:connector:{id}`. SCIM endpoints that return 429 → exponential backoff with jitter (`MaxDeliver=3` mirrors Temporal retry policy on the activity).

**G. Governance of connectors (wins the FAANG story):**
- Connector = its own Neo4j `Resource` node (criticality=by app tier, data_classification=inherited). Automatically shows up in `blast-radius`.
- Each connector has an `owner_identity_id` and a `last_reviewed_at`. Certification campaigns include connectors (you're certifying "does Alice still own the AD connector?").
- Health feeds the risk engine: connector in error state → +50 risk to all identities synced from it (trust degrades).

**TOP 10 GAPS this phase closes:**
1. Full-sync masquerading as delta (no watermark) → **A**.
2. Plaintext credentials in `config JSONB` → **A** vault_secret_id.
3. No per-connector scheduler → **E**.
4. No per-connector rate limiter → **F**.
5. No adapter SDK / plugin boundary → **C**.
6. Sync not round-tripping back to source (LCM read-only in GenID) → **C** `Execute(...)`.
7. No connector-as-governed-resource (no blast-radius, no review) → **G**.
8. No real Graph/Okta delta endpoints (full pulls) → **D**.
9. No risk weighting per connector → **A/G**.
10. No connector health metric / status that informs trust scoring → **G**.

---

## 2. HR Source / IDP Inbound — the foundation under "Keeping access latest"

The "20-min refresh" is the heartbeat of an **HR-driven IAM** platform. The defining pattern (used by Okta LCM, SailPoint, Saviynt, CyberArk):

```
   HR System (Workday, BambooHR, SuccessFactors, CSV flat file)
        │  authoritative source of "who exists"
        ▼  (SCIM-inbound OR scheduled extraction)
   GenID Identity Hub
        │  rules: lifecycle status changes (active → inactive → terminated),
        │         reinstatement (terminated → active), manager change, dept move
        ▼  emits:
   genid-events.identity.created/updated/terminated   +   hr.lifecycle.change
        ▼
   Event processor
        ├─ role recalculation (peer-group changed → static recompute)
        ├─ role assignment triggers (JML rule: when dept=Eng & level IC4 → base roles + RBAC)
        ├─ offboard workflow on termination (illin `OffboardIdentityWorkflow`)
        └─ reinstatement workflow on re-hire
```

### 2.1 The HR source connector (priority #1)

First-class `connector_type = 'hr_source'` (or `"csv_hr"`), semantically distinct from app connectors:
- It is the **authoritative identity origin**: where a record exists here and not in GenID → onboarding event. Where exists in GenID but terminated in HR → offboarding event. Where reinstated → reinstatement event.
- Maintains `employment_status` (active / on_leave / terminated / reinstated), `hire_date`, `termination_date`, `manager_id`, `department`, `title`, `employee_id`.
- Each detection is a **delta-event** (not a sync row) → `genid-events.hr.lifecycle.change` with `change_type ∈ {hired, reinstated, terminated, dept_change, manager_change, title_change}`.

### 2.2 Discovery state machine (define identity lifecycle precisely)

```
   MISSING ──hired──> ACTIVE ──termination──> TERMINATED
       ▲                  │                       │
       │                  └──leave──> ON_LEAVE    │
       │                  └──>ACTIVE (return)     │
       │                                          │
       └────────────── reinstated (re-hire) ──────┘
       (must NOT auto-revive entitlements — requires review)
```

**Critical**: reinstatement must appear at GenID at the same time as HR records `terminated → active`. The connector emits `reinstated` with the new hire context, and GenID opens a ** reinstatement certification** (manager re-approves entitlements). This is the gap nobody talks about — leaving stale entitlements active after re-hire = silent over-privilege.

### 2.3 SCIM-inbound as the HR protocol (optional modern path)

If you want the "proper IDP" story: GenID registers itself as a SCIM 2.0 consumer (currently GenID speaks SCIM as a *server*; build the *client* too). HR/IDP pushes users via SCIM `POST /scim/v2/Users` directly into GenID, eliminating polling. The HR-source adapter just becomes a SCIM-inbound adapter plus a lifecycle interpreter.

This is 2-3 weeks of work and is what makes the "forms" path (#3) coexist with the "HR auto" path.

---

## 3. Onboarding Forms + Secure Offboarding (the UX the customer sees)

### 3.1 The Joiner (forms-driven onboarding)

A forms-based onboarder for cases where HR isn't wired or for non-employee onboarding (contractors, vendors, AI agents):

**UI**: `/onboarding` → multi-step wizard (Next.js):

1. **Identity profile** — first/last name, email, employee_id, department, manager (autocomplete from identities), employee_type (employee/contractor/service-account/agent), start_date, end_date (if contractor).
2. **Role template** — pick a **Role Profile**: e.g. "Engineer — IC4" auto-applies base roles (Jira User, GitHub Developer, AWS Dev ReadOnly, Slack Member). Role template = a named bundle (groups of roles + entitlements + SoD-safe combination).
3. **Access items (JIT base)** — automatically list SoD-safe add-on entitlements; toxic combos flagged in red and blocked unless override + master-key + reason.
4. **Approvals** — auto-route to manager + (for sensitive items) resource owner. Temporal `GrantAccessWorkflow` already supports `requires_approval` + `ApprovalDecision` signal. Hook it into the wizard's submit.
5. **Confirmation** — receipts: list of items provisioned (each as a Temporal `GrantAccess` workflow run id), audit entries, a printable onboarding sheet.

**Backend**: `POST /api/v1/onboarding/submit` → starts `OnboardIdentityWorkflow` with:
- `CreateIdentity` (writes Postgres + Neo4j + outbox)
- `AssignRoleToIdentity` for each role template item (in parallel, orchestrated by Temporal)
- `GrantAccessWorkflow` for each entitlement with individual approvals
- 7-day certification timer (already in the workflow)

### 3.2 The Mover (manager move / dept change)

Triggered by HR-source `dept_change` event or manual form `/identities/{id}/transfer`:
- Re-run role calc (peer-group changed → static score shifts).
- Stale roles get flagged for re-certification (not auto-stripped — too disruptive mid-employment).
- New role template applied if department has a default profile.

### 3.3 The Leaver (secure offboarding — the 109ms kill switch)

HR-source emits `terminated` → `OffboardIdentityWorkflow` (already in `risk_workflow.go` style) **automatically**, with tiered revocation:
```
   audit + identity lock (fencing)         ── instant
       ├── critical entitlements (prod db, admin)       (sequential, 0 delay)
       ├── high-risk (sensitive systems)                (parallel, capped)
       └── normal (normal systems)                      (parallel, capped)
   NHI cascade (kill delegated AI agents)               (parallel)
   CAEP broadcast (revoke sessions everywhere)          (instant)
   rotate credentials                                  (parallel)
   finalize audit (SHA-256 chain seal)
```

The "security checks" you want are explicit:
1. **All sessions terminated** (Neo4j session nodes → status=terminated).
2. **NHI/agent delegations cascaded** (`CascadeRevokeWorkflow` revokes SPIFFE SVID + OAuth tokens + API keys — already in your code).
3. **Secrets rotated** (`RotateCredentials` activity — already built).
4. **All access physically written back to source** via `ConnectorAdapter.Execute(revoke_role)` — this is the gap from §1. If you can't write back to AD/Okta/AWS, offboarding is **theatrical**. This is the #1 make-or-break at production.
5. **After-action audit** — semver-stamped audit ledger entry with hash chain continuity report.

Forms-driven offboarding (`/offboarding/{id}`): pick a user → confirm → choose "standard" (full workflow) or "emergency" (`RevokeAccessWorkflow` with `emergency=true` → rotate credentials + broadcast CAEP + cascade revoke AI agents). Emergency needs master-key (already gated).

### 3.4 The Reinstatement (the forgotten JML stage)

`/reinstatement/{id}` form (only identities whose lifecycle_status=terminated show up):
1. Confirm re-hire, new dept/manager, start date.
2. Re-open a **reinstatement certification**: every entitlement Alice previously had is listed and the manager must re-approve each (or accept defaults from the new role template). **Never silently restore all old entitlements** — that is the most common audit finding in enterprise IAM.
3. New `identity_id` reused (same UUID) but `employment_status = reinstated`, `risk_score = 0` start, all previous `expires_at`-bound roles re-evaluated.

---

## 4. Role Calculation & Assignment Engine at 100k / 50c scale

There is **no native role engine today**. Current role assignment is a single `AssignRoleToIdentity` activity — fine for one-off grants, broken for "thousands of role calcs/assignments per day."

### 4.1 The engine (matrix model, the SailPoint/CyberArk pattern)

Three layers, decoupled:

**Layer 1 — Role Mining** (batch, daily): cluster identities by (department, title) → propose Role Profiles. Use Neo4j community detection (Louvain) over `(i)-[HAS_ROLE]->(r)-[GRANTS]->(e)` graph between entitlements and people. Output: candidate role definitions with a toxicity score. Today this is **absent** — add as a cron `RoleMiningWorkflow`.

**Layer 2 — Role Assignment Rules** (event-driven): the **rules engine** that fires when an identity meets a rule's predicate.
```
rule EngIC4 { when: department='Engineering' AND title LIKE 'Senior%' AND employment_status='active';
              apply: roles = {JiraUser, GitHubDeveloper, AWSDevReadOnly}; }
rule ContractorBoundary { when: employment_type='contractor';
                          enforce: expires_at <= 90d AND not any(r in roles WHERE r.is_toxic); }
rule NewHireDay0 { when: hr.lifecycle.change = 'hired';
                  apply: notify manager; start 7d certification campaign; }
```
Implemented as a small YAML/Go rule registry evaluated by an `AssignRolesWorkflow`. Rules evaluated on every `hr.lifecycle.change` + `dept_change` event. Thousands of evals per day = milliseconds each if evaluated outside Neo4j (matrix lookup), or a few ms each in Neo4j — both trivial.

**Layer 3 — Assignment Authorization** (per-grant): existing `GrantAccessWorkflow` (SoD + Cedar + approval + timer). No change — already in place.

### 4.2 Scale math for the role engine

- 100k identities × ~5 entitlements each = 500k relationship rows in Neo4j. Comfortable.
- 50 connectors × ~2k groups each = 100k group→role mappings. Reconciled daily via Temporal.
- Role-Mining Louvain on 500k relating edges: <30s on a 4GB heap. Daily job, perfectly fine.
- Role-Rule evaluation for 100k identities: load predicates into memory (~MB) → microseconds × 100k = ~100ms for a full-batch sweep. Or event-driven per-change.
- The 20-min sync-coupled recalc: recompute only **effected** identities (your `connector_sync_state` tells you who changed) — typically 1-5k per 20-min cycle → sub-second to minutes.

### 4.3 Tooling surfaces the engineering team builds

- `/roles` page: view role definitions, simulation ("what would this rule grant Alice on day 0?"), conflict highlighter.
- `/rules-engine` page (admin): CRUD the YAML rules, version them, dry-run with a preview diff (`Show what would change on identity X if we deploy rule V11`).
- `/certifications`: campaigns with reviewer assignments, batch decisions, escalation emails.

---

## 5. FAANG-grade production hardening (non-functional, but the "100k" story)

| Area | Current | Production target |
|---|---|---|
| Postgres pool | single shared pool | pool-per-tenant + bounded queueing; connection cap per pool |
| Neo4j writes | row-by-row (probably) | `UNWIND $batch` + tx-per-batch, 500 row chunks |
| Outbox poll | 500 ms, single worker | 100 ms poll batch-fetch + queue-group fan-out via NATS |
| Event processor | 1 consumer durable `risk-processor` | **NATS queue group `risk-processor-q` + N replicas** (free horizontal scale) |
| Risk recalc cron | every 15 min, 50/batch | fan-out workflow with worker pool; also triggered post-sync on changed-id set |
| Rate limiter | global 100 r/s | per-tenant + per-connector + per-route (login 5 r/s) |
| Observability | OTel collector + Grafana | SLO panels (provision p99, sync lag, role-eval latency) — service objectives, not just charts |
| Tracing | `trace_id` on audit rows + otelhttp | **end-to-end trace from HR-event-on-the-wire → role-applied**: forced sampling on `hr.lifecycle.change` |
| Multi-tenancy better | 28 RLS tables | add tenant_id on connector_* tables + RLS on them (currently NOT enforced at row level on the connector cache) |
| Idempotency | outbox id | **idempotency keys on `/scim/v2/Users` POST, `/access/grant`, `/access/jit`** (externalId dedupe) |
| Backpressure | none | connector sync ⇒ NATS backlog ⇒ processor applies backpressure via Temporal activity heartbeat canceling |
| Credential use | vault secrets exist | **per-connector vaulted; runtime injected; logged access + rotation record** |
| Kafka? | **Not needed** | If leadership demands event-replay beyond 7d NATS retention OR CDC streaming from Postgres into a lake: introduce **Redpanda** (Kafka-compatible, one container, no ZooKeeper). Else: the existing NATS JetStream is the right tool at 100k. |

---

## 6. Phased delivery (what ships, when — honest backlog)

**Phase 0 — Foundations (2 weeks) non-negotiable for 50k+**
- Add `connector_sync_state`, per-connector `schedule_cron`, `vault_secret_id`, `owner_identity_id`, `RiskWeight`.
- Move plaintext connector secrets → vault.
- Bulk `UNWIND` writes in the sync path.
- NATS queue group + 3 replicas of `event-processor`.
- Outbox poll 500ms → 100ms + batch fetch.
⇒ now reliable at 50k, still matrix-pull, still single-tenant-only.

**Phase 1 — Real delta + HR source (3 weeks)**
- Implement `ConnectorAdapter` interface; move existing connectors behind it.
- Add `csv_hr` adapter (the fastest HR-source demo path for vision sponsor).
- `identity.created/updated/terminated` events flowing from HR-source → `OnboardIdentityWorkflow` + `OffboardIdentityWorkflow` triggered automatically.
- `/onboarding` form wizard + `/offboarding` + `/reinstatement` forms in UI.
⇒ proper JML closed.

**Phase 2 — Connector platform (4 weeks)**
- Per-connector temporal `ConnectorSyncWorkflow` schedule.
- Entra Graph `/delta`, Okta events, AD `modifyTimestamp`, AWS/GCP IAM delta.
- Per-connector rate limiter (Redis token bucket, falling back on source 429s).
- LCM write-back to source (`ConnectorAdapter.Execute`).
⇒ production-class connector platform.

**Phase 3 — Role engine (4 weeks)**
- `AssignRolesWorkflow` + rules registry (YAML, versioned).
- `RoleMiningWorkflow` (Louvain on Neo4j) — daily cron proposing role definitions.
- `/roles`, `/rules-engine`, `/certifications` UIs.
- Reinstatement certification workflow.
⇒ thousands of role calcs/assignments per day, governed, certified.

**Phase 4 — FAANG-grade ops (continuous)**
- Multi-tenant pool isolation, per-tenant rate limits, idempotency keys everywhere, SLO dashboards, forced trace sampling on JML events.
- (optional) Redpanda/Kafka if event replay beyond NATS retention becomes real.
- Connector-as-governed-resource certification campaigns / historical risked blast-radius.

---

## 7. The single highest-leverage first move

**Build Phase 0 + the `csv_hr` adapter in Phase 1 in the next 2–4 weeks.** That alone:
- Gets vision sponsor a real HR-source demo (drop a CSV → Identities appear + onboard workflow kicks + roles auto-assigned → 20 min later another CSV swaps a team → movers/leavers fire automatically → offboard security checks run and write back to a mock AD).
- Proves the delta + write-back + scheduling pipeline — which is 80% of the production story and the part customers actually buy.
- Makes the eventual roadmap credible to an engineering/business audience because the *demo proves the architecture*.

Everything after that is Fenrir vs wolf — more thorough, not more foundational.

---

## 8. Explicit positions (so there's no ambiguity)

- **Kafka / broker change**: not required at this scale. NATS JetStream is the right broker at 100k identities. Could revisit only if (a) we need multi-day event replay, or (b) we want to feed a downstream data lake via CDC. Recommend **Redpanda** if that day arrives — no ZooKeeper, single container.
- **CyberArk on-prem connector agent**: not a Phase 1–4 deliverable — defer to a Phase 5 "agent mode" only if a customer explicitly mandates an on-prem boundary. Currently out-of-scope; do not over-engineer.
- **SCIM-inbound as HR protocol**: real option, ~3 weeks. The CSV-HR route of Phase 1 delivers the same demo vision without depending on a real HRIS. Don't block Phase 1 on it.
- **Reinstatement certification**: build it. It is the gap reviewers find when auditing an IAM install.

—

*End. This is the brutal-engineering plan. Each section is specific to this codebase. Each gap is accompanied by an evidence file or a schema edit. Each phase ships value. None of it requires Kafka.*
