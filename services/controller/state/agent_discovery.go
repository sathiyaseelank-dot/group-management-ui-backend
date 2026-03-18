package state

import (
	"database/sql"
	"strings"
	"time"
)

// AgentDiscoveredService represents a service discovered on an agent's LAN.
type AgentDiscoveredService struct {
	ID          int64  `json:"id"`
	AgentID     string `json:"agent_id"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	BoundIP     string `json:"bound_ip"`
	ServiceName string `json:"service_name"`
	ProcessName string `json:"process_name"`
	FirstSeen   int64  `json:"first_seen"`
	LastSeen    int64  `json:"last_seen"`
	WorkspaceID string `json:"workspace_id"`
	Dismissed   int    `json:"dismissed"`
	Status      string `json:"status"`
}

const selectCols = `id, agent_id, port, protocol, bound_ip, service_name, process_name, first_seen, last_seen, workspace_id, dismissed, status`

// UpsertAgentDiscoveredService inserts or updates a discovered service.
// Also resets status to 'active' when the service is seen again.
func UpsertAgentDiscoveredService(db *sql.DB, svc AgentDiscoveredService) error {
	now := time.Now().UTC().Unix()
	_, err := db.Exec(
		Rebind(`INSERT INTO agent_discovered_services
			(agent_id, port, protocol, bound_ip, service_name, process_name, first_seen, last_seen, workspace_id, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'active')
			ON CONFLICT(agent_id, port, protocol) DO UPDATE SET
				bound_ip = excluded.bound_ip,
				service_name = CASE WHEN excluded.service_name != '' THEN excluded.service_name ELSE agent_discovered_services.service_name END,
				process_name = CASE WHEN excluded.process_name != '' THEN excluded.process_name ELSE agent_discovered_services.process_name END,
				last_seen = excluded.last_seen,
				workspace_id = excluded.workspace_id,
				status = 'active'`),
		svc.AgentID, svc.Port, svc.Protocol, svc.BoundIP, svc.ServiceName, svc.ProcessName,
		now, now, svc.WorkspaceID,
	)
	return err
}

// TouchAgentDiscoveryLastSeen bumps last_seen on all active services for an agent.
// Used by the discovery heartbeat to keep last_seen fresh without full re-reports.
func TouchAgentDiscoveryLastSeen(db *sql.DB, agentID string) error {
	now := time.Now().UTC().Unix()
	_, err := db.Exec(
		Rebind(`UPDATE agent_discovered_services SET last_seen = ? WHERE agent_id = ? AND status = 'active'`),
		now, agentID,
	)
	return err
}

// ReconcileDiscoveredServices marks any active services for an agent as 'gone'
// if they are NOT in the provided set of (port, protocol) tuples. This handles
// the case where an agent reconnects with a fresh session and reports its current
// ports — any previously-active ports not in the report are stale.
func ReconcileDiscoveredServices(db *sql.DB, agentID string, reported []PortProto) (int64, error) {
	if len(reported) == 0 {
		// Agent reports zero services — mark everything gone
		res, err := db.Exec(
			Rebind(`UPDATE agent_discovered_services SET status = 'gone' WHERE agent_id = ? AND status = 'active'`),
			agentID,
		)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}

	// Build exclusion set: keep these active, mark the rest gone
	// Use individual conditions since PostgreSQL doesn't support ROW() IN with placeholders easily
	var count int64
	rows, err := db.Query(
		Rebind(`SELECT port, protocol FROM agent_discovered_services WHERE agent_id = ? AND status = 'active'`),
		agentID,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	reportedSet := make(map[PortProto]struct{}, len(reported))
	for _, r := range reported {
		reportedSet[r] = struct{}{}
	}

	for rows.Next() {
		var port int
		var proto string
		if err := rows.Scan(&port, &proto); err != nil {
			continue
		}
		if _, ok := reportedSet[PortProto{Port: port, Protocol: proto}]; !ok {
			_, err := db.Exec(
				Rebind(`UPDATE agent_discovered_services SET status = 'gone' WHERE agent_id = ? AND port = ? AND protocol = ? AND status = 'active'`),
				agentID, port, proto,
			)
			if err != nil {
				return count, err
			}
			count++
		}
	}
	return count, rows.Err()
}

// PortProto is a (port, protocol) tuple used for reconciliation.
type PortProto struct {
	Port     int
	Protocol string
}

// MarkServicesGone sets status='gone' for services that an agent no longer reports.
func MarkServicesGone(db *sql.DB, agentID string, ports []int, protocol string) error {
	if len(ports) == 0 {
		return nil
	}
	for _, port := range ports {
		_, err := db.Exec(
			Rebind(`UPDATE agent_discovered_services SET status = 'gone' WHERE agent_id = ? AND port = ? AND protocol = ? AND status = 'active'`),
			agentID, port, protocol,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// wsFilter appends "AND workspace_id = ?" to a query when workspaceID is non-empty.
// Returns the (possibly extended) query and args slice.
func wsFilter(query string, args []any, workspaceID string) (string, []any) {
	if workspaceID != "" {
		query += " AND workspace_id = ?"
		args = append(args, workspaceID)
	}
	return query, args
}

// ListAgentDiscoveredServices returns non-dismissed services for a specific agent.
func ListAgentDiscoveredServices(db *sql.DB, agentID string, workspaceID string) ([]AgentDiscoveredService, error) {
	q := `SELECT ` + selectCols + ` FROM agent_discovered_services WHERE agent_id = ? AND dismissed = 0`
	args := []any{agentID}
	q, args = wsFilter(q, args, workspaceID)
	q += " ORDER BY last_seen DESC"
	return listAgentServices(db, Rebind(q), args...)
}

// ListAgentDiscoveredServicesAll returns all services (including dismissed) for a specific agent.
func ListAgentDiscoveredServicesAll(db *sql.DB, agentID string, workspaceID string) ([]AgentDiscoveredService, error) {
	q := `SELECT ` + selectCols + ` FROM agent_discovered_services WHERE agent_id = ?`
	args := []any{agentID}
	q, args = wsFilter(q, args, workspaceID)
	q += " ORDER BY last_seen DESC"
	return listAgentServices(db, Rebind(q), args...)
}

// ListAllAgentDiscoveredServices returns all non-dismissed discovered services.
func ListAllAgentDiscoveredServices(db *sql.DB, workspaceID string) ([]AgentDiscoveredService, error) {
	q := `SELECT ` + selectCols + ` FROM agent_discovered_services WHERE dismissed = 0`
	args := []any{}
	q, args = wsFilter(q, args, workspaceID)
	q += " ORDER BY last_seen DESC"
	rows, err := db.Query(Rebind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentDiscoveredServices(rows)
}

// ListAllAgentDiscoveredServicesIncludingDismissed returns all discovered services.
func ListAllAgentDiscoveredServicesIncludingDismissed(db *sql.DB, workspaceID string) ([]AgentDiscoveredService, error) {
	q := `SELECT ` + selectCols + ` FROM agent_discovered_services WHERE 1=1`
	args := []any{}
	q, args = wsFilter(q, args, workspaceID)
	q += " ORDER BY last_seen DESC"
	rows, err := db.Query(Rebind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentDiscoveredServices(rows)
}

// DiscoverySummary holds aggregate stats for the dashboard widget.
type DiscoverySummary struct {
	Total     int `json:"total"`
	New24h    int `json:"new_24h"`
	Unmanaged int `json:"unmanaged"`
	Gone      int `json:"gone"`
	Stale     int `json:"stale"`
}

// unmanagedQuery builds the "unmanaged" count query, scoping the resources
// subquery by workspace when workspaceID is non-empty so that cross-tenant
// resources don't incorrectly mark services as managed.
func unmanagedQuery(workspaceID string) string {
	base := `SELECT COUNT(*) FROM agent_discovered_services ds WHERE ds.dismissed = 0 AND ds.status = 'active' AND NOT EXISTS (SELECT 1 FROM resources r WHERE r.port_from = ds.port AND LOWER(r.protocol) = LOWER(ds.protocol)`
	if workspaceID != "" {
		base += " AND r.workspace_id = ds.workspace_id"
	}
	base += ")"
	return base
}

// GetDiscoverySummary returns aggregate discovery stats.
func GetDiscoverySummary(db *sql.DB, workspaceID string) (DiscoverySummary, error) {
	var s DiscoverySummary
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Unix()

	type countQuery struct {
		dest  *int
		query string
		args  []any
	}

	queries := []countQuery{
		{&s.Total, `SELECT COUNT(*) FROM agent_discovered_services WHERE dismissed = 0 AND status = 'active'`, nil},
		{&s.New24h, `SELECT COUNT(*) FROM agent_discovered_services WHERE dismissed = 0 AND status = 'active' AND first_seen > ?`, []any{cutoff}},
		{&s.Unmanaged, unmanagedQuery(workspaceID), nil},
		{&s.Gone, `SELECT COUNT(*) FROM agent_discovered_services WHERE dismissed = 0 AND status = 'gone'`, nil},
		{&s.Stale, `SELECT COUNT(*) FROM agent_discovered_services WHERE dismissed = 0 AND status = 'stale'`, nil},
	}

	for _, cq := range queries {
		q := cq.query
		args := cq.args
		if workspaceID != "" {
			// For the unmanaged query, scope the workspace on the ds alias
			if strings.Contains(q, " ds ") {
				q += " AND ds.workspace_id = ?"
			} else {
				q += " AND workspace_id = ?"
			}
			args = append(args, workspaceID)
		}
		if err := db.QueryRow(Rebind(q), args...).Scan(cq.dest); err != nil {
			return s, err
		}
	}

	return s, nil
}

// DismissService sets dismissed=1 for a service by ID.
func DismissService(db *sql.DB, id int64, workspaceID string) error {
	q := `UPDATE agent_discovered_services SET dismissed = 1 WHERE id = ?`
	args := []any{id}
	q, args = wsFilter(q, args, workspaceID)
	_, err := db.Exec(Rebind(q), args...)
	return err
}

// UndismissService sets dismissed=0 for a service by ID.
func UndismissService(db *sql.DB, id int64, workspaceID string) error {
	q := `UPDATE agent_discovered_services SET dismissed = 0 WHERE id = ?`
	args := []any{id}
	q, args = wsFilter(q, args, workspaceID)
	_, err := db.Exec(Rebind(q), args...)
	return err
}

func listAgentServices(db *sql.DB, query string, args ...any) ([]AgentDiscoveredService, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentDiscoveredServices(rows)
}

// PruneDiscoveredServices deletes gone rows with last_seen before the given cutoff.
// Only prunes services marked as 'gone' — active services from offline agents are preserved.
func PruneDiscoveredServices(db *sql.DB, before time.Time) (int64, error) {
	res, err := db.Exec(
		Rebind(`DELETE FROM agent_discovered_services WHERE status = 'gone' AND last_seen < ?`),
		before.UTC().Unix(),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PurgeDiscoveredServices deletes all rows, optionally filtered by agent_id and workspace_id.
func PurgeDiscoveredServices(db *sql.DB, agentID string, workspaceID string) (int64, error) {
	q := `DELETE FROM agent_discovered_services WHERE 1=1`
	var args []any
	if agentID != "" {
		q += " AND agent_id = ?"
		args = append(args, agentID)
	}
	if workspaceID != "" {
		q += " AND workspace_id = ?"
		args = append(args, workspaceID)
	}
	res, err := db.Exec(Rebind(q), args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// MarkStaleDiscoveredServices marks services as 'stale' when the owning agent
// is offline and the service last_seen exceeds the threshold.
func MarkStaleDiscoveredServices(db *sql.DB, threshold time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-threshold).Unix()
	res, err := db.Exec(
		Rebind(`UPDATE agent_discovered_services SET status = 'stale'
			WHERE status = 'active' AND last_seen < ?
			AND agent_id IN (SELECT id FROM agents WHERE status = 'offline')`),
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ReactivateStaleServices flips stale services back to active when an agent reconnects.
func ReactivateStaleServices(db *sql.DB, agentID string) (int64, error) {
	res, err := db.Exec(
		Rebind(`UPDATE agent_discovered_services SET status = 'active' WHERE agent_id = ? AND status = 'stale'`),
		agentID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanAgentDiscoveredServices(rows *sql.Rows) ([]AgentDiscoveredService, error) {
	var results []AgentDiscoveredService
	for rows.Next() {
		var s AgentDiscoveredService
		if err := rows.Scan(&s.ID, &s.AgentID, &s.Port, &s.Protocol, &s.BoundIP,
			&s.ServiceName, &s.ProcessName,
			&s.FirstSeen, &s.LastSeen, &s.WorkspaceID, &s.Dismissed, &s.Status); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

