# GenID Demo for vision sponsor — Script

**URL:** `http://localhost:3001`
**Length:** ~10 minutes (interactive)
**Login:** automatic (dev mode — no password prompt)

The demo is two stories back-to-back: **(A) HR-driven JML** — the platform moves identities automatically based on the source of truth (CSV / HRIS); **(B) Real-time risk response** — behavior drives the score and triggers auto-remediation.

---

## Act A — "HR drives identity, not the other way around"

**Setup:** The connector `csv-hr-demo` points at `/tmp/genid-hr-demo.csv` (5-row HR roster).

### Step 1 — Open the Connectors page
URL: `http://localhost:3001/connectors`
You'll see the `csv-hr-demo` connector card with two buttons:
- `⟳ Sync Now` — caches raw users into the connector cache
- `🧬 Sync HR` — runs the **JML pipeline** (create/update/terminate/reinstate)

### Step 2 — Run the HR sync
Click **🧬 Sync HR** on `csv-hr-demo`. An alert appears with the JML summary:
```
HR sync complete for csv-hr-demo
+5 created · ~0 updated · −0 terminated
```
**Talk track:** "We drop a CSV from Workday, BambooHR, any HRIS — the platform creates the identity in Postgres, fires the `identity.created` outbox event, the outbox processor mirrors it into Neo4j, and the risk engine sets the score to 0. Five new employees, all from one CSV."

### Step 3 — Verify
Navigate to `http://localhost:3001/identities` — show the 5 new identities (priya, rahul, arjun, neha, vikram) with `source=hris`. Or via terminal:
```bash
docker exec genid-postgres psql -U observeid -d observeid -c \
  "SELECT email, department, employee_id, status FROM identities WHERE source='hris' AND employee_id LIKE 'EMP-%';"
```

### Step 4 — Show the "mover" (dept change)
Modify the CSV — Arjun moves from Sales to Enterprise Sales:
```bash
# /tmp/genid-hr-demo.csv line for arjun:
# BEFORE: arjun.patel@observeid.io,Arjun Patel,Arjun,Patel,Sales,Account Executive,EMP-003
# AFTER:  arjun.patel@observeid.io,Arjun Patel,Arjun,Patel,Enterprise Sales,Senior Account Executive,EMP-003
```
Push into the container and re-sync:
```bash
docker cp /tmp/genid-hr-demo.csv genid-identity-service:/tmp/genid-hr-demo.csv
# Click 🧬 Sync HR again
```
Alert: `~1 updated · 0 created · 0 terminated`
**Talk track:** "Mover event: same employee, new department, no entitlement change yet — but the system detects the diff, fires `identity.updated`, peer-risk recomputes, and downstream systems will re-evaluate role fit."

### Step 5 — Show the "leaver"
Remove Vikram's row from the CSV (he's been terminated), re-sync:
```bash
# /tmp/genid-hr-demo.csv — delete the vikram.singh row
docker cp /tmp/genid-hr-demo.csv genid-identity-service:/tmp/genid-hr-demo.csv
# Click 🧬 Sync HR
```
Alert: `0 created · 0 updated · −1 terminated`
**Talk track:** "Leaver event: HR record disappears. Status flips to `terminated` in Postgres, `identity.deleted` outbox event fires, the offboarding workflow kicks in — sessions revoked, NHI cascade, audit chain sealed. No human in the loop."

### Step 6 — Show the "reinstatement"
Re-add Vikram to the CSV (he came back). Re-sync:
Alert: `~1 updated · 0 created · 0 terminated` — Vikram's status flips back to active.
**Talk track:** "Reinstatement is the most-missed JML stage in legacy IAM. We re-activate and **require a certification** — never silently restore old entitlements. Manager re-approves each. That's how you avoid the silent over-privilege drift."

### Step 7 — Idempotency proof
Click **🧬 Sync HR** again with no CSV change → alert: `0 · 0 · 0`
**Talk track:** "Every sync is idempotent. Run it 100 times, same result. That's the 20-min cron schedule doing its job without flooding the event bus."

---

## Act B — "Risk responds to behavior, not entitlement"

