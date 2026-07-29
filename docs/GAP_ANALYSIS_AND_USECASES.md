# GenID — Gap Analysis & Use Cases
**Date:** 2026-07-28 · **Scope:** Full codebase audit (`backend/internal`, `frontend/src/app`) + competitive benchmark + risk math architecture
**Method:** Every backend handler and frontend page was read. Live dummy tests were executed against the running system and are cited as `[LIVE TEST]`.

---

# Part 0: Critical Architectural Findings (Read First)

These four findings sit underneath everything else in this document. They explain why the system *looks* complete but does not yet deliver product value.

## 0.1 The system has TWO disconnected graph models

| | Model A — "Governance Fabric" | Model B — "Runtime Access Fabric" |
|---|---|---|
| Shape | `Identity → HAS_ROLE → Role → DIRECTLY_OWNS → Entitlement → ACCESSES → Resource` | `Identity → HAS_DIRECT_ACCESS / HAS_TEMPORARY_ACCESS → Resource {id}` |
| Used by | Blast radius, certifications, SoD scan | Access check, Grant/Revoke/JIT workflows |
| Resource key | `Resource.name` / `uuid` | `Resource.id` |

**Consequence `[LIVE TEST]`:** After seeding `admin → Platform Admins → aws-prod-deploy → res-aws-prod`, the blast-radius graph shows the path perfectly, but `POST /api/v1/access/check` returns `{"allowed": false, "reason": "no_entitlement_path"}`. Granting a role never makes access-check allow; granting direct access never appears in blast radius. **The "Identity Fabric" is currently two fabrics that never intersect.** A unified path semantic (CheckAccess evaluating the same chain blast-radius draws) is the single highest-leverage fix in the entire codebase.

## 0.2 The risk score is a static constant, not an engine

`[LIVE TEST]` Every human identity is created with `risk_score = 0.0` (`identity_service.go:3013`), every agent with `risk_score = 0.3` (`activities.go:1000`). Nothing ever recomputes them. The UI faithfully color-codes these constants (green/amber/red), which creates the *appearance* of a risk engine where none exists. The entitlement-level mapping `low→0.4 / med→0.7 / high→1.0` exists only inside `QueryIdentityEntitlements` and is never aggregated. Part 3 specifies the real engine.

## 0.3 Frontend↔backend contract drift is the recurring failure mode

Fixed this week and worth remembering as a pattern: agents API returned `uuid` while the UI read `id`; `is_governed`/`risk_score` arrived as strings; `RegisterAgent` required `agent_type` + a hard owner `MATCH` while the UI sent `type` + `owner_name`; pages used raw `fetch()` without JWT → 401s; dev-login tokens lacked the `admin` role → every guarded button 403'd. **Rule going forward: every new endpoint ships with its TypeScript interface in `api.ts` in the same commit.**

## 0.4 What is genuinely strong (do not lose this)

- Temporal workflow spine (9 workflows incl. cascade revocation, JIT dual-selector, certification campaign) — this is beyond what early-stage competitors demo.
- Tamper-evident audit ledger (SHA-256 chain, live-verified: UPDATE and DELETE tampering both detected with row IDs).
- OIDC provider in-process (RS256, JWKS, device code, PKCE) + JIT token minting with Redis blocklist on kill switch.
- RLS-ready PostgreSQL schema with tenant isolation policies written (enforcement needs the `SET app.current_tenant` discipline in every tx).
- Neo4j graph queries for blast radius work for humans *and* agents.

---

# Part 1: Comprehensive UI Module Audit

Legend: ✅ works · 🟡 exists but degraded/hardcoded · ❌ missing

## 1.1 Dashboard (`/dashboard`)
**Existing:** 4 stat cards (identities, agents, JIT sessions, critical revocations), system health panel, data-layer panel, architecture map, 15s auto-refresh.
**Missing:** time-series charts (recharts is installed but unused), top-risk identities list, recent critical events feed, "pending certifications" widget, SoD violations widget, connector sync failures widget, customizable layout/pinning.
**Hardcoded:** "Critical Revocations"/"Active JIT Sessions" read keys (`active_jit`, `critical_revocations`) that `/api/v1/audit/stats` does not emit → always show "—" 🟡. Architecture/Data-layer panels are static text.

