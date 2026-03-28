package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	controllerpb "controller/gen/controllerpb"

	"controller/ca"
	"controller/state"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EnrollmentServer implements controller.v1.EnrollmentService.
type EnrollmentServer struct {
	controllerpb.UnimplementedEnrollmentServiceServer

	CA           *ca.CA
	CAPEM        []byte
	TrustDomain  string
	DB           *sql.DB
	Tokens       *state.TokenStore
	Registry     *state.Registry
	Notifier     AgentNotifier
	Workspaces   *state.WorkspaceStore // nil if multi-tenant disabled
	SystemDomain string                // e.g. "zerotrust.com"
	CRL          *ca.CRL
}

type AgentNotifier interface {
	NotifyAgentAllowed(agentID, spiffeID, version, hostname, ip string)
}

// NewEnrollmentServer creates a new EnrollmentServer.
func NewEnrollmentServer(caInst *ca.CA, caPEM []byte, trustDomain string, tokens *state.TokenStore, registry *state.Registry, notifier AgentNotifier) *EnrollmentServer {
	return &EnrollmentServer{
		CA:          caInst,
		CAPEM:       caPEM,
		TrustDomain: trustDomain,
		Tokens:      tokens,
		Registry:    registry,
		Notifier:    notifier,
	}
}

// EnrollConnector enrolls a connector and issues a short-lived certificate.
// If the enrollment token belongs to a workspace, the cert is issued by the workspace CA
// with the workspace's trust domain. Otherwise, the global intermediate CA is used.
func (s *EnrollmentServer) EnrollConnector(
	ctx context.Context,
	req *controllerpb.EnrollRequest,
) (*controllerpb.EnrollResponse, error) {

	if !validID(req.GetId()) {
		return nil, status.Error(codes.InvalidArgument, "missing connector id")
	}
	if req.GetPrivateIp() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing private ip")
	}
	if req.GetVersion() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing version")
	}

	pubKey, err := parsePublicKey(req.GetPublicKey())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid public key: %v", err)
	}
	logPublicKey("enroll-connector", pubKey, req.GetPublicKey())

	workspaceID, err := s.authorizeConnectorTokenWithWorkspace(req.GetToken(), req.GetId())
	if err != nil {
		return nil, err
	}

	// Check if this connector was previously enrolled with a different workspace
	// This prevents workspace hopping attacks where an entity tries to switch workspaces
	if workspaceID != "" && s.Registry != nil {
		if rec, ok := s.Registry.Get(req.GetId()); ok && rec.WorkspaceID != "" {
			if rec.WorkspaceID != workspaceID {
				return nil, status.Error(codes.PermissionDenied,
					"connector was enrolled with a different workspace; use original workspace token")
			}
		}
	}

	// Determine which CA and trust domain to use.
	issuerCA := s.CA
	issuerCAPEM := s.CAPEM
	trustDomain := s.TrustDomain

	if workspaceID != "" && s.Workspaces != nil {
		ws, err := s.Workspaces.GetWorkspace(workspaceID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "workspace lookup failed: %v", err)
		}
		wsCA, err := ca.LoadCA([]byte(ws.CACertPEM), []byte(ws.CAKeyPEM))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "workspace CA load failed: %v", err)
		}
		issuerCA = wsCA
		issuerCAPEM = []byte(ws.CACertPEM)
		trustDomain = ws.TrustDomain
	}

	spiffeID := fmt.Sprintf("spiffe://%s/connector/%s", trustDomain, req.GetId())
	var ipAddrs []net.IP
	if ip := net.ParseIP(req.GetPrivateIp()); ip != nil {
		ipAddrs = []net.IP{ip}
	}

	certPEM, err := ca.IssueWorkloadCert(
		issuerCA,
		spiffeID,
		pubKey,
		5*time.Minute,
		nil,
		ipAddrs,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "certificate issuance failed: %v", err)
	}
	logIssuedCert("enroll-connector", spiffeID, certPEM)

	logEnrollment("connector", req.GetId(), req.GetPrivateIp(), req.GetVersion())
	if s.Registry != nil {
		s.Registry.RegisterWithWorkspace(req.GetId(), req.GetPrivateIp(), "", req.GetVersion(), workspaceID)
	}

	return &controllerpb.EnrollResponse{
		Certificate:   certPEM,
		CaCertificate: issuerCAPEM,
	}, nil
}

