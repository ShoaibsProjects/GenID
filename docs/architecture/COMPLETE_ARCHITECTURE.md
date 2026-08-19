# GenID — Complete Architecture & Feature Map

> Production-grade Identity & Access Intelligence Platform
> Risk Scoring: 0-1000 scale (industry standard: SailPoint, Saviynt, CyberArk)

---

## System Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              GENID PLATFORM                                  │
│                                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
│  │  IDENTITY     │  │  RISK        │  │  ACCESS      │  │  GOVERNANCE  │   │
│  │  INTELLIGENCE │  │  INTELLIGENCE│  │  INTELLIGENCE│  │  & COMPLIANCE│   │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘   │
│         │                 │                 │                 │             │
│         └─────────────────┴─────────────────┴─────────────────┘             │
│                                    │                                         │
│                           ┌────────┴────────┐                                │
│                           │   EVENT BUS     │                                │
│                           │   (NATS)        │                                │
│                           └────────┬────────┘                                │
│                                    │                                         │
│  ┌─────────────────────────────────┴─────────────────────────────────┐      │
│  │                         DATA LAYER                                 │      │
│  │  PostgreSQL (RLS) │ Neo4j (Graph) │ Redis (Cache) │ Temporal       │      │
│  └───────────────────────────────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Module 1: Identity Intelligence (Analytics)

### What It Does
Complete visibility into every identity, access pattern, and relationship in the organization.

### Features (from ObserveID → GenID)

| # | Feature | Status | API Endpoint | Description |
|---|---------|--------|--------------|-------------|
| 1 | **Identities** | ✅ | `/api/v1/identities` | All identities with attributes |
| 2 | **Identities with Attributes** | ✅ | `/api/v1/identities?expand=attributes` | Identity detail view |
| 3 | **Accounts** | ✅ | `/api/v1/accounts` | All accounts across integrations |
| 4 | **Accounts with Attributes** | ✅ | `/api/v1/accounts?expand=attributes` | Account detail view |
| 5 | **Entitlements** | ✅ | `/api/v1/entitlements` | All entitlements |
| 6 | **Entitlements with Attributes** | ✅ | `/api/v1/entitlements?expand=attributes` | Entitlement detail view |
| 7 | **Resources** | ✅ | `/api/v1/resources` | All resources |
| 8 | **Resources with Attributes** | ✅ | `/api/v1/resources?expand=attributes` | Resource detail view |
| 9 | **Roles** | ✅ | `/api/v1/groups` | All roles/groups |
| 10 | **Roles with Attributes** | ✅ | `/api/v1/groups?expand=attributes` | Role detail view |
| 11 | **Detected Roles** | 🔜 | `/api/v1/roles/detected` | ML-discovered roles |
| 12 | **Detected Entitlements** | 🔜 | `/api/v1/entitlements/discovered` | Discovered entitlements |
| 13 | **Detected Sessions** | 🔜 | `/api/v1/sessions/detected` | Anomalous session detection |
| 14 | **Who has access to what** | ✅ | `/api/v1/access/graph` | Access path visualization |
| 15 | **Who has what roles** | ✅ | `/api/v1/groups/{id}/identities` | Role membership |
| 16 | **Role Entitlements** | ✅ | `/api/v1/groups/{id}/entitlements` | Entitlements in roles |
| 17 | **Integrations** | ✅ | `/api/v1/connectors` | All integrations |
| 18 | **Integrations with Attributes** | ✅ | `/api/v1/connectors?expand=attributes` | Integration detail |
| 19 | **Integrations Inventory** | ✅ | `/api/v1/connectors/inventory` | Full inventory |
| 20 | **Credential Log** | 🔜 | `/api/v1/audit/credentials` | Credential actions |
| 21 | **Audit Log** | ✅ | `/api/v1/audit` | Full audit trail |
| 22 | **Workflows** | ✅ | `/api/v1/workflows` | Workflow tracking |
| 23 | **Requests** | ✅ | `/api/v1/requests` | Access requests |
| 24 | **Requests with Attributes** | ✅ | `/api/v1/requests?expand=attributes` | Request detail |
| 25 | **Tasks** | ✅ | `/api/v1/tasks` | Workflow tasks |
| 26 | **Tasks with Attributes** | ✅ | `/api/v1/tasks?expand=attributes` | Task detail |

