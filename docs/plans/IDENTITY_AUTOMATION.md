# Identity Automation Catalog

**Author:** GenID Engineering — Architect-level design (Palo Alto Networks SWE/Architect standard)
**Status:** Design + roadmap. Foundations built; gaps identified below.
**Last updated:** Today

This document catalogs **every identity automation workflow** a modern enterprise IAM platform should expose. Each workflow is decomposed into trigger → eligibility → policy → approval → execution → audit → notify. All workflows run on **Temporal** for durable, retryable, observable execution.

---

## 1. Domain Taxonomy

| Domain | # workflows | Risk surface | Examples |
|---|---|---|---|
| Identity Lifecycle (JML) | 8 | Medium | Onboard, Mover, Leaver, Rehire, Contractor→FTE, M&A merge |
| Access Lifecycle | 12 | High | Grant, JIT, PAM, Firecall, API key issuance, SSH cert, Cloud role assumption |
| Access Modification | 7 | Medium | Modify entitlements, group membership, attributes |
| Account Lifecycle | 6 | High | Deactivate, Lock, Archive, Purge (GDPR right-to-be-forgotten) |
| Authentication | 6 | High | Password reset, MFA enrollment, recovery codes |
| Service Account | 5 | Critical | Create, rotate, transfer, decommission, cert renewal |
| Role Engineering | 7 | Critical | Create, update, delete (with cascade impact), clone, version |
| Approval | 8 | Medium | Sequential, parallel, dynamic, delegated, risk-gated, time-bound |
| Certification | 6 | Medium | Periodic review, manager recert, resource-owner review, campaign |
| Break Glass | 5 | Critical | Firecall, emergency deprov, kill-switch, mass lockdown |
| Compliance / Audit | 9 | Low | Attestation, orphan report, dormant, SoD, privilege creep |
| HR & Source Mgmt | 7 | Low | Sync, delta, validation, manual override, dedupe, split |
| Bulk Operations | 6 | Medium | Import, grant, revoke, role assignment, attribute update, wave |
| Data & Integration | 6 | Low | Export, connect, test, decommission, shell import |
| Custom / Extensibility | 5 | Medium | Custom workflow, webhook, scheduled, event-driven, exception |
| Zero-Standing-Privilege (ZSP) | 4 | High | JIT-only for prod, drift detection, CIEM, SSPM |
| Notification | 4 | Low | Slack/Teams bot, email digest, real-time alert, escalation |

**Total: 121 workflows across 17 domains**

---

## 2. Reference Implementation Pattern

Every workflow follows the same skeleton:

```
┌────────┐    ┌────────────┐    ┌──────────┐    ┌─────────┐    ┌──────────┐
│ TRIGGER │ → │ ELIGIBILITY │ → │  POLICY  │ → │ APPROVE │ → │ EXECUTE  │
└────────┘    └────────────┘    └──────────┘    └─────────┘    └──────────┘
   UI/API         "Can they?"       "Should they?"    "Who?"        Temporal
   HR event       target rules      SoD check         manager       workflow
   schedule       scope             risk band         resource-owner  child activities
   webhook        delegation        geo/time          security       compensation
   risk event     admin grant       over-priv         dynamic        rollback
                                                                  audit + notify
```

**Storage schema (3 tables):**
```sql
workflow_requests (
  id UUID PRIMARY KEY,
  type TEXT,                 -- e.g. "access.request.jit"
  status TEXT,               -- pending|eligible|approved|denied|executed|failed|cancelled
  requester_id UUID,
  target_id UUID,            -- identity receiving the action
  payload JSONB,             -- action-specific details
  idempotency_key TEXT UNIQUE,
  temporal_workflow_id TEXT,
  created_at, updated_at, completed_at
)

workflow_approvals (
  id UUID PRIMARY KEY,
  request_id UUID REFERENCES workflow_requests,
  level INT,                -- 1, 2, 3... (sequential order)
  approver_id UUID,         -- resolved dynamically
  status TEXT,              -- pending|approved|denied|skipped|escalated
  comment TEXT,
  decided_at TIMESTAMP
)

workflow_audit (
  id BIGSERIAL PRIMARY KEY,
  request_id UUID,
  step TEXT,                -- e.g. "eligibility.check", "approval.resolved", "activity.provisioned"
  actor TEXT,               -- user_id or "system"
  details JSONB,
  ts TIMESTAMP DEFAULT NOW()
)
```

