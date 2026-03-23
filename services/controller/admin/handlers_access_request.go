package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"controller/state"
)

func (s *Server) handleAccessRequests(w http.ResponseWriter, r *http.Request) {
	wsID := workspaceIDFromContext(r.Context())
	if wsID == "" {
		http.Error(w, "workspace required", http.StatusBadRequest)
		return
	}
	if s.AccessRequests == nil {
		http.Error(w, "access requests not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		requests, err := s.AccessRequests.ListForWorkspace(wsID)
		if err != nil {
			http.Error(w, "failed to list requests", http.StatusInternalServerError)
			return
		}
		if requests == nil {
			requests = []state.AccessRequest{}
		}
		writeJSON(w, http.StatusOK, requests)

	case http.MethodPost:
		var req struct {
			ResourceID    string `json:"resource_id"`
			Reason        string `json:"reason"`
			DurationHours int    `json:"duration_hours"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.ResourceID == "" {
			http.Error(w, "resource_id is required", http.StatusBadRequest)
			return
		}
		if req.DurationHours <= 0 {
			req.DurationHours = 1
		}
		if req.DurationHours > 168 { // max 7 days
			req.DurationHours = 168
		}

		userID := userIDFromContext(r.Context())
		email := sessionEmailFromContext(r.Context())
		id := fmt.Sprintf("areq_%d", time.Now().UTC().UnixMilli())
		now := time.Now().Unix()

		accessReq := &state.AccessRequest{
			ID:             id,
			WorkspaceID:    wsID,
			RequesterID:    userID,
			RequesterEmail: email,
			ResourceID:     req.ResourceID,
			Reason:         req.Reason,
			DurationHours:  req.DurationHours,
			CreatedAt:      now,
			ExpiresAt:      now + int64(48*3600), // request expires in 48h if not decided
		}
		if err := s.AccessRequests.Create(accessReq); err != nil {
			http.Error(w, "failed to create request", http.StatusInternalServerError)
			return
		}
		s.audit(r, "access_request.create", id, "ok")
		writeJSON(w, http.StatusOK, accessReq)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAccessRequestSubroutes(w http.ResponseWriter, r *http.Request) {
	wsID := workspaceIDFromContext(r.Context())
	if wsID == "" {
		http.Error(w, "workspace required", http.StatusBadRequest)
		return
	}
	if s.AccessRequests == nil {
		http.Error(w, "access requests not configured", http.StatusServiceUnavailable)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/access-requests/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	requestID := parts[0]

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req, err := s.AccessRequests.Get(requestID)
		if err != nil {
			http.Error(w, "request not found", http.StatusNotFound)
			return
		}
		if req.WorkspaceID != wsID {
			http.Error(w, "request not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, req)
		return
	}

	action := parts[1]
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	actor := sessionEmailFromContext(r.Context())
	actorID := userIDFromContext(r.Context())
	actorRole := workspaceRoleFromContext(r.Context())

	switch action {
	case "approve":
		// Only admins/owners can approve access requests.
		if !roleAtLeast(actorRole, "admin") {
			http.Error(w, "insufficient permissions: only admins can approve access requests", http.StatusForbidden)
			return
		}
		req, err := s.AccessRequests.Get(requestID)
		if err != nil || req.WorkspaceID != wsID {
			http.Error(w, "request not found", http.StatusNotFound)
			return
		}
		// Prevent self-approval.
		if actorID == req.RequesterID {
			http.Error(w, "cannot approve your own access request", http.StatusForbidden)
			return
		}
		if req.Status != "pending" {
			http.Error(w, "request already decided", http.StatusConflict)
			return
		}
		if err := s.AccessRequests.Approve(requestID, actor, body.Reason); err != nil {
			http.Error(w, "failed to approve", http.StatusInternalServerError)
			return
		}
		// Create time-bound grant
		grantID := fmt.Sprintf("agrnt_%d", time.Now().UTC().UnixMilli())
		now := time.Now().Unix()
		grant := &state.AccessRequestGrant{
			ID:          grantID,
			RequestID:   requestID,
			WorkspaceID: wsID,
			ResourceID:  req.ResourceID,
			UserID:      req.RequesterID,
			GrantedAt:   now,
			ExpiresAt:   now + int64(req.DurationHours*3600),
		}
		if err := s.AccessRequests.CreateGrant(grant); err != nil {
			http.Error(w, "failed to create grant", http.StatusInternalServerError)
			return
		}
		s.audit(r, "access_request.approved", requestID, "ok")
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "approved", "grant": grant})

	case "reject":
		// Only admins/owners can reject access requests.
		if !roleAtLeast(actorRole, "admin") {
			http.Error(w, "insufficient permissions: only admins can reject access requests", http.StatusForbidden)
			return
		}
		req, err := s.AccessRequests.Get(requestID)
		if err != nil || req.WorkspaceID != wsID {
			http.Error(w, "request not found", http.StatusNotFound)
			return
		}
		if err := s.AccessRequests.Reject(requestID, actor, body.Reason); err != nil {
			http.Error(w, "failed to reject", http.StatusInternalServerError)
			return
		}
		s.audit(r, "access_request.rejected", requestID, "ok")
		writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})

	default:
		http.Error(w, "unknown action", http.StatusNotFound)
	}
}
