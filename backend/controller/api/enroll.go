package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
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

	CA          *ca.CA
	CAPEM       []byte
	TrustDomain string
	Tokens      *state.TokenStore
	Registry    *state.Registry
	Notifier    TunnelerNotifier
}

type TunnelerNotifier interface {
	NotifyTunnelerAllowed(tunnelerID, spiffeID string)
}

// NewEnrollmentServer creates a new EnrollmentServer.
func NewEnrollmentServer(caInst *ca.CA, caPEM []byte, trustDomain string, tokens *state.TokenStore, registry *state.Registry, notifier TunnelerNotifier) *EnrollmentServer {
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

	if err := s.authorizeConnectorToken(req.GetToken(), req.GetId()); err != nil {
		return nil, err
	}

	spiffeID := fmt.Sprintf(
		"spiffe://%s/connector/%s",
		s.TrustDomain,
		req.GetId(),
	)
	var ipAddrs []net.IP
	if ip := net.ParseIP(req.GetPrivateIp()); ip != nil {
		ipAddrs = []net.IP{ip}
	}

	certPEM, err := ca.IssueWorkloadCert(
		s.CA,
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

	// Registration side-effect: log enrollment details.
	logEnrollment("connector", req.GetId(), req.GetPrivateIp(), req.GetVersion())
	if s.Registry != nil {
		s.Registry.Register(req.GetId(), req.GetPrivateIp(), req.GetVersion())
	}

	return &controllerpb.EnrollResponse{
		Certificate:   certPEM,
		CaCertificate: s.CAPEM,
	}, nil
}

// EnrollTunneler enrolls a tunneler and issues a short-lived certificate.
func (s *EnrollmentServer) EnrollTunneler(
	ctx context.Context,
	req *controllerpb.EnrollRequest,
) (*controllerpb.EnrollResponse, error) {

	if !validID(req.GetId()) {
		return nil, status.Error(codes.InvalidArgument, "missing tunneler id")
	}
	if req.GetToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing enrollment token")
	}

	pubKey, err := parsePublicKey(req.GetPublicKey())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid public key: %v", err)
	}
	logPublicKey("enroll-tunneler", pubKey, req.GetPublicKey())

	if err := s.authorizeConnectorToken(req.GetToken(), req.GetId()); err != nil {
		return nil, err
	}

	spiffeID := fmt.Sprintf(
		"spiffe://%s/tunneler/%s",
		s.TrustDomain,
		req.GetId(),
	)

	certPEM, err := ca.IssueWorkloadCert(
		s.CA,
		spiffeID,
		pubKey,
		30*time.Minute,
		nil,
		nil,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "certificate issuance failed: %v", err)
	}
	logIssuedCert("enroll-tunneler", spiffeID, certPEM)
	if s.Notifier != nil {
		s.Notifier.NotifyTunnelerAllowed(req.GetId(), spiffeID)
	}

	return &controllerpb.EnrollResponse{
		Certificate:   certPEM,
		CaCertificate: s.CAPEM,
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

	role, id, err := s.identityFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if id != req.GetId() {
		return nil, status.Error(codes.PermissionDenied, "id mismatch for renewal")
	}

	spiffeID := fmt.Sprintf("spiffe://%s/%s/%s", s.TrustDomain, role, req.GetId())

	ttl := 30 * time.Minute
	if role == "connector" {
		ttl = 5 * time.Minute
	}
	var ipAddrs []net.IP
	if role == "connector" && s.Registry != nil {
		if rec, ok := s.Registry.Get(req.GetId()); ok {
			if ip := net.ParseIP(rec.PrivateIP); ip != nil {
				ipAddrs = []net.IP{ip}
			}
		}
	}

	certPEM, err := ca.IssueWorkloadCert(s.CA, spiffeID, pubKey, ttl, nil, ipAddrs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "certificate renewal failed: %v", err)
	}
	logIssuedCert("renew", spiffeID, certPEM)

	return &controllerpb.EnrollResponse{
		Certificate:   certPEM,
		CaCertificate: s.CAPEM,
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

func (s *EnrollmentServer) authorizeConnectorToken(token, connectorID string) error {
	if s.Tokens == nil {
		return status.Error(codes.FailedPrecondition, "token service unavailable")
	}
	if err := s.Tokens.ConsumeToken(token, connectorID); err != nil {
		return status.Error(codes.PermissionDenied, "invalid enrollment token")
	}
	return nil
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
