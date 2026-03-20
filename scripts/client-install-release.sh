#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Zero-Trust Client — Release Installer
#
# Downloads ztna-client from GitHub Releases and installs it as a systemd
# service.  No Cargo or local repo checkout required.
#
# Usage:
#   # Install latest release (as root):
#   sudo bash client-install-release.sh
#
#   # Install a specific version:
#   sudo VERSION=v1.2.3 bash client-install-release.sh
#     — or —
#   sudo bash client-install-release.sh --version v1.2.3
#
# Upgrade-safe: existing /etc/ztna-client/client.conf is never overwritten.
#
# Supported architectures:
#   x86_64  (amd64)
#   aarch64 (arm64)
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

# ── Configuration ─────────────────────────────────────────────────────────────
GITHUB_REPO="vairabarath/zero-trust"
GITHUB_RELEASES_URL="https://github.com/${GITHUB_REPO}/releases"
GITHUB_API_URL="https://api.github.com/repos/${GITHUB_REPO}/releases"

INSTALL_BIN="/usr/bin/ztna-client"
CONFIG_DIR="/etc/ztna-client"
CONFIG_FILE="${CONFIG_DIR}/client.conf"
STATE_DIR="/var/lib/ztna-client"
SYSTEMD_DST="/etc/systemd/system/ztna-client.service"

# ── Colors ─────────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'
ok()   { echo -e "  ${GREEN}[OK]${NC}  $*"; }
warn() { echo -e "  ${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "  ${RED}[ERR]${NC} $*" >&2; }
info() { echo -e "  ${CYAN}[INFO]${NC} $*"; }

# ── Argument parsing ──────────────────────────────────────────────────────────
VERSION="${VERSION:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version|-v)
      VERSION="${2:-}"
      shift 2
      ;;
    --version=*)
      VERSION="${1#*=}"
      shift
      ;;
    --help|-h)
      sed -n '2,20p' "$0" | sed 's/^# \?//'
      exit 0
      ;;
    *)
      err "Unknown argument: $1"
      err "Usage: sudo bash $0 [--version v1.2.3]"
      exit 1
      ;;
  esac
done

# ── Root check ────────────────────────────────────────────────────────────────
if [[ "${EUID}" -ne 0 ]]; then
  err "This script must be run as root (sudo)."
  echo ""
  echo "     sudo bash $0 [--version v1.2.3]"
  exit 1
fi

# ── Architecture detection ────────────────────────────────────────────────────
detect_arch() {
  local machine
  machine="$(uname -m)"
  case "${machine}" in
    x86_64)   echo "amd64" ;;
    aarch64)  echo "arm64" ;;
    arm64)    echo "arm64" ;;
    *)
      err "Unsupported architecture: ${machine}"
      err "Supported: x86_64 (amd64), aarch64 (arm64)"
      err ""
      err "Pre-built binaries are only available for the above architectures."
      err "To install on ${machine}, build from source:"
      err "  https://github.com/${GITHUB_REPO}"
      exit 1
      ;;
  esac
}

# ── Download helper (curl or wget) ────────────────────────────────────────────
http_get() {
  local url="$1"
  local dest="$2"
  local extra_args="${3:-}"

  if command -v curl &>/dev/null; then
    # -fSL: fail on HTTP errors, show errors, follow redirects
    # -o:   output file
    curl -fSL ${extra_args} --progress-bar -o "${dest}" "${url}"
  elif command -v wget &>/dev/null; then
    wget --quiet --show-progress -O "${dest}" "${url}"
  else
    err "Neither curl nor wget is installed."
    err "Install one of them and retry:"
    err "  sudo apt-get install curl"
    exit 1
  fi
}

# JSON-parse a single field without jq (safe for simple strings).
# Usage: json_field <json_string> <field_name>
json_field() {
  local json="$1"
  local field="$2"
  echo "${json}" \
    | grep -o "\"${field}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" \
    | head -1 \
    | sed 's/.*"[^"]*"[[:space:]]*:[[:space:]]*"\(.*\)"/\1/'
}