## 1.2 Identities (`/identities`)
**Existing:** search (debounced), status/type/source/department filters, sortable columns, pagination (page sizes), add modal, delete, detail slide-out (profile, attributes, entitlements, blast radius list), per-row Blast button → full-screen force graph.
**Missing:** bulk select + bulk actions (terminate, suspend, export selected), column show/hide, saved views, risk-score explanation tooltip (score is static — see 0.2), "view persona/correlated accounts" (Persona data exists in Neo4j, no UI), identity timeline (JML events), manager-view ("my team"), orphan-account filter, dormant filter (>90d — `last_accessed_at` exists but is never written).
**Hardcoded:** type/source/status filter lists are fixed arrays — should derive from data.

## 1.3 AI Agents (`/agents`)
**Existing:** table (name/type/status/risk/governed/owner), register modal, kill switch with confirm + toast, explainer card.
**Missing:** agent detail drawer (agent card/A2A document exists at `GET /agents/{id}/card` — no UI), delegation chain viewer (`DELEGATED_FROM` edges exist), permission-boundary editor, anomaly findings list (`ScanAgentBehavior` produces findings — no UI surface), "govern this agent" toggle, expiry/rotation scheduling (`expires_at` in schema), per-agent session/JIT token list, human-in-the-loop threshold config.
**Hardcoded:** agent types list (4 options) vs backend's 7 NHI types.

## 1.4 Access Control (`/access`)
**Existing:** Check Access tab (identity/resource/action → allowed/denied + latency), JIT Request tab (duration presets, reason, workflow link to Temporal UI), how-it-works cards.
**Missing:** **active JIT sessions table** (page promises it; no endpoint lists live grants — need `GET /api/v1/access/jit/active` from Redis scan), revoke-JIT-early button (workflow supports the signal!), standing-access grants list, access request *approval inbox* (`SendApprovalRequest` activity exists — no inbox UI, approvals never actually gate anything), policy simulator ("what-if" editor against Cedar), access request history per identity.

## 1.5 Certifications (`/certifications`)
**Existing:** campaign generation button, campaign + entries table (identity, resource, risk score), Approve/Revoke per entry (writes PG, audit-logged), risk badge.
**Missing:** campaign scheduler (cron — the workflow supports `quarterly/triggered/emergency` but nothing schedules it), multi-tier review (manager → app owner → security), reviewer assignment, due dates + reminders/escalation, delegation ("reassign to…"), bulk decide, progress bar per campaign, micro-certifications (event-triggered single-access review), self-certification flow, decision comments viewer (notes stored, not displayed).

## 1.6 Audit Logs (`/audit`)
**Existing:** live tail (3s), level/method/status/path/IP/time filters, pagination, detail modal, ring-buffer stats, **integrity pill (live `/audit/verify`) + per-row chain hash**.
**Missing:** saved searches, export (CSV/JSON/CEF for SIEM), event-type view (ledger has workflow events — UI shows only HTTP), actor-centric view ("everything user X did"), alert rules (e.g., "pager on 5xx burst"), long-term archive (ring buffer drops at 10k).

## 1.7 Connectors (`/connectors`)
**Existing:** responsive card grid (name/type/status), Sync Now.
**Regression note:** the previous full-featured version (add/test/delete/expand tabs for accounts/groups/entitlements/resources/schema) was simplified per spec — those capabilities exist in the **backend** (15+ connector endpoints) and should return as a card → detail drawer.
**Missing (UI only, backend has most):** add/edit connector wizard, test-connection, field mapping editor (**the single most important missing UI in the product** — see 2.3), sync schedules, sync history/logs per connector, dry-run mode, attribute-level conflict resolution (source-wins vs target-wins), HR-source designation toggle ("this connector is authoritative").

