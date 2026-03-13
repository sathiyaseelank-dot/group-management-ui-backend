#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Zero-Trust Connector Installer
# Installs the locally-built connector as a systemd service.
# Must be run as root (sudo).
#
# Usage:
#   sudo ./install-connector.sh
#
# Pre-set env vars to skip prompts:
#   sudo CONNECTOR_ID=con_abc CONNECTOR_TOKEN=<hex> ./install-connector.sh
# ─────────────────────────────────────────────────────────────────────────────

REPO_DIR="${REPO_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
DIST_DIR="${DIST_DIR:-${REPO_DIR}/dist}"
SYSTEMD_SRC_DIR="${SYSTEMD_SRC_DIR:-${REPO_DIR}/systemd}"
CA_SRC_PATH="${CA_SRC_PATH:-${REPO_DIR}/services/controller/ca/ca.crt}"

CONTROLLER_ADDR="${CONTROLLER_ADDR:-localhost:8443}"
TRUST_DOMAIN="${TRUST_DOMAIN:-mycorp.internal}"
CONNECTOR_LISTEN_ADDR="${CONNECTOR_LISTEN_ADDR:-127.0.0.1:9443}"

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

if [[ "${EUID}" -ne 0 ]]; then
  err "This script must be run as root (sudo)."
  exit 1
fi

echo ""
echo "════════════════════════════════════════════════════"
echo "  Zero-Trust Connector Installer"
echo "════════════════════════════════════════════════════"
echo ""
echo "  Repo:             ${REPO_DIR}"
echo "  Binary:           ${DIST_DIR}/connector"
echo "  CA cert:          ${CA_SRC_PATH}"
echo "  Controller addr:  ${CONTROLLER_ADDR}"
echo "  Trust domain:     ${TRUST_DOMAIN}"
echo "  Listen addr:      ${CONNECTOR_LISTEN_ADDR}"
echo ""

# ── Collect inputs ────────────────────────────────────────────────────────────
prompt_if_empty CONNECTOR_ID    "Connector ID (e.g. con_abc123)"
prompt_if_empty CONNECTOR_TOKEN "Connector enrollment token (hex)" true
echo ""

# ── Pre-flight checks ─────────────────────────────────────────────────────────
echo "── Pre-flight checks ────────────────────────────────"

if [[ ! -f "${DIST_DIR}/connector" ]]; then
  err "Connector binary not found at ${DIST_DIR}/connector"
  echo ""
  echo "     Build it first:"
  echo "       make build-connector"
  exit 1
fi
ok "Connector binary found"

if [[ ! -f "${CA_SRC_PATH}" ]]; then
  err "Controller CA cert not found at ${CA_SRC_PATH}"
  exit 1
fi
ok "Controller CA cert found"

if [[ ! -f "${SYSTEMD_SRC_DIR}/connector.service" ]]; then
  err "Systemd unit file not found at ${SYSTEMD_SRC_DIR}/connector.service"
  exit 1
fi
ok "Systemd unit file found"
echo ""

# ── Install ───────────────────────────────────────────────────────────────────
echo "── Installing Connector ─────────────────────────────"

install -m 0755 "${DIST_DIR}/connector" /usr/bin/connector
ok "Binary installed → /usr/bin/connector"

# Clear stale enrollment state so the new ID enrolls cleanly.
rm -f /var/lib/connector/cert.pem /var/lib/connector/key.der /var/lib/connector/ca.pem
ok "Stale enrollment state cleared"

mkdir -p /etc/connector
chmod 0700 /etc/connector
install -m 0644 "${CA_SRC_PATH}" /etc/connector/ca.crt
ok "CA cert installed → /etc/connector/ca.crt"

cat >/etc/connector/connector.conf <<EOF
CONTROLLER_ADDR=${CONTROLLER_ADDR}
CONNECTOR_ID=${CONNECTOR_ID}
ENROLLMENT_TOKEN=${CONNECTOR_TOKEN}
TRUST_DOMAIN=${TRUST_DOMAIN}
CONNECTOR_LISTEN_ADDR=${CONNECTOR_LISTEN_ADDR}
EOF
chmod 0600 /etc/connector/connector.conf
ok "Config written → /etc/connector/connector.conf"

install -m 0644 "${SYSTEMD_SRC_DIR}/connector.service" /etc/systemd/system/connector.service
ok "Systemd unit installed → /etc/systemd/system/connector.service"
echo ""

# ── Enable and start ──────────────────────────────────────────────────────────
echo "── Starting Connector ───────────────────────────────"
systemctl daemon-reload
systemctl enable connector.service
systemctl restart connector.service
ok "connector.service enabled and started"

unset CONNECTOR_TOKEN

echo ""
echo "════════════════════════════════════════════════════"
echo "  Connector installation complete!"
echo "════════════════════════════════════════════════════"
echo ""
echo "  Check status:  sudo systemctl status connector.service"
echo "  Follow logs:   sudo journalctl -u connector.service -f"
echo ""