# ── Version resolution ────────────────────────────────────────────────────────
resolve_version() {
  if [[ -n "${VERSION}" ]]; then
    # Normalise: ensure the tag has a leading 'v'
    if [[ "${VERSION}" != v* ]]; then
      VERSION="v${VERSION}"
    fi
    info "Installing requested version: ${VERSION}"
    return
  fi

  info "Resolving latest release..."
  local api_url="${GITHUB_API_URL}/latest"
  local tmpfile
  tmpfile="$(mktemp)"

  if command -v curl &>/dev/null; then
    curl -fSL --silent -H "Accept: application/vnd.github+json" \
      -o "${tmpfile}" "${api_url}" 2>/dev/null || true
  elif command -v wget &>/dev/null; then
    wget --quiet -O "${tmpfile}" "${api_url}" 2>/dev/null || true
  fi

  local tag
  tag="$(json_field "$(cat "${tmpfile}")" "tag_name")"
  rm -f "${tmpfile}"

  if [[ -z "${tag}" ]]; then
    err "Could not determine latest release from GitHub API."
    err ""
    err "Check your network connection, or specify a version explicitly:"
    err "  sudo VERSION=v1.2.3 bash $0"
    err ""
    err "Releases: ${GITHUB_RELEASES_URL}"
    exit 1
  fi

  VERSION="${tag}"
  info "Latest release: ${VERSION}"
}

# ── Banner ────────────────────────────────────────────────────────────────────
echo ""
echo "════════════════════════════════════════════════════"
echo "  Zero-Trust Client — Release Installer"
echo "════════════════════════════════════════════════════"
echo ""

# ── Detect architecture and resolve version ───────────────────────────────────
ARCH="$(detect_arch)"
ok "Architecture: $(uname -m) → ${ARCH}"

resolve_version
ok "Version: ${VERSION}"

# ── Build download URL ────────────────────────────────────────────────────────
ASSET_NAME="ztna-client-linux-${ARCH}"
DOWNLOAD_URL="${GITHUB_RELEASES_URL}/download/${VERSION}/${ASSET_NAME}"
echo ""

# ── Pre-flight: check for existing service ───────────────────────────────────
echo "── Pre-flight ───────────────────────────────────────"
UPGRADE=false
if [[ -f "${INSTALL_BIN}" ]]; then
  existing_ver="$("${INSTALL_BIN}" --version 2>/dev/null | awk '{print $NF}' || echo "unknown")"
  warn "Existing binary detected: ${INSTALL_BIN} (${existing_ver})"
  UPGRADE=true
fi

if systemctl is-active --quiet ztna-client.service 2>/dev/null; then
  echo "── Stopping existing service ────────────────────────"
  systemctl stop ztna-client.service
  ok "Stopped ztna-client.service"
  echo ""
fi
echo ""

# ── Download binary ───────────────────────────────────────────────────────────
echo "── Downloading ${ASSET_NAME} ─────────────────────────"
info "URL: ${DOWNLOAD_URL}"

TMP_BIN="$(mktemp -t ztna-client.XXXXXXXX)"
trap 'rm -f "${TMP_BIN}"' EXIT

if ! http_get "${DOWNLOAD_URL}" "${TMP_BIN}"; then
  err "Download failed."
  err ""
  err "Possible causes:"
  err "  • Version ${VERSION} does not exist — check ${GITHUB_RELEASES_URL}"
  err "  • Asset '${ASSET_NAME}' not yet uploaded for this release"
  err "  • Network connectivity issue"
  exit 1
fi

# Verify the downloaded file looks like an ELF binary
if ! file "${TMP_BIN}" 2>/dev/null | grep -q "ELF"; then
  err "Downloaded file does not appear to be a valid binary."
  err "It may be an HTML error page from GitHub."
  err ""
  err "Check the releases page: ${GITHUB_RELEASES_URL}"
  exit 1
fi

ok "Downloaded ${ASSET_NAME} ($(du -sh "${TMP_BIN}" | cut -f1))"
echo ""

# ── Install binary ────────────────────────────────────────────────────────────
echo "── Installing ───────────────────────────────────────"
install -m 0755 "${TMP_BIN}" "${INSTALL_BIN}"
ok "Binary installed → ${INSTALL_BIN}"

installed_ver="$("${INSTALL_BIN}" --version 2>/dev/null | awk '{print $NF}' || echo "unknown")"
ok "Installed version: ${installed_ver}"

