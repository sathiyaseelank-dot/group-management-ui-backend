package api

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"controller/state"
)

type goneEntry struct {
	AgentID  string
	Port     int
	Protocol string
}

// DiscoveryBatcher batches discovery diff writes into periodic flushes.
type DiscoveryBatcher struct {
	mu      sync.Mutex
	upserts []state.AgentDiscoveredService
	gones   []goneEntry
	db      *sql.DB
}

// NewDiscoveryBatcher creates a new batcher.
func NewDiscoveryBatcher(db *sql.DB) *DiscoveryBatcher {
	return &DiscoveryBatcher{
		db: db,
	}
}

// QueueUpsert adds a service upsert to the pending batch.
func (b *DiscoveryBatcher) QueueUpsert(svc state.AgentDiscoveredService) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.upserts = append(b.upserts, svc)
}

// QueueGone adds a gone service to the pending batch.
func (b *DiscoveryBatcher) QueueGone(agentID string, port int, protocol string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gones = append(b.gones, goneEntry{AgentID: agentID, Port: port, Protocol: protocol})
}

// Flush writes all pending operations to the database in a single transaction.
func (b *DiscoveryBatcher) Flush() {
	b.mu.Lock()
	upserts := b.upserts
	gones := b.gones
	b.upserts = nil
	b.gones = nil
	b.mu.Unlock()

	if len(upserts) == 0 && len(gones) == 0 {
		return
	}

	tx, err := b.db.Begin()
	if err != nil {
		log.Printf("discovery_batcher: begin tx error: %v", err)
		return
	}

	for _, svc := range upserts {
		now := time.Now().UTC().Unix()
		_, err := tx.Exec(
			state.Rebind(`INSERT INTO agent_discovered_services
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
		if err != nil {
			log.Printf("discovery_batcher: upsert error agent=%s port=%d: %v", svc.AgentID, svc.Port, err)
		}
	}

	for _, g := range gones {
		_, err := tx.Exec(
			state.Rebind(`UPDATE agent_discovered_services SET status = 'gone' WHERE agent_id = ? AND port = ? AND protocol = ? AND status = 'active'`),
			g.AgentID, g.Port, g.Protocol,
		)
		if err != nil {
			log.Printf("discovery_batcher: mark gone error agent=%s port=%d: %v", g.AgentID, g.Port, err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("discovery_batcher: commit error: %v", err)
		return
	}

	log.Printf("discovery_batcher: flushed %d upserts + %d gones", len(upserts), len(gones))
}

// Run starts the periodic flush loop.
func (b *DiscoveryBatcher) Run() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		b.Flush()
	}
}