### Step 8 — Reset demo-bob's risk
The risk demo identity is `demo-bob` (uuid `demo-bob` in Neo4j). Currently clean (score 0, band minimal).

### Step 9 — Open the Risk page
URL: `http://localhost:3001/risk`
Show the dashboard distribution: `total_identities`, bands, average score. demo-bob sits in `minimal`.

### Step 10 — Send failed logins
URL: `http://localhost:3001/events` → pick identity `demo-bob`, event `auth.failed_login`, severity `high`, click **Send**. Repeat 2 more times.

After 3 events, navigate back to `/risk` → click on demo-bob → score has climbed to **450 / elevated**, band updated to `elevated`, 2 micro-reviews auto-created.
**Talk track:** "No entitlement changed. The score went up because of **behavior** — failed logins. The event processor (queue-group on NATS JetStream) subscribed, applied the risk formula, mutated Neo4j atomically."

### Step 11 — Push into critical
Send one event: `credential_leaked`, severity `critical`. After ~1 second, refresh `/risk` → demo-bob is now **1000 / critical**. Sessions terminated. Critical-risk micro-review due in 3 days.
**Talk track:** "Critical band auto-remediates: every active session terminated, micro-review created with due-date, audit chain entry recorded. The whole loop took milliseconds — Postgres outbox → NATS JetStream → queue-group risk-processor → Neo4j → micro-review creation."

### Step 12 — Show the queue-group durability
```bash
curl -s 'http://localhost:8222/jsz?streams=1&consumers=1' \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(json.dumps(d, indent=2))" \
  | grep -E 'queue-group|name|risk-processor' | head
```
**Talk track:** "Risk-processor runs in a NATS queue group — scale horizontally by adding replicas. No duplicate work, no shared state needed."

---

## Closing — The platform story

**The fundamental shift:** today's IAM is **entitlement-centric** (focus on what people *have*). GenID is **behavior-centric** (focus on what they *do*). Two employees with identical access can have wildly different risk scores — and the score moves in real time.

**What's wired (and live today):**
- **HR source → JML** (Phase 1) — CSV drop creates, modifies, terminates, reinstates
- **Outbox → Neo4j sync** at 100ms poll, batched `UNWIND` writes
- **NATS queue-group risk processor** — scale horizontally
- **Postgres-backed AES-256-GCM vault** for connector secrets
- **RLS** on 28+ connector tables

**What ships in 2–3 weeks of follow-on work** (the brutal-engineering plan, Phase 2+):
- Per-connector Temporal ScheduleWorkflow (true cron sync)
- LCM write-back to source system (terminate in real AD/Okta when leaver fires)
- CSV-onboarding form UI
- Role engine (thousands of rule-based role evaluations per day)
- Multi-tenant isolation at the connector cache layer

---

## Cheat-sheet: quick commands

| What | How |
|---|---|
| Login (admin JWT) | `curl -s -X POST localhost:8080/api/v1/dev/login -H 'Content-Type: application/json' -d '{"username":"admin@observeid.io","password":"dev-login"}'` |
| Reset risk on demo-bob | `docker exec genid-neo4j cypher-shell -u neo4j -p observeid123 "MATCH (i:Identity {uuid:'demo-bob'}) SET i.risk_score=0.0, i.risk_dynamic=0.0, i.risk_band='minimal', i.risk_event_count=0;"` |
| Reset HR identities | `docker exec genid-postgres psql -U observeid -d observeid -c "DELETE FROM identities WHERE source='hris' AND employee_id LIKE 'EMP-%';"` |
| Force outbox sync | `docker compose restart identity-service` |
| View event-processor logs | `docker logs -f genid-event-processor` |
| View NATS consumers | `curl -s 'http://localhost:8222/jsz'` |
| Temporal web UI | `http://localhost:8234` |

---

## What to NOT show

- The `audit chain append failed: column "hash" does not exist` warnings — those are from an old audit column name and don't affect functionality (will be fixed in Phase 2)
- The legacy connectors (`HR Source`, `csv-hr-source`, etc.) — clutter, leave them alone
- The `openfga` container being unhealthy — it's unused at the moment, will be wired in Phase 2
