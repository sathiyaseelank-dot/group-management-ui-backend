package state

import (
	"sync"
	"time"
)

type ConnectorRecord struct {
	ID          string
	PrivateIP   string
	TunnelAddr  string
	Version     string
	LastSeen    time.Time
	WorkspaceID string
}

type Registry struct {
	mu      sync.RWMutex
	records map[string]ConnectorRecord
}

func NewRegistry() *Registry {
	return &Registry{records: make(map[string]ConnectorRecord)}
}

func (r *Registry) Register(id, privateIP, tunnelAddr, version string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing := r.records[id]
	r.records[id] = ConnectorRecord{
		ID:          id,
		PrivateIP:   privateIP,
		TunnelAddr:  tunnelAddr,
		Version:     version,
		LastSeen:    time.Now().UTC(),
		WorkspaceID: existing.WorkspaceID,
	}
}

func (r *Registry) RegisterWithWorkspace(id, privateIP, tunnelAddr, version, workspaceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[id] = ConnectorRecord{
		ID:          id,
		PrivateIP:   privateIP,
		TunnelAddr:  tunnelAddr,
		Version:     version,
		LastSeen:    time.Now().UTC(),
		WorkspaceID: workspaceID,
	}
}

func (r *Registry) Get(id string) (ConnectorRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.records[id]
	return rec, ok
}

func (r *Registry) List() []ConnectorRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ConnectorRecord, 0, len(r.records))
	for _, rec := range r.records {
		out = append(out, rec)
	}
	return out
}

// ListByWorkspace returns only connectors belonging to the given workspace.
func (r *Registry) ListByWorkspace(wsID string) []ConnectorRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ConnectorRecord, 0)
	for _, rec := range r.records {
		if rec.WorkspaceID == wsID {
			out = append(out, rec)
		}
	}
	return out
}

func (r *Registry) Delete(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, id)
}

func (r *Registry) RecordHeartbeat(connectorID, privateIP, tunnelAddr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.records[connectorID]
	if !ok {
		rec = ConnectorRecord{ID: connectorID}
	}
	rec.PrivateIP = privateIP
	rec.TunnelAddr = tunnelAddr
	rec.LastSeen = time.Now().UTC()
	r.records[connectorID] = rec
}
