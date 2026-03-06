package state

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func OpenSQLite(path string) (*sql.DB, error) {
	if path == "" {
		path = "controller.db"
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func initSchema(db *sql.DB) error {
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
		`CREATE TABLE IF NOT EXISTS tunnelers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			spiffe_id TEXT NOT NULL DEFAULT '',
			connector_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'offline',
			version TEXT NOT NULL DEFAULT '',
			hostname TEXT NOT NULL DEFAULT '',
			remote_network_id TEXT NOT NULL DEFAULT '',
			last_seen INTEGER NOT NULL DEFAULT 0
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
			connector_id TEXT NOT NULL DEFAULT ''
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
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			principal_spiffe TEXT NOT NULL DEFAULT '',
			tunneler_id TEXT NOT NULL DEFAULT '',
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
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
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
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			connector_id TEXT NOT NULL DEFAULT '',
			timestamp TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			log.Printf("schema init error: %v (stmt: %.80s…)", err, s)
			return err
		}
	}
	return nil
}
