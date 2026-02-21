package state

import (
	"sort"
	"sync"
	"time"
)

type TunnelerRecord struct {
	ID          string
	SPIFFEID    string
	ConnectorID string
	LastSeen    time.Time
}

type TunnelerStatusRegistry struct {
	mu        sync.RWMutex
	tunnelers map[string]*TunnelerRecord
}

func NewTunnelerStatusRegistry() *TunnelerStatusRegistry {
	return &TunnelerStatusRegistry{
		tunnelers: make(map[string]*TunnelerRecord),
	}
}

func (r *TunnelerStatusRegistry) Record(id, spiffeID, connectorID string) {
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.tunnelers[id]
	if !ok {
		rec = &TunnelerRecord{ID: id}
		r.tunnelers[id] = rec
	}
	if spiffeID != "" {
		rec.SPIFFEID = spiffeID
	}
	if connectorID != "" {
		rec.ConnectorID = connectorID
	}
	rec.LastSeen = time.Now().UTC()
}

func (r *TunnelerStatusRegistry) setLastSeen(id string, t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.tunnelers[id]
	if !ok {
		rec = &TunnelerRecord{ID: id}
		r.tunnelers[id] = rec
	}
	rec.LastSeen = t.UTC()
}

func (r *TunnelerStatusRegistry) Get(id string) (TunnelerRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.tunnelers[id]
	if !ok {
		return TunnelerRecord{}, false
	}
	return *rec, true
}

func (r *TunnelerStatusRegistry) List() []TunnelerRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TunnelerRecord, 0, len(r.tunnelers))
	for _, rec := range r.tunnelers {
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out
}