---

## 3. Workflow Catalog by Domain

### 3.1 Identity Lifecycle (JML) — Joiner/Mover/Leaver

| Workflow | Trigger | Eligibility | Approval | Execution | Notes |
|---|---|---|---|---|---|
| **Onboard (Joiner)** | HR record insert | `employee_status='active'`, role defined | Auto (policy-driven) | `OnboardIdentityWorkflow` | ✅ Built |
| **Mover** | HR record update (dept/mgr/title) | dept/role mapped | Auto | `MoverIdentityWorkflow` | ⚠️ Partial — dept change fires, role move manual |
| **Leaver** | HR record status='terminated' OR HR row missing | terminated status confirmed | Auto | `OffboardIdentityWorkflow` + `CascadeRevokeWorkflow` | ✅ Built |
| **Rehire / Reinstate** | HR returns terminated → active | prior identity exists | Manager + Security | `ReinstateIdentityWorkflow` (NEW) | Manager must re-approve — **never silently restore** |
| **Long-term Leave** | HR attribute `status='on_leave'` | active employee | Manager | `SuspendIdentityWorkflow` (NEW) | Freeze access without termination |
| **Contractor → FTE** | HR attribute `worker_type='employee'` | contractor identity exists | Manager + HR | `ConvertContractorToFTEWorkflow` (NEW) | Migrate groups, retain tenure |
| **M&A Identity Merge** | External CSV with `merge_into` | source identity in target tenant | Security + Privacy | `MergeIdentitiesWorkflow` (NEW) | GDPR-sensitive; full audit trail |
| **Identity Split** | HR record duplicated | duplicate detection match | Security | `SplitIdentityWorkflow` (NEW) | Rare; usually data fix |

### 3.2 Access Lifecycle

| Workflow | Trigger | Eligibility | Approval | Execution | Notes |
|---|---|---|---|---|---|
| **Permanent Access Grant** | UI/API request | role rules, dept eligible | Manager → Resource Owner | `GrantAccessWorkflow` | ✅ Built |
| **JIT Access (≤8h)** | UI/API request | scope is prod-non-critical | Manager (auto-approve if risk<300) | `JustInTimeAccessWorkflow` | ✅ Built — auto-expires |
| **PAM Session** | Sensitive system access request | JIT-only target | Manager + Security | `PrivilegedAccessWorkflow` (NEW) | Session recording, dual-control |
| **Firecall / Break-Glass** | Emergency page | on-call rotation | Auto-approve + post-review | `FirecallAccessWorkflow` (NEW) | **MUST generate post-event audit + alert** |
| **Time-bound Role Assumption** | AWS/Azure/GCP role | cloud role mapped | Resource owner | `AssumeCloudRoleWorkflow` (NEW) | Issues STS creds, 1h TTL |
| **API Key Issuance** | Service account request | service account exists | Resource owner | `IssueAPIKeyWorkflow` (NEW) | Scoped, expiring, vaulted |
| **API Key Rotation** | Schedule (90d) or manual | key older than TTL | Auto | `RotateAPIKeyWorkflow` (NEW) | Zero-downtime overlap window |
| **SSH Certificate Issuance** | Request | user has SSH role | Just-in-time | `IssueSSHCertWorkflow` (NEW) | 8h cert, signed by Vault CA |
| **Cloud Console Access** | PAM request | prod access only via JIT | Security | `GrantCloudConsoleWorkflow` (NEW) | Browser session recorded |
| **Database Read Access** | Request | DB access role defined | Data owner | `GrantDBAccessWorkflow` (NEW) | Query-level scoping |
| **Production Deploy Rights** | CI/CD pipeline | SRE role | Auto-approve (service account) | `GrantDeployRightsWorkflow` (NEW) | Ephemeral |
| **Network Admin (firewall/segment)** | Request | network-admin role | Security + CISO delegate | `GrantNetworkAdminWorkflow` (NEW) | High-risk, mandatory justification |

