#!/usr/bin/env bash
set -euo pipefail

#
# Uninstall script for the group-management-ui-backend connector ("grpcconnector2").
# - Stops + disables systemd unit
# - Removes systemd unit file
# - Removes binary + config directory
#
# Notes:
# - This removes local connector artifacts only. It does not delete connector records
#   from the controller DB (run controller-side cleanup separately if desired).
#

UNIT_NAME="grpcconnector.service"
BIN_PATH="/usr/bin/grpcconnector"
CONFIG_DIR="/etc/grpcconnector"
UNIT_PATH="/etc/systemd/system/${UNIT_NAME}"
RUNTIME_DIR="/run/grpcconnector"

DRY_RUN="false"

usage() {
  cat <<'EOF'
Usage: connector-uninstall.sh [--dry-run]

Uninstalls the grpcconnector connector from this machine:
  - systemctl stop/disable grpcconnector.service
  - removes /etc/systemd/system/grpcconnector.service
  - removes /usr/bin/grpcconnector
  - removes /etc/grpcconnector
  - removes /run/grpcconnector (best-effort)

Options:
  --dry-run   Print actions without making changes.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN="true"
fi

if [[ "${EUID}" -ne 0 ]]; then
  echo "ERROR: uninstall must be run as root (use sudo)." >&2
  exit 1
fi

run() {
  if [[ "${DRY_RUN}" == "true" ]]; then
    echo "[dry-run] $*"
    return 0
  fi
  echo "+ $*"
  "$@"
}

echo "Uninstalling ${UNIT_NAME} (dry-run=${DRY_RUN})"

# Stop noisy restart loops first.
if command -v systemctl >/dev/null 2>&1; then
  if systemctl list-unit-files "${UNIT_NAME}" >/dev/null 2>&1; then
    run systemctl stop "${UNIT_NAME}" || true
    run systemctl disable "${UNIT_NAME}" || true
    run systemctl reset-failed "${UNIT_NAME}" || true
  fi
fi

# Remove unit file.
if [[ -f "${UNIT_PATH}" ]]; then
  run rm -f "${UNIT_PATH}"
fi

# Remove runtime state (best-effort).
if [[ -d "${RUNTIME_DIR}" ]]; then
  run rm -rf "${RUNTIME_DIR}" || true
fi

# Remove binary and config.
if [[ -f "${BIN_PATH}" ]]; then
  run rm -f "${BIN_PATH}"
fi

if [[ -d "${CONFIG_DIR}" ]]; then
  run rm -rf "${CONFIG_DIR}"
fi

if command -v systemctl >/dev/null 2>&1; then
  run systemctl daemon-reload || true
fi

echo "Done. If you also installed grpcconnector.service (from the grpccontroller repo), uninstall that separately."

