package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const DefaultDBPath = "ztna.db"

func OpenSQLite(path string) (*sql.DB, error) {
	if path == "" {
		path = DefaultDBPath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	// modernc.org/sqlite expects a file path.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := migrateSQLite(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func migrateSQLite(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tokens (
			hash TEXT PRIMARY KEY,
			expires_at INTEGER NOT NULL,
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
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS user_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS user_group_members (
			user_id TEXT NOT NULL,
			group_id TEXT NOT NULL,
			added_at INTEGER NOT NULL,
			PRIMARY KEY (user_id, group_id)
		);`,
		`CREATE TABLE IF NOT EXISTS remote_networks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			location TEXT,
			tags_json TEXT,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS connector_remote_networks (
			connector_id TEXT NOT NULL,
			remote_network_id TEXT NOT NULL,
			assigned_at INTEGER NOT NULL,
			PRIMARY KEY (connector_id, remote_network_id)
		);`,
		`CREATE TABLE IF NOT EXISTS connectors (
			id TEXT PRIMARY KEY,
			name TEXT,
			status TEXT,
			hostname TEXT,
			private_ip TEXT,
			version TEXT,
			last_seen INTEGER NOT NULL,
			remote_network_id TEXT,
			installed INTEGER NOT NULL DEFAULT 0,
			last_policy_version INTEGER NOT NULL DEFAULT 0,
			last_seen_at TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS tunnelers (
			id TEXT PRIMARY KEY,
			spiffe_id TEXT,
			connector_id TEXT,
			last_seen INTEGER NOT NULL,
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
			created_at INTEGER NOT NULL
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
			id INTEGER PRIMARY KEY AUTOINCREMENT,
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
			expires_at INTEGER,
			description TEXT,
			PRIMARY KEY (principal_spiffe, resource_id)
		);`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			principal_spiffe TEXT,
			tunneler_id TEXT,
			resource_id TEXT,
			destination TEXT,
			protocol TEXT,
			port INTEGER,
			decision TEXT,
			reason TEXT,
			connection_id TEXT,
			created_at INTEGER NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("sqlite migrate failed: %w", err)
		}
	}
	if err := ensureColumn(db, "users", "certificate_identity", "TEXT"); err != nil {
		return err
	}
	if err := ensureUniqueIndex(db, "idx_users_certificate_identity", "users(certificate_identity)"); err != nil {
		return err
	}
	if err := ensureColumn(db, "remote_networks", "location", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "remote_networks", "tags_json", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "remote_networks", "created_at", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "remote_networks", "updated_at", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "connectors", "name", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "connectors", "status", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "connectors", "hostname", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "connectors", "remote_network_id", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "connectors", "installed", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "connectors", "last_policy_version", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "connectors", "last_seen_at", "TEXT"); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE connectors SET last_seen_at = last_seen WHERE last_seen_at IS NULL`); err != nil {
		return err
	}
	if err := ensureColumn(db, "tunnelers", "name", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "tunnelers", "status", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "tunnelers", "version", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "tunnelers", "hostname", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "tunnelers", "remote_network_id", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "resources", "name", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "resources", "ports", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "resources", "protocol", "TEXT NOT NULL DEFAULT 'TCP'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "resources", "port_from", "INTEGER"); err != nil {
		return err
	}
	if err := ensureColumn(db, "resources", "port_to", "INTEGER"); err != nil {
		return err
	}
	if err := ensureColumn(db, "resources", "alias", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "resources", "description", "TEXT"); err != nil {
		return err
	}
	return nil
}

func ensureColumn(db *sql.DB, table, column, definition string) error {
	if db == nil {
		return nil
	}
	exists, err := columnExists(db, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	if err != nil {
		return fmt.Errorf("sqlite alter table %s add column %s failed: %w", table, column, err)
	}
	return nil
}

func ensureUniqueIndex(db *sql.DB, name, on string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s", name, on))
	if err != nil {
		return fmt.Errorf("sqlite create index %s failed: %w", name, err)
	}
	return nil
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, nil
}

func PruneAuditLogs(db *sql.DB, olderThan time.Time) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, olderThan.Unix())
	return err
}