## 1.8 Policy Engine (`/policies`)
**Existing:** Cedar code block (hardcoded sample — `GET /api/v1/policies` does not exist), Hot Reload pill (true — Cedar reloads from PG every 30s).
**Missing:** everything — list real policies from `cedar_policies` table, create/edit/disable, policy test/simulator, version history, decision log ("which policy allowed/denied this check").

## 1.9 Settings (`/settings`)
**Existing:** API URL override, static system status list.
**Missing:** tenant management, API key management (issue/revoke — keys are env-only), risk-weight sliders (Part 3), notification channels (email/webhook/Slack), OIDC client management UI (endpoint exists, `/idp` page is hidden), branding, data retention config.
**Hardcoded:** system status is static text, not live 🟡.

## 1.10 Hidden pages (`/groups`, `/sod`, `/vault`, `/csv`, `/idp`)
All exist with real functionality but no nav entry. `/sod` is fully static text 🟡 despite `DetectSoDViolationsWorkflow` existing in the backend — worst value-to-effort ratio in the app: wire it to a real endpoint.

---

# Part 2: Core IGA/IAM Capability Grades

| # | Pillar | Grade | What works | What's missing |
|---|---|---|---|---|
| 1 | **JML Workflows** | **4/10** | `/api/v1/lcm` executes 12 provisioning actions (create/update/delete/enable/disable user, group ops, assign/revoke role, full sync); Onboard/Offboard Temporal workflows; LCM history endpoint (in-memory). | No trigger config (attribute-change → action), no scheduled transitions, no mover template (revoke-old + grant-new atomically), no pre-hire/future-dated joiner, no UI at all for building JML rules, history lost on restart. |
| 2 | **Correlation & Identity Resolution** | **4/10** | Real stitching: connector sync matches on email/employee_id → creates `Persona` node + `RESOLVES_TO` edges (`manager.go:730+`). | Match only fires during sync; no fuzzy/weighted matching config, no manual merge/split UI, no Persona viewer, no orphan-account report (accounts with no Persona), no correlation preview ("this import will create 12 new, match 48"). |
| 3 | **Provisioning (inbound/outbound)** | **6/10** | 5 connector types (SCIM, Entra, LDAP/AD, CSV, generic); full/delta sync; groups/entitlements/resources/schema reads; write-back via provisioning engine actions. | No outbound provisioning triggers from JML events (engine runs only via explicit `/lcm` call), no attribute transformation language, no retry queue w/ dead-letter view, no per-object provisioning status (who got created where), SOAP/DB-table connectors absent. |
| 4 | **HR Source Integration** | **3/10** | CSV import w/ column mapping preview (`/csv` page + preview endpoint). | No Workday/SuccessFactors/BambooHR connectors, no "authoritative source" concept in UI, no hire-date/future-start handling, no manager-chain derivation from HR feed, no delta-import diff preview before apply. |
| 5 | **AI Agent Setup & Governance** | **7/10** | Best-in-class core: registration, kill switch (JWT blocklist + cascade revoke via Temporal), delegation chains, agent cards (A2A doc), anomaly detection workflow (governance gaps, deep chains, stale entitlements). | No boundary editor UI, no anomaly findings UI, no approval thresholds ("agent may self-authorize up to X"), no agent attestation expiry, no per-agent rate limits, no agent behavior baseline. |
| 6 | **Certifications & Access Reviews** | **6/10** | Campaign workflow (Temporal), entries with risk score, approve/revoke persisted + audit-chained, generation endpoint guarded. | No scheduler, no multi-tier/manager routing, no reminders/escalations, no revocation-on-reject automation (decision is recorded, revocation is manual), no micro-certs, no campaign templates. |
| 7 | **Audit, Telemetry & Monitoring** | **6/10** | Tamper-evident ledger (unique vs every competitor at this size), live log viewer, `/metrics` (18 Prometheus series), health/ready probes, CAEP broadcast w/ HMAC. | No alerting rules, no SIEM export formats (CEF/LEEF/syslog), no dashboards-as-code for identity KPIs, OTel exporter endpoint hardcoded (`localhost:4317`), no long-term audit retention. |

