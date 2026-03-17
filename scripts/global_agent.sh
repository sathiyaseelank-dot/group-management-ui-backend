#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Zero-Trust Agent — Quick Connect
# Share this script with anyone on the same LAN.
# They only need to enter their Agent ID and Enrollment Token.
#
# Usage:
#   sudo ./global_agent.sh
# ─────────────────────────────────────────────────────────────────────────────

# ── SET YOUR CONTROLLER LAN IP HERE ──────────────────────────────────────────
CONTROLLER_IP="192.168.1.81"          # <-- change this to your machine's LAN IP
CONTROLLER_GRPC_PORT="8443"
CONTROLLER_HTTP_PORT="8081"
CONNECTOR_PORT="9443"
TRUST_DOMAIN="mycorp.internal"
# ─────────────────────────────────────────────────────────────────────────────

CONTROLLER_ADDR="${CONTROLLER_IP}:${CONTROLLER_GRPC_PORT}"
CONNECTOR_ADDR="${CONTROLLER_IP}:${CONNECTOR_PORT}"
CONTROLLER_HTTP="http://${CONTROLLER_IP}:${CONTROLLER_HTTP_PORT}"

CONFIG_DIR="/etc/agent"
CA_FILE="${CONFIG_DIR}/ca.crt"
CONFIG_FILE="${CONFIG_DIR}/agent.conf"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
ok()  { echo -e "  ${GREEN}[OK]${NC}  $*"; }
err() { echo -e "  ${RED}[ERR]${NC}  $*" >&2; }

# ── Root check ────────────────────────────────────────────────────────────────
if [[ "${EUID}" -ne 0 ]]; then
  err "Run as root: sudo ./global_agent.sh"
  exit 1
fi

# ── Check agent binary ────────────────────────────────────────────────────────
if ! command -v agent >/dev/null 2>&1 && [[ ! -f /usr/bin/agent ]]; then
  err "Agent binary not found. Copy the 'agent' binary to this machine first:"
  err "  scp <your-machine-ip>:/path/to/dist/agent /usr/bin/agent"
  err "  chmod +x /usr/bin/agent"
  exit 1
fi
AGENT_BIN=$(command -v agent 2>/dev/null || echo /usr/bin/agent)
ok "Agent binary: ${AGENT_BIN}"

# ── Prompt for credentials ────────────────────────────────────────────────────
echo ""
echo "════════════════════════════════════════════════════"
echo "  Zero-Trust Network — Agent Connect"
echo "  Controller: ${CONTROLLER_ADDR}"
echo "════════════════════════════════════════════════════"
echo ""

if [[ -z "${AGENT_ID:-}" ]]; then
  read -rp "  Agent ID (from dashboard): " AGENT_ID
fi
if [[ -z "${AGENT_TOKEN:-}" ]]; then
  read -rsp "  Enrollment Token:          " AGENT_TOKEN
  echo
fi
echo ""

# ── Fetch CA cert from controller ─────────────────────────────────────────────
mkdir -p "${CONFIG_DIR}"
chmod 0700 "${CONFIG_DIR}"

if command -v curl >/dev/null 2>&1; then
  curl -fsSL --connect-timeout 10 "${CONTROLLER_HTTP}/ca.crt" -o "${CA_FILE}" 2>/dev/null || true
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${CA_FILE}" "${CONTROLLER_HTTP}/ca.crt" 2>/dev/null || true
fi

if [[ ! -s "${CA_FILE}" ]]; then
  err "Could not fetch CA cert from ${CONTROLLER_HTTP}/ca.crt"
  err "Make sure the controller is running and port ${CONTROLLER_HTTP_PORT} is open."
  exit 1
fi
chmod 0644 "${CA_FILE}"
ok "CA cert fetched from controller"

# ── Write config ──────────────────────────────────────────────────────────────
cat > "${CONFIG_FILE}" <<EOF
CONTROLLER_ADDR=${CONTROLLER_ADDR}
CONNECTOR_ADDR=${CONNECTOR_ADDR}
AGENT_ID=${AGENT_ID}
ENROLLMENT_TOKEN=${AGENT_TOKEN}
TRUST_DOMAIN=${TRUST_DOMAIN}
EOF
chmod 0600 "${CONFIG_FILE}"
ok "Config written → ${CONFIG_FILE}"

unset AGENT_TOKEN

# ── Run agent ─────────────────────────────────────────────────────────────────
echo ""
echo "  Starting agent... (Ctrl+C to stop)"
echo ""

export $(grep -v '^#' "${CONFIG_FILE}" | xargs)
exec "${AGENT_BIN}" run
