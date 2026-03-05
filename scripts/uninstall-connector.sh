#!/usr/bin/env bash
set -euo pipefail

# Uninstall Zero-Trust gRPC Connector

if [[ "${EUID}" -ne 0 ]]; then
  echo "ERROR: uninstall must be run as root." >&2
  exit 1
fi

echo "Stopping and disabling grpcconnector2 service..."
systemctl stop grpcconnector2.service 2>/dev/null || true
systemctl disable grpcconnector2.service 2>/dev/null || true

echo "Removing systemd unit..."
rm -f /etc/systemd/system/grpcconnector2.service
systemctl daemon-reload

echo "Removing binary..."
rm -f /usr/bin/grpcconnector2

echo "Removing configuration..."
rm -rf /etc/grpcconnector2

echo "Removing runtime directory..."
rm -rf /run/grpcconnector2

echo "Connector uninstalled."