### 3.3 Access Modification

| Workflow | Trigger | Eligibility | Approval | Execution |
|---|---|---|---|---|
| **Modify Entitlements** | UI/API | owns identity | Manager | `ModifyEntitlementsWorkflow` |
| **Group Membership Add/Remove** | UI/API | group rules | Manager (auto if mandatory group) | `ModifyGroupMembershipWorkflow` |
| **Attribute Update (mgr/dept/loc)** | HR sync OR manual | self OR HR admin | Auto (HR) / Manager (manual) | `UpdateAttributesWorkflow` |
| **Profile Update (phone/photo)** | Self-service | self | Auto | `UpdateProfileWorkflow` |
| **Manager Change** | HR sync | new manager valid | Auto | `ChangeManagerWorkflow` |
| **Cost Center Change** | HR/Finance | finance-validated | Finance + Manager | `ChangeCostCenterWorkflow` |
| **Sponsor Change (for contractors)** | Manual | sponsor exists | Auto if sponsor active | `ChangeSponsorWorkflow` |

### 3.4 Account Lifecycle

| Workflow | Trigger | Eligibility | Approval | Execution |
|---|---|---|---|---|
| **Deactivate (soft)** | Manual or leaver | identity exists | Auto (leaver) / Manager (manual) | `DeactivateIdentityWorkflow` |
| **Lock (temporary)** | Risk event or manual | risk>700 OR security | Auto (risk) / Security (manual) | `LockIdentityWorkflow` |
| **Unlock** | Risk resolved OR manual | risk<300 | Auto (risk) / Manager | `UnlockIdentityWorkflow` |
| **Archive (dormant)** | 90d no login | dormant detected | Auto | `ArchiveIdentityWorkflow` |
| **Purge (GDPR)** | Data subject request | legal hold expired | Privacy Officer + Legal | `PurgeIdentityWorkflow` (NEW) |
| **Reinstate from Archive** | HR returns active | archived<365d | Manager + Security | `RestoreArchivedIdentityWorkflow` |

### 3.5 Authentication

| Workflow | Trigger | Eligibility | Approval | Execution |
|---|---|---|---|---|
| **Self-Service Password Reset** | UI forgot-password | identity verified (KBA/TOTP) | Auto | `PasswordResetWorkflow` |
| **Forced Password Change** | Risk event | identity at risk | Auto | `ForcePasswordChangeWorkflow` |
| **MFA Enrollment** | First login OR policy | identity exists | Auto | `MFAEnrollWorkflow` |
| **MFA Reset** | Lost device | KBA verified | Security | `MFAResetWorkflow` |
| **Recovery Codes** | Lost MFA | KBA + Manager | Manager | `IssueRecoveryCodesWorkflow` |
| **Account Unlock (after lockout)** | Manual | lockout<24h | Auto | `UnlockAfterLockoutWorkflow` |

### 3.6 Service Account

| Workflow | Trigger | Eligibility | Approval | Execution |
|---|---|---|---|---|
| **Service Account Creation** | App team request | owner assigned | App owner + Security | `CreateServiceAccountWorkflow` |
| **Credential Rotation** | Schedule (90d) | service account active | Auto | `RotateServiceAccountWorkflow` |
| **Ownership Transfer** | Manual | new owner exists | Old owner + New owner + Security | `TransferServiceAccountWorkflow` |
| **Decommission** | App retired | service account exists | App owner | `DecommissionServiceAccountWorkflow` |
| **Certificate Renewal** | 30d before expiry | cert<30d | Auto | `RenewServiceAccountCertWorkflow` |

### 3.7 Role Engineering

