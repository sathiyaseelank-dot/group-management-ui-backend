#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Zero-Trust Agent Installer
# Installs the locally-built agent as a systemd service.
# Must be run as root (sudo).
#
# Usage:
#   sudo ./install-agent.sh
#
# Pre-set env vars to skip prompts:
#   sudo AGENT_ID=agent_abc AGENT_TOKEN=<hex> CONNECTOR_ADDR=127.0.0.1:9443 \
#        ./install-agent.sh
#
# Legacy compatibility:
#   TUNNELER_ID / TUNNELER_TOKEN are accepted as fallbacks.
# ─────────────────────────────────────────────────────────────────────────────

REPO_DIR="${REPO_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
DIST_DIR="${DIST_DIR:-${REPO_DIR}/dist}"
SYSTEMD_SRC_DIR="${SYSTEMD_SRC_DIR:-${REPO_DIR}/systemd}"
CA_SRC_PATH="${CA_SRC_PATH:-${REPO_DIR}/services/controller/ca/ca.crt}"

CONTROLLER_ADDR="${CONTROLLER_ADDR:-localhost:8443}"
TRUST_DOMAIN="${TRUST_DOMAIN:-mycorp.internal}"
CONNECTOR_ADDR="${CONNECTOR_ADDR:-127.0.0.1:9443}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'
ok()   { echo -e "  ${GREEN}[OK]${NC}  $*"; }
warn() { echo -e "  ${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "  ${RED}[ERR]${NC} $*" >&2; }

prompt_if_empty() {
  local varname="$1"
  local prompt_text="$2"
  local secret="${3:-false}"
  if [[ -z "${!varname:-}" ]]; then
    if [[ "$secret" == "true" ]]; then
      read -rsp "  ${prompt_text}: " "${varname?}"
      echo
    else
      read -rp "  ${prompt_text}: " "${varname?}"
    fi
  fi
}

if [[ -z "${AGENT_ID:-}" && -n "${TUNNELER_ID:-}" ]]; then
  AGENT_ID="${TUNNELER_ID}"
fi

if [[ -z "${AGENT_TOKEN:-}" && -n "${TUNNELER_TOKEN:-}" ]]; then
  AGENT_TOKEN="${TUNNELER_TOKEN}"
fi

if [[ "${EUID}" -ne 0 ]]; then
  err "This script must be run as root (sudo)."
  exit 1
fi

echo ""
echo "════════════════════════════════════════════════════"
echo "  Zero-Trust Agent Installer"
echo "════════════════════════════════════════════════════"
echo ""
echo "  Repo:              ${REPO_DIR}"
echo "  Binary:            ${DIST_DIR}/agent"
echo "  CA cert:           ${CA_SRC_PATH}"
echo "  Controller addr:   ${CONTROLLER_ADDR}"
echo "  Trust domain:      ${TRUST_DOMAIN}"
echo "  Connector addr:    ${CONNECTOR_ADDR}"
echo ""

# ── Collect inputs ────────────────────────────────────────────────────────────
prompt_if_empty AGENT_ID    "Agent ID (e.g. agent-local-01)"
prompt_if_empty AGENT_TOKEN "Agent enrollment token (hex)" true
echo ""

# ── Pre-flight checks ─────────────────────────────────────────────────────────
echo "── Pre-flight checks ────────────────────────────────"

if [[ ! -f "${DIST_DIR}/agent" ]]; then
  err "Agent binary not found at ${DIST_DIR}/agent"
  echo ""
  echo "     Build it first:"
  echo "       make build-agent"
  exit 1
fi
ok "Agent binary found"

if [[ ! -f "${CA_SRC_PATH}" ]]; then
  err "Controller CA cert not found at ${CA_SRC_PATH}"
  exit 1
fi
ok "Controller CA cert found"

if [[ ! -f "${SYSTEMD_SRC_DIR}/agent.service" ]]; then
  err "Systemd unit file not found at ${SYSTEMD_SRC_DIR}/agent.service"
  exit 1
fi
ok "Systemd unit file found"
echo ""

# ── Install ───────────────────────────────────────────────────────────────────
echo "── Installing Agent ────────────────────────────────"

install -m 0755 "${DIST_DIR}/agent" /usr/bin/agent
ok "Binary installed → /usr/bin/agent"

# Clear stale enrollment state so the new ID enrolls cleanly.
rm -f /var/lib/agent/cert.pem /var/lib/agent/key.der /var/lib/agent/ca.pem
ok "Stale enrollment state cleared"

mkdir -p /etc/agent
chmod 0700 /etc/agent
install -m 0644 "${CA_SRC_PATH}" /etc/agent/ca.crt
ok "CA cert installed → /etc/agent/ca.crt"

cat >/etc/agent/agent.conf <<EOF
CONTROLLER_ADDR=${CONTROLLER_ADDR}
CONNECTOR_ADDR=${CONNECTOR_ADDR}
AGENT_ID=${AGENT_ID}
ENROLLMENT_TOKEN=${AGENT_TOKEN}
TRUST_DOMAIN=${TRUST_DOMAIN}
EOF
chmod 0600 /etc/agent/agent.conf
ok "Config written → /etc/agent/agent.conf"

install -m 0644 "${SYSTEMD_SRC_DIR}/agent.service" /etc/systemd/system/agent.service
ok "Systemd unit installed → /etc/systemd/system/agent.service"
echo ""

# ── Enable and start ──────────────────────────────────────────────────────────
echo "── Starting Agent ──────────────────────────────────"
systemctl daemon-reload
systemctl enable agent.service
systemctl restart agent.service
ok "agent.service enabled and started"

unset AGENT_TOKEN TUNNELER_TOKEN

echo ""
echo "════════════════════════════════════════════════════"
echo "  Agent installation complete!"
echo "════════════════════════════════════════════════════"
echo ""
echo "  Check status:  sudo systemctl status agent.service"
echo "  Follow logs:   sudo journalctl -u agent.service -f"
echo ""
