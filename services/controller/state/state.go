package state

import (
	"database/sql"
	"fmt"
)

// OpenSQLite is disabled. Use Open() with DATABASE_URL.
func OpenSQLite(path string) (*sql.DB, error) {
	return nil, fmt.Errorf("SQLite is disabled; use Open() with DATABASE_URL")
}
