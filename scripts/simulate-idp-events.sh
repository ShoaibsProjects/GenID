#!/usr/bin/env bash
#
# simulate-idp-events.sh — stakeholder POC, end to end, on a laptop.
#
# Fires a burst of Microsoft Entra-style failed sign-in events at the GenID
# ingestion edge (POST /api/v1/events/ingest/entra). The events flow:
#
#   curl → ingestion edge (normalize) → NATS JetStream → risk processor
#        → Neo4j risk_score/risk_band update → (critical) session kill
#
# Prerequisites:
#   - Full stack running:  cd infrastructure && docker compose up -d
#   - Backend running:     cd backend && go run cmd/identity-service/main.go
#   - Event processor:     cd backend && go run cmd/event-processor/main.go
#   - jq + curl installed
#
# Usage:
#   ./scripts/simulate-idp-events.sh [identity_id] [count] [severity]
#
set -euo pipefail

API="${GENID_API:-http://localhost:8080/api/v1}"
IDENTITY_ID="${1:-}"
COUNT="${2:-3}"
SEVERITY="${3:-high}"

echo "==> GenID IdP-event simulation (source: microsoft-entra)"
echo "    API: $API"

# ─── 1. Dev login ────────────────────────────────────────────────
TOKEN=$(curl -sf -X POST "$API/dev/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin@genid.io","password":"dev-login"}' | jq -r .access_token)
if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
  echo "!! dev login failed — is the backend running with DEV_LOGIN_ENABLED=true?"
  exit 1
fi
echo "==> Authenticated (dev login)"

# ─── 2. Resolve target identity ──────────────────────────────────
if [[ -z "$IDENTITY_ID" ]]; then
  IDENTITY_ID=$(curl -sf "$API/identities?page=1&pageSize=1" \
    -H "Authorization: Bearer $TOKEN" | jq -r '.identities[0].id // .items[0].id // .[0].id // empty')
fi
if [[ -z "$IDENTITY_ID" ]]; then
  echo "!! no identity found — create one first (UI → Identities, or POST /identities)"
  exit 1
fi
echo "==> Target identity: $IDENTITY_ID"

show_risk() {
  curl -sf "$API/identities/$IDENTITY_ID" -H "Authorization: Bearer $TOKEN" \
    | jq '{id: (.id // .identity.id), risk_score: (.risk_score // .identity.risk_score), risk_band: (.risk_band // .identity.risk_band)}' 2>/dev/null || true
}

echo "==> Risk BEFORE:"
show_risk

# ─── 3. Fire the failed-login burst ──────────────────────────────
for i in $(seq 1 "$COUNT"); do
  RESP=$(curl -sf -X POST "$API/events/ingest/entra" \
    -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' \
    -d "{
      \"eventType\": \"SignInFailure\",
      \"userPrincipalName\": \"$IDENTITY_ID\",
      \"riskLevel\": \"$SEVERITY\",
      \"ipAddress\": \"203.0.113.$i\",
      \"appDisplayName\": \"Azure Portal\",
      \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"
    }")
  echo "    [$i/$COUNT] $(echo "$RESP" | jq -c '{eventId, eventType, identityId}')"
  sleep 0.3
done

echo "==> Waiting for the risk processor to consume (NATS → Neo4j)..."
sleep 3

echo "==> Risk AFTER:"
show_risk

cat <<EOF

Done. Each SignInFailure at severity "$SEVERITY" adds +150 risk
(weight 100 × severity multiplier 1.5), capped at 1000, with 5 pts/hour decay.

Bands:  <100 minimal · 100+ low · 300+ elevated · 600+ high · 800+ critical
At 800+ (critical): all sessions are terminated and a micro-review is
auto-opened — check the /inbox and /risk pages in the UI.

To push an identity to critical in one run:
  ./scripts/simulate-idp-events.sh $IDENTITY_ID 6 critical

Verify the plumbing without the UI:
  docker exec \$(docker ps -qf name=nats) nats stream info genid-events
EOF
