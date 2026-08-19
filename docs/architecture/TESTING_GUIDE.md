# GenID — Testing & Navigation Guide

> Quick reference for testing the platform and understanding how everything connects.

---

## How to Test (Step by Step)

### Step 1: Verify Services Are Running
```bash
cd "/Users/shoaibakthar/Documents/Shoaib's IAM/GenID/infrastructure"
docker ps --format "table {{.Names}}\t{{.Status}}"
```

Expected: All 11 services showing "Up" and "healthy"

### Step 2: Check Identity Service Health
```bash
curl -s http://localhost:8080/health | jq
```
Expected: `{"status":"ok","service":"genid-identity"}`

### Step 3: Ingest a Risk Event
```bash
curl -s -X POST http://localhost:8080/api/v1/events/ingest \
  -H "Content-Type: application/json" \
  -H "X-API-Key: admin-secret-key-please-change" \
  -d '{
    "event_type": "auth.failed_login",
    "identity_id": "test-identity-001",
    "source": "azure_ad",
    "severity": "medium"
  }' | jq
```
Expected: `{"eventId":"...","status":"accepted"}`

### Step 4: Check Risk Score in Neo4j
```bash
docker exec genid-neo4j cypher-shell -u neo4j -p observeid123 \
  "MATCH (i:Identity {uuid: 'test-identity-001'}) RETURN i.risk_score, i.risk_band, i.risk_event_count;"
```
Expected: Score should increase by 100 per event

### Step 5: View Event Processor Logs
```bash
docker logs genid-event-processor 2>&1 | tail -5
```
Expected: Shows risk score progression

---

## Risk Score Quick Reference

### Scale: 0 - 1000

```
0-99:     MINIMAL   → Standard access
100-299:  LOW       → Log and monitor
300-599:  ELEVATED  → Trigger micro-review, increase monitoring
600-799:  HIGH      → Step-up MFA required, alert manager
800-1000: CRITICAL  → Auto-revoke sessions, block access, alert security
```

### Event Deltas (per event)

| Event | Delta | With Critical Severity |
|-------|-------|----------------------|
| failed_login | +100 | +200 |
| mfa_failure | +75 | +150 |
| impossible_travel | +150 | +300 |
| password_spray | +125 | +250 |
| brute_force | +175 | +350 |
| account.locked | +50 | +100 |
| session.anomalous | +100 | +200 |
| entitlement.escalation | +200 | +400 |
| access.off_hours | +50 | +100 |
| peer_deviation | +80 | +160 |
| dormant_account | +60 | +120 |
| privilege_escalation | +250 | +500 |
| credential_leaked | +300 | +600 |

### Risk Decay
- **5 points per hour** without new events
- Prevents stale high scores
- Ensures active monitoring

---

## Navigation Map (Frontend Pages)

```
Dashboard
├── Risk Overview Panel
│   ├── Critical Identities Count
│   ├── High Risk Identities Count
│   ├── Recent Risk Events
│   └── Risk Trend Chart
│
├── Identity Intelligence
│   ├── Identities (list/detail)
│   ├── Accounts (list/detail)
│   ├── Entitlements (list/detail)
│   ├── Resources (list/detail)
│   ├── Roles (list/detail)
│   ├── Access Graph (visualization)
│   └── Who Has Access to What
│
├── Risk Intelligence
│   ├── Risk Dashboard
│   ├── Risk Scores (per identity)
│   ├── Risk Events (timeline)
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
├── Access Requests
│   ├── Self-Service Portal
│   │   ├── Request Access (Self)
│   │   ├── Request Access (Others)
│   │   ├── JIT Access
│   │   └── Privileged Access
│   ├── Pending Approvals
│   └── Request History
│
├── Integrations
│   ├── Applications
│   ├── Connectors
│   ├── Discovery Jobs
│   └── Provisioning Logs
│
└── Settings
    ├── Risk Configuration
    │   ├── Event Weights
    │   ├── Decay Rate
    │   ├── Risk Band Thresholds
    │   └── Severity Multipliers
    ├── Policy Engine
    ├── Workflow Designer
    └── Tenant Settings
```

