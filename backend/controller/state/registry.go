package state

import (
	"sort"
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
	mu         sync.RWMutex
	connectors map[string]*ConnectorRecord
}

func NewRegistry() *Registry {
	return &Registry{
		connectors: make(map[string]*ConnectorRecord),
	}
}

func (r *Registry) Register(id, privateIP, version string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.connectors[id]
	if !ok {
		rec = &ConnectorRecord{ID: id}
		r.connectors[id] = rec
	}
	rec.PrivateIP = privateIP
	rec.Version = version
	rec.LastSeen = time.Now().UTC()
}

func (r *Registry) RecordHeartbeat(id, privateIP string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.connectors[id]
	if !ok {
		rec = &ConnectorRecord{ID: id}
		r.connectors[id] = rec
	}
	if privateIP != "" {
		rec.PrivateIP = privateIP
	}
	rec.LastSeen = time.Now().UTC()
}

func (r *Registry) setLastSeen(id string, t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.connectors[id]
	if !ok {
		rec = &ConnectorRecord{ID: id}
		r.connectors[id] = rec
	}
	rec.LastSeen = t.UTC()
}

func (r *Registry) List() []ConnectorRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ConnectorRecord, 0, len(r.connectors))
	for _, rec := range r.connectors {
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out
}

func (r *Registry) Get(id string) (ConnectorRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.connectors[id]
	if !ok {
		return ConnectorRecord{}, false
	}
	return *rec, true
}

func (r *Registry) Delete(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connectors, id)
}
