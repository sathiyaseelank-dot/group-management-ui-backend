package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const DefaultDBPath = "/home/inkyank-01/Desktop/tls-mtls/grpccontroller/controller.db"

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
			private_ip TEXT,
			version TEXT,
			last_seen INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS tunnelers (
			id TEXT PRIMARY KEY,
			spiffe_id TEXT,
			connector_id TEXT,
			last_seen INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS resources (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			address TEXT,
			remote_network_id TEXT,
			user_group_ids_json TEXT
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
	return nil
}

func PruneAuditLogs(db *sql.DB, olderThan time.Time) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, olderThan.Unix())
	return err
}
