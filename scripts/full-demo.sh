#!/usr/bin/env bash
#
# full-demo.sh — the complete GenID demo, one terminal, end to end.
#
# Flow:
#   1. Health check (identity-service must be up)
#   2. IdP event ingestion  → risk moves in real time (3 failed logins)
#   3. Conditional access    → 6-case matrix (auto-approve / step-up / deny)
#   4. Open the UI and walk the pages
#
# Prereqs:
#   - Full stack:          cd infrastructure && docker compose up -d
#   - Identity service:    cd backend && go run cmd/identity-service/main.go
#   - Event processor:     cd backend && go run cmd/event-processor/main.go
#   - jq + curl installed
#
# Usage:
#   ./scripts/full-demo.sh
#
# Env overrides: GENID_BASE, GENID_API_KEY, GENID_IDENTITY, GENID_RESOURCE

set -uo pipefail

BASE="${GENID_BASE:-http://localhost:8080}"
UI_URL="${GENID_UI_URL:-http://localhost:3000}"

PASS=0
FAIL=0

ok()   { echo "  [OK]   $1"; PASS=$((PASS + 1)); }
warn() { echo "  [WARN] $1"; }
fail() { echo "  [FAIL] $1"; FAIL=$((FAIL + 1)); }

echo "============================================================"
echo "  GENID DEMO — identity-first, event-driven access control"
echo "============================================================"
echo "  API: ${BASE}   UI: ${UI_URL}"
echo

# ─── 1. Prerequisites ───────────────────────────────────────────
echo "[1/4] Prerequisite checks"
for bin in curl jq; do
  command -v "$bin" >/dev/null 2>&1 || { echo "  [FATAL] $bin is required"; exit 1; }
done
if ! curl -sf "${BASE}/health" >/dev/null; then
  echo "  [FATAL] ${BASE} is not reachable."
  echo "          Start the stack:  cd infrastructure && docker compose up -d"
  echo "          Start the API:    cd backend && go run cmd/identity-service/main.go"
  exit 1
fi
ok "API reachable at ${BASE}"
echo

# ─── 2. IdP event ingestion (risk moves in real time) ──────────
echo "[2/4] IdP event ingestion — 3 failed Entra sign-ins"
if ./scripts/simulate-idp-events.sh 2>&1; then
  ok "risk deltas applied (failed login burst)"
else
  fail "simulate-idp-events.sh (see output above)"
fi
echo

# ─── 3. Conditional access (6-case matrix) ─────────────────────
echo "[3/4] Conditional access — 6 policy cases"
if ./scripts/demo-conditional-access.sh 2>&1; then
  ok "conditional-access matrix ran"
else
  fail "demo-conditional-access.sh (see output above)"
fi
echo

# ─── 4. Hand-off to the UI ─────────────────────────────────────
echo "[4/4] Open the UI"
echo "  URL:   ${UI_URL}"
echo "  Login: admin@genid.io / dev-login"
echo
echo "  Walk-through:"
echo "   1. Policy Simulator → 'IT admin, office, managed, low risk' → Allow (auto JIT 2h)"
echo "   2. Risk Dashboard   → score moved after the failed-login burst"
echo "   3. Inbox            → approve/deny a step-up request"
echo "   4. Audit            → verify the hash chain (intact)"
echo "   5. NHI Registry     → issue/revoke a JIT passport"
echo

echo "============================================================"
echo "  RESULT: ${PASS} passed, ${FAIL} failed"
echo "  Check Temporal UI for workflow histories: http://localhost:8088"
echo "============================================================"
[ "$FAIL" -eq 0 ]