# ── Systemd unit ─────────────────────────────────────────────────────────────
#
# The unit is embedded here so users do not need a local repo checkout.
# Update this block when the service definition changes.
#
write_systemd_unit() {
  cat > "${SYSTEMD_DST}" <<'UNIT'
[Unit]
Description=Zero-Trust Network Access Client
After=network-online.target
Wants=network-online.target
# Allow up to 5 restart attempts within 60 seconds; after that systemd will
# stop retrying and the operator must intervene.  This prevents endless
# restart loops when the config is broken or the controller is unreachable.
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
Type=simple

# The client reads its TOML config from /etc/ztna-client/client.conf.
# No EnvironmentFile needed — the binary handles config loading internally.
ExecStart=/usr/bin/ztna-client serve

WorkingDirectory=/var/lib/ztna-client
# Restart only on non-zero exit (crash/error).  A clean exit via
# `systemctl stop` returns 0 and will NOT trigger a restart.
Restart=on-failure
RestartSec=10
# Give the service 15 seconds to handle SIGTERM and clean up kernel routes
# before systemd sends SIGKILL.
TimeoutStopSec=15

# TUN mode requires CAP_NET_ADMIN for creating/managing the TUN device
# and manipulating ip route/rule entries.  Running as root is the simplest
# valid model for now; a dedicated user + ambient capabilities can be
# added later without changing the unit structure.
#
# NOTE: PrivateDevices is intentionally omitted — TUN mode needs /dev/net/tun.

StateDirectory=ztna-client
RuntimeDirectory=ztna-client
RuntimeDirectoryMode=0755

# --- Hardening ---
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ProtectKernelLogs=yes
ProtectKernelModules=yes
ProtectKernelTunables=yes
ProtectHostname=yes
ProtectClock=yes
ProtectControlGroups=yes
PrivateTmp=yes

RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK
RestrictNamespaces=yes
RestrictSUIDSGID=yes
RestrictRealtime=yes
SystemCallArchitectures=native

MemoryDenyWriteExecute=yes
LockPersonality=yes
# DevicePolicy=auto allows access to /dev/net/tun which TUN mode requires.
# PrivateDevices is NOT set for the same reason.
DevicePolicy=auto
DeviceAllow=/dev/net/tun rw
UMask=0077

[Install]
WantedBy=multi-user.target
UNIT
  chmod 0644 "${SYSTEMD_DST}"
}

write_systemd_unit
ok "Systemd unit → ${SYSTEMD_DST}"

# ── Config directory ──────────────────────────────────────────────────────────
mkdir -p "${CONFIG_DIR}"
chmod 0755 "${CONFIG_DIR}"
ok "Config directory → ${CONFIG_DIR}"

# Write default config only if it does not already exist (upgrade-safe).
if [[ -f "${CONFIG_FILE}" ]]; then
  warn "Existing config preserved: ${CONFIG_FILE}"
else
  cat > "${CONFIG_FILE}" <<'CONF'
# ZTNA Client Configuration
# Run 'ztna-client doctor' to verify your setup after editing.
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
# Advanced / transitional (uncomment if needed):
# controller_grpc_addr = "controller.example.com:8443"
# connector_tunnel_addr = "connector.example.com:9444"
# socks5_addr = "127.0.0.1:1080"
# port = 19515
CONF
  chmod 0644 "${CONFIG_FILE}"
  ok "Default config written → ${CONFIG_FILE}"
fi

# ── State directory ───────────────────────────────────────────────────────────
mkdir -p "${STATE_DIR}"
# 0711: owner rwx, others --x (traverse-only; needed for the CLI token file)
chmod 0711 "${STATE_DIR}"
ok "State directory → ${STATE_DIR}"

# ── Enable and start ──────────────────────────────────────────────────────────
echo ""
echo "── Starting service ─────────────────────────────────"
systemctl daemon-reload
systemctl enable ztna-client.service
systemctl start ztna-client.service
ok "ztna-client.service enabled and started"

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "════════════════════════════════════════════════════"
if [[ "${UPGRADE}" == "true" ]]; then
  echo "  Upgrade complete! (→ ${VERSION})"
else
  echo "  Installation complete! (${VERSION})"
fi
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
echo "    3. Verify setup:"
echo "       ztna-client doctor"
echo ""
echo "    4. Login (no sudo needed):"
echo "       ztna-client login"
echo ""
echo "    5. Check status:"
echo "       ztna-client status"
echo "       ztna-client resources"
echo ""
echo "    6. Service logs:"
echo "       sudo journalctl -u ztna-client -f"
echo ""
echo "  Listener ports (defaults):"
echo "    :19515  OAuth callback  — may bind to 0.0.0.0 for LAN testing"
echo "    :19516  Management API  — always bound to 127.0.0.1 (localhost only)"
echo ""
echo "  The OAuth redirect URI registered at your controller must use port 19515:"
echo "    http://<host>:19515/callback"
echo ""
