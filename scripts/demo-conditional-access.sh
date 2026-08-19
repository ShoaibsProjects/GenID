#!/usr/bin/env bash
# demo-conditional-access.sh
# Fires the 6-case conditional-access matrix against /api/v1/access/grant
# and shows: event → enrichment → policy → auto-approve / step-up / deny.
#
# Prereqs:
#   - identity-service rebuilt + running (docker compose build identity-service && docker compose up -d identity-service)
#   - migrations 006 + 008 applied to postgres
#   - X-API-Key matches API_KEYS on the service (compose default: admin-secret-key-please-change)
#
# Usage: ./scripts/demo-conditional-access.sh
# Env:   GENID_BASE, GENID_API_KEY, GENID_IDENTITY, GENID_RESOURCE

set -uo pipefail

BASE="${GENID_BASE:-http://localhost:8080}"
API_KEY="${GENID_API_KEY:-admin-secret-key-please-change}"
MASTER_KEY="${GENID_MASTER_KEY:-dev-only-master-key-32-bytes-long-x}"
IDENTITY="${GENID_IDENTITY:-00000000-0000-0000-0000-000000000002}"
RESOURCE="${GENID_RESOURCE:-demo-resource-001}"
TENANT="00000000-0000-0000-0000-000000000001"

# Deterministic demo clock: Monday 2026-08-10 in America/New_York.
BUSINESS_AT="2026-08-10T10:00:00-04:00"
AFTERHOURS_AT="2026-08-10T22:00:00-04:00"

AUTH=(-H "X-API-Key: ${API_KEY}" -H "X-Master-Key: ${MASTER_KEY}")

echo "== Conditional Access Demo =="
echo "  base=${BASE}  identity=${IDENTITY}  resource=${RESOURCE}"
echo

# health check
if ! curl -sf "${BASE}/health" >/dev/null; then
  echo "ERROR: ${BASE} not reachable. Start the stack first." >&2
  exit 1
fi
sleep 10 # settle: let the Temporal worker finish re-registering after a restart

# run_case NAME IP DEVICE TRUST RISK ROLE AT EXPECTED
run_case() {
  local name="$1" ip="$2" device="$3" trust="$4" risk="$5" role="$6" at="$7" expected="$8"

  echo "------------------------------------------------------------"
  echo "CASE: ${name}"
  echo "  signals: ip=${ip} device=${device} trust=${trust} risk=${risk} role=${role} at=${at}"
  echo "  expected: ${expected}"

  local body
  body=$(jq -nc \
    --arg identity "$IDENTITY" \
    --arg resource "$RESOURCE" \
    --arg ip "$ip" \
    --arg role "$role" \
    --arg at "$at" \
    --argjson risk "$risk" \
    --arg tenant "$TENANT" \
    '{identity_id:$identity, resource_id:$resource, resource_type:"Resource",
      reason:"conditional-access-demo", requested_by:"00000000-0000-0000-0000-000000000001", tenant_id:$tenant,
      duration_hours:0, risk_score:$risk, role:$role, evaluate_at:$at,
      signals:{ip_address:$ip, user_agent:"demo-conditional-access", device_id:"demo-device"}}')

  local resp request_id status
  resp=$(curl -sf -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -H "X-Device-Trust: ${trust}" \
    -d "$body" "${BASE}/api/v1/access/grant") || {
    echo "  ERROR: grant call failed"
    return 1
  }
  request_id=$(echo "$resp" | jq -r '.request_id // empty')
  echo "  event -> workflow.requested (request_id=${request_id})"

  # poll request status up to ~120s (workflow runs async via Temporal and
  # may lag a few seconds behind worker startup)
  local attempt=0 final_status="" gate=""
  while [ $attempt -lt 240 ]; do
    final_status=$(curl -sf "${AUTH[@]}" "${BASE}/api/v1/requests/${request_id}" | jq -r '.request.status // empty')
    case "$final_status" in
      executed|denied|failed|approved|revoked|completed) break ;;
    esac
    if [ "$final_status" = "pending" ]; then
      # step-up path: break as soon as the approval chain exists
      gate=$(curl -sf "${AUTH[@]}" "${BASE}/api/v1/approvals/queue" | jq -r --arg rid "$request_id" '[.approvals[]? | select(.request_id == $rid)] | if length > 0 then "approval gate open (awaiting approver)" else "" end' 2>/dev/null || echo "")
      [ -n "$gate" ] && break
    fi
    sleep 0.5
    attempt=$((attempt + 1))
  done

  [ -n "$gate" ] && final_status="pending -> ${gate}"
  echo "  result: status=${final_status:-timeout}"
  echo "  verdict: ${expected}"
}

# ── The 6-case matrix ────────────────────────────────────────────
run_case "1. Office-Managed-Business-Low"       "10.0.1.5"    "managed"   "managed"   200 "it-admin" "$BUSINESS_AT"   "Allow -> auto_approve_2h (auto JIT 120m, no gate)"
run_case "2. Office-Unmanaged-Business-Low"     "10.0.1.5"    "unmanaged" "unmanaged" 200 "it-admin" "$BUSINESS_AT"   "StepUp -> approval gate (needs human approval)"
run_case "3. Remote-Managed-Business-Low"       "203.0.113.1" "managed"   "managed"   200 "it-admin" "$BUSINESS_AT"   "StepUp -> approval gate (public network)"
run_case "4. Office-Managed-AfterHours-Oncall"  "10.0.1.5"    "managed"   "managed"   200 "oncall"   "$AFTERHOURS_AT" "Allow -> approve_30m_jit (auto JIT 30m)"
run_case "5. Office-Managed-Business-Critical"  "10.0.1.5"    "managed"   "managed"   850 "it-admin" "$BUSINESS_AT"   "Deny -> rejected (critical risk)"
run_case "6. VPN-Managed-Business-Low"          "172.16.0.5"  "managed"   "managed"   200 "it-admin" "$BUSINESS_AT"   "StepUp -> approval gate (vpn zone, no permit)"

echo
echo "== Demo complete. Check Temporal UI (http://localhost:8088) for workflow histories. =="