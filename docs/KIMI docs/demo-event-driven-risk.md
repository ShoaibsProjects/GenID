# Demo Runbook — Event-Driven Risk (our stakeholder's POC)

> **The story:** *"A user fails login 3 times at the IdP. That is an identity event.
> We capture it, correlate it, and the risk score moves — react to change, not to clock."*

**Time:** 10 minutes · **Cost:** $0 (laptop only)

---

## 0. Start the stack

```bash
cd infrastructure && docker compose up -d        # PG, Neo4j, Redis, NATS, Temporal, OpenFGA, Grafana
cd backend && go run cmd/identity-service/main.go &   # API + Temporal worker :8080
cd backend && go run cmd/event-processor/main.go &    # NATS risk consumer
```

Wait for `[NATS] connected` in the backend log and `[PROCESSOR] started: queue-group=risk-processor-q` in the processor log.

## 1. Show the edge exists

```bash
curl -s http://localhost:8080/api/v1/events/sources -H "Authorization: Bearer $TOKEN"
# {"sources":["entra","jira","okta"]}
```

This is the answer to "which hub are we using?": **NATS JetStream inside** (see `docs/architecture/ADR-001-event-backbone.md`); Entra/Okta/Jira/Azure Service Bus attach at this edge.

## 2. Fire the failed-login burst

```bash
./scripts/simulate-idp-events.sh            # picks the first identity, 3 failures, high severity
```

Expected console output: risk score **before** → 3 accepted events (`auth.failed_login`) → risk **after** ≈ +450 (3 × 100 weight × 1.5 high-severity multiplier).

## 3. Show it in the product

- **UI → Risk (`/risk`):** the identity's score and band have moved; `risk_last_event = auth.failed_login`, `risk_last_source = microsoft-entra`.
- **Push to critical** (6 events at `critical` severity = +1200 → capped 1000):

  ```bash
  ./scripts/simulate-idp-events.sh <identity_id> 6 critical
  ```

  At 800+: **all sessions for that identity are terminated** and a **micro-review is auto-opened** → visible in **UI → Inbox**. This is the "prevent before detect" moment of the demo.

## 4. Audit trail

Every hop is on the hash-chained ledger: **UI → Audit**. The NATS stream itself is inspectable:

```bash
docker exec $(docker ps -qf name=nats) nats stream info genid-events
```

## 5. Wire a real provider (when ready)

| Provider | What you do |
|---|---|
| **Microsoft Entra ID** | Diagnostic settings → Event Hub / webhook → POST to `/api/v1/events/ingest/entra`. Payload fields map out of the box (`eventType`, `userPrincipalName`, `riskLevel`). |
| **Okta** | Event Hook → `/api/v1/events/ingest/okta`. Set `EVENT_SOURCE_OKTA_SECRET` and configure Okta's HMAC header as `X-GenID-Signature: sha256=<hex>`. |
| **Jira / ITSM** | Webhook → `/api/v1/events/ingest/jira`. |
| **Azure Service Bus** | Set `EVENT_SOURCES_CONFIG` to a JSON file with an `azure-sb` source, or add a subscriber adapter — the ASB→NATS bridge is a Phase 1.5 ticket (see master plan). |
| **Anything else** | Drop a JSON definition into the sources config file — no Go code needed: `{ "name": "...", "mapping": { "event_type": "dot.path", "identity_id": "dot.path" }, "event_type_map": { "ProviderName": "auth.failed_login" } }` |

## Risk math reference (processor weights)

| Canonical event | Base weight |
|---|---|
| `auth.failed_login` | 100 |
| `auth.mfa_failure` | 75 |
| `auth.impossible_travel` | 150 |
| `auth.password_spray` | 125 |
| `auth.brute_force` | 175 |
| `account.locked` | 50 |
| `session.anomalous` | 100 |
| `entitlement.escalation` | 200 |
| `privilege_escalation` | 250 |
| `credential_leaked` | 300 |

Severity multipliers: low ×0.5 · medium ×1.0 · high ×1.5 · critical ×2.0.
Decay: −5 points/hour since last event. Bands: <100 minimal · 100 low · 300 elevated · 600 high · 800 critical.
