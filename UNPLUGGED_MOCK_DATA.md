# Unplugged Mock Data

The frontend no longer uses the local mock SQLite data path for runtime API responses.

Changes:
- Removed the remaining `getDb()` usage in API routes by proxying to backend endpoints.
- Frontend API routes now source data from the backend/admin services.

Notes:
- The backend SQLite DB is now the single source of truth.
- Frontend local SQLite has been removed and is no longer used.

Mock data artifacts (removed from runtime):
- `frontend/lib/db.ts` (deleted)
- `frontend/lib/data.ts` (deleted)

## Current DB State (corrected)

- **Backend DB** (controller SQLite) is the only active datastore.
- **Frontend DB** has been fully removed; all frontend API routes proxy to backend endpoints.
- **State update:** Backend database schema and data sources are corrected and aligned with the UI as of 2026-03-03.

## Sample Mock Data (per table)

**users**
```json
{ "id": "usr_123", "name": "Alice Johnson", "email": "alice@example.com", "certificate_identity": "identity-uuid-1", "status": "active", "created_at": "2026-02-28" }
```

**groups**
```json
{ "id": "grp_123", "name": "Engineering", "description": "Core engineering team", "created_at": "2026-02-28" }
```

**group_members**
```json
{ "group_id": "grp_123", "user_id": "usr_123" }
```

**service_accounts**
```json
{ "id": "svc_123", "name": "ci-bot", "status": "active", "associated_resource_count": 1, "created_at": "2026-02-28" }
```

**remote_networks**
```json
{ "id": "net_123", "name": "AWS-Prod", "location": "AWS", "created_at": "2026-02-28" }
```

**connectors**
```json
{ "id": "con_123", "name": "aws-connector-1", "status": "online", "version": "1.0.0", "hostname": "aws-connector-1.local", "remote_network_id": "net_123", "last_seen": "2026-02-28T12:00:00Z" }
```

**tunnelers**
```json
{ "id": "tun_123", "name": "dev-laptop", "status": "online", "version": "1.0.0", "hostname": "dev-laptop.local", "remote_network_id": "net_123" }
```

**resources**
```json
{ "id": "res_123", "name": "prod-db", "type": "STANDARD", "address": "10.0.10.50", "protocol": "TCP", "port_from": 5432, "port_to": 5432, "description": "Postgres", "remote_network_id": "net_123" }
```

**access_rules**
```json
{ "id": "rule_123", "name": "Engineering DB Access", "resource_id": "res_123", "enabled": 1, "created_at": "2026-02-28", "updated_at": "2026-02-28" }
```

**access_rule_groups**
```json
{ "rule_id": "rule_123", "group_id": "grp_123" }
```

**connector_logs**
```json
{ "id": 1, "connector_id": "con_123", "timestamp": "2026-02-28T12:00:00Z", "message": "connector started" }
```

**connector_policy_versions**
```json
{ "connector_id": "con_123", "version": 3, "compiled_at": "2026-02-28T12:05:00Z", "policy_hash": "abc123..." }
```

## Former Frontend DB Schema (for reference)

**users**
- `id`, `name`, `email`, `certificate_identity`, `status`, `created_at`

**groups**
- `id`, `name`, `description`, `created_at`

**group_members**
- `group_id`, `user_id`

**service_accounts**
- `id`, `name`, `status`, `associated_resource_count`, `created_at`

**remote_networks**
- `id`, `name`, `location`, `created_at`

**connectors**
- `id`, `name`, `status`, `version`, `hostname`, `remote_network_id`, `last_seen`, `last_policy_version`, `last_seen_at`, `installed`

**tunnelers**
- `id`, `name`, `status`, `version`, `hostname`, `remote_network_id`

**resources**
- `id`, `name`, `type`, `address`, `ports`, `protocol`, `port_from`, `port_to`, `alias`, `description`, `remote_network_id`

**access_rules**
- `id`, `name`, `resource_id`, `enabled`, `created_at`, `updated_at`

**access_rule_groups**
- `rule_id`, `group_id`

**connector_logs**
- `id`, `connector_id`, `timestamp`, `message`

**connector_policy_versions**
- `connector_id`, `version`, `compiled_at`, `policy_hash`
