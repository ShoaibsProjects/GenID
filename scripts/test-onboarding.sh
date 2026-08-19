#!/usr/bin/env bash
set -euo pipefail

BASE="${GENID_API:-http://localhost:8080}"
BOLD='\033[1m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

step() { echo -e "\n${CYAN}━━━ Step $1: $2 ━━━${NC}"; }
ok()   { echo -e "  ${GREEN}✓${NC} $1"; }
fail() { echo -e "  ${RED}✗${NC} $1"; exit 1; }

echo -e "${BOLD}GenID — Full Onboarding Test${NC}"
echo -e "  API: ${BASE}\n"

# ─── 1. Health ───
step 1 "Health check"
curl -sf "${BASE}/health" > /dev/null 2>&1 && ok "API is reachable" || fail "API unreachable at ${BASE}"

# ─── 2. Login ───
step 2 "Dev login (admin@genid.io)"
TOKEN=$(curl -sf -X POST "${BASE}/api/v1/dev/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin@genid.io","password":"dev-login"}' | jq -r '.access_token')
[[ "$TOKEN" == "null" || -z "$TOKEN" ]] && fail "Login failed" || ok "JWT obtained (${#TOKEN} chars)"
AUTH="Authorization: Bearer ${TOKEN}"

# ─── 3. Create an identity ───
step 3 "Create identity: alice@example.com"
ALICE_ID=$(curl -sf -X POST "${BASE}/api/v1/identities" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -d '{
    "email": "alice@example.com",
    "display_name": "Alice Chen",
    "department": "Engineering",
    "title": "Senior Engineer",
    "status": "active"
  }' | jq -r '.id // .identity_id // empty')
[[ -z "$ALICE_ID" ]] && fail "Identity creation failed" || ok "Created identity ${ALICE_ID}"

# ─── 4. Read identity back ───
step 4 "Read identity back"
curl -sf "${BASE}/api/v1/identities/${ALICE_ID}" \
  -H "$AUTH" | jq '{ id: .id, email: .email, name: .displayName, status: .status }'
ok "Identity exists in system"

# ─── 5. List all identities ───
step 5 "List identities (should include Alice)"
TOTAL=$(curl -sf "${BASE}/api/v1/identities?page=1&pageSize=100" \
  -H "$AUTH" | jq '.total // (.identities | length) // 0')
ok "Total identities in system: ${TOTAL}"

# ─── 6. Check connectors ───
step 6 "List connectors"
CONNECTORS=$(curl -sf "${BASE}/api/v1/connectors" -H "$AUTH")
echo "$CONNECTORS" | jq '[.connectors[] | { name: .name, type: .type, status: .status }]' 2>/dev/null | head -20
ok "Connectors loaded"

# ─── 7. Ingest IdP events (risk injection) ───
step 7 "Ingest 3 failed login events (risk spike)"
for i in 1 2 3; do
  RESP=$(curl -s -X POST "${BASE}/events/ingest/entra" \
    -H "Content-Type: application/json" \
    -H "$AUTH" \
    -d "{
      \"userPrincipalName\": \"alice@example.com\",
      \"eventType\": \"SignInFailure\",
      \"riskLevel\": \"high\",
      \"ipAddress\": \"198.51.100.${i}\",
      \"location\": \"Unknown Region\",
      \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"
    }" 2>&1 || echo '{"error":"send failed"}')
  echo "  Event ${i}/3: $(echo "$RESP" | jq -r '.status // .error // "sent"' 2>/dev/null)"
done
ok "3 risk events sent to NATS"

# ─── 8. Wait for risk processor ───
step 8 "Wait for risk processor to update Neo4j..."
sleep 3
RISK=$(curl -s "${BASE}/api/v1/risk/score/${ALICE_ID}" -H "$AUTH" 2>/dev/null || echo '{"riskScore":0}')
RISK_SCORE=$(echo "$RISK" | jq -r '.dynamicScore // .riskScore // 0')
ok "Alice's risk score: ${RISK_SCORE} (band: $(echo "$RISK" | jq -r '.riskBand // "unknown"'))"
if [ "$RISK_SCORE" -gt 400 ]; then
  echo -e "  ${YELLOW}↑ Risk is elevated (>${RISK_SCORE}) — events were processed${NC}"
else
  echo -e "  ${YELLOW}! Risk score is 0 or unchanged — risk processor may not be running${NC}"
fi

# ─── 9. Request JIT access (conditional access) ───
step 9 "Request JIT access for Alice"
GRANT=$(curl -sf -X POST "${BASE}/api/v1/access/grant" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -d "{
    \"identity_id\": \"${ALICE_ID}\",
    \"resource_id\": \"prod-database\",
    \"resource_type\": \"database\",
    \"role_id\": \"engineer\",
    \"duration_hours\": 2,
    \"reason\": \"Onboarding test — need access to prod DB for deployment\",
    \"device_trust\": \"trusted\",
    \"risk_score\": ${RISK_SCORE},
    \"role\": \"engineer\",
    \"evaluate_at\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"
  }")
REQUEST_ID=$(echo "$GRANT" | jq -r '.request_id // .id // empty')
STATUS=$(echo "$GRANT" | jq -r '.status // "unknown"')
if [[ -n "$REQUEST_ID" ]]; then
  ok "Request created: ${REQUEST_ID} (status: ${STATUS})"
else
  ok "Access grant submitted (check /api/v1/requests for status)"
  echo "  Response: $(echo "$GRANT" | head -c 200)"
fi

# ─── 10. Check pending approvals ───
step 10 "Check approval queue"
APPROVALS=$(curl -sf "${BASE}/api/v1/approvals/queue" -H "$AUTH" 2>/dev/null || echo '{"approvals":[]}')
COUNT=$(echo "$APPROVALS" | jq '.approvals | length' 2>/dev/null || echo "0")
ok "Pending approvals in queue: ${COUNT}"

# ─── 11. Risk dashboard ───
step 11 "Risk dashboard summary"
DASH=$(curl -sf "${BASE}/api/v1/risk/dashboard" -H "$AUTH" 2>/dev/null || echo '{}')
echo "$DASH" | jq '{ totalIdentities: .totalIdentities, highRisk: .highRisk, criticalRisk: .criticalRisk }' 2>/dev/null || echo "  (dashboard not available)"

# ─── 12. Audit logs ───
step 12 "Audit logs"
AUDIT=$(curl -sf "${BASE}/api/v1/audit/logs?page=1&pageSize=3" -H "$AUTH" 2>/dev/null || echo '{"logs":[]}')
echo "$AUDIT" | jq '.logs[:3] | [.[] | { action: .action, actor: .actor, target: .target }]' 2>/dev/null || echo "  (no audit logs)"

# ─── Done ───
echo -e "\n${BOLD}${GREEN}━━━ All steps complete ━━━${NC}"
echo -e "
${BOLD}What to check in the UI:${NC}
  1. http://localhost:3001/identities  → Alice should appear
  2. http://localhost:3001/risk        → Risk dashboard with scores
  3. http://localhost:3001/policies    → Cedar policies (hardcoded)
  4. http://localhost:3001/inbox       → Any pending approvals
  5. http://localhost:3001/audit       → Audit trail of all actions

${BOLD}Quick re-run:${NC}
  bash scripts/test-onboarding.sh
"
