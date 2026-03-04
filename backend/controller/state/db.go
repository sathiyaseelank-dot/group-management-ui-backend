package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// DB is the package-level database handle set by InitDB.
var DB *sql.DB

// DBDriver is "sqlite" or "postgres".
var DBDriver string

// InitDB opens a database connection based on whether databaseURL is set.
// If databaseURL is non-empty, PostgreSQL is used; otherwise SQLite at dbPath.
func InitDB(dbPath, databaseURL string) (*sql.DB, error) {
	var (
		db  *sql.DB
		err error
	)

	if databaseURL != "" {
		db, err = sql.Open("postgres", databaseURL)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		DBDriver = "postgres"
	} else {
		if dbPath == "" {
			dbPath = DefaultDBPath
		}
		if err = os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
			return nil, err
		}
		db, err = sql.Open("sqlite", dbPath)
		if err != nil {
			return nil, fmt.Errorf("open sqlite: %w", err)
		}
		DBDriver = "sqlite"
	}

	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s: %w", DBDriver, err)
	}

	DB = db

	if DBDriver == "postgres" {
		err = migratePostgres(db)
	} else {
		err = migrateSQLite(db)
	}
	if err != nil {
		_ = db.Close()
		DB = nil
		return nil, err
	}

	return db, nil
}

// Rebind converts ? placeholders to $1, $2, ... for PostgreSQL.
// For SQLite it is a no-op.
// It also translates "INSERT OR IGNORE INTO" to "INSERT INTO ... ON CONFLICT DO NOTHING".
func Rebind(query string) string {
	if DBDriver != "postgres" {
		return query
	}

	wasIgnore := strings.Contains(query, "INSERT OR IGNORE")
	query = strings.Replace(query, "INSERT OR IGNORE INTO", "INSERT INTO", 1)

	var b strings.Builder
	b.Grow(len(query) + 32)
	idx := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			fmt.Fprintf(&b, "$%d", idx)
			idx++
		} else {
			b.WriteByte(query[i])
		}
	}

	out := b.String()
	if wasIgnore {
		out += " ON CONFLICT DO NOTHING"
	}
	return out
}

func migratePostgres(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tokens (
			hash TEXT PRIMARY KEY,
			expires_at BIGINT NOT NULL,
			used INTEGER NOT NULL DEFAULT 0,
			connector_id TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			certificate_identity TEXT,
			status TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS user_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS user_group_members (
			user_id TEXT NOT NULL,
			group_id TEXT NOT NULL,
			added_at BIGINT NOT NULL,
			PRIMARY KEY (user_id, group_id)
		);`,
		`CREATE TABLE IF NOT EXISTS remote_networks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			location TEXT,
			tags_json TEXT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS connector_remote_networks (
			connector_id TEXT NOT NULL,
			remote_network_id TEXT NOT NULL,
			assigned_at BIGINT NOT NULL,
			PRIMARY KEY (connector_id, remote_network_id)
		);`,
		`CREATE TABLE IF NOT EXISTS connectors (
			id TEXT PRIMARY KEY,
			name TEXT,
			status TEXT,
			hostname TEXT,
			private_ip TEXT,
			version TEXT,
			last_seen BIGINT NOT NULL,
			remote_network_id TEXT,
			installed INTEGER NOT NULL DEFAULT 0,
			last_policy_version INTEGER NOT NULL DEFAULT 0,
			last_seen_at TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS tunnelers (
			id TEXT PRIMARY KEY,
			spiffe_id TEXT,
			connector_id TEXT,
			last_seen BIGINT NOT NULL,
			name TEXT,
			status TEXT,
			version TEXT,
			hostname TEXT,
			remote_network_id TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS resources (
			id TEXT PRIMARY KEY,
			name TEXT,
			type TEXT NOT NULL,
			address TEXT,
			ports TEXT,
			protocol TEXT NOT NULL DEFAULT 'TCP',
			port_from INTEGER,
			port_to INTEGER,
			alias TEXT,
			description TEXT,
			remote_network_id TEXT,
			user_group_ids_json TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS service_accounts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			associated_resource_count INTEGER NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS access_rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS access_rule_groups (
			rule_id TEXT NOT NULL,
			group_id TEXT NOT NULL,
			PRIMARY KEY (rule_id, group_id)
		);`,
		`CREATE TABLE IF NOT EXISTS connector_logs (
			id BIGSERIAL PRIMARY KEY,
			connector_id TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			message TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS connector_policy_versions (
			connector_id TEXT PRIMARY KEY,
			version INTEGER NOT NULL DEFAULT 0,
			compiled_at TEXT NOT NULL,
			policy_hash TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS authorizations (
			principal_spiffe TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			filters_json TEXT,
			expires_at BIGINT,
			description TEXT,
			PRIMARY KEY (principal_spiffe, resource_id)
		);`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id BIGSERIAL PRIMARY KEY,
			principal_spiffe TEXT,
			tunneler_id TEXT,
			resource_id TEXT,
			destination TEXT,
			protocol TEXT,
			port INTEGER,
			decision TEXT,
			reason TEXT,
			connection_id TEXT,
			created_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS devices (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			device_name TEXT,
			device_fingerprint TEXT NOT NULL,
			trust_state TEXT NOT NULL,
			first_seen_at BIGINT NOT NULL,
			last_seen_at BIGINT,
			trusted_at BIGINT,
			public_key_pem TEXT,
			cert_pem TEXT,
			device_os TEXT,
			UNIQUE (user_id, device_fingerprint)
		);`,
		`CREATE TABLE IF NOT EXISTS invites (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			pkce_verifier TEXT,
			expires_at BIGINT NOT NULL,
			used_at BIGINT,
			created_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS user_verifications (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at BIGINT NOT NULL,
			used_at BIGINT,
			created_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			issued_at BIGINT NOT NULL,
			expires_at BIGINT NOT NULL,
			revoked_at BIGINT,
			state TEXT NOT NULL DEFAULT 'active',
			refresh_token_hash TEXT NOT NULL UNIQUE
		);`,
		`CREATE TABLE IF NOT EXISTS refresh_token_history (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			revoked_at BIGINT NOT NULL,
			created_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS admin_browser_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			user_agent_hash TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			expires_at BIGINT NOT NULL,
			revoked_at BIGINT
		);`,
		`CREATE TABLE IF NOT EXISTS user_roles (
			user_id TEXT NOT NULL,
			role TEXT NOT NULL,
			PRIMARY KEY (user_id, role)
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("postgres migrate failed: %w", err)
		}
	}
	return nil
}