### Data Model
```
Identity → Account → Entitlement → Resource
   ↓           ↓          ↓          ↓
Attributes  Attributes  Attributes  Attributes
   ↓           ↓          ↓          ↓
Roles ←───── Groups ────→ Policies → Risk Score
```

---

## Module 2: Risk Intelligence (0-1000 Scale)

### Risk Scoring Formula

```
┌─────────────────────────────────────────────────────────────────┐
│                    COMPOSITE RISK SCORE                          │
│                                                                 │
│  Final Score = (Static Risk × 0.3) + (Dynamic Risk × 0.7)      │
│                                                                 │
│  Where:                                                         │
│  Static Risk = f(entitlements, roles, resource criticality)     │
│  Dynamic Risk = f(events, behavior, peer deviation)             │
│                                                                 │
│  Range: 0 (no risk) to 1000 (maximum risk)                      │
└─────────────────────────────────────────────────────────────────┘
```

### Static Risk (30% weight)

```
S1 = avg_entitlement_risk (Critical=1000, High=700, Medium=400, Low=100)
S2 = (resource_count × 50) + (max_depth × 100) + (standing_privilege × 30)
S3 = policy_violation_count × 200

Static Risk = min(1000, S1×0.4 + S2×0.3 + S3×0.3)
```

### Dynamic Risk (70% weight)

| Event Type | Base Delta | Severity Multiplier |
|------------|-----------|-------------------|
| auth.failed_login | 100 | critical=2.0, high=1.5, low=0.5 |
| auth.mfa_failure | 75 | critical=2.0, high=1.5, low=0.5 |
| auth.impossible_travel | 150 | critical=2.0, high=1.5, low=0.5 |
| auth.password_spray | 125 | critical=2.0, high=1.5, low=0.5 |
| auth.brute_force | 175 | critical=2.0, high=1.5, low=0.5 |
| account.locked | 50 | critical=2.0, high=1.5, low=0.5 |
| session.anomalous | 100 | critical=2.0, high=1.5, low=0.5 |
| entitlement.escalation | 200 | critical=2.0, high=1.5, low=0.5 |
| access.off_hours | 50 | critical=2.0, high=1.5, low=0.5 |
| peer_deviation | 80 | critical=2.0, high=1.5, low=0.5 |
| dormant_account | 60 | critical=2.0, high=1.5, low=0.5 |
| privilege_escalation | 250 | critical=2.0, high=1.5, low=0.5 |
| credential_leaked | 300 | critical=2.0, high=1.5, low=0.5 |

### Risk Decay
```
decay_rate = 5 points per hour without events
decayed_score = max(0, current_score - (decay_rate × hours_elapsed))
```

### Risk Bands

| Band | Range | Action |
|------|-------|--------|
| **Critical** | 800-1000 | Auto-revoke sessions, block access, alert security |
| **High** | 600-799 | Step-up MFA required, alert manager |
| **Elevated** | 300-599 | Trigger micro-review, increase monitoring |
| **Low** | 100-299 | Log and monitor |
| **Minimal** | 0-99 | Standard access |

### Risk Score API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/risk/score/{identityId}` | GET | Get current risk score |
| `/api/v1/risk/history/{identityId}` | GET | Risk score history |
| `/api/v1/risk/band/{identityId}` | GET | Current risk band |
| `/api/v1/risk/recalculate/{identityId}` | POST | Trigger recalculation |
| `/api/v1/risk/events` | POST | Ingest risk event |
| `/api/v1/risk/peer-deviation/{identityId}` | GET | Peer deviation score |
| `/api/v1/risk/dashboard` | GET | Risk dashboard data |

---

## Module 3: Access Intelligence

### Features

