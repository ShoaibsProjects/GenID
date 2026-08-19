# Risk Engine

The risk engine is the heart of GenID. It computes a **0–1000 composite risk score** per identity (industry-standard scale used by SailPoint, Saviynt, CyberArk) that blends static access posture, live event activity, and peer comparison.

## Composite formula

```
Final = min(1000, Static × 0.3 + Dynamic × 0.5 + Peer × 0.2)
```

| Component | Weight | Range | Meaning |
|-----------|--------|-------|---------|
| **Static** | 0.3 | 0–1000 | What the identity *holds* (entitlements, resources, roles) |
| **Dynamic** | 0.5 | 0–1000 | What the identity *does* (security events, decayed over time) |
| **Peer** | 0.2 | 0–200 | How the identity compares to peers (same department) |

## Risk bands

| Band | Range | Automatic action |
|------|-------|------------------|
| **Critical** | 800–1000 | Terminate all sessions + create `critical_risk` micro-review |
| **High** | 600–799 | Create `high_risk` micro-review |
| **Elevated** | 300–599 | Create `elevated_risk` micro-review |
| **Low** | 100–299 | Log and monitor (no action) |
| **Minimal** | 0–99 | Standard access (no action) |

> The same thresholds are enforced in three places (Cypher CASE, `scoreToBand` in the processor, and `scoreToBand` in combined_risk.go). The legacy `risk_workflow.go` also defines `high → step-up MFA` and `elevated → alert`, but that workflow is **not wired** into the running system — the event processor's inline actions are the live path.

## Static risk (30%)

Graph posture: walks `HAS_ROLE→GRANTS→Entitlement→ACCESSES→Resource` plus direct `HAS_ENTITLEMENT` / `HAS_DIRECT_ACCESS` edges.

### Entitlement risk scaling

| `risk_classification` | Score |
|-----------------------|-------|
| `critical` | 1000 |
| `high` | 700 |
| `medium` | 400 |
| `low` | 100 |
| *(default)* | 200 |

### Resource criticality scaling

| `criticality` | Score |
|---------------|-------|
| `p0` / `critical` | 1000 |
| `p1` / `high` | 700 |
| `p2` / `medium` | 400 |
| `p3` / `low` | 100 |
| *(default)* | 200 |

### Formula

```
entScore     = min(400, avgEntitlementRisk × 0.4)
resScore     = min(300, avgResourceRisk   × 0.3)
volumeScore  = min(300, roleCount×30 + entitlementCount×20 + resourceCount×10)

Static = min(1000, entScore + resScore + volumeScore)
```

### Factors emitted

`high_risk_entitlements` (avg > 600), `critical_resource_access` (avg > 600), `excessive_entitlements=N` (>10), `excessive_roles=N` (>5), `excessive_resources=N` (>15), or `standard_access_profile`.

## Dynamic risk (50%)

The event-driven component — the answer to *"why did the score go up? because of events, not entitlement changes."*

### Event weight table

| Event type | Base weight |
|------------|-------------|
| `credential_leaked` | 300 |
| `privilege_escalation` | 250 |
| `entitlement.escalation` | 200 |
| `auth.brute_force` | 175 |
| `auth.impossible_travel` | 150 |
| `auth.password_spray` | 125 |
| `auth.failed_login` | 100 |
| `session.anomalous` | 100 |
| `peer_deviation` | 80 |
| `auth.mfa_failure` | 75 |
| `dormant_account` | 60 |
| `account.locked` | 50 |
| `access.off_hours` | 50 |

Weights are configurable at runtime via `Processor.SetRiskWeight(eventType, weight)` / `Processor.RiskWeight(eventType)`.

### Severity multiplier

| `severity` | Multiplier |
|------------|------------|
| `critical` | 2.0 |
| `high` | 1.5 |
| `medium` *(default)* | 1.0 |
| `low` | 0.5 |

```
ScoreDelta = baseWeight × severityMultiplier
```

Example: `auth.failed_login` + `high` → +150; `credential_leaked` + `critical` → +600.

### Decay

- Decay rate: **5 points per hour** (`_decay_rate`).
- Applied on every event: past score decays before the new delta is added.

```
decayedScore = max(0, current − 5 × hoursSinceLastEvent)
newScore     = clamp(decayed + delta, 0, 1000)
```

Both `risk_score` (combined) and `risk_dynamic` (dynamic only) decay and accumulate identically. `hoursSinceLastEvent` derives from `risk_last_updated` on the node.

## Peer risk (20%)

Identifies anomalies vs. same-department peers (`PeerDeviation`, 0–200).

```
roleScore   = min(100, |myRoles − avgPeerRoles| × 10)
entScore    = min(150, |myEnts   − avgPeerEnts|   × 15)
directScore = min(50,  myDirectAccessCount        × 5)

Peer = roleScore + entScore + directScore    (cap 200)
```

### Factors

`role_deviation=N_above_peers` (>2), `entitlement_deviation=N_above_peers` (>3), `direct_access=N_unusual` (>2), `peer_aligned`, or `no_peers_found`.

## Band actions (event processor)

On every event the processor:

1. Computes delta, applies decay, updates the node in Neo4j:
   `risk_score`, `risk_dynamic`, `risk_band`, `risk_last_event`, `risk_last_source`, `risk_last_severity`, `risk_event_count`, `risk_last_updated`.
2. Publishes `genid-events.identity.events.risk.updated`.
3. Fires band actions:
   - **critical** → `SessionManager.TerminateAllSessions(identity, "critical_risk_auto_revoke")` then `MicroReview.TriggerReview{trigger_type:"critical_risk", risk_score:800}`.
   - **high** → `TriggerReview{trigger_type:"high_risk", risk_score:600}`.
   - **elevated** → `TriggerReview{trigger_type:"elevated_risk", risk_score:300}`.

Micro-reviews create a `:Review` node (`status:'pending'`, `due_date = created + 3 days`) with a `HAS_REVIEW` edge from the identity.

## How the components combine

| Module | File | Role |
|--------|------|------|
| `eventing/processor.go` | NATS consumer → dynamic score + band actions | |
| `risk/static_risk.go` | `StaticRisk.Calculate` | graph posture |
| `risk/peer_deviation.go` | `PeerDeviation.Calculate` | peer comparison |
| `risk/combined_risk.go` | `CombinedRisk.Calculate` + `Persist` | blend + write to Neo4j |
| `risk/session_manager.go` | create/terminate sessions | |
| `risk/micro_review.go` | create/complete reviews | |
| `risk/dashboard.go` | fleet dashboard + per-identity detail | |
| `risk/risk.go` | legacy V1 0–100 scorer (`CalculateIdentityRisk`) | |

The combined score is persisted via `POST /api/v1/risk/calculate/{id}` or recomputed by `RiskRecalculationCronWorkflow` (every 15 min by default) and after provision/revoke/role activities.

## Reading risk back

- `GET /api/v1/risk/dashboard` — fleet stats + top 10
- `GET /api/v1/risk/score/{identityId}` — per-identity breakdown (static/dynamic/peer, factors, sessions, reviews)
- `GET /api/v1/risk/peer/{identityId}` — peer deviation detail
- `GET /api/v1/risk/calculate/{identityId}` (POST) — recompute + persist
- `GET /api/v1/risk/sessions/{identityId}` — sessions
- `GET /api/v1/risk/reviews/{identityId}` — micro-reviews

See [API → Risk](../api/overview.md#risk-intelligence) for full contracts.

## Related

- [Event Catalog](../events/catalog.md)
- [Data model → Neo4j risk properties](../data-model/neo4j.md)
- [Demo guide](../guides/demo.md)