---

# Part 3: The Risk Score — Math, Meaning & Configuration

## 3.1 Executive summary (layman)

> "How dangerous is this account?" is four questions: **What can they touch?** (severity of entitlements), **How far does the damage spread?** (blast radius through the graph), **Are they breaking the rules?** (separation-of-duties violations), and **Are they unusual compared to their peers?** (a helpdesk tech with database admin rights is more alarming than a DBA with the same rights). We blend the four answers with weights you control, **discount** up to 40% when access is temporary (JIT), and **add** points for dormant accounts. The result is always 0–100, and every point is explainable — clicking the score shows the four sub-scores that produced it.

## 3.2 The formula (verified sound)

$$R = \min\Big(100,\ \big[\textstyle\sum_{i=1}^{4} w_i S_i\big]\cdot(1-\gamma M_{JIT}) + \Delta_{dormant}\Big)$$

| Term | Meaning | Bounds | Current implementation status |
|---|---|---|---|
| $S_1$ Entitlement severity | Weighted avg of entitlement criticalities (Low=10, Med=40, High=70, Critical=100) | [0,100] | 🟡 data exists (`risk_classification` → 0.4/0.7/1.0 per entitlement) — never aggregated |
| $S_2$ Blast radius | $\min(100,\ \alpha\log_2(1+N_{downstream})+\beta D_{transitive})$ — log₂ bounds growth; depth adds linear pressure | [0,100] | 🟡 inputs exist: blast-radius endpoint returns N and path depths — no scoring |
| $S_3$ SoD violations | Severity sum of active toxic-pair conflicts | [0,100] | 🟡 `DetectSoDViolationsWorkflow` finds violations — emits count, not per-identity score |
| $S_4$ Peer anomaly | Jaccard distance $1-\frac{|U\cap P|}{|U\cup P|}$ vs department peer set | [0,100] | ❌ not implemented (needs peer-set builder: group by department/manager) |
| $w_1..w_4$ | Weights, $\sum = 1$ | UI sliders | ❌ no config model |
| $\gamma M_{JIT}$ | Discount ≤ 0.4 when access is ephemeral (count of active JIT vs standing grants) | [0,0.4] | ❌ (JIT grant records exist in Redis — countable) |
| $\Delta_{dormant}$ | +15..+25 if `last_accessed_at` > 90d | additive | ❌ (`last_accessed_at` column exists, never written) |

