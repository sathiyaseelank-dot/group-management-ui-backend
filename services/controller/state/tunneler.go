package state

import (
	"sync"
	"time"
)

type AgentInfo struct {
	ID       string `json:"agent_id"`
	SPIFFEID string `json:"spiffe_id"`
}

type AgentRegistry struct {
	mu    sync.RWMutex
	items []AgentInfo
	seen  map[string]struct{}
}

func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		seen: make(map[string]struct{}),
	}
}

func (r *AgentRegistry) Add(agentID, spiffeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.seen[agentID]; ok {
		return
	}
	r.seen[agentID] = struct{}{}
	r.items = append(r.items, AgentInfo{ID: agentID, SPIFFEID: spiffeID})
}

func (r *AgentRegistry) List() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]AgentInfo, len(r.items))
	copy(out, r.items)
	return out
}

type AgentStatusRecord struct {
	ID          string
	SPIFFEID    string
	ConnectorID string
	LastSeen    time.Time
	IP          string
}

type AgentStatusRegistry struct {
	mu      sync.RWMutex
	records map[string]AgentStatusRecord
}

func NewAgentStatusRegistry() *AgentStatusRegistry {
	return &AgentStatusRegistry{records: make(map[string]AgentStatusRecord)}
}

func (r *AgentStatusRegistry) Record(agentID, spiffeID, connectorID, ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[agentID] = AgentStatusRecord{
		ID:          agentID,
		SPIFFEID:    spiffeID,
		ConnectorID: connectorID,
		LastSeen:    time.Now().UTC(),
		IP:          ip,
	}
}

func (r *AgentStatusRegistry) Get(agentID string) (AgentStatusRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.records[agentID]
	return rec, ok
}

func (r *AgentStatusRegistry) Delete(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, agentID)
}

func (r *AgentStatusRegistry) List() []AgentStatusRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]AgentStatusRecord, 0, len(r.records))
	for _, rec := range r.records {
		out = append(out, rec)
	}
	return out
}