| # | Feature | Status | Description |
|---|---------|--------|-------------|
| 1 | **Access Graph** | ✅ | Visualize who has access to what |
| 2 | **Blast Radius** | ✅ | Impact analysis of identity compromise |
| 3 | **Access Path** | ✅ | How an identity got access |
| 4 | **SoD Detection** | 🔜 | Segregation of Duties violations |
| 5 | **Peer Comparison** | 🔜 | Compare access to peers |
| 6 | **Orphan Accounts** | 🔜 | Accounts without owners |
| 7 | **Dormant Access** | 🔜 | Unused access detection |
| 8 | **Over-privileged** | 🔜 | Excessive access detection |

---

## Module 4: IGA & Certifications

### Features

| # | Feature | Status | Description |
|---|---------|--------|-------------|
| 1 | **Access Certifications** | ✅ | Campaign-based reviews |
| 2 | **Event-driven Reviews** | 🔜 | Risk spike triggers review |
| 3 | **Micro-reviews** | 🔜 | Targeted single-identity reviews |
| 4 | **Manager Reviews** | 🔜 | Manager attestation |
| 5 | **Entitlement Owner Reviews** | 🔜 | Owner attestation |
| 6 | **SoD Policy Enforcement** | 🔜 | Prevent conflicting access |
| 7 | **Policy Violations** | 🔜 | Real-time violation detection |

---

## Module 5: Identity Automation (JML & Lifecycle)

### Features

| # | Feature | Status | Description |
|---|---------|--------|-------------|
| 1 | **Joiner** | ✅ | New hire provisioning |
| 2 | **Mover** | ✅ | Role change provisioning |
| 3 | **Leaver** | ✅ | Termination deprovisioning |
| 4 | **Birthright Access** | 🔜 | Auto-provision based on role |
| 5 | **Recertification** | 🔜 | Periodic access review |
| 6 | **Automated Deprovisioning** | 🔜 | Time-based access removal |
| 7 | **Lifecycle Workflows** | ✅ | Temporal-based lifecycle |

---

## Module 6: Self-Service Access Requests

### Features

| # | Feature | Status | Description |
|---|---------|--------|-------------|
| 1 | **Access Request (Self)** | ✅ | Request access for self |
| 2 | **Access Request (Others)** | ✅ | Request access for others |
| 3 | **Permanent Access** | ✅ | Permanent access request |
| 4 | **Just-In-Time (JIT)** | ✅ | Time-bound access |
| 5 | **Privileged Access (PAM)** | 🔜 | Secure privileged accounts |
| 6 | **Firecall/Break Glass** | 🔜 | Emergency elevated access |
| 7 | **Account Removal** | ✅ | Deactivate accounts |
| 8 | **Password Change** | ✅ | Update account password |
| 9 | **Service Account Creation** | ✅ | Create service accounts |
| 10 | **Role Creation** | ✅ | Create new role |
| 11 | **Role Deletion** | ✅ | Remove role |
| 12 | **Role Update** | ✅ | Modify role permissions |
| 13 | **Emergency Deprovisioning** | 🔜 | Revoke all access |
| 14 | **Data Import** | ✅ | Bulk data upload |
| 15 | **Identity Update** | ✅ | Update identity details |
| 16 | **HR Source Check** | 🔜 | Validate HR records |
| 17 | **Source Account Sync** | 🔜 | Sync source accounts |
| 18 | **Custom Operations** | 🔜 | Custom operations |
| 19 | **Shell Import** | 🔜 | Import resources |

---

## Module 7: Integrations

### Features

| # | Feature | Status | Description |
|---|---------|--------|-------------|
| 1 | **Application Onboarding** | ✅ | Connect applications |
| 2 | **Account Discovery** | ✅ | Discover accounts |
| 3 | **Entitlement Discovery** | ✅ | Discover entitlements |
| 4 | **Resource Discovery** | ✅ | Discover resources |
| 5 | **SCIM Provisioning** | ✅ | SCIM 2.0 gateway |
| 6 | **LDAP Connector** | ✅ | LDAP integration |
| 7 | **Entra ID Connector** | ✅ | Azure AD integration |
| 8 | **HR System Integration** | 🔜 | Workday, SAP SuccessFactors |
| 9 | **SIEM Integration** | 🔜 | Splunk, Sentinel |
| 10 | **Threat Intel Feeds** | 🔜 | External IOC feeds |