// EnrollTunneler enrolls an agent and issues a short-lived certificate.
func (s *EnrollmentServer) EnrollTunneler(
	ctx context.Context,
	req *controllerpb.EnrollRequest,
) (*controllerpb.EnrollResponse, error) {

	if !validID(req.GetId()) {
		return nil, status.Error(codes.InvalidArgument, "missing agent id")
	}
	if req.GetToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing enrollment token")
	}

	pubKey, err := parsePublicKey(req.GetPublicKey())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid public key: %v", err)
	}
	logPublicKey("enroll-agent", pubKey, req.GetPublicKey())

	workspaceID, err := s.authorizeConnectorTokenWithWorkspace(req.GetToken(), req.GetId())
	if err != nil {
		return nil, err
	}

	issuerCA := s.CA
	issuerCAPEM := s.CAPEM
	trustDomain := s.TrustDomain

	if workspaceID != "" && s.Workspaces != nil {
		ws, err := s.Workspaces.GetWorkspace(workspaceID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "workspace lookup failed: %v", err)
		}
		wsCA, err := ca.LoadCA([]byte(ws.CACertPEM), []byte(ws.CAKeyPEM))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "workspace CA load failed: %v", err)
		}
		issuerCA = wsCA
		issuerCAPEM = []byte(ws.CACertPEM)
		trustDomain = ws.TrustDomain
	}

	spiffeID := fmt.Sprintf("spiffe://%s/agent/%s", trustDomain, req.GetId())

	certPEM, err := ca.IssueWorkloadCert(
		issuerCA,
		spiffeID,
		pubKey,
		5*time.Minute,
		nil,
		nil,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "certificate issuance failed: %v", err)
	}
	logIssuedCert("enroll-agent", spiffeID, certPEM)
	if s.Notifier != nil {
		s.Notifier.NotifyAgentAllowed(req.GetId(), spiffeID, req.GetVersion(), req.GetPrivateIp(), req.GetPrivateIp())
	}

	return &controllerpb.EnrollResponse{
		Certificate:   certPEM,
		CaCertificate: issuerCAPEM,
	}, nil
}

