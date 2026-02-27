package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"controller/state"
)

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if s.Users == nil {
		http.Error(w, "user store not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, err := s.Users.ListUsers()
		if err != nil {
			http.Error(w, "failed to list users", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, users)
	case http.MethodPost:
		var req struct {
			Name   string `json:"name"`
			Email  string `json:"email"`
			Status string `json:"status"`
			Role   string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Email == "" {
			http.Error(w, "name and email are required", http.StatusBadRequest)
			return
		}
		if req.Status == "" {
			req.Status = "Active"
		}
		if req.Role == "" {
			req.Role = "Member"
		}
		user := state.User{
			Name:      req.Name,
			Email:     req.Email,
			Status:    req.Status,
			Role:      req.Role,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := s.Users.CreateUser(&user); err != nil {
			http.Error(w, fmt.Sprintf("failed to create user: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, user)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUserSubroutes(w http.ResponseWriter, r *http.Request) {
	if s.Users == nil {
		http.Error(w, "user store not configured", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "user id required", http.StatusBadRequest)
		return
	}
	userID := path
	switch r.Method {
	case http.MethodGet:
		user, err := s.Users.GetUser(userID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, user)
	case http.MethodPut, http.MethodPatch:
		var req struct {
			Name   string `json:"name"`
			Email  string `json:"email"`
			Status string `json:"status"`
			Role   string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		existing, err := s.Users.GetUser(userID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.Email != "" {
			existing.Email = req.Email
		}
		if req.Status != "" {
			existing.Status = req.Status
		}
		if req.Role != "" {
			existing.Role = req.Role
		}
		if err := s.Users.UpdateUser(existing); err != nil {
			http.Error(w, fmt.Sprintf("failed to update user: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, existing)
	case http.MethodDelete:
		if err := s.Users.DeleteUser(userID); err != nil {
			http.Error(w, fmt.Sprintf("failed to delete user: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUserGroups(w http.ResponseWriter, r *http.Request) {
	if s.Users == nil {
		http.Error(w, "user store not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		groups, err := s.Users.ListGroups()
		if err != nil {
			http.Error(w, "failed to list groups", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, groups)
	case http.MethodPost:
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		group := state.UserGroup{
			Name:        req.Name,
			Description: req.Description,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}
		if err := s.Users.CreateGroup(&group); err != nil {
			http.Error(w, fmt.Sprintf("failed to create group: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, group)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUserGroupMembers(w http.ResponseWriter, r *http.Request) {
	if s.Users == nil {
		http.Error(w, "user store not configured", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/user-groups/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		http.Error(w, "group id required", http.StatusBadRequest)
		return
	}
	groupID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			group, err := s.Users.GetGroup(groupID)
			if err != nil {
				http.Error(w, "group not found", http.StatusNotFound)
				return
			}
			writeJSON(w, http.StatusOK, group)
		case http.MethodPut, http.MethodPatch:
			var req struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			group, err := s.Users.GetGroup(groupID)
			if err != nil {
				http.Error(w, "group not found", http.StatusNotFound)
				return
			}
			if req.Name != "" {
				group.Name = req.Name
			}
			if req.Description != "" {
				group.Description = req.Description
			}
			group.UpdatedAt = time.Now().UTC()
			if err := s.Users.UpdateGroup(group); err != nil {
				http.Error(w, fmt.Sprintf("failed to update group: %v", err), http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, group)
		case http.MethodDelete:
			if err := s.Users.DeleteGroup(groupID); err != nil {
				http.Error(w, fmt.Sprintf("failed to delete group: %v", err), http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	if parts[1] != "members" {
		http.Error(w, "unknown subresource", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		members, err := s.Users.ListGroupMembers(groupID)
		if err != nil {
			http.Error(w, "failed to list members", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, members)
	case http.MethodPost:
		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.UserID == "" {
			http.Error(w, "user_id required", http.StatusBadRequest)
			return
		}
		if err := s.Users.AddUserToGroup(req.UserID, groupID); err != nil {
			http.Error(w, fmt.Sprintf("failed to add member: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case http.MethodDelete:
		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.UserID == "" {
			http.Error(w, "user_id required", http.StatusBadRequest)
			return
		}
		if err := s.Users.RemoveUserFromGroup(req.UserID, groupID); err != nil {
			http.Error(w, fmt.Sprintf("failed to remove member: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