---

## Module 8: Non-Human Identity (NHI) & AI Agents

### Features

| # | Feature | Status | Description |
|---|---------|--------|-------------|
| 1 | **Service Account Registry** | ✅ | Track service accounts |
| 2 | **API Key Management** | 🔜 | API key lifecycle |
| 3 | **AI Agent Governance** | 🔜 | AI agent identity management |
| 4 | **Machine Identity Risk** | 🔜 | NHI risk scoring |
| 5 | **Bot Detection** | 🔜 | Automated identity detection |
| 6 | **Credential Rotation** | 🔜 | Automated rotation |

---

## Data Flow Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           EVENT INGESTION                                    │
│  HR Systems │ IDPs │ SIEM │ SCIM │ Cloud │ Endpoints │ AI Agents           │
└─────────────────────────────┬───────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           NATS EVENT BUS                                     │
│  identity.events.* │ auth.events.* │ access.events.* │ risk.events.*       │
└─────────────────────────────┬───────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
┌──────────────────┐ ┌───────────────┐ ┌──────────────────┐
│  EVENT PROCESSOR │ │  RISK ENGINE  │ │  LIFECYCLE       │
│  (Consume)       │ │  (Score)      │ │  WORKFLOWS       │
└────────┬─────────┘ └───────┬───────┘ └────────┬─────────┘
         │                   │                   │
         ▼                   ▼                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              DATA LAYER                                      │
│                                                                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │ PostgreSQL   │  │ Neo4j       │  │ Redis       │  │ Temporal    │        │
│  │ (RLS)       │  │ (Graph)     │  │ (Cache)     │  │ (Workflows) │        │
│  │             │  │             │  │             │  │             │        │
│  │ Identities  │  │ Access      │  │ Sessions    │  │ JML         │        │
│  │ Accounts    │  │ Paths       │  │ Risk Cache  │  │ Approvals   │        │
│  │ Entitlements│  │ Relationships│ │ Rate Limits │  │ Certifications│      │
│  │ Audit Log   │  │ Risk Scores │  │ JTI Block   │  │ Provisioning│        │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘        │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Risk Score Calculation Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        RISK SCORE CALCULATION                                │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ STATIC RISK (30%)                                                    │    │
│  │                                                                      │    │
│  │  Entitlement Risk ──┐                                                │    │
│  │  Role Risk ─────────┼──→ Weighted Average ──→ S1                    │    │
│  │  Resource Criticality┘                                                │    │
│  │                                                                      │    │
│  │  Policy Violations ──→ S2 (count × 200)                             │    │
│  │                                                                      │    │
│  │  Static = min(1000, S1×0.7 + S2×0.3)                                │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                              │                                               │
│                              ▼                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ DYNAMIC RISK (70%)                                                   │    │
│  │                                                                      │    │
│  │  Events ──→ Event Deltas ──→ Sum ──→ D1                             │    │
│  │                                                                      │    │
│  │  Peer Deviation ──→ D2 (0-200)                                      │    │
│  │                                                                      │    │
│  │  Behavior Anomaly ──→ D3 (0-150)                                    │    │
│  │                                                                      │    │
│  │  Decay ──→ -5/hour since last event                                 │    │
│  │                                                                      │    │
│  │  Dynamic = min(1000, max(0, D1 + D2 + D3 - decay))                  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                              │                                               │
│                              ▼                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ FINAL SCORE                                                         │    │
│  │                                                                      │    │
│  │  Final = (Static × 0.3) + (Dynamic × 0.7)                           │    │
│  │                                                                      │    │
│  │  Band = f(Final): minimal | low | elevated | high | critical        │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Navigation Map (How Everything Links)

