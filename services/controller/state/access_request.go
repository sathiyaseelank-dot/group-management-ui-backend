package state

import (
	"database/sql"
	"time"
)

type AccessRequest struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspace_id"`
	RequesterID    string `json:"requester_id"`
	RequesterEmail string `json:"requester_email"`
	ResourceID     string `json:"resource_id"`
	Reason         string `json:"reason"`
	Status         string `json:"status"` // pending, approved, rejected
	DurationHours  int    `json:"duration_hours"`
	CreatedAt      int64  `json:"created_at"`
	ExpiresAt      int64  `json:"expires_at"`
	DecidedAt      int64  `json:"decided_at"`
	DecidedBy      string `json:"decided_by"`
	DecisionReason string `json:"decision_reason"`
}

type AccessRequestGrant struct {
	ID          string `json:"id"`
	RequestID   string `json:"request_id"`
	WorkspaceID string `json:"workspace_id"`
	ResourceID  string `json:"resource_id"`
	UserID      string `json:"user_id"`
	GrantedAt   int64  `json:"granted_at"`
	ExpiresAt   int64  `json:"expires_at"`
	Revoked     bool   `json:"revoked"`
}

type AccessRequestStore struct {
	db *sql.DB
}

func NewAccessRequestStore(db *sql.DB) *AccessRequestStore {
	return &AccessRequestStore{db: db}
}

func (s *AccessRequestStore) Create(req *AccessRequest) error {
	_, err := s.db.Exec(
		Rebind(`INSERT INTO access_requests (id, workspace_id, requester_id, requester_email, resource_id, reason, status, duration_hours, created_at, expires_at)
            VALUES (?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?)`),
		req.ID, req.WorkspaceID, req.RequesterID, req.RequesterEmail, req.ResourceID, req.Reason,
		req.DurationHours, req.CreatedAt, req.ExpiresAt,
	)
	return err
}

func (s *AccessRequestStore) Get(id string) (*AccessRequest, error) {
	var req AccessRequest
	err := s.db.QueryRow(
		Rebind(`SELECT id, workspace_id, requester_id, requester_email, resource_id, reason, status, duration_hours, created_at, expires_at, decided_at, decided_by, decision_reason
            FROM access_requests WHERE id = ?`), id,
	).Scan(&req.ID, &req.WorkspaceID, &req.RequesterID, &req.RequesterEmail, &req.ResourceID, &req.Reason,
		&req.Status, &req.DurationHours, &req.CreatedAt, &req.ExpiresAt, &req.DecidedAt, &req.DecidedBy, &req.DecisionReason)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (s *AccessRequestStore) ListForWorkspace(wsID string) ([]AccessRequest, error) {
	rows, err := s.db.Query(
		Rebind(`SELECT id, workspace_id, requester_id, requester_email, resource_id, reason, status, duration_hours, created_at, expires_at, decided_at, decided_by, decision_reason
            FROM access_requests WHERE workspace_id = ? ORDER BY created_at DESC`), wsID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccessRequest
	for rows.Next() {
		var req AccessRequest
		if err := rows.Scan(&req.ID, &req.WorkspaceID, &req.RequesterID, &req.RequesterEmail, &req.ResourceID, &req.Reason,
			&req.Status, &req.DurationHours, &req.CreatedAt, &req.ExpiresAt, &req.DecidedAt, &req.DecidedBy, &req.DecisionReason); err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

func (s *AccessRequestStore) Approve(id, decidedBy, reason string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(
		Rebind(`UPDATE access_requests SET status = 'approved', decided_at = ?, decided_by = ?, decision_reason = ? WHERE id = ? AND status = 'pending'`),
		now, decidedBy, reason, id,
	)
	return err
}

func (s *AccessRequestStore) Reject(id, decidedBy, reason string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(
		Rebind(`UPDATE access_requests SET status = 'rejected', decided_at = ?, decided_by = ?, decision_reason = ? WHERE id = ? AND status = 'pending'`),
		now, decidedBy, reason, id,
	)
	return err
}

func (s *AccessRequestStore) CreateGrant(grant *AccessRequestGrant) error {
	_, err := s.db.Exec(
		Rebind(`INSERT INTO access_request_grants (id, request_id, workspace_id, resource_id, user_id, granted_at, expires_at, revoked)
            VALUES (?, ?, ?, ?, ?, ?, ?, 0)`),
		grant.ID, grant.RequestID, grant.WorkspaceID, grant.ResourceID, grant.UserID, grant.GrantedAt, grant.ExpiresAt,
	)
	return err
}

func (s *AccessRequestStore) RevokeGrant(id string) error {
	_, err := s.db.Exec(Rebind(`UPDATE access_request_grants SET revoked = 1 WHERE id = ?`), id)
	return err
}

func (s *AccessRequestStore) CleanExpiredGrants() (int64, error) {
	result, err := s.db.Exec(
		Rebind(`UPDATE access_request_grants SET revoked = 1 WHERE expires_at < ? AND revoked = 0`),
		time.Now().Unix(),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// HasActiveGrant checks if a user has an active (non-expired, non-revoked) grant for a resource.
func (s *AccessRequestStore) HasActiveGrant(userID, resourceID, workspaceID string) bool {
	var count int
	err := s.db.QueryRow(
		Rebind(`SELECT COUNT(*) FROM access_request_grants WHERE user_id = ? AND resource_id = ? AND workspace_id = ? AND revoked = 0 AND expires_at > ?`),
		userID, resourceID, workspaceID, time.Now().Unix(),
	).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}
