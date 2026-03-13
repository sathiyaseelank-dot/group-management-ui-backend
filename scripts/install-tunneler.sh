#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Zero-Trust Tunneler Installer
# Installs the locally-built tunneler as a systemd service.
# Must be run as root (sudo).
#
# Usage:
#   sudo ./install-tunneler.sh
#
# Pre-set env vars to skip prompts:
#   sudo TUNNELER_ID=tun_abc TUNNELER_TOKEN=<hex> CONNECTOR_ADDR=127.0.0.1:9443 \
#        ./install-tunneler.sh
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

if [[ "${EUID}" -ne 0 ]]; then
  err "This script must be run as root (sudo)."
  exit 1
fi

echo ""
echo "════════════════════════════════════════════════════"
echo "  Zero-Trust Tunneler Installer"
echo "════════════════════════════════════════════════════"
echo ""
echo "  Repo:              ${REPO_DIR}"
echo "  Binary:            ${DIST_DIR}/tunneler"
echo "  CA cert:           ${CA_SRC_PATH}"
echo "  Controller addr:   ${CONTROLLER_ADDR}"
echo "  Trust domain:      ${TRUST_DOMAIN}"
echo "  Connector addr:    ${CONNECTOR_ADDR}"
echo ""

# ── Collect inputs ────────────────────────────────────────────────────────────
prompt_if_empty TUNNELER_ID    "Tunneler ID (e.g. tun_abc123)"
prompt_if_empty TUNNELER_TOKEN "Tunneler enrollment token (hex)" true
echo ""

# ── Pre-flight checks ─────────────────────────────────────────────────────────
echo "── Pre-flight checks ────────────────────────────────"

if [[ ! -f "${DIST_DIR}/tunneler" ]]; then
  err "Tunneler binary not found at ${DIST_DIR}/tunneler"
  echo ""
  echo "     Build it first:"
  echo "       make build-tunneler"
  exit 1
fi
ok "Tunneler binary found"

if [[ ! -f "${CA_SRC_PATH}" ]]; then
  err "Controller CA cert not found at ${CA_SRC_PATH}"
  exit 1
fi
ok "Controller CA cert found"

if [[ ! -f "${SYSTEMD_SRC_DIR}/tunneler.service" ]]; then
  err "Systemd unit file not found at ${SYSTEMD_SRC_DIR}/tunneler.service"
  exit 1
fi
ok "Systemd unit file found"
echo ""

# ── Install ───────────────────────────────────────────────────────────────────
echo "── Installing Tunneler ──────────────────────────────"

install -m 0755 "${DIST_DIR}/tunneler" /usr/bin/tunneler
ok "Binary installed → /usr/bin/tunneler"

# Clear stale enrollment state so the new ID enrolls cleanly.
rm -f /var/lib/tunneler/cert.pem /var/lib/tunneler/key.der /var/lib/tunneler/ca.pem
ok "Stale enrollment state cleared"

mkdir -p /etc/tunneler
chmod 0700 /etc/tunneler
install -m 0644 "${CA_SRC_PATH}" /etc/tunneler/ca.crt
ok "CA cert installed → /etc/tunneler/ca.crt"

cat >/etc/tunneler/tunneler.conf <<EOF
CONTROLLER_ADDR=${CONTROLLER_ADDR}
CONNECTOR_ADDR=${CONNECTOR_ADDR}
TUNNELER_ID=${TUNNELER_ID}
ENROLLMENT_TOKEN=${TUNNELER_TOKEN}
TRUST_DOMAIN=${TRUST_DOMAIN}
EOF
chmod 0600 /etc/tunneler/tunneler.conf
ok "Config written → /etc/tunneler/tunneler.conf"

install -m 0644 "${SYSTEMD_SRC_DIR}/tunneler.service" /etc/systemd/system/tunneler.service
ok "Systemd unit installed → /etc/systemd/system/tunneler.service"
echo ""

# ── Enable and start ──────────────────────────────────────────────────────────
echo "── Starting Tunneler ────────────────────────────────"
systemctl daemon-reload
systemctl enable tunneler.service
systemctl restart tunneler.service
ok "tunneler.service enabled and started"

unset TUNNELER_TOKEN

echo ""
echo "════════════════════════════════════════════════════"
echo "  Tunneler installation complete!"
echo "════════════════════════════════════════════════════"
echo ""
echo "  Check status:  sudo systemctl status tunneler.service"
echo "  Follow logs:   sudo journalctl -u tunneler.service -f"
echo ""