| Workflow | Trigger | Eligibility | Approval | Execution |
|---|---|---|---|---|
| **Role Creation** | IAM admin | role spec valid | Security + SoD validation | `CreateRoleWorkflow` |
| **Role Update** | IAM admin | role exists | Security + Impact analysis | `UpdateRoleWorkflow` |
| **Role Deletion** | IAM admin | 0 active grants OR cascade plan | Security + Owner | `DeleteRoleWorkflow` |
| **Role Clone** | Template | source exists | Security | `CloneRoleWorkflow` |
| **Role Version Diff** | Audit | role changed | Auto | `RoleVersionDiffWorkflow` |
| **Role Decomposition** | Migrate to fine-grained | role exists | Security | `DecomposeRoleWorkflow` |
| **Role Composition** | Parent→child | parent exists | Security | `ComposeRoleWorkflow` |

### 3.8 Approval Workflows (patterns, not workflows themselves)

| Pattern | When | Behavior |
|---|---|---|
| **Sequential** | High-risk grants | Manager → Security → Owner, each must approve in order |
| **Parallel (any-of)** | Multiple resource owners | Any one approves = grant proceeds |
| **Parallel (all-of)** | Multi-system grants | All must approve |
| **Dynamic approver resolution** | Resource-specific | Look up role.resource_owner_email, send |
| **Skip-level** | High-privilege (CISO scope) | Skip manager, go to skip-level + Security |
| **Delegated approval** | OOO | Resolve to delegate if manager OOO flag set |
| **Risk-gated** | Risk<300 | Auto-approve; 300-700 = manager; >700 = security |
| **Time-bound approval** | Request expires in 24h | If no decision, auto-deny with reason |

### 3.9 Certifications & Reviews

| Workflow | Trigger | Eligibility | Approval | Execution |
|---|---|---|---|---|
| **Quarterly Access Review** | Schedule (90d) | campaign generated | Manager / Resource owner | `AccessCertificationWorkflow` ✅ |
| **Manager Recertification** | Hire anniversary | employee > 365d | Manager | `ManagerRecertWorkflow` |
| **Resource Owner Review** | Schedule | owns resource | Resource owner | `ResourceOwnerReviewWorkflow` |
| **Privileged Access Review** | Monthly | has privileged role | Security | `PrivilegedAccessReviewWorkflow` |
| **New Hire 30/60/90 review** | Schedule | tenure<90d | Manager | `NewHireReviewWorkflow` |
| **Contractor Refresh** | Schedule (90d) | contractor status | Sponsor | `ContractorRefreshWorkflow` |

### 3.10 Break Glass / Emergency

| Workflow | Trigger | Eligibility | Approval | Execution | Special |
|---|---|---|---|---|---|
| **Firecall Access** | On-call page | in on-call rotation | Auto + post-review | `FirecallAccessWorkflow` | **MUST** trigger security review within 24h |
| **Emergency Deprovisioning** | Security incident | identity confirmed in incident | Security lead | `EmergencyDeprovisionWorkflow` | All grants revoked, sessions terminated, audit-frozen |
| **Agent Kill Switch** | Agent risk>700 | has agent identity | Auto | `AgentKillSwitchWorkflow` | Revoke JWTs, cascade revoke delegated children |
| **Mass Lockdown (geo/role)** | Breach detected | geo OR role flagged | Security CISO | `MassLockdownWorkflow` | Suspend all matching identities in 60s |
| **Audit Chain Freeze** | Investigation | in-flight investigation | Privacy/Legal | `FreezeAuditChainWorkflow` | Seals audit log with cryptographic hash |

### 3.11 Compliance / Audit

| Workflow | Trigger | Eligibility | Output |
|---|---|---|---|
| **Identity Attestation Report** | Monthly | all identities | CSV/SOC2-ready |
| **Access Attestation Report** | Monthly | all grants | CSV/SOC2-ready |
| **Orphaned Account Report** | Weekly | manager=null AND age>30d | Email to Security |
| **Dormant Account Report** | Weekly | last_login>90d | Email to Manager |
| **Excessive Privilege Report** | Monthly | grants>role_threshold | Email to IAM lead |
| **Toxic Combination (SoD)** | Daily | SoD violation | Email to Security |
| **Privilege Creep Analysis** | Monthly | grant growth >5%/month | Email to Manager |
| **Audit Chain Verify** | On-demand | chain exists | "intact" / "tampered at block N" |
| **Compliance Evidence Pack** | SOC2/Q2 | date range | ZIP with all reports |

