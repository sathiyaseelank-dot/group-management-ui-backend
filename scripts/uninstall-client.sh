#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Zero-Trust Client — Uninstaller
#
# Removes the ztna-client binary, systemd service, configuration, and state.
# Must be run as root (sudo).
#
# Usage:
#   sudo bash uninstall-client.sh
#   sudo bash uninstall-client.sh --keep-config   # preserve /etc/ztna-client
# ─────────────────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'
ok()   { echo -e "  ${GREEN}[OK]${NC}  $*"; }
warn() { echo -e "  ${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "  ${RED}[ERR]${NC} $*" >&2; }

KEEP_CONFIG=false
for arg in "$@"; do
  case "$arg" in
    --keep-config) KEEP_CONFIG=true ;;
    --help|-h)
      echo "Usage: sudo bash $0 [--keep-config]"
      echo ""
      echo "  --keep-config  Preserve /etc/ztna-client/ (config and CA cert)"
      exit 0
      ;;
  esac
done

# ── Root check ───────────────────────────────────────────────────────────────
if [[ "${EUID}" -ne 0 ]]; then
  err "This script must be run as root (sudo)."
  exit 1
fi

echo ""
echo "════════════════════════════════════════════════════"
echo "  Zero-Trust Client — Uninstaller"
echo "════════════════════════════════════════════════════"
echo ""

# ── Disconnect active tunnels ────────────────────────────────────────────────
if command -v ztna-client &>/dev/null; then
  echo "── Disconnecting active sessions ────────────────────"
  # Best-effort: disconnect all workspaces before stopping
  ztna-client disconnect --all 2>/dev/null || true
  ok "Disconnect attempted (best-effort)"
  echo ""
fi

# ── Stop and disable systemd service ────────────────────────────────────────
echo "── Stopping service ─────────────────────────────────"
if systemctl is-active --quiet ztna-client.service 2>/dev/null; then
  systemctl stop ztna-client.service
  ok "Service stopped"
else
  ok "Service not running"
fi

if systemctl is-enabled --quiet ztna-client.service 2>/dev/null; then
  systemctl disable ztna-client.service
  ok "Service disabled"
else
  ok "Service not enabled"
fi
echo ""

# ── Remove systemd unit ────────────────────────────────────────────────────
echo "── Removing components ──────────────────────────────"
if [[ -f /etc/systemd/system/ztna-client.service ]]; then
  rm -f /etc/systemd/system/ztna-client.service
  systemctl daemon-reload
  ok "Systemd unit removed"
else
  ok "Systemd unit not found (already removed)"
fi

# ── Remove binary ──────────────────────────────────────────────────────────
if [[ -f /usr/bin/ztna-client ]]; then
  rm -f /usr/bin/ztna-client
  ok "Binary removed (/usr/bin/ztna-client)"
else
  ok "Binary not found (already removed)"
fi

# ── Remove TUN device and routes (cleanup) ─────────────────────────────────
if ip link show ztna0 &>/dev/null; then
  ip link delete ztna0 2>/dev/null || true
  ok "TUN device ztna0 removed"
fi

# Clean up any leftover ip rules added by the client
ip rule del lookup 100 2>/dev/null || true
ip route flush table 100 2>/dev/null || true

# ── Remove state directory ─────────────────────────────────────────────────
if [[ -d /var/lib/ztna-client ]]; then
  rm -rf /var/lib/ztna-client
  ok "State directory removed (/var/lib/ztna-client)"
else
  ok "State directory not found"
fi

# ── Remove runtime directory ──────────────────────────────────────────────
if [[ -d /run/ztna-client ]]; then
  rm -rf /run/ztna-client
  ok "Runtime directory removed (/run/ztna-client)"
else
  ok "Runtime directory not found"
fi

# ── Remove config directory (unless --keep-config) ────────────────────────
if [[ "${KEEP_CONFIG}" == "true" ]]; then
  warn "Config preserved (--keep-config): /etc/ztna-client/"
else
  if [[ -d /etc/ztna-client ]]; then
    rm -rf /etc/ztna-client
    ok "Config directory removed (/etc/ztna-client)"
  else
    ok "Config directory not found"
  fi
fi

# ── Remove user-level state (optional) ───────────────────────────────────
# Dev-mode state lives in ~/.local/share/ztna-client for non-root users.
# We don't remove it here since it belongs to the user, not the system install.

echo ""
echo "════════════════════════════════════════════════════"
echo "  Uninstall complete!"
echo "════════════════════════════════════════════════════"
echo ""
if [[ "${KEEP_CONFIG}" == "true" ]]; then
  echo "  Config preserved at /etc/ztna-client/"
  echo "  To reinstall later: sudo bash client-setup.sh"
else
  echo "  All ztna-client files have been removed."
fi
echo ""
echo "  Note: User-level state in ~/.local/share/ztna-client/"
echo "  (if any) was not removed. Delete manually if needed."
echo ""
