#!/usr/bin/env bash
set -euo pipefail

# Uninstall Zero-Trust Agent

if [[ "${EUID}" -ne 0 ]]; then
  echo "ERROR: uninstall must be run as root." >&2
  exit 1
fi

echo "Stopping and disabling agent service..."
systemctl stop agent.service 2>/dev/null || true
systemctl disable agent.service 2>/dev/null || true

echo "Removing systemd unit..."
rm -f /etc/systemd/system/agent.service
systemctl daemon-reload

echo "Removing binary..."
rm -f /usr/bin/agent

echo "Removing configuration..."
rm -rf /etc/agent

echo "Removing state directory..."
rm -rf /var/lib/agent

echo "Removing runtime directory..."
rm -rf /run/agent

echo "Cleaning up nftables rules..."
nft delete table inet ztna 2>/dev/null || true

echo "Agent uninstalled."
