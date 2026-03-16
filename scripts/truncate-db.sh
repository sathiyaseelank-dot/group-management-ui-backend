#!/usr/bin/env bash
# Truncate all controller DB tables (PostgreSQL).
# Loads DATABASE_URL from services/controller/.env if not set in environment.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$SCRIPT_DIR/../services/controller/.env"

# Load .env if DATABASE_URL is not already set
if [[ -z "${DATABASE_URL:-}" && -f "$ENV_FILE" ]]; then
  DATABASE_URL=$(grep -E '^DATABASE_URL=' "$ENV_FILE" | cut -d= -f2-)
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "ERROR: DATABASE_URL is not set and could not be loaded from $ENV_FILE" >&2
  exit 1
fi

echo "Target: $DATABASE_URL"
echo ""
echo "This will PERMANENTLY delete all data from every table."
read -r -p "Type 'yes' to confirm: " confirm

if [[ "$confirm" != "yes" ]]; then
  echo "Aborted."
  exit 0
fi

psql "$DATABASE_URL" <<'SQL'
TRUNCATE TABLE
  connectors,
  tunnelers,
  resources,
  authorizations,
  tokens,
  audit_logs,
  users,
  user_groups,
  user_group_members,
  remote_networks,
  remote_network_connectors,
  connector_policy_versions,
  access_rules,
  access_rule_groups,
  service_accounts,
  connector_logs,
  tunneler_logs,
  invite_tokens,
  admin_audit_logs,
  workspaces,
  workspace_members,
  workspace_invites,
  identity_providers,
  sessions,
  device_auth_requests,
  device_posture,
  device_trusted_profiles
RESTART IDENTITY CASCADE;
SQL

echo ""
echo "All tables truncated successfully."
