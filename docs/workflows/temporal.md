# Temporal Workflows

Durable workflow orchestration for JML, JIT access, approvals, revocation, and periodic risk/compliance scans. All app workflows run on the **`critical-offboarding`** task queue (default namespace).

## Workflow inventory

| Workflow | Purpose | Key steps | Activities |
|----------|---------|-----------|------------|
| **OnboardIdentityWorkflow** | Joiner: identity + initial roles | Create identity (PG+Neo4j) → assign roles in parallel → 7-day certification timer | `CreateIdentity`, `AssignRoleToIdentity` |
| **OffboardIdentityWorkflow** | Multi-phase deprovisioning | Audit + identity lock (fencing) → entitlements revoked risk-tiered (critical seq, high/normal parallel) → NHI cascade for agents → revoke identity → CAEP broadcast → finalize audit | `InitiateAuditTrail`, `AcquireIdentityLock`, `ReleaseIdentityLock`, `QueryIdentityEntitlements`, `FindDelegatedAgents`, `RevokeIdentityAccess`, `BroadcastCAEPEvent`, `FinalizeAuditTrail`; children `RevokeAccessChildWorkflow`, `CascadeRevokeWorkflow` |
| **GrantAccessWorkflow** | SoD-aware granting | Cedar `CheckAccessPolicy` (action=grant) → `CheckSoDConflicts` (role path; risk >0.7 hard deny) → optional approval (waits on `ApprovalDecision` signal) → provision (temporary if duration set, else permanent) → timer + `RevokeBeforeExpiry` signal → auto-revoke on expiry | `CheckAccessPolicy`, `CheckSoDConflicts`, `SendApprovalRequest`, `ProvisionAccess`, `ProvisionTemporaryAccess`; child `RevokeAccessChildWorkflow` |
| **RevokeAccessWorkflow** | Emergency revocation | Emergency: `RevokeIdentityAccess` → `RotateCredentials` → `BroadcastCAEPEvent`. Non-emergency: `RevokeTargetAccess` single entitlement | `RevokeIdentityAccess`, `RotateCredentials`, `BroadcastCAEPEvent`, `RevokeTargetAccess` |
| **JustInTimeAccessWorkflow** | JIT elevation (default read / 60 min) | Cedar `CheckAccessPolicy` (grant_type=jit) → `ProvisionTemporaryAccess` (mints RS256 JWT, jti in Redis) → timer + `RevokeBeforeExpiry` → `RevokeTemporaryAccess` | `CheckAccessPolicy`, `ProvisionTemporaryAccess`, `RevokeTemporaryAccess` |
| **RevokeAccessChildWorkflow** | Single-system revocation with backoff | `RevokeTargetAccess` (2-min timeout, 5 attempts; non-retryable forbidden/not-found) | `RevokeTargetAccess` |
| **CascadeRevokeWorkflow** | Kill-switch cascade for delegated NHI | Revoke SPIFFE SVID → OAuth tokens → API keys → rotate credentials; tolerates partial failures | `RevokeSPIFFESVID`, `RevokeOAuthTokens`, `RevokeAPIKeys`, `RotateCredentials` |
| **AgentAnomalyDetectionWorkflow** | Cron anomaly scan | `ScanAgentBehavior`; any critical anomaly or score >0.8 → auto-remediate via `CascadeRevokeWorkflow` | `ScanAgentBehavior`; child `CascadeRevokeWorkflow` |
| **DetectSoDViolationsWorkflow** | Cron SoD scan | `ScanSoDViolations`; logs critical conflicts | `ScanSoDViolations` |
| **RiskRecalculationCronWorkflow** | Periodic risk recalc (every 15 min default) | `ListActiveIdentityIDs` → fan-out `CalculateIdentityRisk` in batches of 50 | `ListActiveIdentityIDs`, `CalculateIdentityRisk` |
| **AccessCertificationWorkflow** | Certification campaign | `CreateCertificationCampaign` (default quarterly) → `PopulateCertificationEntries` | `CreateCertificationCampaign`, `PopulateCertificationEntries` |

> **`RiskAlertWorkflow`** (`risk_workflow.go`): band-gated escalation (critical → auto-revoke, high → step-up MFA, elevated → alert). Registered on the separate `risk-tasks` queue and **not wired** into the live system — the event processor's inline band actions are the active path.

## Entry points (what starts them)

| Workflow | Trigger |
|----------|---------|
| OffboardIdentityWorkflow | `DELETE /scim/v2/Users/{id}` (id `offboard-<id>`) |
| RevokeAccessWorkflow | `POST /api/v1/access/revoke`, `POST /api/v1/agents/{id}/kill-switch` |
| GrantAccessWorkflow | `POST /api/v1/access/grant` (id `grant-access-<id>-<rand8>`) |
| JustInTimeAccessWorkflow | `POST /api/v1/access/jit` (id `jit-access-<id>-<rand8>`) |
| AccessCertificationWorkflow | `POST /api/v1/certifications/generate` (id `certify-<uuid>`) |
| RiskRecalculationCronWorkflow | Scheduled at boot (`*/15 * * * *`, id `risk-recalc-cron`) |
| OnboardIdentityWorkflow | Registered; no REST caller (use SCIM create or `CreateIdentity` activity path) |

## Signals

| Signal | Used by | Payload |
|--------|---------|---------|
| `ApprovalDecision` | GrantAccessWorkflow | bool |
| `RevokeBeforeExpiry` | GrantAccessWorkflow, JustInTimeAccessWorkflow | bool |

## Activities

All on `ActivityService` (PG + Neo4j + Redis + Temporal + Cedar + OIDC + audit):

**Lifecycle:** `CreateIdentity`, `AssignRoleToIdentity`, `ListActiveIdentityIDs`, `CalculateIdentityRisk`
**Provisioning:** `ProvisionAccess`, `ProvisionTemporaryAccess`, `RevokeTemporaryAccess`, `RevokeTargetAccess`, `RevokeIdentityAccess`
**Offboarding:** `InitiateAuditTrail`, `FinalizeAuditTrail`, `QueryIdentityEntitlements`, `FindDelegatedAgents`
**Credentials:** `RotateCredentials`, `RevokeSPIFFESVID`, `RevokeOAuthTokens`, `RevokeAPIKeys`
**Policy:** `CheckAccessPolicy` (Cedar + 30s Redis cache), `CheckSoDConflicts` (toxic/transitive/rubberband)
**Compliance:** `ScanAgentBehavior` (governance_gap, missing_framework, deep_delegation_chain, stale_entitlements), `ScanSoDViolations`, `CreateCertificationCampaign`, `PopulateCertificationEntries`, `SendApprovalRequest`
**Messaging:** `BroadcastCAEPEvent` (HMAC-SHA256 webhook if configured)
**Concurrency:** `AcquireIdentityLock` / `ReleaseIdentityLock` (Redis SETNX + fencing token + TTL watchdog)

## Namespaces & retention

`temporal-admin` bootstraps: `critical-offboarding` (720h), `provisioning` (336h), `reconciliation` (168h), `analysis` (168h).

## Observability

Temporal web UI: **http://localhost:8234** (namespaces/critical-offboarding).
