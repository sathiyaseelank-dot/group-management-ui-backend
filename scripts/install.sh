#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Zero-Trust local installation script
# Installs locally-built connector + agent as systemd services.
# Must be run as root (sudo).
#
# Usage:
#   sudo ./install.sh
#
# All values can be pre-set as environment variables to skip interactive
# prompts. Example:
#   sudo CONNECTOR_ID=con_abc \
#        CONNECTOR_TOKEN=<hex> \
#        AGENT_ID=agent_abc \
#        AGENT_TOKEN=<hex> \
#        ./install.sh
# ─────────────────────────────────────────────────────────────────────────────

# ── Defaults (override via env) ───────────────────────────────────────────────
REPO_DIR="${REPO_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
DIST_DIR="${DIST_DIR:-${REPO_DIR}/dist}"
SYSTEMD_SRC_DIR="${SYSTEMD_SRC_DIR:-${REPO_DIR}/systemd}"
CA_SRC_PATH="${CA_SRC_PATH:-${REPO_DIR}/services/controller/ca/ca.crt}"

CONTROLLER_ADDR="${CONTROLLER_ADDR:-localhost:8443}"
TRUST_DOMAIN="${TRUST_DOMAIN:-mycorp.internal}"
CONNECTOR_LISTEN_ADDR="${CONNECTOR_LISTEN_ADDR:-127.0.0.1:9443}"
# Agent connects to connector — defaults to same as connector's listen addr
CONNECTOR_ADDR="${CONNECTOR_ADDR:-${CONNECTOR_LISTEN_ADDR}}"

# ── Helpers ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'
ok() { echo -e "  ${GREEN}[OK]${NC}  $*"; }
warn() { echo -e "  ${YELLOW}[WARN]${NC} $*"; }
err() { echo -e "  ${RED}[ERR]${NC} $*" >&2; }

detect_package_manager() {
  if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
  fi

  if command -v pacman >/dev/null 2>&1; then
    PKG_MANAGER="pacman"
  elif command -v apt-get >/dev/null 2>&1; then
    PKG_MANAGER="apt-get"
  elif command -v dnf >/dev/null 2>&1; then
    PKG_MANAGER="dnf"
  elif command -v yum >/dev/null 2>&1; then
    PKG_MANAGER="yum"
  elif command -v apk >/dev/null 2>&1; then
    PKG_MANAGER="apk"
  elif command -v zypper >/dev/null 2>&1; then
    PKG_MANAGER="zypper"
  else
    PKG_MANAGER=""
  fi
  OS_ID="${ID:-unknown}"
}

install_nftables() {
  case "${PKG_MANAGER}" in
    pacman)
      pacman -Sy --noconfirm nftables
      ;;
    apt-get)
      apt-get update
      apt-get install -y nftables
      ;;
    dnf)
      dnf install -y nftables
      ;;
    yum)
      yum install -y nftables
      ;;
    apk)
      apk add --no-cache nftables
      ;;
    zypper)
      zypper --non-interactive install nftables
      ;;
    *)
      return 1
      ;;
  esac
}

ensure_nft() {
  detect_package_manager
  if command -v nft >/dev/null 2>&1; then
    ok "nft already installed → $(command -v nft)"
    return 0
  fi

  if [[ "$(uname -s)" != "Linux" ]]; then
    err "nftables auto-install is supported only on Linux hosts"
    exit 1
  fi

  if [[ -z "${PKG_MANAGER}" ]]; then
    err "No supported package manager found on Linux host (os=${OS_ID}). Install nftables manually and re-run."
    exit 1
  fi

  warn "nft not found; installing nftables via ${PKG_MANAGER} (os=${OS_ID})"
  install_nftables || {
    err "Failed to install nftables via ${PKG_MANAGER}. Install it manually and re-run."
    exit 1
  }

  if ! command -v nft >/dev/null 2>&1; then
    err "nftables installation completed but `nft` is still missing. Agent firewall protection will not work."
    exit 1
  fi

  ok "nft installed → $(command -v nft)"
}

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

# ── Root check ────────────────────────────────────────────────────────────────
if [[ "${EUID}" -ne 0 ]]; then
  err "This script must be run as root (sudo)."
  exit 1
fi

