# Event Catalog

The event-driven backbone: writes emit outbox events → outbox processor → Neo4j + NATS `genid-events.>` → event processor applies risk deltas and band actions.

## Pipeline

```
[POST /api/v1/events/ingest]                  [internal producers]
        |  (accepts any event_type)            (outbox, access grants, auth)
        v
   NATS JetStream stream: genid-events (subjects genid-events.>)
        |
        v
   event-processor  (durable consumer "risk-processor",
                     ManualAck, MaxDeliver=3, ACK on success)
        |
        +--> update Identity risk score (Neo4j, atomic SUM)
        +--> risk_band: minimal/low/elevated/high/critical
        +--> band actions: critical -> TerminateAllSessions + TriggerReview(critical_risk)
        |                  high    -> TriggerReview(high_risk)
        |                  elevated-> TriggerReview(elevated_risk)
        v
   Risk recalculation cron (every 15 min) reconciles drifted scores
```

## Event types & risk weights

| event_type | Weight | Triggered by |
|-----------|--------|--------------|
| `credential_leaked` | 300 | Credential ingestion / breach feed |
| `privilege_escalation` | 250 | Privilege change detection |
| `entitlement.escalation` | 200 | Entitlement change detection |
| `auth.brute_force` | 175 | Brute-force detection |
| `auth.impossible_travel` | 150 | Impossible travel detection |
| `auth.password_spray` | 125 | Password spray detection |
| `auth.failed_login` | 100 | Repeated failed logins |
| `session.anomalous` | 100 | Anomalous session detection |
| `peer_deviation` | 80 | Peer-comparison deviation |
| `auth.mfa_failure` | 75 | MFA failure |
| `dormant_account` | 60 | Inactivity detection |
| `account.locked` | 50 | Account lock |
| `access.off_hours` | 50 | Off-hours access |

## Severity multiplier

| Severity | Multiplier |
|----------|------------|
| critical | 2.0 |
| high | 1.5 |
| medium | 1.0 |
| low | 0.5 |

`effective_weight = base_weight × severity_multiplier` — capped at 1000.

## Decay

Risk decays **5 points/hour** on each recalc, floor 0. Static + peer components recomputed every 15 min.

## Band actions

| Band | Range | Action |
|------|-------|--------|
| minimal | 0–99 | none |
| low | 100–299 | none |
| elevated | 300–599 | micro-review (`elevated_risk`) |
| high | 600–799 | micro-review (`high_risk`) |
| critical | 800–1000 | terminate all sessions + micro-review (`critical_risk`, due +3d) |

## Ingestion

```
POST /api/v1/events/ingest
{
  "event_type": "auth.failed_login",   // required
  "identity_id": "<uuid>",             // target
  "source": "auth-service",
  "severity": "high",
  "metadata": { "attempts": 3, "ip_address": "10.0.0.5" }
}
```
→ `202 {"status":"accepted","eventId":"<uuid>"}`. Unknown event types still ingest (generic weight 50) and are surfaced as anomalies.

## Internal event subjects

Outbox events (`identity.*`, `role.*`, `entitlement.*`) republish into the same `genid-events.>` stream for risk/audit consumers.

## Related

- [Risk engine](../architecture/risk-engine.md)
- [Events API](../api/overview.md#events)
- [NATS / event-processor](../architecture/overview.md)
