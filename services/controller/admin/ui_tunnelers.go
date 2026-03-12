package admin

import (
	"net/http"

	"controller/state"
)

func (s *Server) handleUITunnelers(w http.ResponseWriter, r *http.Request) {
	db, ok := s.uiDB(w)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	wsID := workspaceIDFromContext(r.Context())
	wsClause, wsArgs := wsWhereOnly(wsID, "")
	rows, err := db.Query(state.Rebind(`SELECT id, name, status, version, hostname, remote_network_id FROM tunnelers`+wsClause+` ORDER BY name ASC`), wsArgs...)
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
