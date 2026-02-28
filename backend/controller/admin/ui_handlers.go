package admin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"controller/api"
	"github.com/google/uuid"
)

type uiUser struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	DisplayLabel        string   `json:"displayLabel"`
	Email               string   `json:"email"`
	Status              string   `json:"status"`
	Groups              []string `json:"groups"`
	CertificateIdentity string   `json:"certificateIdentity,omitempty"`
	CreatedAt           string   `json:"createdAt"`
}

type uiGroup struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	DisplayLabel  string `json:"displayLabel"`
	Description   string `json:"description"`
	MemberCount   int    `json:"memberCount"`
	ResourceCount int    `json:"resourceCount"`
	CreatedAt     string `json:"createdAt"`
}

type uiGroupMember struct {
	UserID   string `json:"userId"`
	UserName string `json:"userName"`
	Email    string `json:"email"`
}

type uiResource struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	Address       string  `json:"address"`
	Protocol      string  `json:"protocol"`
	PortFrom      *int    `json:"portFrom"`
	PortTo        *int    `json:"portTo"`
	Alias         *string `json:"alias,omitempty"`
	Description   string  `json:"description"`
	RemoteNetwork *string `json:"remoteNetworkId,omitempty"`
}

type uiAccessRule struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	ResourceID    string   `json:"resourceId"`
	AllowedGroups []string `json:"allowedGroups"`
	Enabled       bool     `json:"enabled"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
}

type uiConnector struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Status            string  `json:"status"`
	Version           string  `json:"version"`
	Hostname          string  `json:"hostname"`
	RemoteNetworkID   string  `json:"remoteNetworkId"`
	LastSeen          string  `json:"lastSeen"`
	Installed         bool    `json:"installed"`
	LastPolicyVersion int     `json:"lastPolicyVersion"`
	LastSeenAt        *string `json:"lastSeenAt"`
}

type uiRemoteNetwork struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Location             string `json:"location"`
	ConnectorCount       int    `json:"connectorCount"`
	OnlineConnectorCount int    `json:"onlineConnectorCount"`
	ResourceCount        int    `json:"resourceCount"`
	CreatedAt            string `json:"createdAt"`
	UpdatedAt            string `json:"updatedAt"`
}

type uiConnectorLog struct {
	ID        int    `json:"id"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

type uiTunneler struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	Version         string `json:"version"`
	Hostname        string `json:"hostname"`
	RemoteNetworkID string `json:"remoteNetworkId"`
}

type uiServiceAccount struct {
	ID                      string `json:"id"`
	Name                    string `json:"name"`
	Type                    string `json:"type"`
	DisplayLabel            string `json:"displayLabel"`
	Status                  string `json:"status"`
	AssociatedResourceCount int    `json:"associatedResourceCount"`
	CreatedAt               string `json:"createdAt"`
}

type uiSubject struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	DisplayLabel string `json:"displayLabel"`
}