echo ""
echo "════════════════════════════════════════════════════"
echo "  Zero-Trust Local Component Installer"
echo "════════════════════════════════════════════════════"
echo ""
echo "  Repo:               ${REPO_DIR}"
echo "  Binaries:           ${DIST_DIR}"
echo "  CA cert:            ${CA_SRC_PATH}"
echo "  Controller addr:    ${CONTROLLER_ADDR}"
echo "  Trust domain:       ${TRUST_DOMAIN}"
echo "  Connector listen:   ${CONNECTOR_LISTEN_ADDR}"
echo "  Agent→connector:    ${CONNECTOR_ADDR}"
echo ""

# ── Collect required inputs ───────────────────────────────────────────────────
echo "── Connector ────────────────────────────────────────"
prompt_if_empty CONNECTOR_ID "Connector ID (e.g. con_abc123)"
prompt_if_empty CONNECTOR_TOKEN "Connector enrollment token (hex)" true

echo ""
echo "── Agent ────────────────────────────────────────────"
prompt_if_empty AGENT_ID "Agent ID (e.g. agent-local-01)"
prompt_if_empty AGENT_TOKEN "Agent enrollment token (hex)" true
echo ""

# ── Pre-flight checks ─────────────────────────────────────────────────────────
echo "── Pre-flight checks ────────────────────────────────"

if [[ ! -f "${DIST_DIR}/connector" ]]; then
  err "Connector binary not found at ${DIST_DIR}/connector"
  echo ""
  echo "     Build it first:"
  echo "       cd ${REPO_DIR}/services/connector && cargo build --release"
  echo "       cp target/release/connector ${DIST_DIR}/connector"
  exit 1
fi
ok "Connector binary found"

if [[ ! -f "${DIST_DIR}/agent" ]]; then
  err "Agent binary not found at ${DIST_DIR}/agent"
  echo ""
  echo "     Build it first:"
  echo "       cd ${REPO_DIR}/services/agent && cargo build --release"
  echo "       cp target/release/agent ${DIST_DIR}/agent"
  exit 1
fi
ok "Agent binary found"

if [[ ! -f "${CA_SRC_PATH}" ]]; then
  err "Controller CA cert not found at ${CA_SRC_PATH}"
  exit 1
fi
ok "Controller CA cert found"

if [[ ! -f "${SYSTEMD_SRC_DIR}/connector.service" || ! -f "${SYSTEMD_SRC_DIR}/agent.service" ]]; then
  err "Systemd unit files not found in ${SYSTEMD_SRC_DIR}/"
  exit 1
fi
ok "Systemd unit files found"

ensure_nft
echo ""

# ── Install Connector ─────────────────────────────────────────────────────────
echo "── Installing Connector ─────────────────────────────"

install -m 0755 "${DIST_DIR}/connector" /usr/bin/connector
ok "Binary installed → /usr/bin/connector"

rm -f /var/lib/connector/cert.pem /var/lib/connector/key.der /var/lib/connector/ca.pem
ok "Stale connector enrollment state cleared"

mkdir -p /etc/connector
chmod 0700 /etc/connector
install -m 0644 "${CA_SRC_PATH}" /etc/connector/ca.crt
ok "CA cert installed → /etc/connector/ca.crt"

# The service file uses LoadCredential=CONTROLLER_CA:/etc/connector/ca.crt
# which makes the CA available via $CREDENTIALS_DIRECTORY/CONTROLLER_CA.
# No CONTROLLER_CA_PATH needed here.
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

# ── Install Agent ─────────────────────────────────────────────────────────────
echo "── Installing Agent ─────────────────────────────────"

install -m 0755 "${DIST_DIR}/agent" /usr/bin/agent
ok "Binary installed → /usr/bin/agent"

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

# ── Enable and start services ─────────────────────────────────────────────────
echo "── Starting Services ────────────────────────────────"
systemctl daemon-reload

systemctl enable connector.service
systemctl restart connector.service
ok "connector.service enabled and started"

# Wait for connector to be up before agent tries to connect
sleep 2

systemctl enable agent.service
systemctl restart agent.service
ok "agent.service enabled and started"

# Clear sensitive vars
unset CONNECTOR_TOKEN AGENT_TOKEN

echo ""
echo "════════════════════════════════════════════════════"
echo "  Installation complete!"
echo "════════════════════════════════════════════════════"
echo ""
echo "  Check status:"
echo "    sudo systemctl status connector.service"
echo "    sudo systemctl status agent.service"
echo ""
echo "  Follow logs:"
echo "    sudo journalctl -u connector.service -f"
echo "    sudo journalctl -u agent.service -f"
echo ""
