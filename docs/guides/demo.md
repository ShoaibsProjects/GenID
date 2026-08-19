# Demo Guide

A ~15 minute scripted demo of the core value proposition: **identity risk that responds in real time to behavior — not just static entitlements.**

> **The headline story**: "Bob is a normal employee. Three failed logins and a leaked credential later, his risk score crosses into the **critical** band — and the system automatically terminates his sessions and opens a compliance review. *All without a single entitlement change.*"

## Prerequisites

- Stack running: `docker compose up -d` → 13 healthy containers, UI at **http://localhost:3001**
- Demo graph seeded (auto-seeded on fresh volumes; see [reset](#reset-demo-state) if re-running)

## Demo identities

| Identity | Role | Purpose |
|----------|------|---------|
| `demo-alice` | SAP User | Low/clean baseline |
| `demo-bob` | Jira User | **The star — clean slate for the risk storyline** |
| `demo-carol` | Sales | Peer baseline |
| `demo-dave` | ERP Admin + Prod DB | Over-privileged: static 840, peer 80, **zero events** |

> The identity dropdown in the UI lists **Postgres** users. The `demo-*` identities live only in Neo4j — select **Manual ID** in the Event Simulator and paste the UUID from the identities list.

## Act 1 — The dashboard tells a story (2 min)

1. Open **http://localhost:3001** → auto-login → **Dashboard**.
2. Show the risk distribution: total identities, critical/high/elevated/low/minimal counts, average score.
3. Open **Identities** → note `demo-dave` — a high score with **no events**. Point at the factor breakdown: static (over-privileged) + peer deviation. *"Risk without behavior."*

## Act 2 — The privileged insider (4 min)

1. Click **demo-dave** → drill into risk: `risk_band`, static score, peer score.
2. **Access map / blast radius**: see ERP Admin + Prod DB via direct path.
3. Show the review created for Dave (static/peer triggered). *"Even before Bob's story, the system flags over-privilege — the #1 insider-threat predictor."*

## Act 3 — Bob's bad night (5 min)

1. Go to **Event Simulator**.
2. Identity: **Manual ID** → paste `demo-bob`'s UUID. Event type: `auth.failed_login`, severity `high`, metadata `{"attempts":3}` → **Send**.
3. Send it **two more times** (3 total). Watch the score climb with each one — the dynamic component responds in real time.
4. Open **Bob's risk panel**: score now in the **elevated** band → a micro-review has been auto-created.
5. Now send **`credential_leaked`**, severity `critical` → the combined score crosses into the **critical** band (800+).
6. Open **Reviews**: a `critical_risk` review with a due date. **Sessions** view: Bob's sessions were **auto-terminated**.
7. Navigate to **Events** to replay the exact chain: 3× failed_login → credential_leaked.

> **The pitch**: "Not a single entitlement changed — the score went up because of *behavior*."

## Act 4 — Workflow & audit trail (2 min)

1. **Temporal** at http://localhost:8234 → show workflow history for the triggered actions.
2. **Audit logs** → show the SHA-256 tamper-evident chain entries for each event ingestion.
3. **Grafana** at http://localhost:3000 (admin/observeid123) → risk metrics live.

## Reset demo state

```cypher
MATCH (r:Review) DETACH DELETE r;
MATCH (i:Identity) SET i.risk_score=0, i.risk_dynamic=0, i.risk_band='minimal',
                     i.risk_event_count=0;
```

Run against Neo4j (http://localhost:7474 or `cypher-shell`). Optionally drop/restart `event-processor` to clear the NATS consumer backlog:

```bash
docker compose restart event-processor
```

## Troubleshooting

- **Identity dropdown missing demo-*** → they're Neo4j-only; use Manual ID.
- **Score not moving** → check `event-processor` logs; ensure NATS stream exists; `docker compose restart event-processor`.
- **Old volumes / missing tables** → see [quickstart](../getting-started/quickstart.md#known-gotcha-outbox_events-missing-on-old-volumes).
