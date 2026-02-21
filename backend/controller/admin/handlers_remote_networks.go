package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"controller/state"
)

func (s *Server) handleRemoteNetworks(w http.ResponseWriter, r *http.Request) {
	if s.RemoteNet == nil {
		http.Error(w, "remote networks not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		nets, err := s.RemoteNet.ListNetworks()
		if err != nil {
			http.Error(w, "failed to list remote networks", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, nets)
	case http.MethodPost:
		var req struct {
			Name string            `json:"name"`
			Tags map[string]string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		n := state.RemoteNetwork{
			Name:      req.Name,
			Tags:      req.Tags,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := s.RemoteNet.CreateNetwork(&n); err != nil {
			http.Error(w, fmt.Sprintf("failed to create network: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, n)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRemoteNetworkConnectors(w http.ResponseWriter, r *http.Request) {
	if s.RemoteNet == nil {
		http.Error(w, "remote networks not configured", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/remote-networks/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		http.Error(w, "network id required", http.StatusBadRequest)
		return
	}
	networkID := parts[0]
	if len(parts) < 2 || parts[1] != "connectors" {
		http.Error(w, "unknown subresource", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		ids, err := s.RemoteNet.ListNetworkConnectors(networkID)
		if err != nil {
			http.Error(w, "failed to list connectors", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, ids)
	case http.MethodPost:
		var req struct {
			ConnectorID string `json:"connector_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.ConnectorID == "" {
			http.Error(w, "connector_id required", http.StatusBadRequest)
			return
		}
		if err := s.RemoteNet.AssignConnector(networkID, req.ConnectorID); err != nil {
			http.Error(w, fmt.Sprintf("failed to assign connector: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case http.MethodDelete:
		var req struct {
			ConnectorID string `json:"connector_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.ConnectorID == "" {
			http.Error(w, "connector_id required", http.StatusBadRequest)
			return
		}
		if err := s.RemoteNet.RemoveConnector(networkID, req.ConnectorID); err != nil {
			http.Error(w, fmt.Sprintf("failed to remove connector: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