// Renew re-issues a certificate for an existing workload based on its SPIFFE identity.
func (s *EnrollmentServer) Renew(
	ctx context.Context,
	req *controllerpb.EnrollRequest,
) (*controllerpb.EnrollResponse, error) {

	if !validID(req.GetId()) {
		return nil, status.Error(codes.InvalidArgument, "missing id")
	}

	pubKey, err := parsePublicKey(req.GetPublicKey())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid public key: %v", err)
	}
	logPublicKey("renew", pubKey, req.GetPublicKey())

	callerSPIFFE, _ := SPIFFEIDFromContext(ctx)
	role, id, err := s.identityFromContextMultiDomain(ctx)
	if err != nil {
		return nil, err
	}
	if id != req.GetId() {
		return nil, status.Error(codes.PermissionDenied, "id mismatch for renewal")
	}

	// Check if the connector/agent is revoked before renewing.
	if s.CRL != nil && s.DB != nil {
		table := "connectors"
		if role == "agent" {
			table = "agents"
		}
		var revoked int
		if err := s.DB.QueryRow(state.Rebind(fmt.Sprintf(`SELECT revoked FROM %s WHERE id = ?`, table)), req.GetId()).Scan(&revoked); err == nil && revoked != 0 {
			return nil, status.Error(codes.PermissionDenied, "certificate renewal denied: entity revoked")
		}
	}

	// Use the same trust domain from the caller's existing SPIFFE ID.
	spiffeID := callerSPIFFE

	// Determine which CA to use for renewal.
	// Primary: Look up workspace from Registry (stored during initial enrollment)
	// Fallback: Derive workspace from SPIFFE trust domain
	issuerCA := s.CA
	issuerCAPEM := s.CAPEM

	var workspaceID string
	// Registry lookup works for both connectors and agents
	if (role == "connector" || role == "agent") && s.Registry != nil {
		if rec, ok := s.Registry.Get(req.GetId()); ok && rec.WorkspaceID != "" {
			workspaceID = rec.WorkspaceID
		}
	}

	// Fallback to SPIFFE-based lookup if Registry doesn't have workspace
	if workspaceID == "" && s.Workspaces != nil {
		if wsID, ok := s.workspaceIDForSPIFFE(spiffeID); ok {
			workspaceID = wsID
		}
	}

	// Load workspace CA if we have a workspace ID
	if workspaceID != "" && s.Workspaces != nil {
		if ws, err := s.Workspaces.GetWorkspace(workspaceID); err == nil {
			if wsCA, err := ca.LoadCA([]byte(ws.CACertPEM), []byte(ws.CAKeyPEM)); err == nil {
				issuerCA = wsCA
				issuerCAPEM = []byte(ws.CACertPEM)
			}
		}
	}

	ttl := 5 * time.Minute
	var ipAddrs []net.IP
	if (role == "connector" || role == "agent") && s.Registry != nil {
		if rec, ok := s.Registry.Get(req.GetId()); ok {
			if ip := net.ParseIP(rec.PrivateIP); ip != nil {
				ipAddrs = []net.IP{ip}
			}
		}
	}

	certPEM, err := ca.IssueWorkloadCert(issuerCA, spiffeID, pubKey, ttl, nil, ipAddrs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "certificate renewal failed: %v", err)
	}
	logIssuedCert("renew", spiffeID, certPEM)
	if role == "agent" && s.Notifier != nil {
		s.Notifier.NotifyAgentAllowed(req.GetId(), spiffeID, req.GetVersion(), req.GetPrivateIp(), req.GetPrivateIp())
	}
	if role == "connector" && s.DB != nil {
		nowISO := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		_, _ = s.DB.Exec(
			state.Rebind(`INSERT INTO connector_logs (connector_id, timestamp, message) VALUES (?, ?, ?)`),
			req.GetId(),
			nowISO,
			"certificate renewed successfully",
		)
	}

	return &controllerpb.EnrollResponse{
		Certificate:   certPEM,
		CaCertificate: issuerCAPEM,
	}, nil
}