**Why this shape is right:** the outer `min(100,…)` and the log₂ inside $S_2$ kill unbounded growth (flaw #1); the JIT multiplier prevents penalizing modern ephemeral access (flaw #2); $S_2$'s depth term and $S_4$'s peer comparison cure graph blindness and context blindness (flaw #3).

## 3.3 Engineering specification (how an engineer configures it)

**Data model:** `risk_config` table per tenant — `weights jsonb`, `gamma float`, `dormant_days int`, `dormant_points int`, `criticality_map jsonb`, `alpha float`, `beta float`. Seed defaults: `w = {s1:0.35, s2:0.25, s3:0.25, s4:0.15}, γ=0.4, α=25, β=10, dormant=90d/+15`.

**Computation:** Temporal cron workflow `RiskRecalcWorkflow` (hourly) + event-driven recalc on: entitlement change, SoD scan completion, JIT grant/revoke, dormancy scan. Writes `risk_score`, `risk_factors` (the four sub-scores, for the UI explainer), and emits a CAEP `risk-level-change` event when the band changes.

**UI:** Settings → Risk Engine card: 4 weight sliders with live Σ=1.0 validation, gamma slider, dormancy inputs, "Recalculate now" button, and a preview table of the 10 highest-scored identities under the draft config before saving. Identity detail → risk score becomes a popover showing the four sub-scores and the exact arithmetic.

---

# Part 4: 50-Feature Competitive Gap Analysis

Benchmarks: Okta (IGA + Lifecycle Mgmt), SailPoint IdentityNow, Saviynt EIC, Microsoft Entra (ID Governance + PIM), AWS IAM (Access Analyzer), Alibaba Cloud RAM/IDaaS, DingTalk & Feishu (HR-org-first identity, approval-flow governance), and Chinese IDaaS patterns (no-code app onboarding, social IdP, MLPS compliance reporting).

### Lifecycle Management & JML (1–10)

| # | Feature | Category | Industry standard behavior | GenID | Effort | Approach hint | Missing UI control |
|---|---|---|---|---|---|---|---|
| 1 | Joiner provisioning | JML | HR record → accounts created in all birthright apps day-1 | Partial | Med | LCM + connector write-back exists; needs birthright rule table | Birthright rules editor |
| 2 | Mover transfer | JML | Attribute change → revoke old dept access, grant new | Partial | Med | Temporal workflow chained on attribute watch | Mover rule builder |
| 3 | Leaver deprovisioning | JML | Termination → disable all accounts same day, manager gets assets | Partial | Low | Offboard workflow exists; needs HR trigger + asset-transfer step | Termination trigger config |
| 4 | Pre-hire onboarding | JML | Future start date → accounts activate on date | No | Low | Add `start_date` + scheduled Temporal timer | Start-date field + scheduler |
| 5 | Scheduled state changes | JML | "Suspend on X, reactivate on Y" (leaves of absence) | No | Low | Temporal timer per identity | Date-picker action menu |
| 6 | JML rule engine | JML | `when attribute X changes from A→B, do Y` no-code rules | No | High | Rules table + watcher on outbox events | Rules CRUD page |
| 7 | Bulk lifecycle ops | JML | Multi-select → action on N identities | No | Low | Frontend bulk toolbar over existing PATCH/DELETE | Bulk action toolbar |
| 8 | Non-employee/contingent | JML | Contractors with sponsor + auto-expiry | No | Med | `expires_at` exists on NHI — extend to humans + sponsor field | Sponsorship fields |
| 9 | Rehire detection | JML | Returning employee matches historical identity, restores access | No | Med | Persona + soft-delete match on re-import | "Reinstated" flow in import |
| 10 | Manager self-service | JML | Managers initiate JML for their reports | No | Med | Manager chain in schema already | "My Team" view |

### Identity Correlation & Attribute Mapping (11–18)

| # | Feature | Category | Industry standard | GenID | Effort | Approach hint | Missing UI control |
|---|---|---|---|---|---|---|---|
| 11 | Correlation rules config | Correlation | Choose match keys + priority per source | Partial | Med | Stitching is email/employee_id only, hardcoded | Correlation rule editor |
| 12 | Fuzzy/weighted matching | Correlation | SailPoint correlation config: weighted attributes, threshold | No | High | Score = Σ w_i·sim(attr_i); sim = exact/levenshtein | Match-threshold slider |
| 13 | Manual merge/split | Correlation | Admin merges two accounts into one Persona | No | Med | Merge = move RESOLVES_TO edges; audit-chained | Merge dialog |
| 14 | Persona viewer | Correlation | One person → all their accounts across systems | No | Low | Persona nodes exist in Neo4j | Persona tab on identity |
| 15 | Orphan account report | Correlation | Accounts with no owner/Persona, aged | No | Low | Query: identity with no Persona + no manager | Orphans filter + page |
| 16 | Attribute mapping editor | Correlation | Per-connector: source field → identity field, transforms | No | High | Mapping JSON per connector, applied in sync | **Mapping grid UI (top priority)** |
| 17 | Authoritative source priority | Correlation | HR wins on name/dept; AD wins on email | No | Med | `source_priority` list per attribute | Source priority drag-list |
| 18 | Import diff preview | Correlation | "12 new, 3 changed, 1 conflict" before apply | Partial | Low | CSV preview endpoint exists — extend to diff | Diff summary on import |

### Entitlements & SoD (19–26)

| # | Feature | Category | Industry standard | GenID | Effort | Approach hint | Missing UI control |
|---|---|---|---|---|---|---|---|
| 19 | Entitlement catalog | Entitlements | Searchable catalog w/ owners + risk tier | Partial | Low | Table exists + list endpoint | Catalog page w/ owner column |
| 20 | Entitlement risk tiers | Entitlements | Criticality drives review cadence + risk score | Partial | Low | `risk_classification` exists — surface it | Tier badge + edit |
| 21 | SoD ruleset builder | SoD | "Role A ⊥ Role B" with scope/exceptions | Partial | High | Backend toxic-pair scan exists; rules are code | **SoD rules CRUD (wire /sod page)** |
| 22 | SoD violation inbox | SoD | List violations, assign, track remediation | Partial | Med | Workflow emits results — needs table + endpoint | Violations table on /sod |
| 23 | Toxic-pair simulation | SoD | "If I grant X, which violations appear?" | No | Med | Run scan as-of hypothetical edge | Simulate button in grant flow |
| 24 | Access modeler / role mining | SoD | Cluster entitlements → suggested roles | No | High | Neo4j community detection over HAS_ROLE | "Suggest roles" action |
| 25 | Permission boundaries | Entitlements | Max envelope an identity can ever hold (AWS-style) | Partial | Med | Cedar forbid policies ≈ boundaries — expose as such | Boundary editor on /policies |
| 26 | Unused access report | Entitlements | AWS Access Advisor: granted but unused 90d | No | Med | Needs last-used tracking per entitlement | Unused tab on entitlements |

### Certifications & Access Reviews (27–34)

| # | Feature | Category | Industry standard | GenID | Effort | Approach hint | Missing UI control |
|---|---|---|---|---|---|---|---|
| 27 | Scheduled campaigns | Certs | Quarterly auto-launch, scoped | Partial | Low | Cron start of existing workflow | Schedule form |
| 28 | Reviewer routing | Certs | Manager → app owner → security chain | No | High | Resolve manager_id chain per entry | Routing config per campaign |
| 29 | Reminders & escalation | Certs | Nag at 50%, escalate at breach | No | Med | Temporal timers + notify activity | SLA fields + status badges |
| 30 | Revoke-on-reject automation | Certs | Rejection triggers deprovisioning automatically | Partial | Med | Decision endpoint → call RevokeAccessWorkflow | Auto-revoke toggle |
| 31 | Micro-certifications | Certs | Event-triggered single-access review (e.g., after privilege grant) | No | Med | Hook GrantAccessWorkflow → tiny campaign | Event triggers config |
| 32 | Bulk decisions | Certs | Select page → approve all low-risk | No | Low | Frontend bulk over decide endpoint | Checkbox column |
| 33 | Campaign analytics | Certs | Completion %, decision split, repeat offenders | No | Low | Aggregate certification_entries | Progress bars + stats row |
| 34 | Evidence export | Certs | Signed PDF/CSV for auditors (SOX) | No | Low | Render from audit_log + campaign rows | Export button |

### AI Agent Governance & Ephemeral Access (35–42)

| # | Feature | Category | Industry standard | GenID | Effort | Approach hint | Missing UI control |
|---|---|---|---|---|---|---|---|
| 35 | Agent registry w/ attestation | Agents | SPIFFE-style workload identity | Partial | High | Agent cards exist; SPIFFE deferred per plan (ok) | Card viewer drawer |
| 36 | Kill switch | Agents | Instant global revoke | **Yes** | — | Shipped + cascade | — |
| 37 | Delegation chain viewer | Agents | Who spawned whom, depth limits | Partial | Low | `DELEGATED_FROM` edges exist | Chain graph (reuse BlastRadiusGraph) |
| 38 | Agent permission boundaries | Agents | Agent can never exceed owner envelope | No | Med | Cedar: agent principal ⊂ owner principal rule | Boundary editor |
| 39 | Human-in-the-loop thresholds | Agents | Actions > X risk need human approval | No | Med | Approval gate exists in GrantAccess — parameterize by risk | Threshold slider per agent |
| 40 | Agent anomaly findings UI | Agents | Baseline deviation alerts | Partial | Low | `ScanAgentBehavior` findings exist | Findings list on /agents |
| 41 | JIT eligible-assignment model | JIT | Entra PIM: eligible vs active, MFA + approval to activate | Partial | High | JIT exists; add "eligible" role state + activation flow | Eligibility tab on /access |
| 42 | Ephemeral session inventory | JIT | All live sessions, TTL countdown, kill any | Partial | Low | Redis has the keys — need list endpoint | **Active sessions table on /access** |

### Telemetry, Risk & Compliance (43–50)

| # | Feature | Category | Industry standard | GenID | Effort | Approach hint | Missing UI control |
|---|---|---|---|---|---|---|---|
| 43 | Composite risk engine | Risk | Weighted multi-signal score (Part 3) | **No** | High | Part 3 spec | Risk sliders + explainer popover |
| 44 | UEBA behavior baseline | Risk | Entra Identity Protection: sign-in anomaly | No | High | Post-MVP; needs event stream features | — |
| 45 | Alert rules engine | Monitoring | condition → channel (webhook/Slack/email) | No | Med | Rules table + outbox hook | Alerts page |
| 46 | SIEM export | Monitoring | CEF/syslog stream | No | Med | NATS consumer → syslog forwarder | Export config |
| 47 | Compliance report packs | Compliance | SOX/ISO27001/MLPS 2.0 evidence one-click | Partial | Med | Ledger gives integrity; need report templates | Reports page |
| 48 | Standing-vs-temporary access ratio | Risk | KPI: % of access that is JIT (target ↑) | No | Low | Count Redis JIT keys vs PG grants | Dashboard widget |
| 49 | Dashboard time series | Monitoring | Identities/grants/revocations over time | No | Low | recharts + audit_log aggregation | Charts on dashboard |
| 50 | Tenant admin & API keys | Platform | Self-serve tenants, scoped API keys | Partial | Med | RLS policies exist; keys are env-only | Settings → tenants + keys |

**Score: Yes 1 / Partial 18 / No 31.** The spine (workflows, ledger, graph, JIT, kill switch) is ahead of schedule; the **configuration surface** (the UI that lets an engineer run all of it) is where 31 gaps live.

---

# Part 5: Five Real-World Use Cases (with configs)

## Scenario 1 — Onboard a new SaaS app via UI with JIT rules

**Goal:** Engineer adds "AcmeCRM" (SCIM 2.0), maps its fields, syncs, and makes its admin role JIT-only.

```jsonc
// POST /api/v1/connectors  ✅ exists
{ "name": "AcmeCRM", "type": "scim",
  "endpoint": "https://api.acmecrm.com/scim/v2", "password": "Bearer eyJ..." }

// POST /api/v1/connectors/{id}/sync  ✅ exists

// ❌ MISSING: attribute mapping (blocked on Part 4 #16)
// PUT /api/v1/connectors/{id}/mapping  ← does not exist
{ "mappings": [
    { "source": "userName",  "target": "email",        "transform": "lowercase" },
    { "source": "name.givenName + name.familyName", "target": "display_name", "transform": "join(' ')" },
    { "source": "title",     "target": "department",   "transform": "none" } ],
  "correlation_keys": ["email"] }  // ← Part 4 #11

// ❌ MISSING: make an entitlement JIT-only (blocked on Part 4 #41)
// POST /api/v1/entitlements/{id}/jit-policy  ← does not exist
{ "eligible": true, "max_duration_mins": 60, "approval": "manager", "mfa": true }
```
**Where it breaks today:** connector create/sync works; mapping & JIT-eligibility don't exist. **Workaround:** seed mapping in code — unacceptable for the "configure anything from UI" bar.

## Scenario 2 — Compromised AI agent: detect → isolate → sever

```jsonc
// 1. Anomaly workflow flags agent  ✅ exists (ScanAgentBehavior) — but no UI shows findings
// GET /api/v1/agents/{id}  ✅ exists

// 2. Kill switch  ✅ LIVE-TESTED end-to-end
// POST /api/v1/agents/{id}/kill-switch  {"reason":"anomalous bulk read"}
// → status revoked, JWT jti blocklisted in Redis, CascadeRevokeWorkflow launched

// 3. Verify containment
// GET /api/v1/identities/{id}/blast-radius  ✅ graph modal shows exactly what it could have touched
// GET /api/v1/audit/verify  ✅ proves the response trail wasn't altered
```
**Where it breaks today:** step 1's findings are invisible (Part 4 #40); everything else works. This is GenID's strongest story — it should be the demo.

## Scenario 3 — Quarterly certification of production databases

```jsonc
// POST /api/v1/certifications/generate  ✅ LIVE-TESTED (workflow started)
{ "campaign_name": "Q3 Prod DB Review", "campaign_type": "quarterly",
  "scope": { "resource_criticality": ["critical"] } }  // ❌ scope is ignored — cert queries are fixed

// Reviewer flow: GET /api/v1/certifications → decide per entry  ✅ exists
// POST /api/v1/certifications/entries/{id}/decide
{ "decision": "revoked", "notes": "contract ended 6/30" }
// ❌ revocation is not triggered automatically (Part 4 #30) — a human must then call /access/revoke
```
**Where it breaks:** scheduling, reviewer routing, auto-revoke, reminders (Part 4 #27–30).

## Scenario 4 — Mover: Engineering → Sales, atomic swap

```jsonc
// Ideal: PATCH identity department → watcher fires mover rule ❌ no rule engine (Part 4 #6)
// Today: manual two-step via LCM  ✅ endpoint exists
// POST /api/v1/lcm  (guarded — LIVE-TESTED with admin JWT)
{ "action": "revoke_role",
  "connector_ids": ["..."], "external_id": "jdoe",
  "group": { "name": "Engineering" } }
// then assign Sales role — two calls, not atomic; a crash between them leaves a toxic mix
```
**Where it breaks:** no mover template, no attribute-watch trigger, no atomic revoke+grant workflow (the pieces exist in Temporal — they need one `MoverWorkflow`).

## Scenario 5 — Orphan account correlation & escalation

```jsonc
// ❌ MISSING report (Part 4 #15):
// GET /api/v1/identities?orphaned=true&dormant_days=90  ← does not exist
// Expected payload once built:
{ "identities": [
    { "id": "…", "email": "svc-old-batch@corp", "last_accessed_at": null,
      "persona": null, "risk_score": 0.0 } ],
  "suggested_actions": ["assign_owner", "suspend", "certify"] }

// What exists today that feeds it: Persona/RESOLVES_TO stitching ✅,
// last_accessed_at column ✅ (never written — also needed for dormant risk penalty)
```
**Where it breaks:** the query is one SQL statement away, but nothing writes `last_accessed_at` yet — fix both together with the risk engine (Part 3).

---

# Recommended Build Order (next 5 moves)

1. **Unify the two graph models** (0.1) — make CheckAccess walk the same chain blast-radius draws; make Provision write `HAS_ROLE/DIRECTLY_OWNS/ACCESSES`-compatible edges. *This converts two half-features into one whole product.*
2. **Attribute-mapping + correlation editor** (#16, #11) — unlocks "any app, UI-only onboarding" (Scenario 1).
3. **Risk engine v1** (Part 3: S1+S2 only, weights in Settings) — makes the headline number real with two weeks of math, not ML.
4. **Active JIT sessions + approval inbox on /access** (#42, #1.4) — completes the access story end-to-end.
5. **SoD page wiring + mover workflow** (#21–22, #6) — turns two static pages into governance features.
