package admin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"controller/state"
)

func (s *Server) handleUIGroups(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		wsID := workspaceIDFromContext(r.Context())
		wsClause, wsArgs := wsWhereOnly(wsID, "")
		rows, err := db.Query(state.Rebind(`SELECT id, name, description,
			CAST(created_at AS TEXT) as created_at,
			CAST(updated_at AS TEXT) as updated_at,
			trusted_profile_id
			FROM user_groups`+wsClause+` ORDER BY name ASC`), wsArgs...)
		if err != nil {
			http.Error(w, "failed to list groups", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		memberCountStmt, _ := db.Prepare(state.Rebind(`SELECT COUNT(*) FROM user_group_members WHERE group_id = ?`))
		resourceCountStmt, _ := db.Prepare(state.Rebind(`SELECT COUNT(DISTINCT ar.resource_id) FROM access_rules ar JOIN access_rule_groups arg ON arg.rule_id = ar.id WHERE arg.group_id = ?`))
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
			var created, updated sql.NullString
			var trustedProfileID sql.NullString
			if err := rows.Scan(&id, &name, &desc, &created, &updated, &trustedProfileID); err != nil {
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
			updatedAt := ""
			if updated.Valid {
				updatedAt = updated.String
			}
			out = append(out, uiGroup{
				ID:               id,
				Name:             name,
				Type:             "GROUP",
				DisplayLabel:     fmt.Sprintf("Group: %s", name),
				Description:      desc,
				MemberCount:      memberCount,
				ResourceCount:    resourceCount,
				TrustedProfileID: trustedProfileID.String,
				CreatedAt:        createdAt,
				UpdatedAt:        updatedAt,
			})
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req struct {
			Name             string `json:"name"`
			Description      string `json:"description"`
			TrustedProfileID string `json:"trustedProfileId"`
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
		now := isoStringNow()
		wsID := workspaceIDFromContext(r.Context())
		if _, err := db.Exec(state.Rebind(`INSERT INTO user_groups (id, name, description, created_at, updated_at, workspace_id, trusted_profile_id) VALUES (?, ?, ?, ?, ?, ?, ?)`), id, req.Name, req.Description, now, now, wsID, req.TrustedProfileID); err != nil {
			http.Error(w, "failed to create group", http.StatusBadRequest)
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
		switch r.Method {
		case http.MethodPut, http.MethodPatch:
			var req struct {
				Name             string `json:"name"`
				Description      string `json:"description"`
				TrustedProfileID string `json:"trustedProfileId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			now := isoStringNow()
			if _, err := db.Exec(state.Rebind(`UPDATE user_groups SET name = ?, description = ?, trusted_profile_id = ?, updated_at = ? WHERE id = ?`), req.Name, req.Description, req.TrustedProfileID, now, groupID); err != nil {
				http.Error(w, "failed to update group", http.StatusInternalServerError)
				return
			}
			if s.ACLNotify != nil {
				s.ACLNotify.NotifyPolicyChange()
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		case http.MethodDelete:
			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "failed to delete group", http.StatusInternalServerError)
				return
			}
			_, _ = tx.Exec(state.Rebind(`DELETE FROM user_group_members WHERE group_id = ?`), groupID)
			_, _ = tx.Exec(state.Rebind(`DELETE FROM access_rule_groups WHERE group_id = ?`), groupID)
			_, _ = tx.Exec(state.Rebind(`DELETE FROM access_rules WHERE id IN (
				SELECT ar.id
				FROM access_rules ar
				LEFT JOIN access_rule_groups arg ON arg.rule_id = ar.id
				WHERE arg.rule_id IS NULL
			)`))
			if _, err := tx.Exec(state.Rebind(`DELETE FROM user_groups WHERE id = ?`), groupID); err != nil {
				_ = tx.Rollback()
				http.Error(w, "failed to delete group", http.StatusInternalServerError)
				return
			}
			_ = tx.Commit()
			if s.ACLNotify != nil {
				s.ACLNotify.NotifyPolicyChange()
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		case http.MethodGet:
			// fall through to GET handler below
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		row := db.QueryRow(state.Rebind(`SELECT id, name, description,
			CAST(created_at AS TEXT) as created_at,
			CAST(updated_at AS TEXT) as updated_at,
			trusted_profile_id
			FROM user_groups WHERE id = ?`), groupID)
		var id, name, desc string
		var created, updated sql.NullString
		var trustedProfileID sql.NullString
		if err := row.Scan(&id, &name, &desc, &created, &updated, &trustedProfileID); err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{"group": nil, "members": []uiGroupMember{}, "resources": []uiResource{}})
			return
		}
		members := []uiGroupMember{}
		memRows, _ := db.Query(state.Rebind(`SELECT u.id, u.name, u.email FROM user_group_members m JOIN users u ON u.id = m.user_id WHERE m.group_id = ? ORDER BY u.name ASC`), groupID)
		if memRows != nil {
			for memRows.Next() {
				var m uiGroupMember
				if err := memRows.Scan(&m.UserID, &m.UserName, &m.Email); err == nil {
					members = append(members, m)
				}
			}
			memRows.Close()
		}
		resRows, _ := db.Query(state.Rebind(`SELECT r.id, r.name, r.type, r.address, r.protocol, r.port_from, r.port_to, r.alias, r.description, r.remote_network_id, r.firewall_status
			FROM access_rules ar
			JOIN access_rule_groups arg ON arg.rule_id = ar.id
			JOIN resources r ON r.id = ar.resource_id
			WHERE arg.group_id = ?
			GROUP BY r.id
			ORDER BY r.name ASC`), groupID)
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
		updatedAt := ""
		if updated.Valid {
			updatedAt = updated.String
		}
		group := uiGroup{
			ID:               id,
			Name:             name,
			Type:             "GROUP",
			DisplayLabel:     fmt.Sprintf("Group: %s", name),
			Description:      desc,
			MemberCount:      len(members),
			ResourceCount:    len(resources),
			TrustedProfileID: trustedProfileID.String,
			CreatedAt:        createdAt,
			UpdatedAt:        updatedAt,
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
			if _, err := tx.Exec(state.Rebind(`DELETE FROM user_group_members WHERE group_id = ?`), groupID); err != nil {
				_ = tx.Rollback()
				http.Error(w, "failed to update members", http.StatusInternalServerError)
				return
			}
			stmt, _ := tx.Prepare(state.Rebind(`INSERT INTO user_group_members (group_id, user_id, joined_at) VALUES (?, ?, ?)`))
			for _, id := range req.MemberIDs {
				if stmt != nil {
					_, _ = stmt.Exec(groupID, id, time.Now().UTC().Unix())
				}
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
			_, _ = db.Exec(state.Rebind(`DELETE FROM user_group_members WHERE group_id = ? AND user_id = ?`), groupID, userID)
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
		_ = db.QueryRow(state.Rebind(`SELECT name FROM user_groups WHERE id = ?`), groupID).Scan(&groupName)
		if groupName == "" {
			groupName = "Unknown Group"
		}
		now := dateStringNow()
		wsID := workspaceIDFromContext(r.Context())
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "failed to add resources", http.StatusInternalServerError)
			return
		}
		checkStmt, _ := tx.Prepare(state.Rebind(`SELECT ar.id FROM access_rules ar JOIN access_rule_groups arg ON arg.rule_id = ar.id WHERE ar.resource_id = ? AND arg.group_id = ?`))
		insertRule, _ := tx.Prepare(state.Rebind(`INSERT INTO access_rules (id, name, resource_id, enabled, created_at, updated_at, workspace_id) VALUES (?, ?, ?, 1, ?, ?, ?)`))
		insertRuleGroup, _ := tx.Prepare(state.Rebind(`INSERT INTO access_rule_groups (rule_id, group_id) VALUES (?, ?)`))
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
				_, _ = insertRule.Exec(ruleID, fmt.Sprintf("%s access", groupName), resourceID, now, now, wsID)
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
