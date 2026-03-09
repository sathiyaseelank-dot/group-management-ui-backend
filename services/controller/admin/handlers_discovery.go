package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"controller/api"
	"controller/state"
)

// handleStartScan initiates a network discovery scan on a connector.
// POST /api/admin/discovery/scan
func (s *Server) handleStartScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ConnectorID string   `json:"connector_id"`
		Targets     []string `json:"targets"`
		Ports       []uint16 `json:"ports"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if req.ConnectorID == "" {
		http.Error(w, "connector_id is required", http.StatusBadRequest)
		return
	}
	if len(req.Targets) == 0 {
		http.Error(w, "targets is required", http.StatusBadRequest)
		return
	}
	if len(req.Ports) == 0 {
		http.Error(w, "ports is required", http.StatusBadRequest)
		return
	}
	if len(req.Ports) > 16 {
		http.Error(w, "too many ports (max 16)", http.StatusBadRequest)
		return
	}

	// Generate request ID
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		http.Error(w, "failed to generate request id", http.StatusInternalServerError)
		return
	}
	requestID := hex.EncodeToString(idBytes)

	// Create scan job
	job := &state.ScanJob{
		RequestID:   requestID,
		ConnectorID: req.ConnectorID,
		Targets:     req.Targets,
		Ports:       req.Ports,
	}
	s.ScanStore.Create(job)

	// Send scan command to connector via gRPC stream
	scanCmd := map[string]interface{}{
		"request_id":  requestID,
		"targets":     req.Targets,
		"ports":       req.Ports,
		"max_targets": 512,
		"timeout_sec": 5,
	}
	payload, err := json.Marshal(scanCmd)
	if err != nil {
		s.ScanStore.Fail(requestID, "failed to marshal scan command")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := s.ControlPlane.SendToConnector(req.ConnectorID, "scan_command", payload); err != nil {
		s.ScanStore.Fail(requestID, "connector not connected: "+err.Error())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"request_id": requestID,
			"error":      "connector not connected",
		})
		return
	}

	s.ScanStore.MarkInProgress(requestID)

	writeJSON(w, http.StatusOK, map[string]string{
		"request_id": requestID,
	})
}

// handleScanStatus returns the status and results of a scan job.
// GET /api/admin/discovery/scan/{requestId}
func (s *Server) handleScanStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requestID := strings.TrimPrefix(r.URL.Path, "/api/admin/discovery/scan/")
	if requestID == "" {
		http.Error(w, "request_id is required", http.StatusBadRequest)
		return
	}

	job, ok := s.ScanStore.Get(requestID)
	if !ok {
		http.Error(w, "scan not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, job)
}

// handleDiscoveryResults returns all discovered resources across completed scans.
// GET /api/admin/discovery/results?connector_id=xxx
func (s *Server) handleDiscoveryResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	connectorFilter := r.URL.Query().Get("connector_id")

	jobs := s.ScanStore.List()
	var results []state.DiscoveredResource
	for _, job := range jobs {
		if job.Status != state.ScanStatusCompleted {
			continue
		}
		if connectorFilter != "" && job.ConnectorID != connectorFilter {
			continue
		}
		results = append(results, job.Results...)
	}

	if results == nil {
		results = []state.DiscoveredResource{}
	}

	writeJSON(w, http.StatusOK, results)
}

// DiscoverySender is the interface needed to send messages to connectors.
type DiscoverySender interface {
	SendToConnector(connectorID string, msgType string, payload []byte) error
}

// Ensure ControlPlaneServer implements DiscoverySender at compile time.
var _ DiscoverySender = (*api.ControlPlaneServer)(nil)
