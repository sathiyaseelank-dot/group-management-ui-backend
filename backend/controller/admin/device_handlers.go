package admin

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"strings"

	deviceca "controller/ca"
	"controller/state"
)

type deviceServer struct {
	devices     *state.DeviceStore
	deviceCA    *x509.Certificate
	deviceCAKey *rsa.PrivateKey
	adminToken  string
}

// RegisterDeviceRoutes registers device management and enrollment endpoints.
func RegisterDeviceRoutes(mux *http.ServeMux, ds *state.DeviceStore, caCert *x509.Certificate, caKey *rsa.PrivateKey, adminToken string) {
	srv := &deviceServer{
		devices:     ds,
		deviceCA:    caCert,
		deviceCAKey: caKey,
		adminToken:  adminToken,
	}
	// Admin endpoints (Bearer token required)
	mux.Handle("/api/admin/devices", withCORS(http.HandlerFunc(srv.handleAdminDevices)))
	mux.Handle("/api/admin/devices/", withCORS(http.HandlerFunc(srv.handleAdminDeviceByID)))
	// Public enrollment (no auth)
	mux.Handle("/api/devices/enroll", withCORS(http.HandlerFunc(srv.handleEnroll)))
	// UI listing (CORS, no auth — consumed by frontend BFF)
	mux.Handle("/api/devices", withCORS(http.HandlerFunc(srv.handleListDevices)))
}

func (s *deviceServer) handleAdminDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.checkAdminAuth(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	devices, err := s.devices.ListAllDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

func (s *deviceServer) handleAdminDeviceByID(w http.ResponseWriter, r *http.Request) {
	if !s.checkAdminAuth(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// path: /api/admin/devices/{id}/trust  or  /api/admin/devices/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/devices/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]

	if len(parts) == 2 && parts[1] == "trust" && r.Method == http.MethodPatch {
		var body struct {
			TrustState string `json:"trust_state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if body.TrustState != "trusted" && body.TrustState != "blocked" {
			http.Error(w, "trust_state must be 'trusted' or 'blocked'", http.StatusBadRequest)
			return
		}
		if err := s.devices.SetTrustState(id, body.TrustState); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"trust_state": body.TrustState})
		return
	}

	if r.Method == http.MethodDelete {
		if err := s.devices.DeleteDevice(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	http.Error(w, "not found", http.StatusNotFound)
}

func (s *deviceServer) handleListDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	devices, err := s.devices.ListAllDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

func (s *deviceServer) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.deviceCA == nil || s.deviceCAKey == nil {
		http.Error(w, "device CA not configured", http.StatusInternalServerError)
		return
	}

	var body struct {
		VerificationToken string `json:"verification_token"`
		DeviceName        string `json:"device_name"`
		CSRPEM            string `json:"csr_pem"`
		OS                string `json:"os"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.CSRPEM == "" || body.DeviceName == "" {
		http.Error(w, "device_name and csr_pem required", http.StatusBadRequest)
		return
	}
	if body.VerificationToken == "" {
		http.Error(w, "verification_token required", http.StatusBadRequest)
		return
	}

	userID, err := s.devices.ConsumeVerificationToken(body.VerificationToken)
	if err != nil {
		http.Error(w, "invalid or expired verification token", http.StatusUnauthorized)
		return
	}

	certPEM, fingerprint, err := deviceca.SignDeviceCSR([]byte(body.CSRPEM), s.deviceCA, s.deviceCAKey)
	if err != nil {
		http.Error(w, "sign CSR: "+err.Error(), http.StatusBadRequest)
		return
	}

	deviceID, trustState, err := s.devices.EnrollDevice(userID, body.DeviceName, fingerprint, body.CSRPEM, string(certPEM), body.OS)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"device_id":   deviceID,
		"trust_state": trustState,
		"cert_pem":    string(certPEM),
	})
}

// DeviceCertFingerprint computes the SHA256 fingerprint of a certificate's public key.
// Used on the :8444 mTLS endpoint to look up a device by its presented client cert.
func DeviceCertFingerprint(cert *x509.Certificate) (string, error) {
	pubDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(pubDER)
	h := make([]byte, len(sum))
	copy(h, sum[:])
	out := make([]byte, len(h)*2)
	const hexChars = "0123456789abcdef"
	for i, b := range h {
		out[i*2] = hexChars[b>>4]
		out[i*2+1] = hexChars[b&0xf]
	}
	return string(out), nil
}

// BuildDeviceMTLSConfig returns a tls.Config for the :8444 server that requires
// and verifies client certificates signed by the device CA.
func BuildDeviceMTLSConfig(deviceCACert *x509.Certificate, serverCert tls.Certificate) *tls.Config {
	pool := x509.NewCertPool()
	pool.AddCert(deviceCACert)
	return &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}
}

func (s *deviceServer) checkAdminAuth(r *http.Request) bool {
	return r.Header.Get("Authorization") == "Bearer "+s.adminToken
}
