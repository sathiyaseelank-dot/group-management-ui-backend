package state

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "github.com/lib/pq"
)

// Open returns a *sql.DB connected to PostgreSQL. SQLite is not supported.
func Open(databaseURL, sqlitePath string) (*sql.DB, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required (SQLite disabled)")
	}
	DBDriver = "postgres"
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	if err := initSchemaDialect(db, "postgres"); err != nil {
		db.Close()
		return nil, err
	}
	log.Println("state: connected to PostgreSQL")
	return db, nil
}

// DBDriver is "sqlite" or "postgres", set by Open().
var DBDriver string

// Rebind converts ? placeholders to $1, $2, … for PostgreSQL.
// For SQLite it is a no-op.
func Rebind(query string) string {
	if DBDriver != "postgres" {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 32)
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteString(fmt.Sprintf("$%d", n))
		} else {
			b.WriteByte(query[i])
		}
	}
	return b.String()
}

// initSchemaDialect runs the full CREATE TABLE DDL for the given dialect.
// The only syntax difference handled here is AUTOINCREMENT (SQLite) vs BIGSERIAL (Postgres).
func initSchemaDialect(db *sql.DB, dialect string) error {
	serial := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if dialect == "postgres" {
		serial = "BIGSERIAL PRIMARY KEY"
	}

	// Rename legacy tables/columns if they still exist under old names.
	if dialect == "postgres" {
		_, _ = db.Exec(`ALTER TABLE IF EXISTS tunnelers RENAME TO agents`)
		_, _ = db.Exec(`ALTER TABLE IF EXISTS tunneler_logs RENAME TO agent_logs`)
		_, _ = db.Exec(`ALTER TABLE IF EXISTS agent_logs RENAME COLUMN tunneler_id TO agent_id`)
		_, _ = db.Exec(`ALTER TABLE IF EXISTS audit_logs RENAME COLUMN tunneler_id TO agent_id`)
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS connectors (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'offline',
			version TEXT NOT NULL DEFAULT '',
			hostname TEXT NOT NULL DEFAULT '',
			private_ip TEXT NOT NULL DEFAULT '',
			remote_network_id TEXT NOT NULL DEFAULT '',
			last_seen INTEGER NOT NULL DEFAULT 0,
			last_seen_at TEXT NOT NULL DEFAULT '',
			installed INTEGER NOT NULL DEFAULT 0,
			last_policy_version INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			spiffe_id TEXT NOT NULL DEFAULT '',
			connector_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'offline',
			version TEXT NOT NULL DEFAULT '',
			hostname TEXT NOT NULL DEFAULT '',
			remote_network_id TEXT NOT NULL DEFAULT '',
			last_seen INTEGER NOT NULL DEFAULT 0,
			revoked INTEGER NOT NULL DEFAULT 0,
			last_seen_at TEXT NOT NULL DEFAULT '',
			installed INTEGER NOT NULL DEFAULT 0,
			ip TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS resources (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT 'dns',
			address TEXT NOT NULL DEFAULT '',
			ports TEXT NOT NULL DEFAULT '',
			protocol TEXT NOT NULL DEFAULT 'TCP',
			port_from INTEGER,
			port_to INTEGER,
			alias TEXT,
			description TEXT NOT NULL DEFAULT '',
			remote_network_id TEXT NOT NULL DEFAULT '',
			connector_id TEXT NOT NULL DEFAULT '',
			firewall_status TEXT NOT NULL DEFAULT 'unprotected'
		)`,
		`CREATE TABLE IF NOT EXISTS authorizations (
			resource_id TEXT NOT NULL,
			principal_spiffe TEXT NOT NULL,
			filters TEXT NOT NULL DEFAULT '[]',
			PRIMARY KEY (resource_id, principal_spiffe)
		)`,
		`CREATE TABLE IF NOT EXISTS tokens (
			token TEXT PRIMARY KEY,
			connector_id TEXT NOT NULL DEFAULT '',
			expires_at INTEGER NOT NULL DEFAULT 0,
			consumed INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id ` + serial + `,
			principal_spiffe TEXT NOT NULL DEFAULT '',
			agent_id TEXT NOT NULL DEFAULT '',
			resource_id TEXT NOT NULL DEFAULT '',
			destination TEXT NOT NULL DEFAULT '',
			protocol TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 0,
			decision TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			connection_id TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'Active',
			role TEXT NOT NULL DEFAULT 'Member',
			certificate_identity TEXT,
			google_sub TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_unique ON users (LOWER(email))`,
		`CREATE TABLE IF NOT EXISTS user_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS user_group_members (
			user_id TEXT NOT NULL,
			group_id TEXT NOT NULL,
			joined_at INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (user_id, group_id)
		)`,
		`CREATE TABLE IF NOT EXISTS remote_networks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			location TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS remote_network_connectors (
			network_id TEXT NOT NULL,
			connector_id TEXT NOT NULL,
			PRIMARY KEY (network_id, connector_id)
		)`,
		`CREATE TABLE IF NOT EXISTS connector_policy_versions (
			connector_id TEXT PRIMARY KEY,
			version INTEGER NOT NULL DEFAULT 0,
			compiled_at TEXT NOT NULL DEFAULT '',
			policy_hash TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS access_rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			resource_id TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS access_rule_groups (
			rule_id TEXT NOT NULL,
			group_id TEXT NOT NULL,
			PRIMARY KEY (rule_id, group_id)
		)`,
		`CREATE TABLE IF NOT EXISTS service_accounts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS connector_logs (
			id ` + serial + `,
			connector_id TEXT NOT NULL DEFAULT '',
			timestamp TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS agent_logs (
			id ` + serial + `,
			agent_id TEXT NOT NULL DEFAULT '',
			timestamp TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS invite_tokens (
			token TEXT PRIMARY KEY,
			email TEXT NOT NULL DEFAULT '',
			expires_at INTEGER NOT NULL DEFAULT 0,
			used INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS admin_audit_logs (
			id ` + serial + `,
			timestamp INTEGER NOT NULL DEFAULT 0,
			actor TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL DEFAULT '',
			target TEXT NOT NULL DEFAULT '',
			result TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			slug TEXT NOT NULL UNIQUE,
			trust_domain TEXT NOT NULL UNIQUE,
			ca_cert_pem TEXT NOT NULL DEFAULT '',
			ca_key_pem TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS workspace_members (
			workspace_id TEXT NOT NULL REFERENCES workspaces(id),
			user_id TEXT NOT NULL REFERENCES users(id),
			role TEXT NOT NULL DEFAULT 'member',
			joined_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (workspace_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS workspace_invites (
			token TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL REFERENCES workspaces(id),
			email TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'member',
			expires_at INTEGER NOT NULL DEFAULT 0,
			used INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS identity_providers (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			provider_type TEXT NOT NULL,
			client_id TEXT NOT NULL,
			client_secret_encrypted TEXT NOT NULL,
			redirect_uri TEXT NOT NULL DEFAULT '',
			issuer_url TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL DEFAULT '',
			session_type TEXT NOT NULL,
			device_id TEXT NOT NULL DEFAULT '',
			refresh_token_hash TEXT NOT NULL DEFAULT '',
			ip_address TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0,
			expires_at INTEGER NOT NULL DEFAULT 0,
			revoked INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS device_auth_requests (
			state TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			code_challenge TEXT NOT NULL,
			redirect_uri TEXT NOT NULL,
			idp_id TEXT NOT NULL DEFAULT '',
			platform TEXT NOT NULL DEFAULT 'mobile',
			created_at INTEGER NOT NULL DEFAULT 0,
			expires_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS agent_discovered_services (
			id ` + serial + `,
			agent_id TEXT NOT NULL,
			port INTEGER NOT NULL,
			protocol TEXT NOT NULL DEFAULT 'tcp',
			bound_ip TEXT NOT NULL DEFAULT '',
			first_seen INTEGER NOT NULL DEFAULT 0,
			last_seen INTEGER NOT NULL DEFAULT 0,
			workspace_id TEXT NOT NULL DEFAULT '',
			UNIQUE(agent_id, port, protocol)
		)`,
		`CREATE TABLE IF NOT EXISTS invite_auth_requests (
			state          TEXT PRIMARY KEY,
			invite_token   TEXT NOT NULL,
			code_challenge TEXT NOT NULL,
			redirect_uri   TEXT NOT NULL,
			created_at     INTEGER NOT NULL DEFAULT 0,
			expires_at     INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS device_posture (
			device_id           TEXT NOT NULL,
			workspace_id        TEXT NOT NULL DEFAULT '',
			spiffe_id           TEXT NOT NULL DEFAULT '',
			os_type             TEXT NOT NULL DEFAULT '',
			os_version          TEXT NOT NULL DEFAULT '',
			hostname            TEXT NOT NULL DEFAULT '',
			firewall_enabled    INTEGER NOT NULL DEFAULT 0,
			disk_encrypted      INTEGER NOT NULL DEFAULT 0,
			screen_lock_enabled INTEGER NOT NULL DEFAULT 0,
			client_version      TEXT NOT NULL DEFAULT '',
			collected_at        TEXT NOT NULL DEFAULT '',
			reported_at         TEXT NOT NULL DEFAULT '',
			user_id             TEXT NOT NULL DEFAULT '',
			device_name         TEXT NOT NULL DEFAULT '',
			device_model        TEXT NOT NULL DEFAULT '',
			device_make         TEXT NOT NULL DEFAULT '',
			serial_number       TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (device_id, workspace_id)
		)`,
		`CREATE TABLE IF NOT EXISTS device_trusted_profiles (
			id                      TEXT PRIMARY KEY,
			workspace_id            TEXT NOT NULL DEFAULT '',
			name                    TEXT NOT NULL DEFAULT '',
			require_firewall        INTEGER NOT NULL DEFAULT 0,
			require_disk_encryption INTEGER NOT NULL DEFAULT 0,
			require_screen_lock     INTEGER NOT NULL DEFAULT 0,
			min_os_version          TEXT NOT NULL DEFAULT '',
			created_at              TEXT NOT NULL DEFAULT '',
			updated_at              TEXT NOT NULL DEFAULT ''
		)`,
	}

	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			log.Printf("schema init error [%s]: %v (stmt: %.80s…)", dialect, err, s)
			return err
		}
	}
	// Add new columns for existing databases.
	if dialect == "postgres" {
		_, _ = db.Exec(`ALTER TABLE agent_discovered_services DROP COLUMN IF EXISTS pid`)
		_, _ = db.Exec(`ALTER TABLE agent_discovered_services DROP COLUMN IF EXISTS process_name`)
		_, _ = db.Exec(`ALTER TABLE agent_discovered_services ADD COLUMN IF NOT EXISTS dismissed INTEGER NOT NULL DEFAULT 0`)
		_, _ = db.Exec(`ALTER TABLE agent_discovered_services ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active'`)
		_, _ = db.Exec(`ALTER TABLE agent_discovered_services ADD COLUMN IF NOT EXISTS service_name TEXT NOT NULL DEFAULT ''`)
		_, _ = db.Exec(`ALTER TABLE agent_discovered_services ADD COLUMN IF NOT EXISTS process_name TEXT NOT NULL DEFAULT ''`)
		_, _ = db.Exec(`ALTER TABLE device_auth_requests ADD COLUMN IF NOT EXISTS platform TEXT NOT NULL DEFAULT 'mobile'`)
		_, _ = db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS google_sub TEXT NOT NULL DEFAULT ''`)
		_, _ = db.Exec(`ALTER TABLE connectors ADD COLUMN IF NOT EXISTS revoked INTEGER NOT NULL DEFAULT 0`)
		_, _ = db.Exec(`ALTER TABLE agents ADD COLUMN IF NOT EXISTS revoked INTEGER NOT NULL DEFAULT 0`)
		_, _ = db.Exec(`ALTER TABLE agents ADD COLUMN IF NOT EXISTS last_seen_at TEXT NOT NULL DEFAULT ''`)
		_, _ = db.Exec(`ALTER TABLE agents ADD COLUMN IF NOT EXISTS installed INTEGER NOT NULL DEFAULT 0`)
		_, _ = db.Exec(`ALTER TABLE agents ADD COLUMN IF NOT EXISTS ip TEXT NOT NULL DEFAULT ''`)
		_, _ = db.Exec(`ALTER TABLE user_groups ADD COLUMN IF NOT EXISTS trusted_profile_id TEXT NOT NULL DEFAULT ''`)
		_, _ = db.Exec(`ALTER TABLE device_posture ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT ''`)
		_, _ = db.Exec(`ALTER TABLE device_posture ADD COLUMN IF NOT EXISTS device_name TEXT NOT NULL DEFAULT ''`)
		_, _ = db.Exec(`ALTER TABLE device_posture ADD COLUMN IF NOT EXISTS device_model TEXT NOT NULL DEFAULT ''`)
		_, _ = db.Exec(`ALTER TABLE device_posture ADD COLUMN IF NOT EXISTS device_make TEXT NOT NULL DEFAULT ''`)
		_, _ = db.Exec(`ALTER TABLE device_posture ADD COLUMN IF NOT EXISTS serial_number TEXT NOT NULL DEFAULT ''`)
	}

	// Phase 2 migration: add workspace_id columns to existing tables.
	if err := migrateWorkspaceColumns(db, dialect); err != nil {
		return err
	}
	if err := backfillWorkspaceScope(db); err != nil {
		log.Printf("migration warning [workspace backfill]: %v", err)
	}

	// Phase 3 migration: add firewall_status column to resources.
	if dialect == "postgres" {
		_, _ = db.Exec(`ALTER TABLE resources ADD COLUMN IF NOT EXISTS firewall_status TEXT NOT NULL DEFAULT 'unprotected'`)
	} else {
		if !sqliteColumnExists(db, "resources", "firewall_status") {
			_, _ = db.Exec(`ALTER TABLE resources ADD COLUMN firewall_status TEXT NOT NULL DEFAULT 'unprotected'`)
		}
	}

	// Phase 4 migration: backfill workspace_id on rows created before tenant scoping was
	// enforced (those rows have workspace_id = '').  Assign them to the first workspace so
	// they remain visible after the withWorkspaceContext middleware started requiring a JWT.
	backfillTables := []string{
		"connectors", "agents", "resources", "tokens",
		"remote_networks", "access_rules", "user_groups",
		"service_accounts",
	}
	for _, tbl := range backfillTables {
		_, _ = db.Exec(fmt.Sprintf(
			`UPDATE %s SET workspace_id = (SELECT id FROM workspaces ORDER BY created_at ASC LIMIT 1)
			 WHERE workspace_id = '' AND (SELECT id FROM workspaces LIMIT 1) IS NOT NULL`, tbl))
	}

	return nil
}

// migrateWorkspaceColumns adds workspace_id columns to tables that need tenant scoping.
// Uses dialect-appropriate syntax to handle the "column already exists" case.
func migrateWorkspaceColumns(db *sql.DB, dialect string) error {
	tables := []string{
		"connectors", "agents", "resources", "tokens",
		"remote_networks", "access_rules", "user_groups",
		"service_accounts", "audit_logs",
	}
	for _, table := range tables {
		if dialect == "postgres" {
			stmt := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS workspace_id TEXT NOT NULL DEFAULT ''`, table)
			if _, err := db.Exec(stmt); err != nil {
				log.Printf("migration warning [%s.workspace_id]: %v", table, err)
			}
		} else {
			// SQLite: check if column exists via PRAGMA, add if missing.
			if !sqliteColumnExists(db, table, "workspace_id") {
				stmt := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN workspace_id TEXT NOT NULL DEFAULT ''`, table)
				if _, err := db.Exec(stmt); err != nil {
					log.Printf("migration warning [%s.workspace_id]: %v", table, err)
				}
			}
		}
	}
	return nil
}

// backfillWorkspaceScope repairs legacy rows that predate workspace scoping.
// It first infers workspace ownership from existing relationships, then falls back
// to a full-table assignment only when exactly one workspace exists.
func backfillWorkspaceScope(db *sql.DB) error {
	for i := 0; i < 3; i++ {
		if err := inferWorkspaceScope(db); err != nil {
			return err
		}
	}

	rows, err := db.Query(`SELECT id FROM workspaces ORDER BY id ASC LIMIT 2`)
	if err != nil {
		return err
	}
	defer rows.Close()

	workspaceIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if strings.TrimSpace(id) != "" {
			workspaceIDs = append(workspaceIDs, id)
		}
	}
	if len(workspaceIDs) != 1 {
		return nil
	}

	return fillBlankWorkspaceScope(db, workspaceIDs[0], []string{
		"connectors",
		"tunnelers",
		"resources",
		"tokens",
		"remote_networks",
		"access_rules",
		"user_groups",
		"service_accounts",
		"audit_logs",
	})
}

func inferWorkspaceScope(db *sql.DB) error {
	statements := []string{
		`UPDATE remote_networks rn
			SET workspace_id = src.workspace_id
		FROM (
			SELECT remote_network_id AS id, MIN(workspace_id) AS workspace_id
			FROM connectors
			WHERE COALESCE(TRIM(remote_network_id), '') <> '' AND COALESCE(TRIM(workspace_id), '') <> ''
			GROUP BY remote_network_id
			HAVING COUNT(DISTINCT workspace_id) = 1
			UNION
			SELECT remote_network_id AS id, MIN(workspace_id) AS workspace_id
			FROM resources
			WHERE COALESCE(TRIM(remote_network_id), '') <> '' AND COALESCE(TRIM(workspace_id), '') <> ''
			GROUP BY remote_network_id
			HAVING COUNT(DISTINCT workspace_id) = 1
		) src
		WHERE rn.id = src.id AND COALESCE(TRIM(rn.workspace_id), '') = ''`,
		`UPDATE connectors c
			SET workspace_id = rn.workspace_id
		FROM remote_networks rn
		WHERE c.remote_network_id = rn.id
		  AND COALESCE(TRIM(c.workspace_id), '') = ''
		  AND COALESCE(TRIM(rn.workspace_id), '') <> ''`,
		`UPDATE tunnelers t
			SET workspace_id = src.workspace_id
		FROM (
			SELECT id, workspace_id FROM connectors WHERE COALESCE(TRIM(workspace_id), '') <> ''
			UNION
			SELECT id, workspace_id FROM remote_networks WHERE COALESCE(TRIM(workspace_id), '') <> ''
		) src
		WHERE (t.connector_id = src.id OR t.remote_network_id = src.id)
		  AND COALESCE(TRIM(t.workspace_id), '') = ''`,
		`UPDATE resources r
			SET workspace_id = src.workspace_id
		FROM (
			SELECT id, workspace_id FROM remote_networks WHERE COALESCE(TRIM(workspace_id), '') <> ''
			UNION
			SELECT resource_id AS id, MIN(workspace_id) AS workspace_id
			FROM access_rules
			WHERE COALESCE(TRIM(workspace_id), '') <> ''
			GROUP BY resource_id
			HAVING COUNT(DISTINCT workspace_id) = 1
		) src
		WHERE (r.remote_network_id = src.id OR r.id = src.id)
		  AND COALESCE(TRIM(r.workspace_id), '') = ''`,
		`UPDATE access_rules ar
			SET workspace_id = src.workspace_id
		FROM (
			SELECT id, workspace_id FROM resources WHERE COALESCE(TRIM(workspace_id), '') <> ''
			UNION
			SELECT arg.rule_id AS id, MIN(ug.workspace_id) AS workspace_id
			FROM access_rule_groups arg
			JOIN user_groups ug ON ug.id = arg.group_id
			WHERE COALESCE(TRIM(ug.workspace_id), '') <> ''
			GROUP BY arg.rule_id
			HAVING COUNT(DISTINCT ug.workspace_id) = 1
		) src
		WHERE (ar.resource_id = src.id OR ar.id = src.id)
		  AND COALESCE(TRIM(ar.workspace_id), '') = ''`,
		`UPDATE user_groups ug
			SET workspace_id = src.workspace_id
		FROM (
			SELECT arg.group_id AS id, MIN(ar.workspace_id) AS workspace_id
			FROM access_rule_groups arg
			JOIN access_rules ar ON ar.id = arg.rule_id
			WHERE COALESCE(TRIM(ar.workspace_id), '') <> ''
			GROUP BY arg.group_id
			HAVING COUNT(DISTINCT ar.workspace_id) = 1
			UNION
			SELECT ugm.group_id AS id, MIN(wm.workspace_id) AS workspace_id
			FROM user_group_members ugm
			JOIN workspace_members wm ON wm.user_id = ugm.user_id
			WHERE COALESCE(TRIM(wm.workspace_id), '') <> ''
			GROUP BY ugm.group_id
			HAVING COUNT(DISTINCT wm.workspace_id) = 1
		) src
		WHERE ug.id = src.id
		  AND COALESCE(TRIM(ug.workspace_id), '') = ''`,
		`UPDATE tokens tok
			SET workspace_id = c.workspace_id
		FROM connectors c
		WHERE tok.connector_id = c.id
		  AND COALESCE(TRIM(tok.workspace_id), '') = ''
		  AND COALESCE(TRIM(c.workspace_id), '') <> ''`,
		`UPDATE audit_logs a
			SET workspace_id = src.workspace_id
		FROM (
			SELECT id, workspace_id FROM resources WHERE COALESCE(TRIM(workspace_id), '') <> ''
			UNION
			SELECT id, workspace_id FROM tunnelers WHERE COALESCE(TRIM(workspace_id), '') <> ''
		) src
		WHERE (a.resource_id = src.id OR a.tunneler_id = src.id)
		  AND COALESCE(TRIM(a.workspace_id), '') = ''`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func fillBlankWorkspaceScope(db *sql.DB, workspaceID string, tables []string) error {
	for _, table := range tables {
		stmt := fmt.Sprintf(`UPDATE %s SET workspace_id = ? WHERE COALESCE(TRIM(workspace_id), '') = ''`, table)
		if _, err := db.Exec(Rebind(stmt), workspaceID); err != nil {
			return err
		}
	}
	return nil
}

func sqliteColumnExists(db *sql.DB, table, column string) bool {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			continue
		}
		if name == column {
			return true
		}
	}
	return false
}