### 3.12 HR & Source Management

| Workflow | Trigger | Eligibility | Execution |
|---|---|---|---|
| **HR Full Sync** | Schedule | connector healthy | `SyncConnectorHR` ✅ |
| **HR Delta Sync** | Schedule (15min) | connector healthy | `SyncConnectorDelta` |
| **HR Source Validation** | Manual | source configured | `ValidateHRSourceWorkflow` |
| **Manual Identity Override** | IAM admin | IAM admin role | `CreateIdentityManualWorkflow` |
| **Identity Merge (dedupe)** | Detection rule | match confidence>0.95 | `MergeIdentitiesWorkflow` |
| **Identity Split (rare)** | Data fix | confirmed split | `SplitIdentitiesWorkflow` |
| **Source Account Sync** | Schedule | source connector | `SyncSourceAccountsWorkflow` |

### 3.13 Bulk Operations

| Workflow | Trigger | Eligibility | Approval | Execution |
|---|---|---|---|---|
| **Bulk Identity Import** | CSV upload | schema valid | IAM admin | `BulkIdentityImportWorkflow` |
| **Bulk Access Grant (campaign)** | Manual | list valid | Security | `BulkGrantWorkflow` |
| **Bulk Access Revoke** | Manual | list valid | Security | `BulkRevokeWorkflow` |
| **Bulk Role Assignment** | Manual | role valid | Security | `BulkRoleAssignWorkflow` |
| **Bulk Attribute Update** | Manual | schema valid | IAM admin | `BulkUpdateAttributesWorkflow` |
| **Bulk Onboarding Wave** | HR batch | validated CSV | HR + IAM | `BulkOnboardWaveWorkflow` |

### 3.14 Data & Integration

| Workflow | Trigger | Eligibility | Execution |
|---|---|---|---|
| **Export Identities (CSV/SCIM)** | Manual | scope authorized | `ExportIdentitiesWorkflow` |
| **Export Audit Log** | Manual | legal hold OK | `ExportAuditWorkflow` |
| **New Integration Connect** | Manual | connector spec exists | `ConnectIntegrationWorkflow` |
| **Integration Test** | Manual | connector configured | `TestIntegrationWorkflow` |
| **Integration Decommission** | Manual | integration exists | `DecommissionIntegrationWorkflow` |
| **Shell Import (resource as code)** | Git push | repo authorized | `ImportShellWorkflow` |

### 3.15 Custom / Extensibility

| Workflow | Trigger | Notes |
|---|---|---|
| **Custom Workflow (DSL)** | API | Define in YAML: trigger + steps + approvals |
| **Webhook Trigger** | External | Slack command, ServiceNow ticket, etc. |
| **Scheduled Trigger** | Cron | E.g., weekly entitlement reconciliation |
| **Event-Driven Trigger** | Risk event | E.g., on `credential_leaked` → auto-revoke sessions |
| **Approval Policy Override (exception)** | Manual | Temporary exception with expiry + reviewer |

### 3.16 Zero-Standing-Privilege (PA Networks specialty)

| Workflow | Notes |
|---|---|
| **JIT-only for prod access** | No standing prod grants — all via 1h JIT |
| **Drift Detection** | Compare granted vs used entitlements; revoke unused>60d |
| **CIEM** | Cloud Infrastructure Entitlement Management — AWS/Azure/GCP roles |
| **SSPM Integration** | SaaS Security Posture — pull posture data, gate access |

### 3.17 Notifications & Comms

| Workflow | Trigger | Output |
|---|---|---|
| **Slack/Teams bot integration** | Webhook | Approve/deny from chat |
| **Email digests** | Schedule | Weekly access reviews due |
| **Real-time alerts** | Risk event>500 | Slack to on-call |
| **Escalation notifications** | Approval pending >24h | Escalate to skip-level |

---

## 4. Architecture Layers

