package admin

import (
	"net/http"
	"time"
)

// Package-level rate limiters for device auth endpoints.
var (
	deviceAuthorizeRL    *rateLimiter
	deviceAuthStartRL    *rateLimiter
	deviceTokenRL        *rateLimiter
	deviceAuthCompleteRL *rateLimiter
	deviceRefreshRL      *rateLimiter
)

func init() {
	deviceAuthorizeRL = newRateLimiter(1*time.Minute, 10)
	deviceAuthStartRL = newRateLimiter(1*time.Minute, 10)
	deviceTokenRL = newRateLimiter(1*time.Minute, 5)
	deviceAuthCompleteRL = newRateLimiter(1*time.Minute, 5)
	deviceRefreshRL = newRateLimiter(1*time.Minute, 10)
}

// RegisterDeviceAuthRoutes registers the device PKCE auth endpoints.
func (s *Server) RegisterDeviceAuthRoutes(mux *http.ServeMux) {
	// v2 endpoints (new Twingate-style flow)
	mux.Handle("/api/device/auth/start", withRateLimit(deviceAuthStartRL, withCORS(http.HandlerFunc(s.handleDeviceAuthStart))))
	mux.Handle("/api/device/auth/complete", withRateLimit(deviceAuthCompleteRL, withCORS(http.HandlerFunc(s.handleDeviceAuthComplete))))

	// Legacy endpoints (kept for backward compat with existing desktop ztna-client)
	mux.Handle("/api/device/authorize", withRateLimit(deviceAuthorizeRL, withCORS(http.HandlerFunc(s.handleDeviceAuthorize))))
	mux.Handle("/api/device/callback", http.HandlerFunc(s.handleDeviceCallback))
	mux.Handle("/api/device/token", withRateLimit(deviceTokenRL, withCORS(http.HandlerFunc(s.handleDeviceToken))))

	mux.Handle("/api/device/refresh", withRateLimit(deviceRefreshRL, withCORS(http.HandlerFunc(s.handleDeviceRefresh))))
	mux.Handle("/api/device/revoke", withCORS(s.deviceAuth(http.HandlerFunc(s.handleDeviceRevoke))))
	mux.Handle("/api/device/me", withCORS(s.deviceAuth(http.HandlerFunc(s.handleDeviceMe))))
	mux.Handle("/api/device/sync", withCORS(s.deviceAuth(http.HandlerFunc(s.handleDeviceSync))))
	mux.Handle("/api/device/enroll-cert", withCORS(s.deviceAuth(http.HandlerFunc(s.handleDeviceEnrollCert))))
	mux.Handle("/api/device/posture", withCORS(s.deviceAuth(http.HandlerFunc(s.handleDevicePostureReport))))
}