func (s *Server) handleUIUsers(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, name, email, status, certificate_identity,
			CASE WHEN typeof(created_at) = 'integer' THEN strftime('%Y-%m-%d', created_at, 'unixepoch') ELSE substr(created_at, 1, 10) END as created_at
			FROM users ORDER BY name ASC`)
		if err != nil {
			http.Error(w, "failed to list users", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		groupStmt, err := db.Prepare(`SELECT group_id FROM user_group_members WHERE user_id = ?`)
		if err != nil {
			http.Error(w, "failed to list user groups", http.StatusInternalServerError)
			return
		}
		defer groupStmt.Close()
		out := []uiUser{}
		for rows.Next() {
			var id, name, email, status string
			var certID sql.NullString
			var created sql.NullString
			if err := rows.Scan(&id, &name, &email, &status, &certID, &created); err != nil {
				http.Error(w, "failed to read users", http.StatusInternalServerError)
				return
			}
			groupRows, _ := groupStmt.Query(id)
			groups := []string{}
			for groupRows.Next() {
				var gid string
				if err := groupRows.Scan(&gid); err == nil {
					groups = append(groups, gid)
				}
			}
			groupRows.Close()
			createdAt := ""
			if created.Valid {
				createdAt = created.String
			}
			out = append(out, uiUser{
				ID:                  id,
				Name:                name,
				Type:                "USER",
				DisplayLabel:        fmt.Sprintf("User: %s", name),
				Email:               email,
				Status:              strings.ToLower(status),
				Groups:              groups,
				CertificateIdentity: certID.String,
				CreatedAt:           createdAt,
			})
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req struct {
			Name   string `json:"name"`
			Email  string `json:"email"`
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Email == "" {
			http.Error(w, "name and email are required", http.StatusBadRequest)
			return
		}
		status := "active"
		if strings.ToLower(req.Status) == "inactive" {
			status = "inactive"
		}
		id := fmt.Sprintf("usr_%d", time.Now().UTC().UnixMilli())
		certID := "identity-" + uuid.NewString()
		createdAt := time.Now().UTC().Unix()
		if _, err := db.Exec(`INSERT INTO users (id, name, email, certificate_identity, status, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, req.Name, strings.ToLower(strings.TrimSpace(req.Email)), certID, status, "Member", createdAt, createdAt); err != nil {
			http.Error(w, "failed to create user", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, uiUser{
			ID:                  id,
			Name:                req.Name,
			Type:                "USER",
			DisplayLabel:        fmt.Sprintf("User: %s", req.Name),
			Email:               req.Email,
			Status:              status,
			Groups:              []string{},
			CertificateIdentity: certID,
			CreatedAt:           dateStringFromUnix(createdAt),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIGroups(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, name, description,
			CASE WHEN typeof(created_at) = 'integer' THEN strftime('%Y-%m-%d', created_at, 'unixepoch') ELSE substr(created_at, 1, 10) END as created_at
			FROM user_groups ORDER BY name ASC`)
		if err != nil {
			http.Error(w, "failed to list groups", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		memberCountStmt, _ := db.Prepare(`SELECT COUNT(*) FROM user_group_members WHERE group_id = ?`)
		resourceCountStmt, _ := db.Prepare(`SELECT COUNT(DISTINCT ar.resource_id) FROM access_rules ar JOIN access_rule_groups arg ON arg.rule_id = ar.id WHERE arg.group_id = ?`)
		defer func() {
			if memberCountStmt != nil {
				memberCountStmt.Close()
			}
			if resourceCountStmt != nil {
				resourceCountStmt.Close()
			}
		}()
		out := []uiGroup{}
		for rows.Next() {
			var id, name, desc string
			var created sql.NullString
			if err := rows.Scan(&id, &name, &desc, &created); err != nil {
				http.Error(w, "failed to read groups", http.StatusInternalServerError)
				return
			}
			memberCount := 0
			resourceCount := 0
			if memberCountStmt != nil {
				_ = memberCountStmt.QueryRow(id).Scan(&memberCount)
			}
			if resourceCountStmt != nil {
				_ = resourceCountStmt.QueryRow(id).Scan(&resourceCount)
			}
			createdAt := ""
			if created.Valid {
				createdAt = created.String
			}
			out = append(out, uiGroup{
				ID:            id,
				Name:          name,
				Type:          "GROUP",
				DisplayLabel:  fmt.Sprintf("Group: %s", name),
				Description:   desc,
				MemberCount:   memberCount,
				ResourceCount: resourceCount,
				CreatedAt:     createdAt,
			})
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Description == "" {
			http.Error(w, "name and description are required", http.StatusBadRequest)
			return
		}
		id := fmt.Sprintf("grp_%d", time.Now().UTC().UnixMilli())
		now := time.Now().UTC().Unix()
		if _, err := db.Exec(`INSERT INTO user_groups (id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, id, req.Name, req.Description, now, now); err != nil {
			http.Error(w, "failed to create group", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIGroupsSubroutes(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/groups/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "group id required", http.StatusBadRequest)
		return
	}
	parts := strings.Split(path, "/")
	groupID := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		row := db.QueryRow(`SELECT id, name, description,
			CASE WHEN typeof(created_at) = 'integer' THEN strftime('%Y-%m-%d', created_at, 'unixepoch') ELSE substr(created_at, 1, 10) END as created_at
			FROM user_groups WHERE id = ?`, groupID)
		var id, name, desc string
		var created sql.NullString
		if err := row.Scan(&id, &name, &desc, &created); err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{"group": nil, "members": []uiGroupMember{}, "resources": []uiResource{}})
			return
		}
		members := []uiGroupMember{}
		memRows, _ := db.Query(`SELECT u.id, u.name, u.email FROM user_group_members m JOIN users u ON u.id = m.user_id WHERE m.group_id = ? ORDER BY u.name ASC`, groupID)
		if memRows != nil {
			for memRows.Next() {
				var m uiGroupMember
				if err := memRows.Scan(&m.UserID, &m.UserName, &m.Email); err == nil {
					members = append(members, m)
				}
			}
			memRows.Close()
		}
		resRows, _ := db.Query(`SELECT r.id, r.name, r.type, r.address, r.protocol, r.port_from, r.port_to, r.alias, r.description, r.remote_network_id
			FROM access_rules ar
			JOIN access_rule_groups arg ON arg.rule_id = ar.id
			JOIN resources r ON r.id = ar.resource_id
			WHERE arg.group_id = ?
			GROUP BY r.id
			ORDER BY r.name ASC`, groupID)
		resources := []uiResource{}
		if resRows != nil {
			for resRows.Next() {
				if res, ok := scanUIResource(resRows); ok {
					resources = append(resources, res)
				}
			}
			resRows.Close()
		}
		createdAt := ""
		if created.Valid {
			createdAt = created.String
		}
		group := uiGroup{
			ID:            id,
			Name:          name,
			Type:          "GROUP",
			DisplayLabel:  fmt.Sprintf("Group: %s", name),
			Description:   desc,
			MemberCount:   len(members),
			ResourceCount: len(resources),
			CreatedAt:     createdAt,
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"group":     group,
			"members":   members,
			"resources": resources,
		})
		return
	}
	switch parts[1] {
	case "members":
		if len(parts) == 2 {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				MemberIDs []string `json:"memberIds"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if req.MemberIDs == nil {
				http.Error(w, "memberIds must be an array", http.StatusBadRequest)
				return
			}
			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "failed to update members", http.StatusInternalServerError)
				return
			}
			if _, err := tx.Exec(`DELETE FROM user_group_members WHERE group_id = ?`, groupID); err != nil {
				_ = tx.Rollback()
				http.Error(w, "failed to update members", http.StatusInternalServerError)
				return
			}
			stmt, _ := tx.Prepare(`INSERT INTO user_group_members (group_id, user_id, added_at) VALUES (?, ?, ?)`)
			for _, id := range req.MemberIDs {
				_, _ = stmt.Exec(groupID, id, time.Now().UTC().Unix())
			}
			if stmt != nil {
				stmt.Close()
			}
			_ = tx.Commit()
			if s.ACLNotify != nil {
				s.ACLNotify.NotifyPolicyChange()
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		if len(parts) == 3 {
			if r.Method != http.MethodDelete {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			userID := parts[2]
			_, _ = db.Exec(`DELETE FROM user_group_members WHERE group_id = ? AND user_id = ?`, groupID, userID)
			if s.ACLNotify != nil {
				s.ACLNotify.NotifyPolicyChange()
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
	case "resources":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ResourceIDs []string `json:"resourceIds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.ResourceIDs == nil {
			http.Error(w, "resourceIds must be an array", http.StatusBadRequest)
			return
		}
		if len(req.ResourceIDs) == 0 {
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		var groupName string
		_ = db.QueryRow(`SELECT name FROM user_groups WHERE id = ?`, groupID).Scan(&groupName)
		if groupName == "" {
			groupName = "Unknown Group"
		}
		now := dateStringNow()
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "failed to add resources", http.StatusInternalServerError)
			return
		}
		checkStmt, _ := tx.Prepare(`SELECT ar.id FROM access_rules ar JOIN access_rule_groups arg ON arg.rule_id = ar.id WHERE ar.resource_id = ? AND arg.group_id = ?`)
		insertRule, _ := tx.Prepare(`INSERT INTO access_rules (id, name, resource_id, enabled, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)`)
		insertRuleGroup, _ := tx.Prepare(`INSERT INTO access_rule_groups (rule_id, group_id) VALUES (?, ?)`)
		for _, resourceID := range req.ResourceIDs {
			var existing string
			if checkStmt != nil {
				_ = checkStmt.QueryRow(resourceID, groupID).Scan(&existing)
			}
			if existing != "" {
				continue
			}
			ruleID := fmt.Sprintf("rule_%d_%s_%s", time.Now().UTC().UnixMilli(), groupID, resourceID)
			if insertRule != nil {
				_, _ = insertRule.Exec(ruleID, fmt.Sprintf("%s access", groupName), resourceID, now, now)
			}
			if insertRuleGroup != nil {
				_, _ = insertRuleGroup.Exec(ruleID, groupID)
			}
		}
		if checkStmt != nil {
			checkStmt.Close()
		}
		if insertRule != nil {
			insertRule.Close()
		}
		if insertRuleGroup != nil {
			insertRuleGroup.Close()
		}
		_ = tx.Commit()
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	http.Error(w, "unknown subresource", http.StatusNotFound)
}

func (s *Server) handleUIResources(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, name, type, address, protocol, port_from, port_to, alias, description, remote_network_id FROM resources ORDER BY name ASC`)
		if err != nil {
			http.Error(w, "failed to list resources", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []uiResource{}
		for rows.Next() {
			if res, ok := scanUIResource(rows); ok {
				out = append(out, res)
			}
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req struct {
			NetworkID string  `json:"network_id"`
			Name      string  `json:"name"`
			Type      string  `json:"type"`
			Address   string  `json:"address"`
			Protocol  string  `json:"protocol"`
			PortFrom  *int    `json:"port_from"`
			PortTo    *int    `json:"port_to"`
			Alias     *string `json:"alias"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Type == "" || req.Address == "" || req.Protocol == "" {
			http.Error(w, "name, type, address, and protocol are required", http.StatusBadRequest)
			return
		}
		ports := buildPorts(req.PortFrom, req.PortTo)
		id := fmt.Sprintf("res_%d", time.Now().UTC().UnixMilli())
		if _, err := db.Exec(`INSERT INTO resources (id, name, type, address, ports, protocol, port_from, port_to, alias, description, remote_network_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, req.Name, req.Type, req.Address, ports, req.Protocol, nullInt(req.PortFrom), nullInt(req.PortTo), req.Alias, fmt.Sprintf("A new %s resource", strings.ToLower(req.Type)), req.NetworkID); err != nil {
			http.Error(w, "failed to create resource", http.StatusBadRequest)
			return
		}
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIResourcesSubroutes(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/resources/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "resource id required", http.StatusBadRequest)
		return
	}
	resourceID := strings.Split(path, "/")[0]
	switch r.Method {
	case http.MethodGet:
		row := db.QueryRow(`SELECT id, name, type, address, protocol, port_from, port_to, alias, description, remote_network_id FROM resources WHERE id = ?`, resourceID)
		res, ok := scanUIResource(row)
		if !ok {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"resource":    nil,
				"accessRules": []uiAccessRule{},
			})
			return
		}
		accessRows, _ := db.Query(`SELECT id, name, resource_id, enabled, created_at, updated_at FROM access_rules WHERE resource_id = ? ORDER BY created_at ASC`, resourceID)
		accessRules := []uiAccessRule{}
		if accessRows != nil {
			groupStmt, _ := db.Prepare(`SELECT group_id FROM access_rule_groups WHERE rule_id = ? ORDER BY group_id ASC`)
			for accessRows.Next() {
				var ar uiAccessRule
				var enabled int
				if err := accessRows.Scan(&ar.ID, &ar.Name, &ar.ResourceID, &enabled, &ar.CreatedAt, &ar.UpdatedAt); err == nil {
					ar.Enabled = enabled != 0
					ar.AllowedGroups = []string{}
					if groupStmt != nil {
						rows, _ := groupStmt.Query(ar.ID)
						for rows != nil && rows.Next() {
							var gid string
							if err := rows.Scan(&gid); err == nil {
								ar.AllowedGroups = append(ar.AllowedGroups, gid)
							}
						}
						if rows != nil {
							rows.Close()
						}
					}
					accessRules = append(accessRules, ar)
				}
			}
			if groupStmt != nil {
				groupStmt.Close()
			}
			accessRows.Close()
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"resource":    res,
			"accessRules": accessRules,
		})
	case http.MethodPut:
		var req struct {
			NetworkID string  `json:"network_id"`
			Name      string  `json:"name"`
			Type      string  `json:"type"`
			Address   string  `json:"address"`
			Protocol  string  `json:"protocol"`
			PortFrom  *int    `json:"port_from"`
			PortTo    *int    `json:"port_to"`
			Alias     *string `json:"alias"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Type == "" || req.Address == "" || req.Protocol == "" {
			http.Error(w, "name, type, address, and protocol are required", http.StatusBadRequest)
			return
		}
		ports := buildPorts(req.PortFrom, req.PortTo)
		_, err := db.Exec(`UPDATE resources SET name = ?, type = ?, address = ?, ports = ?, protocol = ?, port_from = ?, port_to = ?, alias = ?, remote_network_id = ? WHERE id = ?`,
			req.Name, req.Type, req.Address, ports, req.Protocol, nullInt(req.PortFrom), nullInt(req.PortTo), req.Alias, req.NetworkID, resourceID)
		if err != nil {
			http.Error(w, "failed to update resource", http.StatusBadRequest)
			return
		}
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIAccessRules(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ResourceID string   `json:"resourceId"`
		Name       string   `json:"name"`
		GroupIDs   []string `json:"groupIds"`
		Enabled    bool     `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.ResourceID == "" || req.Name == "" || req.GroupIDs == nil {
		http.Error(w, "resourceId, name, and groupIds are required", http.StatusBadRequest)
		return
	}
	ruleID := fmt.Sprintf("rule_%d", time.Now().UTC().UnixMilli())
	now := dateStringNow()
	enabled := 1
	if !req.Enabled {
		enabled = 0
	}
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "failed to create access rule", http.StatusInternalServerError)
		return
	}
	_, err = tx.Exec(`INSERT INTO access_rules (id, name, resource_id, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, ruleID, req.Name, req.ResourceID, enabled, now, now)
	if err != nil {
		_ = tx.Rollback()
		http.Error(w, "failed to create access rule", http.StatusBadRequest)
		return
	}
	stmt, _ := tx.Prepare(`INSERT INTO access_rule_groups (rule_id, group_id) VALUES (?, ?)`)
	for _, gid := range req.GroupIDs {
		_, _ = stmt.Exec(ruleID, gid)
	}
	if stmt != nil {
		stmt.Close()
	}
	_ = tx.Commit()
	if s.ACLNotify != nil {
		s.ACLNotify.NotifyPolicyChange()
	}
	writeJSON(w, http.StatusOK, uiAccessRule{
		ID:            ruleID,
		Name:          req.Name,
		ResourceID:    req.ResourceID,
		AllowedGroups: req.GroupIDs,
		Enabled:       req.Enabled,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
}

func (s *Server) handleUIAccessRulesSubroutes(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/access-rules/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "rule id required", http.StatusBadRequest)
		return
	}
	parts := strings.Split(path, "/")
	ruleID := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_, _ = db.Exec(`DELETE FROM access_rules WHERE id = ?`, ruleID)
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if len(parts) == 2 && parts[1] == "identity-count" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var count int
		err := db.QueryRow(`SELECT COUNT(DISTINCT u.id)
			FROM access_rule_groups arg
			JOIN user_group_members gm ON gm.group_id = arg.group_id
			JOIN users u ON u.id = gm.user_id
			WHERE arg.rule_id = ? AND u.certificate_identity IS NOT NULL`, ruleID).Scan(&count)
		if err != nil {
			http.Error(w, "failed to compute identity count", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"count": count})
		return
	}
	http.Error(w, "unknown subresource", http.StatusNotFound)
}

func (s *Server) handleUIRemoteNetworks(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`
			SELECT n.id, n.name, n.location,
				CASE WHEN typeof(n.created_at) = 'integer' THEN strftime('%Y-%m-%d', n.created_at, 'unixepoch') ELSE substr(n.created_at, 1, 10) END as created_at,
				CASE WHEN typeof(n.updated_at) = 'integer' THEN strftime('%Y-%m-%d', n.updated_at, 'unixepoch') ELSE substr(n.updated_at, 1, 10) END as updated_at,
				(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
				(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
				(SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
			FROM remote_networks n
			ORDER BY n.created_at ASC`)
		if err != nil {
			http.Error(w, "failed to list remote networks", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []uiRemoteNetwork{}
		for rows.Next() {
			var id, name, location string
			var created, updated sql.NullString
			var connCount, onlineCount, resCount int
			if err := rows.Scan(&id, &name, &location, &created, &updated, &connCount, &onlineCount, &resCount); err != nil {
				http.Error(w, "failed to read remote networks", http.StatusInternalServerError)
				return
			}
			createdAt := ""
			if created.Valid {
				createdAt = created.String
			}
			updatedAt := ""
			if updated.Valid {
				updatedAt = updated.String
			}
			if location == "" {
				location = "OTHER"
			}
			out = append(out, uiRemoteNetwork{
				ID:                   id,
				Name:                 name,
				Location:             location,
				ConnectorCount:       connCount,
				OnlineConnectorCount: onlineCount,
				ResourceCount:        resCount,
				CreatedAt:            createdAt,
				UpdatedAt:            updatedAt,
			})
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req struct {
			Name     string `json:"name"`
			Location string `json:"location"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if req.Location == "" {
			req.Location = "OTHER"
		}
		id := fmt.Sprintf("net_%d", time.Now().UTC().UnixMilli())
		now := time.Now().UTC().Unix()
		_, err := db.Exec(`INSERT INTO remote_networks (id, name, location, tags_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, id, req.Name, req.Location, "{}", now, now)
		if err != nil {
			http.Error(w, "failed to create network", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIRemoteNetworksSubroutes(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/remote-networks/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "network id required", http.StatusBadRequest)
		return
	}
	networkID := strings.Split(path, "/")[0]
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	row := db.QueryRow(`
		SELECT n.id, n.name, n.location,
			CASE WHEN typeof(n.created_at) = 'integer' THEN strftime('%Y-%m-%d', n.created_at, 'unixepoch') ELSE substr(n.created_at, 1, 10) END as created_at,
			CASE WHEN typeof(n.updated_at) = 'integer' THEN strftime('%Y-%m-%d', n.updated_at, 'unixepoch') ELSE substr(n.updated_at, 1, 10) END as updated_at,
			(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
			(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
			(SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
		FROM remote_networks n
		WHERE n.id = ?`, networkID)
	var id, name, location string
	var created, updated sql.NullString
	var connCount, onlineCount, resCount int
	if err := row.Scan(&id, &name, &location, &created, &updated, &connCount, &onlineCount, &resCount); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"network": nil, "connectors": []uiConnector{}, "resources": []uiResource{}})
		return
	}
	if location == "" {
		location = "OTHER"
	}
	createdAt := ""
	if created.Valid {
		createdAt = created.String
	}
	updatedAt := ""
	if updated.Valid {
		updatedAt = updated.String
	}
	network := uiRemoteNetwork{
		ID:                   id,
		Name:                 name,
		Location:             location,
		ConnectorCount:       connCount,
		OnlineConnectorCount: onlineCount,
		ResourceCount:        resCount,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
	}
	connectorRows, _ := db.Query(`SELECT id, name, status, version, hostname, remote_network_id, CAST(last_seen AS TEXT) as last_seen, last_seen_at, installed, last_policy_version FROM connectors WHERE remote_network_id = ? ORDER BY name ASC`, networkID)
	connectors := []uiConnector{}
	if connectorRows != nil {
		for connectorRows.Next() {
			if conn, ok := scanUIConnector(connectorRows); ok {
				connectors = append(connectors, conn)
			}
		}
		connectorRows.Close()
	}
	resourceRows, _ := db.Query(`SELECT id, name, type, address, protocol, port_from, port_to, alias, description, remote_network_id FROM resources WHERE remote_network_id = ? ORDER BY name ASC`, networkID)
	resources := []uiResource{}
	if resourceRows != nil {
		for resourceRows.Next() {
			if res, ok := scanUIResource(resourceRows); ok {
				resources = append(resources, res)
			}
		}
		resourceRows.Close()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"network":    network,
		"connectors": connectors,
		"resources":  resources,
	})
}

func (s *Server) handleUIConnectors(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, name, status, version, hostname, remote_network_id, CAST(last_seen AS TEXT) as last_seen, last_seen_at, installed, last_policy_version FROM connectors ORDER BY name ASC`)
		if err != nil {
			http.Error(w, "failed to list connectors", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []uiConnector{}
		for rows.Next() {
			if conn, ok := scanUIConnector(rows); ok {
				out = append(out, conn)
			}
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req struct {
			Name            string `json:"name"`
			RemoteNetworkID string `json:"remoteNetworkId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.RemoteNetworkID == "" {
			http.Error(w, "name and remoteNetworkId are required", http.StatusBadRequest)
			return
		}
		id := fmt.Sprintf("con_%d", time.Now().UTC().UnixMilli())
		hostname := strings.ToLower(strings.ReplaceAll(req.Name, " ", "-")) + ".local"
		nowUnix := time.Now().UTC().Unix()
		nowISO := isoStringNow()
		_, err := db.Exec(`INSERT INTO connectors (id, name, status, version, hostname, remote_network_id, last_seen, last_policy_version, last_seen_at, installed) VALUES (?, ?, 'offline', '1.0.0', ?, ?, ?, 0, ?, 0)`, id, req.Name, hostname, req.RemoteNetworkID, nowUnix, nowISO)
		if err != nil {
			http.Error(w, "failed to create connector", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIConnectorsSubroutes(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/connectors/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "connector id required", http.StatusBadRequest)
		return
	}
	parts := strings.Split(path, "/")
	connectorID := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		row := db.QueryRow(`SELECT id, name, status, version, hostname, remote_network_id, CAST(last_seen AS TEXT) as last_seen, last_seen_at, installed, last_policy_version FROM connectors WHERE id = ?`, connectorID)
		connector, ok := scanUIConnector(row)
		if !ok {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"connector": nil,
				"network":   nil,
				"logs":      []uiConnectorLog{},
			})
			return
		}
		networkRow := db.QueryRow(`
			SELECT n.id, n.name, n.location,
				CASE WHEN typeof(n.created_at) = 'integer' THEN strftime('%Y-%m-%d', n.created_at, 'unixepoch') ELSE substr(n.created_at, 1, 10) END as created_at,
				CASE WHEN typeof(n.updated_at) = 'integer' THEN strftime('%Y-%m-%d', n.updated_at, 'unixepoch') ELSE substr(n.updated_at, 1, 10) END as updated_at,
				(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
				(SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
				(SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
			FROM remote_networks n
			WHERE n.id = ?`, connector.RemoteNetworkID)
		var network *uiRemoteNetwork
		{
			var id, name, location string
			var created, updated sql.NullString
			var connCount, onlineCount, resCount int
			if err := networkRow.Scan(&id, &name, &location, &created, &updated, &connCount, &onlineCount, &resCount); err == nil {
				createdAt := ""
				if created.Valid {
					createdAt = created.String
				}
				updatedAt := ""
				if updated.Valid {
					updatedAt = updated.String
				}
				if location == "" {
					location = "OTHER"
				}
				n := uiRemoteNetwork{
					ID:                   id,
					Name:                 name,
					Location:             location,
					ConnectorCount:       connCount,
					OnlineConnectorCount: onlineCount,
					ResourceCount:        resCount,
					CreatedAt:            createdAt,
					UpdatedAt:            updatedAt,
				}
				network = &n
			}
		}
		logs := []uiConnectorLog{}
		logRows, _ := db.Query(`SELECT id, timestamp, message FROM connector_logs WHERE connector_id = ? ORDER BY id ASC`, connectorID)
		if logRows != nil {
			for logRows.Next() {
				var l uiConnectorLog
				if err := logRows.Scan(&l.ID, &l.Timestamp, &l.Message); err == nil {
					logs = append(logs, l)
				}
			}
			logRows.Close()
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"connector": connector,
			"network":   network,
			"logs":      logs,
		})
		return
	}
	if len(parts) >= 2 && parts[1] == "heartbeat" {
		switch r.Method {
		case http.MethodPost:
			nowUnix := time.Now().UTC().Unix()
			nowISO := isoStringNow()
			_, _ = db.Exec(`UPDATE connectors SET status = ?, last_seen = ?, last_seen_at = ?, installed = 1 WHERE id = ?`, "online", nowUnix, nowISO, connectorID)
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		case http.MethodPatch:
			var req struct {
				LastPolicyVersion int `json:"last_policy_version"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			nowUnix := time.Now().UTC().Unix()
			nowISO := isoStringNow()
			_, _ = db.Exec(`UPDATE connectors SET last_seen = ?, last_seen_at = ?, last_policy_version = ? WHERE id = ?`, nowUnix, nowISO, req.LastPolicyVersion, connectorID)
			var currentVersion int
			_ = db.QueryRow(`SELECT version FROM connector_policy_versions WHERE connector_id = ?`, connectorID).Scan(&currentVersion)
			updateAvailable := req.LastPolicyVersion < currentVersion
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"update_available": updateAvailable,
				"current_version":  currentVersion,
			})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	http.Error(w, "unknown subresource", http.StatusNotFound)
}

func (s *Server) handleUITunnelers(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := db.Query(`SELECT id, name, status, version, hostname, remote_network_id FROM tunnelers ORDER BY name ASC`)
	if err != nil {
		http.Error(w, "failed to list tunnelers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []uiTunneler{}
	for rows.Next() {
		var t uiTunneler
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &t.Version, &t.Hostname, &t.RemoteNetworkID); err == nil {
			out = append(out, t)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUISubjects(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	subjectType := strings.ToUpper(r.URL.Query().Get("type"))
	subjects := []uiSubject{}
	if subjectType == "" || subjectType == "USER" {
		rows, _ := db.Query(`SELECT id, name FROM users ORDER BY name ASC`)
		if rows != nil {
			for rows.Next() {
				var id, name string
				if err := rows.Scan(&id, &name); err == nil {
					subjects = append(subjects, uiSubject{ID: id, Name: name, Type: "USER", DisplayLabel: fmt.Sprintf("User: %s", name)})
				}
			}
			rows.Close()
		}
	}
	if subjectType == "" || subjectType == "GROUP" {
		rows, _ := db.Query(`SELECT id, name FROM user_groups ORDER BY name ASC`)
		if rows != nil {
			for rows.Next() {
				var id, name string
				if err := rows.Scan(&id, &name); err == nil {
					subjects = append(subjects, uiSubject{ID: id, Name: name, Type: "GROUP", DisplayLabel: fmt.Sprintf("Group: %s", name)})
				}
			}
			rows.Close()
		}
	}
	if subjectType == "" || subjectType == "SERVICE" {
		rows, _ := db.Query(`SELECT id, name FROM service_accounts ORDER BY name ASC`)
		if rows != nil {
			for rows.Next() {
				var id, name string
				if err := rows.Scan(&id, &name); err == nil {
					subjects = append(subjects, uiSubject{ID: id, Name: name, Type: "SERVICE", DisplayLabel: fmt.Sprintf("Service: %s", name)})
				}
			}
			rows.Close()
		}
	}
	writeJSON(w, http.StatusOK, subjects)
}

func (s *Server) handleUIServiceAccounts(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := db.Query(`SELECT id, name, status, associated_resource_count,
		CASE WHEN typeof(created_at) = 'integer' THEN strftime('%Y-%m-%d', created_at, 'unixepoch') ELSE substr(created_at, 1, 10) END as created_at
		FROM service_accounts ORDER BY name ASC`)
	if err != nil {
		http.Error(w, "failed to list service accounts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []uiServiceAccount{}
	for rows.Next() {
		var sa uiServiceAccount
		var created sql.NullString
		if err := rows.Scan(&sa.ID, &sa.Name, &sa.Status, &sa.AssociatedResourceCount, &created); err == nil {
			sa.Type = "SERVICE"
			sa.DisplayLabel = fmt.Sprintf("Service: %s", sa.Name)
			if created.Valid {
				sa.CreatedAt = created.String
			}
			out = append(out, sa)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

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
	var remoteNetworkID string
	if err := db.QueryRow(`SELECT remote_network_id FROM connectors WHERE id = ?`, connectorID).Scan(&remoteNetworkID); err != nil {
		http.Error(w, "connector not found", http.StatusNotFound)
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
	var remoteNetworkID string
	if err := db.QueryRow(`SELECT remote_network_id FROM connectors WHERE id = ?`, connectorID).Scan(&remoteNetworkID); err != nil {
		http.Error(w, "connector not found", http.StatusNotFound)
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

func (s *Server) uiDB(w http.ResponseWriter) (*sql.DB, bool) {
	if s == nil || s.ACLs == nil || s.ACLs.DB() == nil {
		http.Error(w, "db not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return s.ACLs.DB(), true
}

func scanUIResource(scanner interface{ Scan(dest ...any) error }) (uiResource, bool) {
	var res uiResource
	var protocol sql.NullString
	var portFrom sql.NullInt64
	var portTo sql.NullInt64
	var alias sql.NullString
	var remoteNet sql.NullString
	if err := scanner.Scan(&res.ID, &res.Name, &res.Type, &res.Address, &protocol, &portFrom, &portTo, &alias, &res.Description, &remoteNet); err != nil {
		return uiResource{}, false
	}
	res.Protocol = "TCP"
	if protocol.Valid {
		res.Protocol = protocol.String
	}
	if portFrom.Valid {
		v := int(portFrom.Int64)
		res.PortFrom = &v
	}
	if portTo.Valid {
		v := int(portTo.Int64)
		res.PortTo = &v
	}
	if alias.Valid {
		res.Alias = &alias.String
	}
	if remoteNet.Valid {
		res.RemoteNetwork = &remoteNet.String
	}
	return res, true
}

func scanUIConnector(scanner interface{ Scan(dest ...any) error }) (uiConnector, bool) {
	var c uiConnector
	var lastSeen sql.NullString
	var lastSeenAt sql.NullString
	var installed sql.NullInt64
	var lastPolicyVersion sql.NullInt64
	if err := scanner.Scan(&c.ID, &c.Name, &c.Status, &c.Version, &c.Hostname, &c.RemoteNetworkID, &lastSeen, &lastSeenAt, &installed, &lastPolicyVersion); err != nil {
		return uiConnector{}, false
	}
	if lastSeenAt.Valid {
		c.LastSeen = lastSeenAt.String
		c.LastSeenAt = &lastSeenAt.String
	} else if lastSeen.Valid {
		if ts, err := strconv.ParseInt(lastSeen.String, 10, 64); err == nil {
			iso := isoStringFromUnix(ts)
			c.LastSeen = iso
			c.LastSeenAt = &iso
		} else {
			c.LastSeen = lastSeen.String
			c.LastSeenAt = &lastSeen.String
		}
	}
	c.Installed = installed.Valid && installed.Int64 != 0
	if lastPolicyVersion.Valid {
		c.LastPolicyVersion = int(lastPolicyVersion.Int64)
	}
	return c, true
}

func buildPorts(from, to *int) string {
	if from != nil && to != nil {
		return fmt.Sprintf("%d-%d", *from, *to)
	}
	if from != nil {
		return fmt.Sprintf("%d", *from)
	}
	return ""
}

func nullInt(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func dateStringNow() string {
	return time.Now().UTC().Format("2006-01-02")
}

func dateStringFromUnix(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format("2006-01-02")
}

func isoStringNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func isoStringFromUnix(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format("2006-01-02T15:04:05.000Z")
}

type policyResource = api.PolicyResource

func policyResources(db *sql.DB, remoteNetworkID string) ([]policyResource, error) {
	return api.PolicyResourcesForUI(db, remoteNetworkID)
}

func policyHash(resources []policyResource) string {
	return api.PolicyHashForUI(resources)
}

func policyVersion(db *sql.DB, connectorID, policyHash, compiledAt string) int {
	return api.PolicyVersionForUI(db, connectorID, policyHash, compiledAt)
}
