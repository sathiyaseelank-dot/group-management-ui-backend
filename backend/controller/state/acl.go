package state

import (
	"database/sql"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

type ResourceType string

const (
	ResourceCIDR     ResourceType = "cidr"
	ResourceDNS      ResourceType = "dns"
	ResourceInternet ResourceType = "internet"
)

type Resource struct {
	ID              string       `json:"id"`
	Type            ResourceType `json:"type"`
	Address         string       `json:"address"`
	RemoteNetworkID string       `json:"remote_network_id,omitempty"`
	UserGroupIDs    []string     `json:"user_group_ids,omitempty"`
}

type Filter struct {
	Protocol       string `json:"protocol"`
	PortRangeStart uint16 `json:"port_range_start,omitempty"`
	PortRangeEnd   uint16 `json:"port_range_end,omitempty"`
}

type Authorization struct {
	PrincipalSPIFFE string     `json:"principal_spiffe"`
	ResourceID      string     `json:"resource_id"`
	Filters         []Filter   `json:"filters,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	Description     string     `json:"description,omitempty"`
}

type ACLState struct {
	Resources      []Resource      `json:"resources"`
	Authorizations []Authorization `json:"authorizations"`
}

// ACLStore keeps resources and authorizations in memory.
type ACLStore struct {
	mu             sync.RWMutex
	resources      map[string]Resource
	authorizations map[string]Authorization
	db             *sql.DB
}

func NewACLStore() *ACLStore {
	return &ACLStore{
		resources:      make(map[string]Resource),
		authorizations: make(map[string]Authorization),
	}
}

func NewACLStoreWithDB(db *sql.DB) *ACLStore {
	store := NewACLStore()
	store.db = db
	return store
}

func (s *ACLStore) DB() *sql.DB {
	return s.db
}

func (s *ACLStore) Snapshot() ACLState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resources := make([]Resource, 0, len(s.resources))
	for _, r := range s.resources {
		resources = append(resources, r)
	}
	authorizations := make([]Authorization, 0, len(s.authorizations))
	for _, a := range s.authorizations {
		authorizations = append(authorizations, a)
	}
	return ACLState{Resources: resources, Authorizations: authorizations}
}

func (s *ACLStore) UpsertResource(r Resource) error {
	if r.ID == "" {
		return errors.New("resource id required")
	}
	if !validResourceType(r.Type) {
		return errors.New("invalid resource type")
	}
	if r.Type != ResourceInternet && r.Address == "" {
		return errors.New("resource address required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[r.ID] = r
	return nil
}

func (s *ACLStore) DeleteResource(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.resources, id)
	for k, a := range s.authorizations {
		if a.ResourceID == id {
			delete(s.authorizations, k)
		}
	}
}

func (s *ACLStore) UpdateFilters(resourceID string, filters []Filter) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, a := range s.authorizations {
		if a.ResourceID == resourceID {
			a.Filters = filters
			s.authorizations[key] = a
		}
	}
	return nil
}

func (s *ACLStore) AssignPrincipal(resourceID, principalSPIFFE string, filters []Filter) error {
	if resourceID == "" || principalSPIFFE == "" {
		return errors.New("resource_id and principal_spiffe required")
	}
	key := principalSPIFFE + "|" + resourceID
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authorizations[key] = Authorization{
		PrincipalSPIFFE: principalSPIFFE,
		ResourceID:      resourceID,
		Filters:         filters,
	}
	return nil
}

func (s *ACLStore) RemoveAssignment(resourceID, principalSPIFFE string) {
	key := principalSPIFFE + "|" + resourceID
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.authorizations, key)
}

func (s *ACLStore) SetAuthorizationMeta(principalSPIFFE, resourceID string, expiresAt *time.Time, description string) {
	key := principalSPIFFE + "|" + resourceID
	s.mu.Lock()
	defer s.mu.Unlock()
	auth, ok := s.authorizations[key]
	if !ok {
		return
	}
	auth.ExpiresAt = expiresAt
	auth.Description = description
	s.authorizations[key] = auth
}

func validResourceType(t ResourceType) bool {
	switch t {
	case ResourceCIDR, ResourceDNS, ResourceInternet:
		return true
	default:
		return false
	}
}

// MatchResource checks whether destination matches a resource.
func MatchResource(r Resource, dest string) bool {
	switch r.Type {
	case ResourceInternet:
		return true
	case ResourceDNS:
		return strings.EqualFold(r.Address, dest)
	case ResourceCIDR:
		_, cidr, err := net.ParseCIDR(r.Address)
		if err != nil {
			return false
		}
		ip := net.ParseIP(dest)
		if ip == nil {
			return false
		}
		return cidr.Contains(ip)
	default:
		return false
	}
}
