package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"controller/ca"
	"controller/state"

	"github.com/google/uuid"
)

var (
	slugRegexp    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)
	reservedSlugs = map[string]bool{
		"www": true, "api": true, "admin": true, "app": true,
		"mail": true, "ftp": true, "localhost": true,
	}
)

func validateSlug(slug string) string {
	if len(slug) < 3 || len(slug) > 63 {
		return "slug must be 3-63 characters"
	}
	if !slugRegexp.MatchString(slug) {
		return "slug must be lowercase alphanumeric with hyphens, cannot start or end with hyphen"
	}
	if reservedSlugs[slug] {
		return "slug is reserved"
	}
	return ""
}

// RegisterWorkspaceRoutes registers all workspace-related HTTP routes.
func (s *Server) RegisterWorkspaceRoutes(mux *http.ServeMux) {
	if s.Workspaces == nil {
		return
	}
	// Public lookup endpoint — no auth required.
	mux.Handle("/api/workspaces/lookup", withCORS(http.HandlerFunc(s.handleWorkspaceLookup)))
	mux.Handle("/api/workspaces", withCORS(s.workspaceAuth(http.HandlerFunc(s.handleWorkspaces))))
	mux.Handle("/api/workspaces/", withCORS(s.workspaceAuth(http.HandlerFunc(s.handleWorkspaceSubroutes))))
}

