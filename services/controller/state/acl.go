package state

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
)

type Filter struct {
	Protocol string `json:"protocol,omitempty"`
	PortFrom int    `json:"port_from,omitempty"`
	PortTo   int    `json:"port_to,omitempty"`
}

type Resource struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	Type            string `json:"type,omitempty"`
	Address         string `json:"address"`
	Protocol        string `json:"protocol,omitempty"`
	PortFrom        *int   `json:"port_from,omitempty"`
	PortTo          *int   `json:"port_to,omitempty"`
	ConnectorID     string `json:"connector_id,omitempty"`
	RemoteNetworkID string `json:"remote_network_id,omitempty"`
}

type Authorization struct {
	PrincipalSPIFFE string   `json:"principal_spiffe"`
	ResourceID      string   `json:"resource_id"`
	Filters         []Filter `json:"filters,omitempty"`
}

type ACLSnapshot struct {
	Resources      []Resource      `json:"resources"`
	Authorizations []Authorization `json:"authorizations"`
}

type ACLStore struct {
	mu             sync.RWMutex
	db             *sql.DB
	resources      map[string]Resource
	authorizations []Authorization
}

func NewACLStoreWithDB(db *sql.DB) *ACLStore {
	return &ACLStore{
		db:        db,
		resources: make(map[string]Resource),
	}
}

func (s *ACLStore) DB() *sql.DB {
	return s.db
}

func (s *ACLStore) Snapshot() ACLSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resources := make([]Resource, 0, len(s.resources))
	for _, r := range s.resources {
		resources = append(resources, r)
	}
	auths := make([]Authorization, len(s.authorizations))
	copy(auths, s.authorizations)
	return ACLSnapshot{Resources: resources, Authorizations: auths}
}

func (s *ACLStore) UpsertResource(res Resource) error {
	if res.ID == "" {
		return fmt.Errorf("resource id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[res.ID] = res
	return nil
}

func (s *ACLStore) DeleteResource(resourceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.resources, resourceID)
	filtered := s.authorizations[:0]
	for _, a := range s.authorizations {
		if a.ResourceID != resourceID {
			filtered = append(filtered, a)
		}
	}
	s.authorizations = filtered
}

func (s *ACLStore) UpdateFilters(resourceID string, filters []Filter) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.authorizations {
		if s.authorizations[i].ResourceID == resourceID {
			s.authorizations[i].Filters = filters
		}
	}
	return nil
}

func (s *ACLStore) AssignPrincipal(resourceID, principalSPIFFE string, filters []Filter) error {
	if resourceID == "" || principalSPIFFE == "" {
		return fmt.Errorf("resource_id and principal_spiffe required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.authorizations {
		if a.ResourceID == resourceID && a.PrincipalSPIFFE == principalSPIFFE {
			s.authorizations[i].Filters = filters
			return nil
		}
	}
	s.authorizations = append(s.authorizations, Authorization{
		PrincipalSPIFFE: principalSPIFFE,
		ResourceID:      resourceID,
		Filters:         filters,
	})
	return nil
}

func (s *ACLStore) RemoveAssignment(resourceID, principal string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.authorizations[:0]
	for _, a := range s.authorizations {
		if !(a.ResourceID == resourceID && a.PrincipalSPIFFE == principal) {
			filtered = append(filtered, a)
		}
	}
	s.authorizations = filtered
}

func (s *ACLStore) AddAuthorization(auth Authorization) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authorizations = append(s.authorizations, auth)
}

func (s *ACLStore) AddResource(res Resource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[res.ID] = res
}

// AuthorizationResult holds the result of an access check.
type AuthorizationResult struct {
	Allowed bool
	Filters []Filter
	Reason  string
	RuleID  string // resource_id + principal pair identifier
}

// CheckAuthorization verifies if a principal is authorized to access a resource.
// Returns (allowed, ruleID, decision, reason) where:
//   - allowed: true if explicitly authorized, false otherwise (default-deny)
//   - ruleID: the ID of the matching rule (empty if no rule matched)
//   - decision: 'allowed' or 'denied'
//   - reason: explanation of the decision
//
// This implements default-deny: if no matching authorization is found,
// access is denied.
func (s *ACLStore) CheckAuthorization(principalSPIFFE, resourceID string) (bool, string, string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if resource exists
	if _, exists := s.resources[resourceID]; !exists {
		return false, "", "denied", "resource not found"
	}

	// Look for explicit authorization
	for _, auth := range s.authorizations {
		if auth.ResourceID == resourceID && auth.PrincipalSPIFFE == principalSPIFFE {
			ruleID := fmt.Sprintf("%s:%s", auth.ResourceID, auth.PrincipalSPIFFE)
			return true, ruleID, "allowed", "explicitly authorized"
		}
	}

	// Default-deny: no matching authorization found
	return false, "", "denied", "no authorization found (default-deny)"
}

// CheckAuthorizationDetailed verifies if a principal is authorized to access a resource
// and returns a detailed AuthorizationResult including filters and rule ID.
func (s *ACLStore) CheckAuthorizationDetailed(principalSPIFFE, resourceID string) AuthorizationResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.resources[resourceID]; !exists {
		return AuthorizationResult{Allowed: false, Reason: "resource not found"}
	}

	for _, auth := range s.authorizations {
		if auth.ResourceID == resourceID && auth.PrincipalSPIFFE == principalSPIFFE {
			return AuthorizationResult{
				Allowed: true,
				Filters: auth.Filters,
				Reason:  "explicitly authorized",
				RuleID:  fmt.Sprintf("%s:%s", auth.ResourceID, auth.PrincipalSPIFFE),
			}
		}
	}

	return AuthorizationResult{Allowed: false, Reason: "no authorization found (default-deny)"}
}

// marshalFilters encodes filters to JSON for DB storage.
func marshalFilters(filters []Filter) string {
	if len(filters) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(filters)
	return string(data)
}

// unmarshalFilters decodes filters from JSON DB storage.
func unmarshalFilters(raw string) []Filter {
	var filters []Filter
	_ = json.Unmarshal([]byte(raw), &filters)
	return filters
}
