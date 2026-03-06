package state

import (
	"sync"
	"time"
)

type TunnelerInfo struct {
	ID       string `json:"id"`
	SPIFFEID string `json:"spiffe_id"`
}

type TunnelerRegistry struct {
	mu    sync.RWMutex
	items []TunnelerInfo
	seen  map[string]struct{}
}

func NewTunnelerRegistry() *TunnelerRegistry {
	return &TunnelerRegistry{
		seen: make(map[string]struct{}),
	}
}

func (r *TunnelerRegistry) Add(tunnelerID, spiffeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.seen[tunnelerID]; ok {
		return
	}
	r.seen[tunnelerID] = struct{}{}
	r.items = append(r.items, TunnelerInfo{ID: tunnelerID, SPIFFEID: spiffeID})
}

func (r *TunnelerRegistry) List() []TunnelerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TunnelerInfo, len(r.items))
	copy(out, r.items)
	return out
}

type TunnelerStatusRecord struct {
	ID          string
	SPIFFEID    string
	ConnectorID string
	LastSeen    time.Time
}

type TunnelerStatusRegistry struct {
	mu      sync.RWMutex
	records map[string]TunnelerStatusRecord
}

func NewTunnelerStatusRegistry() *TunnelerStatusRegistry {
	return &TunnelerStatusRegistry{records: make(map[string]TunnelerStatusRecord)}
}

func (r *TunnelerStatusRegistry) Record(tunnelerID, spiffeID, connectorID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[tunnelerID] = TunnelerStatusRecord{
		ID:          tunnelerID,
		SPIFFEID:    spiffeID,
		ConnectorID: connectorID,
		LastSeen:    time.Now().UTC(),
	}
}

func (r *TunnelerStatusRegistry) Get(tunnelerID string) (TunnelerStatusRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.records[tunnelerID]
	return rec, ok
}

func (r *TunnelerStatusRegistry) List() []TunnelerStatusRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TunnelerStatusRecord, 0, len(r.records))
	for _, rec := range r.records {
		out = append(out, rec)
	}
	return out
}
