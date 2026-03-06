package state

import (
	"sync"
	"time"
)

type ConnectorRecord struct {
	ID        string
	PrivateIP string
	Version   string
	LastSeen  time.Time
}

type Registry struct {
	mu      sync.RWMutex
	records map[string]ConnectorRecord
}

func NewRegistry() *Registry {
	return &Registry{records: make(map[string]ConnectorRecord)}
}

func (r *Registry) Register(id, privateIP, version string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[id] = ConnectorRecord{
		ID:        id,
		PrivateIP: privateIP,
		Version:   version,
		LastSeen:  time.Now().UTC(),
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

func (r *Registry) Delete(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, id)
}

func (r *Registry) RecordHeartbeat(connectorID, privateIP string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.records[connectorID]
	if !ok {
		rec = ConnectorRecord{ID: connectorID}
	}
	rec.PrivateIP = privateIP
	rec.LastSeen = time.Now().UTC()
	r.records[connectorID] = rec
}