func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListWorkspaces(w, r)
	case http.MethodPost:
		s.handleCreateWorkspace(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorkspaceSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/workspaces/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		http.Error(w, "workspace id required", http.StatusBadRequest)
		return
	}
	wsID := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.handleGetWorkspace(w, r, wsID)
		case http.MethodPut:
			s.handleUpdateWorkspace(w, r, wsID)
		case http.MethodDelete:
			s.handleDeleteWorkspace(w, r, wsID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	switch parts[1] {
	case "select":
		if r.Method == http.MethodPost {
			s.handleSelectWorkspace(w, r, wsID)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	case "members":
		if len(parts) == 2 {
			switch r.Method {
			case http.MethodGet:
				s.handleListMembers(w, r, wsID)
			case http.MethodPost:
				s.handleAddMember(w, r, wsID)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		if len(parts) == 3 {
			uid := parts[2]
			switch r.Method {
			case http.MethodPut:
				s.handleUpdateMemberRole(w, r, wsID, uid)
			case http.MethodDelete:
				s.handleRemoveMember(w, r, wsID, uid)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// POST /api/workspaces
func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if msg := validateSlug(req.Slug); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// Check slug uniqueness
	if _, err := s.Workspaces.GetWorkspaceBySlug(req.Slug); err == nil {
		http.Error(w, "slug already taken", http.StatusConflict)
		return
	}

	trustDomain := req.Slug + "." + s.SystemDomain

	// Issue workspace sub-CA
	certPEM, keyPEM, err := ca.IssueWorkspaceCA(s.IntermediateCA, trustDomain, 365*24*time.Hour)
	if err != nil {
		http.Error(w, "failed to issue workspace CA: "+err.Error(), http.StatusInternalServerError)
		return
	}

	ws := &state.Workspace{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Slug:        req.Slug,
		TrustDomain: trustDomain,
		CACertPEM:   string(certPEM),
		CAKeyPEM:    string(keyPEM),
	}
	if err := s.Workspaces.CreateWorkspace(ws); err != nil {
		http.Error(w, "failed to create workspace: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Lookup or create user by email, add as owner
	email := sessionEmailFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	if userID == "" {
		// User may not have uid in JWT yet (initial login JWT). Look up by email.
		if u, err := s.Workspaces.GetUserByEmail(email); err == nil {
			userID = u.ID
		} else {
			// Create user
			userID = uuid.New().String()
			now := time.Now().UTC()
			newUser := &state.User{
				ID:        userID,
				Name:      email,
				Email:     email,
				Status:    "Active",
				Role:      "Admin",
				CreatedAt: now,
				UpdatedAt: now,
			}
			if s.Users != nil {
				_ = s.Users.CreateUser(newUser)
			}
		}
	}

	if err := s.Workspaces.AddMember(ws.ID, userID, "owner"); err != nil {
		http.Error(w, "failed to add owner: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Sign workspace JWT
	token, err := s.signWorkspaceJWT(email, userID, ws.ID, ws.Slug, "owner")
	if err != nil {
		http.Error(w, "failed to sign token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Don't expose the CA private key in the response
	resp := map[string]interface{}{
		"workspace": ws,
		"token":     token,
	}
	writeJSON(w, http.StatusCreated, resp)
}

// GET /api/workspaces
func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	email := sessionEmailFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	if userID == "" {
		if u, err := s.Workspaces.GetUserByEmail(email); err == nil {
			userID = u.ID
		} else {
			writeJSON(w, http.StatusOK, []state.Workspace{})
			return
		}
	}

	workspaces, err := s.Workspaces.ListWorkspacesForUser(userID)
	if err != nil {
		http.Error(w, "failed to list workspaces: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Attach user's role for each workspace
	type wsWithRole struct {
		state.Workspace
		Role string `json:"role"`
	}
	result := make([]wsWithRole, 0, len(workspaces))
	for _, ws := range workspaces {
		role := "member"
		if m, err := s.Workspaces.GetMember(ws.ID, userID); err == nil {
			role = m.Role
		}
		result = append(result, wsWithRole{Workspace: ws, Role: role})
	}
	writeJSON(w, http.StatusOK, result)
}

// GET /api/workspaces/{id}
func (s *Server) handleGetWorkspace(w http.ResponseWriter, r *http.Request, wsID string) {
	ws, err := s.Workspaces.GetWorkspace(wsID)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}

	// Verify membership
	email := sessionEmailFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	if userID == "" {
		if u, err := s.Workspaces.GetUserByEmail(email); err == nil {
			userID = u.ID
		}
	}
	member, err := s.Workspaces.GetMember(wsID, userID)
	if err != nil {
		http.Error(w, "not a member of this workspace", http.StatusForbidden)
		return
	}

	type wsWithRole struct {
		*state.Workspace
		Role string `json:"role"`
	}
	writeJSON(w, http.StatusOK, wsWithRole{Workspace: ws, Role: member.Role})
}

// PUT /api/workspaces/{id}
func (s *Server) handleUpdateWorkspace(w http.ResponseWriter, r *http.Request, wsID string) {
	email := sessionEmailFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	if userID == "" {
		if u, err := s.Workspaces.GetUserByEmail(email); err == nil {
			userID = u.ID
		}
	}
	member, err := s.Workspaces.GetMember(wsID, userID)
	if err != nil || !roleAtLeast(member.Role, "admin") {
		http.Error(w, "insufficient permissions", http.StatusForbidden)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	ws, err := s.Workspaces.GetWorkspace(wsID)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	ws.Name = req.Name
	if err := s.Workspaces.UpdateWorkspace(ws); err != nil {
		http.Error(w, "failed to update: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

// DELETE /api/workspaces/{id}
func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request, wsID string) {
	email := sessionEmailFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	if userID == "" {
		if u, err := s.Workspaces.GetUserByEmail(email); err == nil {
			userID = u.ID
		}
	}
	member, err := s.Workspaces.GetMember(wsID, userID)
	if err != nil || member.Role != "owner" {
		http.Error(w, "only owner can delete workspace", http.StatusForbidden)
		return
	}

	if err := s.Workspaces.DeleteWorkspace(wsID); err != nil {
		http.Error(w, "failed to delete: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// POST /api/workspaces/{id}/select
func (s *Server) handleSelectWorkspace(w http.ResponseWriter, r *http.Request, wsID string) {
	email := sessionEmailFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	if userID == "" {
		if u, err := s.Workspaces.GetUserByEmail(email); err == nil {
			userID = u.ID
		}
	}

	ws, err := s.Workspaces.GetWorkspace(wsID)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	if ws.Status != "active" {
		http.Error(w, "workspace is not active", http.StatusForbidden)
		return
	}

	member, err := s.Workspaces.GetMember(wsID, userID)
	if err != nil {
		http.Error(w, "not a member of this workspace", http.StatusForbidden)
		return
	}

	token, err := s.signWorkspaceJWT(email, userID, ws.ID, ws.Slug, member.Role)
	if err != nil {
		http.Error(w, "failed to sign token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// GET /api/workspaces/{id}/members
func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request, wsID string) {
	email := sessionEmailFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	if userID == "" {
		if u, err := s.Workspaces.GetUserByEmail(email); err == nil {
			userID = u.ID
		}
	}
	if _, err := s.Workspaces.GetMember(wsID, userID); err != nil {
		http.Error(w, "not a member of this workspace", http.StatusForbidden)
		return
	}

	members, err := s.Workspaces.ListMembers(wsID)
	if err != nil {
		http.Error(w, "failed to list members: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Enrich with user details
	type memberWithUser struct {
		state.WorkspaceMember
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	result := make([]memberWithUser, 0, len(members))
	for _, m := range members {
		mwu := memberWithUser{WorkspaceMember: m}
		if s.Users != nil {
			if u, err := s.Users.GetUser(m.UserID); err == nil {
				mwu.Email = u.Email
				mwu.Name = u.Name
			}
		}
		result = append(result, mwu)
	}
	writeJSON(w, http.StatusOK, result)
}

// POST /api/workspaces/{id}/members
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request, wsID string) {
	email := sessionEmailFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	if userID == "" {
		if u, err := s.Workspaces.GetUserByEmail(email); err == nil {
			userID = u.ID
		}
	}
	member, err := s.Workspaces.GetMember(wsID, userID)
	if err != nil || !roleAtLeast(member.Role, "admin") {
		http.Error(w, "insufficient permissions", http.StatusForbidden)
		return
	}

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		http.Error(w, "email is required", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}
	if req.Role != "member" && req.Role != "admin" && req.Role != "owner" {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}

	// Find or create user by email
	targetUser, err := s.Workspaces.GetUserByEmail(req.Email)
	if err == sql.ErrNoRows {
		targetUserID := uuid.New().String()
		now := time.Now().UTC()
		newUser := &state.User{
			ID:        targetUserID,
			Name:      req.Email,
			Email:     req.Email,
			Status:    "Active",
			Role:      "Member",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if s.Users != nil {
			if err := s.Users.CreateUser(newUser); err != nil {
				http.Error(w, "failed to create user: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		targetUser = newUser
	} else if err != nil {
		http.Error(w, "failed to lookup user", http.StatusInternalServerError)
		return
	}

	if err := s.Workspaces.AddMember(wsID, targetUser.ID, req.Role); err != nil {
		http.Error(w, "failed to add member: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "added", "user_id": targetUser.ID})
}

// PUT /api/workspaces/{id}/members/{uid}
func (s *Server) handleUpdateMemberRole(w http.ResponseWriter, r *http.Request, wsID, targetUID string) {
	email := sessionEmailFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	if userID == "" {
		if u, err := s.Workspaces.GetUserByEmail(email); err == nil {
			userID = u.ID
		}
	}
	member, err := s.Workspaces.GetMember(wsID, userID)
	if err != nil || member.Role != "owner" {
		http.Error(w, "only owner can change roles", http.StatusForbidden)
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Role != "member" && req.Role != "admin" && req.Role != "owner" {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}

	if err := s.Workspaces.UpdateMemberRole(wsID, targetUID, req.Role); err != nil {
		http.Error(w, "failed to update role: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DELETE /api/workspaces/{id}/members/{uid}
func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request, wsID, targetUID string) {
	email := sessionEmailFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	if userID == "" {
		if u, err := s.Workspaces.GetUserByEmail(email); err == nil {
			userID = u.ID
		}
	}
	member, err := s.Workspaces.GetMember(wsID, userID)
	if err != nil || !roleAtLeast(member.Role, "admin") {
		http.Error(w, "insufficient permissions", http.StatusForbidden)
		return
	}

	// Cannot remove self if owner
	if targetUID == userID {
		http.Error(w, "cannot remove yourself", http.StatusBadRequest)
		return
	}

	if err := s.Workspaces.RemoveMember(wsID, targetUID); err != nil {
		http.Error(w, "failed to remove member: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// handleWorkspaceLookup is a public endpoint for looking up workspaces.
// GET /api/workspaces/lookup?slug=<slug>  — check if a network URL exists
// GET /api/workspaces/lookup?email=<email> — find networks for an email
func (s *Server) handleWorkspaceLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := r.URL.Query().Get("slug")
	email := r.URL.Query().Get("email")

	if slug != "" {
		ws, err := s.Workspaces.GetWorkspaceBySlug(slug)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{"exists": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"exists": true,
			"name":   ws.Name,
			"slug":   ws.Slug,
		})
		return
	}

	if email != "" {
		results, err := s.Workspaces.ListWorkspaceSlugsForEmail(strings.ToLower(strings.TrimSpace(email)))
		if err != nil || len(results) == 0 {
			writeJSON(w, http.StatusOK, map[string]interface{}{"networks": []interface{}{}})
			return
		}
		type network struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		}
		networks := make([]network, len(results))
		for i, r := range results {
			networks[i] = network{Name: r.Name, Slug: r.Slug}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"networks": networks})
		return
	}

	http.Error(w, "slug or email parameter required", http.StatusBadRequest)
}