```
┌─────────────────────────────────────────────────────────────────┐
│  UX Surface                                                      │
│  • Self-service portal (catalog of requestable items)            │
│  • Approval inbox (Slack/Teams + web)                           │
│  • Admin console (workflow_requests table viewer)                │
│  • Audit explorer (workflow_audit)                              │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  API Layer (Go + gorilla/mux)                                    │
│  POST /api/v1/requests          → start workflow                 │
│  GET  /api/v1/requests/{id}     → status                         │
│  POST /api/v1/requests/{id}/approve|deny|cancel                  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  Workflow Engine (Temporal)                                      │
│  121 workflows registered, each:                                 │
│    1. Eligibility activity (target rules, scope)                 │
│    2. Policy activity (SoD, risk, geo, time)                     │
│    3. Approval routing (signals + timers)                       │
│    4. Execution activities (provision/deprov, idempotent)        │
│    5. Compensation activities (rollback if any step fails)      │
│    6. Audit + notify (always last, even on failure)              │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  Storage (Postgres + Neo4j)                                      │
│  workflow_requests / workflow_approvals / workflow_audit         │
│  Outbox → NATS → all downstream (audit chain, notifications)    │
└─────────────────────────────────────────────────────────────────┘
```

---

## 5. Implementation Roadmap

### Phase 0 (Done)
- ✅ Connector sync (HR, SCIM, OIDC)
- ✅ Outbox + queue-group fan-out
- ✅ Postgres-backed vault
- ✅ Risk engine (formula + bands)

### Phase 1 (Done)
- ✅ HR-sync JML pipeline
- ✅ Access request (grant/revoke/JIT)
- ✅ Certification campaigns

### Phase 2 (Next, ~2 weeks)
- [ ] **`workflow_requests` + `workflow_approvals` + `workflow_audit` tables + migration**
- [ ] **Approval routing engine** (sequential, parallel, dynamic, risk-gated, delegated)
- [ ] **Self-service request catalog UI** (`/requests`)
- [ ] **Approval inbox UI** (`/inbox`)
- [ ] **JIT auto-expiry** (Temporal schedule)
- [ ] **Slack/Teams webhook approval**

### Phase 3 (~3 weeks)
- [ ] **Break-glass flows** (Firecall + post-review)
- [ ] **PAM session recording + dual-control**
- [ ] **Service account governance** (rotation, ownership)
- [ ] **Cloud role assumption** (AWS STS, Azure PIM)
- [ ] **Compliance evidence pack generation**

### Phase 4 (~3 weeks)
- [ ] **Zero-Standing-Privilege mode** (CIEM integration)
- [ ] **Event-driven auto-remediation** (on risk>700 → auto-revoke)
- [ ] **Custom workflow DSL** (YAML → Temporal)
- [ ] **Drift detection + least-privilege recommender**

### Phase 5 (CIEM + SSPM)
- [ ] **AWS / Azure / GCP entitlement sync**
- [ ] **SaaS posture integration** (Prisma Cloud / SSPM)
- [ ] **Cross-tenant governance**

---

## 6. What "MSG Sending" Means Here

The user mentioned "MSG sending" — interpreted as **outbound notifications** that fire when workflow events happen. The notification surface is:

| Channel | Use case | Implementation |
|---|---|---|
| **In-app inbox** | Approval pending, completed | `workflow_audit` + WebSocket push (future) |
| **Email** | Approval request, escalation | SMTP via Temporal activity |
| **Slack** | Approval request button | Bot with interactive message |
| **Teams** | Same as Slack | Bot framework |
| **SMS** | Break-glass only | Twilio (NEW) |
| **Webhook** | External system reaction | Generic POST activity |

Every workflow emits these events to NATS:
- `workflow.requested` — sent to approvers
- `workflow.approved` — sent to requester
- `workflow.denied` — sent to requester
- `workflow.executed` — sent to audit subscribers
- `workflow.failed` — sent to on-call

---

## 7. Open Questions for the User

1. **Which Phase 2 workflow do you want first?** (recommend: Firecall + JIT auto-expiry — highest customer pull)
2. **Notification channels first?** (recommend: in-app inbox + email, Slack in Phase 3)
3. **Custom DSL in Phase 4 — yes/no?** (cuts roadmap by 2 weeks if no)
4. **CIEM scope?** (AWS only, or all 3 clouds?)
