#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Zero-Trust Client Installer
# Installs the locally-built ztna-client as a systemd service.
# Must be run as root (sudo).
#
# Usage:
#   sudo ./client-setup.sh
#
# The installer expects a pre-built binary.  Build it first:
#   make build-ztna-client
#
# Upgrade-safe: existing /etc/ztna-client/client.conf is never overwritten.
# ─────────────────────────────────────────────────────────────────────────────

REPO_DIR="${REPO_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
BINARY_SRC="${BINARY_SRC:-${REPO_DIR}/services/ztna-client/target/release/ztna-client}"
SYSTEMD_SRC="${SYSTEMD_SRC:-${REPO_DIR}/systemd/ztna-client.service}"

INSTALL_BIN="/usr/bin/ztna-client"
CONFIG_DIR="/etc/ztna-client"
CONFIG_FILE="${CONFIG_DIR}/client.conf"
STATE_DIR="/var/lib/ztna-client"
SYSTEMD_DST="/etc/systemd/system/ztna-client.service"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'
ok()   { echo -e "  ${GREEN}✓${NC}  $*"; }
warn() { echo -e "  ${YELLOW}!${NC}  $*"; }
err()  { echo -e "  ${RED}✗${NC}  $*" >&2; }

# ── Root check ───────────────────────────────────────────────────────────────
if [[ "${EUID}" -ne 0 ]]; then
  err "This script must be run as root (sudo)."
  exit 1
fi

echo ""
echo "════════════════════════════════════════════════════"
echo "  Zero-Trust Client Installer"
echo "════════════════════════════════════════════════════"
echo ""

# ── Pre-flight checks ───────────────────────────────────────────────────────
echo "Pre-flight checks"
echo "─────────────────────────────────────────────────────"

if [[ ! -f "${BINARY_SRC}" ]]; then
  err "Binary not found at ${BINARY_SRC}"
  echo ""
  echo "     Build it first:"
  echo "       make build-ztna-client"
  exit 1
fi
ok "Binary found"

if [[ ! -f "${SYSTEMD_SRC}" ]]; then
  err "Systemd unit not found at ${SYSTEMD_SRC}"
  exit 1
fi
ok "Systemd unit found"
echo ""

# ── Stop existing service if running ────────────────────────────────────────
if systemctl is-active --quiet ztna-client.service 2>/dev/null; then
  echo "Stopping existing service"
  echo "─────────────────────────────────────────────────────"
  systemctl stop ztna-client.service
  ok "Service stopped"
  echo ""
fi

# ── Install binary ──────────────────────────────────────────────────────────
echo "Installing ztna-client"
echo "─────────────────────────────────────────────────────"

install -m 0755 "${BINARY_SRC}" "${INSTALL_BIN}"
ok "Binary installed → ${INSTALL_BIN}"

# ── Config directory ────────────────────────────────────────────────────────
mkdir -p "${CONFIG_DIR}"
chmod 0755 "${CONFIG_DIR}"
ok "Config directory created → ${CONFIG_DIR}"

# Write default config only if it does not already exist (upgrade-safe).
if [[ -f "${CONFIG_FILE}" ]]; then
  warn "Existing config preserved → ${CONFIG_FILE}"
else
  cat > "${CONFIG_FILE}" <<'CONF'
# ZTNA Client Configuration
#
# Required settings — uncomment and configure:
#
# controller_url = "https://controller.example.com:8081"
# tenant = "my-workspace"
#
# Optional settings:
#
# ca_cert_path = "/etc/ztna-client/ca.crt"
# mode = "tun"                    # or "socks5"
# callback_bind_addr = "127.0.0.1"
# port = 19515
#
# See 'ztna-client doctor' for diagnostics after configuration.
CONF
  chmod 0644 "${CONFIG_FILE}"
  ok "Default config created → ${CONFIG_FILE}"
fi

# ── State directory ─────────────────────────────────────────────────────────
mkdir -p "${STATE_DIR}"
# 0711: owner rwx, others --x (traverse-only).
# The service writes a token to .service.token (mode 0644) inside this dir.
# Non-root CLI processes need execute (traverse) permission to read it by name,
# but must not be able to list directory contents.
chmod 0711 "${STATE_DIR}"
ok "State directory created → ${STATE_DIR}"

# ── Systemd unit ────────────────────────────────────────────────────────────
install -m 0644 "${SYSTEMD_SRC}" "${SYSTEMD_DST}"
ok "Systemd unit installed → ${SYSTEMD_DST}"
echo ""

# ── Enable and start ────────────────────────────────────────────────────────
echo "Starting service"
echo "─────────────────────────────────────────────────────"
systemctl daemon-reload
systemctl enable ztna-client.service
systemctl start ztna-client.service
ok "Service enabled and started"

echo ""
echo "════════════════════════════════════════════════════"
echo "  Installation Complete!"
echo "════════════════════════════════════════════════════"
echo ""
echo "Next Steps:"
echo ""
echo "  1. Configure the client:"
echo "     sudo nano ${CONFIG_FILE}"
echo ""
echo "     Set at minimum:"
echo "       controller_url = \"https://your-controller:8081\""
echo "       tenant = \"your-workspace\""
echo ""
echo "  2. Restart the service:"
echo "     sudo systemctl restart ztna-client"
echo ""
echo "  3. Verify the setup:"
echo "     ztna-client doctor"
echo ""
echo "  4. Log in (no sudo needed):"
echo "     ztna-client login"
echo ""
echo "  5. Check your connection:"
echo "     ztna-client status"
echo "     ztna-client resources"
echo ""
echo "Help:"
echo "  ztna-client --help     Show all commands"
echo "  ztna-client login --help"
echo ""
echo "Service logs:"
echo "  sudo journalctl -u ztna-client -f"
echo ""
