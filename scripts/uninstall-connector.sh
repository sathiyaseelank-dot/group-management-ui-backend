#!/usr/bin/env bash
set -euo pipefail

# Uninstall Zero-Trust Connector

if [[ "${EUID}" -ne 0 ]]; then
  echo "ERROR: uninstall must be run as root." >&2
  exit 1
fi

echo "Stopping and disabling connector service..."
systemctl stop connector.service 2>/dev/null || true
systemctl disable connector.service 2>/dev/null || true

echo "Removing systemd unit..."
rm -f /etc/systemd/system/connector.service
systemctl daemon-reload

echo "Removing binary..."
rm -f /usr/bin/connector

echo "Removing configuration..."
rm -rf /etc/connector

echo "Removing state directory..."
rm -rf /var/lib/connector

echo "Removing runtime directory..."
rm -rf /run/connector

echo "Connector uninstalled."