---

## Key Use Cases for Demo

### Use Case 1: Failed Login Risk Escalation
1. Send 3 failed login events for same identity
2. Watch score: 0 → 100 → 200 → 300 (elevated band)
4. Verify micro-review triggered

### Use Case 2: Critical Risk Auto-Revoke
1. Send 8 failed login events for same identity
2. Watch score reach 800+ (critical band)
3. Verify auto-revoke workflow triggered
4. Check Temporal UI for workflow execution

### Use Case 3: Impossible Travel Detection
1. Send impossible_travel event (+150)
2. Follow with failed_login (+100)
3. Score: 0 → 150 → 250 (low → elevated band)

### Use Case 4: Risk Decay
1. Send events to reach score 500
2. Wait 1 hour (or manually adjust)
3. Score should decrease by 5 points

### Use Case 5: Peer Deviation
1. Create multiple identities in same department
2. Give one identity unusual access
3. Peer deviation score increases

---

## Services & Ports

| Service | Port | URL |
|---------|------|-----|
| Identity Service | 8080 | http://localhost:8080 |
| Frontend | 3001 | http://localhost:3001 |
| Neo4j Browser | 7474 | http://localhost:7474 |
| Neo4j Bolt | 7687 | bolt://localhost:7687 |
| Temporal UI | 8234 | http://localhost:8234 |
| Grafana | 3000 | http://localhost:3000 |
| OpenFGA HTTP | 8090 | http://localhost:8090 |
| OpenFGA gRPC | 8091 | http://localhost:8091 |
| NATS | 4222 | nats://localhost:4222 |
| NATS Monitoring | 8222 | http://localhost:8222 |
| Redis | 6379 | redis://localhost:6379 |
| PostgreSQL | 5434 | postgresql://localhost:5434 |

---

## API Reference

### Event Ingestion
```
POST /api/v1/events/ingest
Headers: X-API-Key: admin-secret-key-please-change
Body: {"event_type": "auth.failed_login", "identity_id": "...", "source": "azure_ad", "severity": "medium"}
```

### Identity CRUD
```
GET    /api/v1/identities
GET    /api/v1/identities/{id}
POST   /api/v1/identities
PUT    /api/v1/identities/{id}
DELETE /api/v1/identities/{id}
GET    /api/v1/identities/{id}/entitlements
GET    /api/v1/identities/{id}/blast-radius
POST   /api/v1/identities/{id}/risk/recalculate
```

### Access Management
```
POST /api/v1/access/grant
POST /api/v1/access/revoke
POST /api/v1/access/jit
GET  /api/v1/access/sessions
QUERY /api/v1/access/check
```

### Integrations
```
GET    /api/v1/connectors
POST   /api/v1/connectors
GET    /api/v1/connectors/{id}
POST   /api/v1/connectors/{id}/sync
GET    /api/v1/connectors/inventory
```

### Audit & Compliance
```
GET /api/v1/audit
GET /api/v1/caep/events
GET /api/v1/certifications
```

### SCIM 2.0
```
GET    /scim/v2/Users
POST   /scim/v2/Users
GET    /scim/v2/Users/{id}
PUT    /scim/v2/Users/{id}
PATCH  /scim/v2/Users/{id}
DELETE /scim/v2/Users/{id}
```

---

## UI Walkthrough (End-User Testing)

The frontend runs at **http://localhost:3001**. It auto-logs-in as `admin@genid.io` via dev login — no credentials needed. All data comes from the live API through nginx.

### Pages

