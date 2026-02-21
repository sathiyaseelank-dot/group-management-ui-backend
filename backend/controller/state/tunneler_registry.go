package state

import "sync"

type TunnelerInfo struct {
	ID       string `json:"tunneler_id"`
	SPIFFEID string `json:"spiffe_id"`
}

// TunnelerRegistry keeps an in-memory record of allowed tunnelers.
type TunnelerRegistry struct {
	mu    sync.RWMutex
	byID  map[string]TunnelerInfo
	order []string
}

func NewTunnelerRegistry() *TunnelerRegistry {
	return &TunnelerRegistry{
		byID: make(map[string]TunnelerInfo),
	}
}

func (r *TunnelerRegistry) Add(id, spiffeID string) {
	if id == "" || spiffeID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byID[id]; !exists {
		r.order = append(r.order, id)
	}
	r.byID[id] = TunnelerInfo{ID: id, SPIFFEID: spiffeID}
}

func (r *TunnelerRegistry) List() []TunnelerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TunnelerInfo, 0, len(r.byID))
	for _, id := range r.order {
		if info, ok := r.byID[id]; ok {
			out = append(out, info)
		}
	}
	return out
}
