package admin

import "net/http"

// RegisterDeviceAuthRoutes registers the device PKCE auth endpoints.
func (s *Server) RegisterDeviceAuthRoutes(mux *http.ServeMux) {
	mux.Handle("/api/device/authorize", withCORS(http.HandlerFunc(s.handleDeviceAuthorize)))
	mux.Handle("/api/device/callback", http.HandlerFunc(s.handleDeviceCallback))
	mux.Handle("/api/device/token", withCORS(http.HandlerFunc(s.handleDeviceToken)))
	mux.Handle("/api/device/refresh", withCORS(http.HandlerFunc(s.handleDeviceRefresh)))
	mux.Handle("/api/device/revoke", withCORS(http.HandlerFunc(s.handleDeviceRevoke)))
}
