package state

import (
	"sync"
	"time"
)

// ScanStatus represents the current state of a scan job.
type ScanStatus string

const (
	ScanStatusPending    ScanStatus = "pending"
	ScanStatusInProgress ScanStatus = "in_progress"
	ScanStatusCompleted  ScanStatus = "completed"
	ScanStatusFailed     ScanStatus = "failed"
)

// DiscoveredResource represents a network resource found during scanning.
type DiscoveredResource struct {
	ID           string `json:"id"`
	IP           string `json:"ip"`
	Port         uint16 `json:"port"`
	Protocol     string `json:"protocol"`
	ServiceName  string `json:"service_name"`
	ReachableFrom string `json:"reachable_from"`
	FirstSeen    int64  `json:"first_seen"`
}

// ScanJob tracks a single discovery scan request.
type ScanJob struct {
	RequestID   string               `json:"request_id"`
	ConnectorID string               `json:"connector_id"`
	Status      ScanStatus           `json:"status"`
	Targets     []string             `json:"targets"`
	Ports       []uint16             `json:"ports"`
	StartedAt   time.Time            `json:"started_at"`
	CompletedAt *time.Time           `json:"completed_at,omitempty"`
	Results     []DiscoveredResource `json:"results,omitempty"`
	Error       string               `json:"error,omitempty"`
}

// ScanStore is a thread-safe in-memory store for scan jobs.
type ScanStore struct {
	mu   sync.RWMutex
	jobs map[string]*ScanJob
}

// NewScanStore creates a new empty ScanStore.
func NewScanStore() *ScanStore {
	return &ScanStore{
		jobs: make(map[string]*ScanJob),
	}
}

// Create adds a new scan job in pending state.
func (s *ScanStore) Create(job *ScanJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.Status = ScanStatusPending
	job.StartedAt = time.Now().UTC()
	s.jobs[job.RequestID] = job
}

// Get returns a scan job by request ID.
func (s *ScanStore) Get(requestID string) (*ScanJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[requestID]
	return job, ok
}

// MarkInProgress marks a scan job as in-progress.
func (s *ScanStore) MarkInProgress(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[requestID]; ok {
		job.Status = ScanStatusInProgress
	}
}

// Complete marks a scan job as completed with results.
func (s *ScanStore) Complete(requestID string, results []DiscoveredResource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[requestID]; ok {
		now := time.Now().UTC()
		job.Status = ScanStatusCompleted
		job.CompletedAt = &now
		job.Results = results
	}
}

// Fail marks a scan job as failed with an error message.
func (s *ScanStore) Fail(requestID string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[requestID]; ok {
		now := time.Now().UTC()
		job.Status = ScanStatusFailed
		job.CompletedAt = &now
		job.Error = errMsg
	}
}

// List returns all scan jobs.
func (s *ScanStore) List() []*ScanJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*ScanJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, j)
	}
	return result
}