| URL | Page | What it shows |
|-----|------|---------------|
| `/dashboard` | Dashboard | System health, service status, architecture map |
| `/risk` | **Risk Intelligence** | Avg score, band distribution, top-risk identities, drill-down per identity (static/dynamic/peer breakdown, sessions, micro-reviews) |
| `/events` | **Event Simulator** | Pick an identity + event type, fire it through the full pipeline (UI → API → NATS → Processor → Neo4j), watch the score gauge move live |
| `/identities` | Identities | Identity catalog (people + non-human) |

### Risk Intelligence Page (`/risk`)
- Auto-refreshes every 10s.
- Click any row in "Top Risk Identities" to open the drill-down:
  - **Score gauge** with static ×0.3 / dynamic ×0.5 / peer ×0.2 breakdown
  - **Contributing factors** (e.g. `high_risk_entitlements`, `critical_resource_access`)
  - **Active sessions** list
  - **Micro-reviews** auto-created when risk crosses a band
- "Recalculate" re-runs the combined risk formula and persists the result.

### Event Simulator Page (`/events`)
1. Pick an identity (or enter a manual ID like `demo-dave`).
2. Pick an event type — weight shown next to each (e.g. `credential_leaked` +300).
3. Optional metadata JSON (e.g. geo info for impossible travel).
4. Click **Send Event** and watch:
   - The **live score gauge** move (polls every 3s)
   - The **event feed** show latency + score delta
   - Band color shift: minimal → low → elevated → high → critical
5. Use **×5 Rapid** / **×8 Attack** for a quick burst.

### Critical Response (what to demo)
When score crosses **800** the processor automatically:
1. **Terminates all active sessions** (`TerminateAllSessions` → sessions set to `terminated`)
2. **Creates a micro-review** (trigger `critical_risk`, due in 3 days)

Verify in the UI: fire `credential_leaked` ×3 on `demo-bob` → score hits 1000/critical → open `/risk/reviews/{id}` (or the drill-down) to see the review; session list shows zero active sessions.

### Demo Data (already seeded in Neo4j)
- `test-identity-001` — Test User, used for basic event testing
- `demo-alice`, `demo-bob`, `demo-carol` — Engineering, standard Jira access only (peer baseline)
- `demo-dave` — Engineering but with **ERP Admin + SAP + Prod DB** (over-privileged) → static risk 840, peer deviation 80 → combined 268 (low)
- Roles: ERP Admin (critical), SAP User (high), Jira User (low)
- Entitlements + Resources with criticality levels

> **Note:** Identities in the dropdown come from PostgreSQL (`/api/v1/identities`); Neo4j graph nodes like `demo-*` are entered via Manual ID.

---

## Demo Script for vision sponsor

### Act 1 — Show the platform is fast & real (2 min)
1. Open **http://localhost:3001/dashboard** — show all systems healthy.
2. Open **Identities** — show the catalog (people + AI agents).

### Act 2 — Prove the event-driven thesis (3 min)
3. Open **Event Simulator** (`/events`), select `demo-bob`.
4. Fire `auth.failed_login` ×3 → gauge moves 0 → 300 (**elevated**), a micro-review auto-appears.
5. Fire `auth.impossible_travel` → 525.
6. Fire `credential_leaked` ×2 → **1000 critical**.
7. Show the **Risk Intelligence** page — `demo-bob` on top of the list, critical count = 1.
8. Open drill-down: static/dynamic/peer breakdown + the auto-created `critical_risk` review.

### Act 3 — Show static + peer intelligence (2 min)
9. In **Risk Intelligence** click `demo-dave` → **Recalculate** → shows static 840 (over-privileged) + peer deviation 80 → combined 268.
10. Explain: even without events, Dave is risky because of *what he has access to* — provisioning that exceeds his peer group.

### Act 4 — Close (1 min)
11. Show the pipeline diagram: UI → API → NATS JetStream → Processor → Neo4j → actions. Every click was a live event, ~50ms end-to-end.

---

*Last updated: 2026-08-12*
