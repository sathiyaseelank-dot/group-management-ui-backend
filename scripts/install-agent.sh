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
#   sudo AGENT_ID=agt_abc ENROLLMENT_TOKEN=<hex> CONNECTOR_ADDR=127.0.0.1:9443 \
#        ./install-agent.sh
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

warn_if_kernel_reboot_needed() {
  if [[ "$(uname -s)" != "Linux" || ! -d /usr/lib/modules ]]; then
    return 0
  fi

  local running_kernel latest_installed
  running_kernel="$(uname -r)"
  latest_installed="$(find /usr/lib/modules -mindepth 1 -maxdepth 1 -type d -printf '%f\n' 2>/dev/null | sort -V | tail -n 1)"

  if [[ -z "${latest_installed}" || "${latest_installed}" == "${running_kernel}" ]]; then
    return 0
  fi

  warn "Running kernel (${running_kernel}) does not match newest installed kernel (${latest_installed})"
  warn "Reboot this device before relying on agent nftables/firewall enforcement"
  warn "If nft commands fail or firewall rules do not apply, reboot first"
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
prompt_if_empty AGENT_ID         "Agent ID (e.g. agt_abc123)"
prompt_if_empty ENROLLMENT_TOKEN "Agent enrollment token (hex)" true
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

ensure_nft
warn_if_kernel_reboot_needed
echo ""

# ── Create system user ─────────────────────────────────────────────────────────
echo "── System user ──────────────────────────────────────"

if ! id -u zero-trust-agent &>/dev/null; then
    useradd --system --no-create-home --shell /usr/sbin/nologin zero-trust-agent
    ok "Created system user zero-trust-agent"
else
    ok "System user zero-trust-agent already exists"
fi
echo ""

# ── Install ───────────────────────────────────────────────────────────────────
echo "── Installing Agent ─────────────────────────────────"

install -m 0755 "${DIST_DIR}/agent" /usr/bin/agent
ok "Binary installed → /usr/bin/agent"

# Clear stale enrollment state so the new ID enrolls cleanly.
rm -f /var/lib/agent/cert.pem /var/lib/agent/key.der /var/lib/agent/ca.pem \
      /var/lib/agent/firewall_state.json /var/lib/agent/discovery_state.json
ok "Stale enrollment state cleared"

# /var/lib/agent — owned by agent user (state writes: certs, firewall, discovery)
mkdir -p /var/lib/agent
chown zero-trust-agent:zero-trust-agent /var/lib/agent
chmod 0700 /var/lib/agent
ok "State directory ready → /var/lib/agent (owner: zero-trust-agent)"

# /etc/agent — config dir, owned by root, readable by agent user
mkdir -p /etc/agent
chown root:zero-trust-agent /etc/agent
chmod 0750 /etc/agent

install -m 0644 "${CA_SRC_PATH}" /etc/agent/ca.crt
chown root:zero-trust-agent /etc/agent/ca.crt
ok "CA cert installed → /etc/agent/ca.crt"

cat >/etc/agent/agent.conf <<EOF
CONTROLLER_ADDR=${CONTROLLER_ADDR}
CONNECTOR_ADDR=${CONNECTOR_ADDR}
AGENT_ID=${AGENT_ID}
ENROLLMENT_TOKEN=${ENROLLMENT_TOKEN}
TRUST_DOMAIN=${TRUST_DOMAIN}
# Discovery: process name detection (disabled by default for least-privilege)
# Set to true to enrich discovered services with process names via /proc/*/fd
# NOTE: Requires CAP_SYS_PTRACE added to agent.service AmbientCapabilities
# and CapabilityBoundingSet to read other processes' fd links.
# DISCOVERY_PROCESS_NAMES=false
# Discovery: include ephemeral ports >32767 (disabled by default)
# DISCOVERY_INCLUDE_EPHEMERAL=false
EOF
chown root:zero-trust-agent /etc/agent/agent.conf
chmod 0640 /etc/agent/agent.conf
ok "Config written → /etc/agent/agent.conf"

install -m 0644 "${SYSTEMD_SRC_DIR}/agent.service" /etc/systemd/system/agent.service
ok "Systemd unit installed → /etc/systemd/system/agent.service"
echo ""

# ── Enable and start ──────────────────────────────────────────────────────────
echo "── Starting Agent ───────────────────────────────────"
systemctl daemon-reload
systemctl enable agent.service
systemctl restart agent.service
ok "agent.service enabled and started"

unset ENROLLMENT_TOKEN
warn_if_kernel_reboot_needed

echo ""
echo "════════════════════════════════════════════════════"
echo "  Agent installation complete!"
echo "════════════════════════════════════════════════════"
echo ""
echo "  Check status:  sudo systemctl status agent.service"
echo "  Follow logs:   sudo journalctl -u agent.service -f"
echo ""