// parsePublicKey parses a PEM-encoded public key.
func parsePublicKey(pemBytes []byte) (interface{}, error) {
	if len(pemBytes) == 0 {
		return nil, fmt.Errorf("public key is empty")
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return pub, nil
}

func (s *EnrollmentServer) authorize(ctx context.Context, expectedRole, expectedID string) error {
	role, id, err := s.identityFromContext(ctx)
	if err != nil {
		return err
	}
	if role != expectedRole {
		return status.Error(codes.PermissionDenied, "role not permitted for enrollment")
	}
	if id != expectedID {
		return status.Error(codes.PermissionDenied, "id mismatch for enrollment")
	}
	return nil
}

func (s *EnrollmentServer) authorizeConnectorTokenWithWorkspace(token, connectorID string) (string, error) {
	if s.Tokens == nil {
		return "", status.Error(codes.FailedPrecondition, "token service unavailable")
	}
	wsID, err := s.Tokens.ConsumeTokenWithWorkspace(token, connectorID)
	if err != nil {
		return "", status.Error(codes.PermissionDenied, "invalid enrollment token")
	}
	return wsID, nil
}

func (s *EnrollmentServer) workspaceIDForSPIFFE(spiffeID string) (string, bool) {
	if s.Workspaces == nil {
		return "", false
	}

	trimmed := strings.TrimPrefix(spiffeID, "spiffe://")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 3 {
		return "", false
	}

	trustDomain := strings.TrimSpace(parts[0])
	if trustDomain == "" {
		return "", false
	}

	// Try exact trust domain match first (works for workspace-specific trust domains)
	ws, err := s.Workspaces.GetWorkspaceByTrustDomain(trustDomain)
	if err == nil && ws != nil && ws.ID != "" {
		return ws.ID, true
	}

	// If trust domain matches default, try to find workspace that uses it
	// This handles the case where a workspace has the same trust domain as the global default
	if trustDomain == s.TrustDomain && s.Workspaces != nil {
		// List workspaces and find one with matching trust domain
		// Note: This is a fallback path; primary lookup should be via Registry
		workspaces, err := s.Workspaces.ListWorkspacesForUser("")
		if err == nil {
			for _, ws := range workspaces {
				if ws.TrustDomain == trustDomain {
					return ws.ID, true
				}
			}
		}
	}

	return "", false
}

func (s *EnrollmentServer) identityFromContext(ctx context.Context) (string, string, error) {
	spiffeID, ok := SPIFFEIDFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Unauthenticated, "missing SPIFFE identity")
	}

	role, ok := RoleFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Unauthenticated, "missing SPIFFE role")
	}

	id := strings.TrimPrefix(spiffeID, fmt.Sprintf("spiffe://%s/%s/", s.TrustDomain, role))
	if id == "" || strings.Contains(id, "/") {
		return "", "", status.Error(codes.Unauthenticated, "invalid SPIFFE id")
	}

	return role, id, nil
}

// identityFromContextMultiDomain extracts role and ID from SPIFFE, supporting any trust domain.
func (s *EnrollmentServer) identityFromContextMultiDomain(ctx context.Context) (string, string, error) {
	spiffeID, ok := SPIFFEIDFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Unauthenticated, "missing SPIFFE identity")
	}
	role, ok := RoleFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Unauthenticated, "missing SPIFFE role")
	}
	// Parse ID from spiffe://DOMAIN/ROLE/ID
	trimmed := strings.TrimPrefix(spiffeID, "spiffe://")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 3 {
		return "", "", status.Error(codes.Unauthenticated, "invalid SPIFFE id")
	}
	id := parts[2]
	if id == "" {
		return "", "", status.Error(codes.Unauthenticated, "invalid SPIFFE id")
	}
	return role, id, nil
}

func logEnrollment(role, id, privateIP, version string) {
	// Keep as a structured line to aid operator log parsing.
	fmt.Printf("enrollment: role=%s id=%s private_ip=%s version=%s\n", role, id, privateIP, version)
}

func logPublicKey(scope string, pubKey interface{}, rawPEM []byte) {
	algo := "unknown"
	bits := 0
	switch k := pubKey.(type) {
	case *rsa.PublicKey:
		algo = "rsa"
		bits = k.N.BitLen()
	case *ecdsa.PublicKey:
		algo = "ecdsa"
		if k.Curve == elliptic.P256() {
			bits = 256
		} else if k.Curve == elliptic.P384() {
			bits = 384
		} else if k.Curve == elliptic.P521() {
			bits = 521
		}
	}
	fp := sha256.Sum256(rawPEM)
	log.Printf("%s public_key: alg=%s bits=%d sha256=%s", scope, algo, bits, hex.EncodeToString(fp[:8]))
}

func logIssuedCert(scope, spiffeID string, certPEM []byte) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		log.Printf("%s issued_cert: spiffe=%s parse_error=invalid_pem", scope, spiffeID)
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		log.Printf("%s issued_cert: spiffe=%s parse_error=%v", scope, spiffeID, err)
		return
	}
	log.Printf(
		"%s issued_cert: spiffe=%s serial=%s not_after=%s",
		scope,
		spiffeID,
		cert.SerialNumber.String(),
		cert.NotAfter.Format(time.RFC3339),
	)
}

func validID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}
