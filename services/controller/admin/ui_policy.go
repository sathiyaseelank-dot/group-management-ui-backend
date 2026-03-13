package admin

import (
	"net/http"
	"strings"
)

func (s *Server) handleUIPolicyCompile(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	connectorID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/policy/compile/"), "/")
	if connectorID == "" {
		http.Error(w, "connector id required", http.StatusBadRequest)
		return
	}
	remoteNetworkID, err := lookupConnectorNetworkID(db, connectorID)
	if err != nil {
		http.Error(w, "connector not found or not assigned to a remote network", http.StatusNotFound)
		return
	}
	resources, err := policyResources(db, remoteNetworkID)
	if err != nil {
		http.Error(w, "failed to compile policy", http.StatusInternalServerError)
		return
	}
	payloadHash := policyHash(resources)
	now := isoStringNow()
	version := policyVersion(db, connectorID, payloadHash, now)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"snapshot_meta": map[string]interface{}{
			"connector_id":   connectorID,
			"policy_version": version,
			"compiled_at":    now,
			"policy_hash":    payloadHash,
		},
		"resources": resources,
	})
}

func (s *Server) handleUIPolicyACL(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	connectorID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/policy/acl/"), "/")
	if connectorID == "" {
		http.Error(w, "connector id required", http.StatusBadRequest)
		return
	}
	remoteNetworkID, err := lookupConnectorNetworkID(db, connectorID)
	if err != nil {
		http.Error(w, "connector not found or not assigned to a remote network", http.StatusNotFound)
		return
	}
	resources, err := policyResources(db, remoteNetworkID)
	if err != nil {
		http.Error(w, "failed to build acl", http.StatusInternalServerError)
		return
	}
	resourceIndex := map[string]map[string]interface{}{}
	aclEntries := []map[string]string{}
	for _, res := range resources {
		resourceIndex[res.ResourceID] = map[string]interface{}{
			"address":   res.Address,
			"protocol":  res.Protocol,
			"port_from": res.PortFrom,
			"port_to":   res.PortTo,
		}
		for _, identity := range res.AllowedIdentities {
			aclEntries = append(aclEntries, map[string]string{
				"identity":    identity,
				"resource_id": res.ResourceID,
			})
		}
	}
	payloadHash := policyHash(resources)
	now := isoStringNow()
	version := policyVersion(db, connectorID, payloadHash, now)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"snapshot_meta": map[string]interface{}{
			"connector_id":   connectorID,
			"policy_version": version,
			"compiled_at":    now,
			"policy_hash":    payloadHash,
		},
		"acl_entries":    aclEntries,
		"resource_index": resourceIndex,
	})
}
