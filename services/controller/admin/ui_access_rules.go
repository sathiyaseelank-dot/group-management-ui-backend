package admin

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"controller/state"
)

func (s *Server) handleUIAccessRules(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		wsID := workspaceIDFromContext(r.Context())
		wsClause, wsArgs := wsWhereOnly(wsID, "ar")
		rows, err := db.Query(state.Rebind(`SELECT ar.id, ar.name, ar.resource_id, ar.enabled, ar.created_at, ar.updated_at FROM access_rules ar`+wsClause+` ORDER BY ar.created_at ASC`), wsArgs...)
		if err != nil {
			http.Error(w, "failed to list access rules", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		groupStmt, _ := db.Prepare(state.Rebind(`SELECT group_id FROM access_rule_groups WHERE rule_id = ? ORDER BY group_id ASC`))
		out := []uiAccessRule{}
		for rows.Next() {
			var ar uiAccessRule
			var enabled int
			if err := rows.Scan(&ar.ID, &ar.Name, &ar.ResourceID, &enabled, &ar.CreatedAt, &ar.UpdatedAt); err == nil {
				ar.Enabled = enabled != 0
				ar.AllowedGroups = []string{}
				if groupStmt != nil {
					gRows, _ := groupStmt.Query(ar.ID)
					for gRows != nil && gRows.Next() {
						var gid string
						if err := gRows.Scan(&gid); err == nil {
							ar.AllowedGroups = append(ar.AllowedGroups, gid)
						}
					}
					if gRows != nil {
						gRows.Close()
					}
				}
				out = append(out, ar)
			}
		}
		if groupStmt != nil {
			groupStmt.Close()
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
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
			log.Printf("access rule: begin tx: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wsID := workspaceIDFromContext(r.Context())
		_, err = tx.Exec(state.Rebind(`INSERT INTO access_rules (id, name, resource_id, enabled, created_at, updated_at, workspace_id) VALUES (?, ?, ?, ?, ?, ?, ?)`), ruleID, req.Name, req.ResourceID, enabled, now, now, wsID)
		if err != nil {
			_ = tx.Rollback()
			log.Printf("access rule: insert rule: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		stmt, err := tx.Prepare(state.Rebind(`INSERT INTO access_rule_groups (rule_id, group_id) VALUES (?, ?)`))
		if err != nil {
			_ = tx.Rollback()
			log.Printf("access rule: prepare group insert: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, gid := range req.GroupIDs {
			if _, err := stmt.Exec(ruleID, gid); err != nil {
				log.Printf("access rule: insert group %s: %v", gid, err)
			}
		}
		stmt.Close()
		_ = tx.Commit()
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		s.audit(r, "access_rule.create", ruleID, "ok")
		writeJSON(w, http.StatusOK, uiAccessRule{
			ID:            ruleID,
			Name:          req.Name,
			ResourceID:    req.ResourceID,
			AllowedGroups: req.GroupIDs,
			Enabled:       req.Enabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUIAccessRulesSubroutes(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	wsID := workspaceIDFromContext(r.Context())
	wsClause, wsArgs := wsWhere(wsID, "")
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
		delArgs := append([]interface{}{ruleID}, wsArgs...)
		_, _ = db.Exec(state.Rebind(`DELETE FROM access_rules WHERE id = ?`+wsClause), delArgs...)
		if s.ACLNotify != nil {
			s.ACLNotify.NotifyPolicyChange()
		}
		s.audit(r, "access_rule.delete", ruleID, "ok")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if len(parts) == 2 && parts[1] == "identity-count" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var count int
		err := db.QueryRow(state.Rebind(`SELECT COUNT(DISTINCT u.id)
			FROM access_rule_groups arg
			JOIN user_group_members gm ON gm.group_id = arg.group_id
			JOIN users u ON u.id = gm.user_id
			WHERE arg.rule_id = ? AND u.certificate_identity IS NOT NULL`), ruleID).Scan(&count)
		if err != nil {
			http.Error(w, "failed to compute identity count", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"count": count})
		return
	}
	http.Error(w, "unknown subresource", http.StatusNotFound)
}