```
HOME
├── Dashboard
│   ├── Risk Overview
│   ├── Identity Count
│   ├── Access Requests
│   └── Compliance Status
│
├── Identity Intelligence
│   ├── Identities
│   │   ├── Identity Detail
│   │   │   ├── Accounts
│   │   │   ├── Entitlements
│   │   │   ├── Roles
│   │   │   ├── Risk Score
│   │   │   ├── Access Graph
│   │   │   └── Activity History
│   │   └── Bulk Operations
│   ├── Accounts
│   ├── Entitlements
│   ├── Resources
│   ├── Roles
│   └── Access Graph
│
├── Risk Intelligence
│   ├── Risk Dashboard
│   ├── Risk Scores
│   ├── Risk Events
│   ├── Peer Analysis
│   └── Anomaly Detection
│
├── Access Intelligence
│   ├── Blast Radius
│   ├── Access Paths
│   ├── SoD Violations
│   └── Orphan Accounts
│
├── IGA & Certifications
│   ├── Certification Campaigns
│   ├── Campaign Detail
│   ├── Micro-reviews
│   ├── SoD Policies
│   └── Policy Violations
│
├── Identity Automation
│   ├── Joiner Workflows
│   ├── Mover Workflows
│   ├── Leaver Workflows
│   └── Lifecycle Policies
│
├── Access Requests
│   ├── Self-Service
│   │   ├── Request Access (Self)
│   │   ├── Request Access (Others)
│   │   ├── JIT Access
│   │   └── Privileged Access
│   ├── Manager Approval
│   └── Emergency Access
│
├── Integrations
│   ├── Applications
│   ├── Connectors
│   ├── Discovery
│   └── Provisioning
│
├── NHI & AI Agents
│   ├── Service Accounts
│   ├── API Keys
│   ├── AI Agents
│   └── Machine Identity Risk
│
└── Settings
    ├── Risk Configuration
    ├── Policy Engine
    ├── Workflow Designer
    └── Tenant Settings
```

---

## Implementation Phases

### Phase 1: Foundation (DONE)
- ✅ Event-driven risk scoring (0-1000)
- ✅ Risk bands (minimal/low/elevated/high/critical)
- ✅ Risk decay (5 points/hour)
- ✅ NATS event bus
- ✅ Neo4j persistence
- ✅ Temporal workflows
- ✅ Event ingestion API

### Phase 2: Intelligence Layer
- 🔜 Peer-group deviation scoring
- 🔜 Static risk from entitlements/roles
- 🔜 Combined risk formula (static + dynamic)
- 🔜 Event-driven micro-reviews
- 🔜 Session termination on critical risk
- 🔜 Risk dashboard API

### Phase 3: Full IGA
- 🔜 Certification campaigns
- 🔜 SoD policy enforcement
- 🔜 Access request workflows
- 🔜 JML automation
- 🔜 Manager approvals

### Phase 4: Enterprise
- 🔜 SIEM integration
- 🔜 Threat intelligence feeds
- 🔜 ML anomaly detection
- 🔜 Response playbooks
- 🔜 NHI governance

---

## Testing Strategy

### Unit Tests
- Risk score calculation
- Risk band assignment
- Decay calculation
- Event processing

### Integration Tests
- Event ingestion → NATS → Processor → Neo4j
- Risk score → Workflow trigger
- API authentication & authorization

### End-to-End Tests
- Full event flow: ingest → process → score → action
- Risk decay over time
- Peer deviation detection
- Certification campaign lifecycle

### Test Data
- 100+ sample identities
- Multiple integration sources
- Various risk scenarios
- SoD policy violations

---

## Key Metrics to Track

| Metric | Target |
|--------|--------|
| Risk score calculation latency | < 100ms |
| Event processing throughput | > 1000 events/sec |
| Risk score accuracy | > 95% |
| False positive rate | < 5% |
| Mean time to detect risk | < 1 minute |
| Mean time to respond | < 5 minutes |

---

*Last updated: 2026-08-11*
*Version: 2.0*
