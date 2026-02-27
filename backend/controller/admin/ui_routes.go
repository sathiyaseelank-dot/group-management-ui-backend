package admin

import "net/http"

func (s *Server) RegisterUIRoutes(mux *http.ServeMux) {
	mux.Handle("/api/users", withCORS(http.HandlerFunc(s.handleUIUsers)))
	mux.Handle("/api/groups", withCORS(http.HandlerFunc(s.handleUIGroups)))
	mux.Handle("/api/groups/", withCORS(http.HandlerFunc(s.handleUIGroupsSubroutes)))
	mux.Handle("/api/resources", withCORS(http.HandlerFunc(s.handleUIResources)))
	mux.Handle("/api/resources/", withCORS(http.HandlerFunc(s.handleUIResourcesSubroutes)))
	mux.Handle("/api/access-rules", withCORS(http.HandlerFunc(s.handleUIAccessRules)))
	mux.Handle("/api/access-rules/", withCORS(http.HandlerFunc(s.handleUIAccessRulesSubroutes)))
	mux.Handle("/api/remote-networks", withCORS(http.HandlerFunc(s.handleUIRemoteNetworks)))
	mux.Handle("/api/remote-networks/", withCORS(http.HandlerFunc(s.handleUIRemoteNetworksSubroutes)))
	mux.Handle("/api/connectors", withCORS(http.HandlerFunc(s.handleUIConnectors)))
	mux.Handle("/api/connectors/", withCORS(http.HandlerFunc(s.handleUIConnectorsSubroutes)))
	mux.Handle("/api/tunnelers", withCORS(http.HandlerFunc(s.handleUITunnelers)))
	mux.Handle("/api/subjects", withCORS(http.HandlerFunc(s.handleUISubjects)))
	mux.Handle("/api/service-accounts", withCORS(http.HandlerFunc(s.handleUIServiceAccounts)))
	mux.Handle("/api/policy/compile/", withCORS(http.HandlerFunc(s.handleUIPolicyCompile)))
	mux.Handle("/api/policy/acl/", withCORS(http.HandlerFunc(s.handleUIPolicyACL)))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
