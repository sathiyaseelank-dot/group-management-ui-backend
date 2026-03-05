#!/usr/bin/env bash
set -euo pipefail

# Uninstall Zero-Trust gRPC Tunneler

if [[ "${EUID}" -ne 0 ]]; then
  echo "ERROR: uninstall must be run as root." >&2
  exit 1
fi

echo "Stopping and disabling grpctunneler service..."
systemctl stop grpctunneler.service 2>/dev/null || true
systemctl disable grpctunneler.service 2>/dev/null || true

echo "Removing systemd unit..."
rm -f /etc/systemd/system/grpctunneler.service
systemctl daemon-reload

echo "Removing binary..."
rm -f /usr/bin/grpctunneler

echo "Removing configuration..."
rm -rf /etc/grpctunneler

echo "Removing runtime directory..."
rm -rf /run/grpctunneler

echo "Tunneler uninstalled."
