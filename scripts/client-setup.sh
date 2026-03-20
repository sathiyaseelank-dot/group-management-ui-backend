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
ok()   { echo -e "  ${GREEN}[OK]${NC}  $*"; }
warn() { echo -e "  ${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "  ${RED}[ERR]${NC} $*" >&2; }

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
echo "── Pre-flight checks ────────────────────────────────"

if [[ ! -f "${BINARY_SRC}" ]]; then
  err "Binary not found at ${BINARY_SRC}"
  echo ""
  echo "     Build it first:"
  echo "       make build-ztna-client"
  exit 1
fi
ok "Binary found: ${BINARY_SRC}"

if [[ ! -f "${SYSTEMD_SRC}" ]]; then
  err "Systemd unit not found at ${SYSTEMD_SRC}"
  exit 1
fi
ok "Systemd unit found: ${SYSTEMD_SRC}"
echo ""

# ── Stop existing service if running ────────────────────────────────────────
if systemctl is-active --quiet ztna-client.service 2>/dev/null; then
  echo "── Stopping existing service ────────────────────────"
  systemctl stop ztna-client.service
  ok "Stopped ztna-client.service"
  echo ""
fi

# ── Install binary ──────────────────────────────────────────────────────────
echo "── Installing ztna-client ─────────────────────────────"

install -m 0755 "${BINARY_SRC}" "${INSTALL_BIN}"
ok "Binary installed → ${INSTALL_BIN}"

# ── Config directory ────────────────────────────────────────────────────────
mkdir -p "${CONFIG_DIR}"
chmod 0755 "${CONFIG_DIR}"
ok "Config directory → ${CONFIG_DIR}"

# Write default config only if it does not already exist (upgrade-safe).
if [[ -f "${CONFIG_FILE}" ]]; then
  warn "Existing config preserved: ${CONFIG_FILE}"
else
  cat > "${CONFIG_FILE}" <<'CONF'
# ZTNA Client Configuration
# Docs: see CLAUDE.md in the project repository
#
# Required — set to your controller's URL:
# controller_url = "https://controller.example.com:8081"
#
# Required — workspace slug for this device:
# tenant = "my-workspace"
#
# Optional — path to CA certificate for controller/connector TLS:
# ca_cert_path = "/etc/ztna-client/ca.crt"
#
# Optional — transport mode: "tun" (default, requires root) or "socks5":
# mode = "tun"
#
# OAuth callback listener (receives the browser redirect after login).
# The callback is served on port (default: 19515).  The management API
# runs on port+1 (default: 19516) and is always bound to localhost only.
#
# By default the callback listener binds to 0.0.0.0 so it is reachable
# from a browser on the same LAN (useful when the client is headless).
# Change this to "127.0.0.1" for local-only callback:
# callback_bind_addr = "127.0.0.1"
#
# If callback_bind_addr is not 127.0.0.1, the service will log a warning
# that the callback endpoint is LAN-exposed.  This is intentional for
# testing; management endpoints remain localhost-only regardless.
#
# The OAuth redirect URI registered at the controller must match:
#   http://<callback_host>:<port>/callback  (default port: 19515)
#
# Advanced / transitional (uncomment if needed):
# controller_grpc_addr = "controller.example.com:8443"
# connector_tunnel_addr = "connector.example.com:9444"
# socks5_addr = "127.0.0.1:1080"
# port = 19515
CONF
  chmod 0644 "${CONFIG_FILE}"
  ok "Default config written → ${CONFIG_FILE}"
fi

# ── State directory ─────────────────────────────────────────────────────────
mkdir -p "${STATE_DIR}"
chmod 0700 "${STATE_DIR}"
ok "State directory → ${STATE_DIR}"

# ── Systemd unit ────────────────────────────────────────────────────────────
install -m 0644 "${SYSTEMD_SRC}" "${SYSTEMD_DST}"
ok "Systemd unit → ${SYSTEMD_DST}"
echo ""

# ── Enable and start ────────────────────────────────────────────────────────
echo "── Starting service ─────────────────────────────────"
systemctl daemon-reload
systemctl enable ztna-client.service
systemctl start ztna-client.service
ok "ztna-client.service enabled and started"

echo ""
echo "════════════════════════════════════════════════════"
echo "  Installation complete!"
echo "════════════════════════════════════════════════════"
echo ""
echo "  Next steps:"
echo ""
echo "    1. Edit the config file:"
echo "       sudo nano ${CONFIG_FILE}"
echo ""
echo "    2. Restart after editing config:"
echo "       sudo systemctl restart ztna-client"
echo ""
echo "    3. Login (no sudo needed):"
echo "       ztna-client login"
echo ""
echo "    4. Check status (no sudo needed):"
echo "       ztna-client status"
echo "       ztna-client resources"
echo ""
echo "    5. Service logs:"
echo "       sudo journalctl -u ztna-client -f"
echo ""
echo "  Listener ports (defaults):"
echo "    :19515  OAuth callback  — may bind to 0.0.0.0 for LAN testing"
echo "    :19516  Management API  — always bound to 127.0.0.1 (localhost only)"
echo ""
echo "  The OAuth redirect URI registered at your controller must use port 19515:"
echo "    http://<host>:19515/callback"
echo ""
