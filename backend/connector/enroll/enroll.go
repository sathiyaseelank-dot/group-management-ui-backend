package enroll

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"connector/internal/tlsutil"
	controllerpb "controller/gen/controllerpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Config controls enrollment behavior.
type Config struct {
	ControllerAddr string
	ConnectorID    string
	TrustDomain    string
	Token          string
	PrivateIP      string
	Version        string
}

// Run performs one-time connector enrollment with the controller.
func Run() error {
	cfg, err := ConfigFromEnvEnroll()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cert, _, caPEM, spiffeID, err := Enroll(ctx, cfg)
	if err != nil {
		return err
	}

	_ = cert
	_ = caPEM
	fmt.Printf("Enrolled connector with SPIFFE ID: %s\n", spiffeID)
	return nil
}

// ConfigFromEnvEnroll builds Config for enroll mode.
func ConfigFromEnvEnroll() (Config, error) {
	controllerAddr := os.Getenv("CONTROLLER_ADDR")
	connectorID := os.Getenv("CONNECTOR_ID")
	trustDomain := os.Getenv("TRUST_DOMAIN")
	token := os.Getenv("ENROLLMENT_TOKEN")
	if trustDomain == "" {
		trustDomain = "mycorp.internal"
	}
	trustDomain = normalizeTrustDomain(trustDomain)

	if controllerAddr == "" {
		return Config{}, fmt.Errorf("CONTROLLER_ADDR is not set")
	}
	if connectorID == "" {
		return Config{}, fmt.Errorf("CONNECTOR_ID is not set")
	}
	if token == "" {
		cred, err := ReadCredential("ENROLLMENT_TOKEN")
		if err != nil {
			return Config{}, err
		}
		token = cred
	}
	if token == "" {
		return Config{}, fmt.Errorf("ENROLLMENT_TOKEN is not set")
	}

	privateIP, err := ResolvePrivateIP(controllerAddr)
	if err != nil {
		return Config{}, err
	}

	version := ResolveVersion()

	return Config{
		ControllerAddr: controllerAddr,
		ConnectorID:    connectorID,
		TrustDomain:    trustDomain,
		Token:          token,
		PrivateIP:      privateIP,
		Version:        version,
	}, nil
}

// ConfigFromEnvRun builds Config for run mode.
func ConfigFromEnvRun() (Config, error) {
	controllerAddr := os.Getenv("CONTROLLER_ADDR")
	connectorID := os.Getenv("CONNECTOR_ID")
	trustDomain := os.Getenv("TRUST_DOMAIN")
	if trustDomain == "" {
		trustDomain = "mycorp.internal"
	}
	trustDomain = normalizeTrustDomain(trustDomain)

	if controllerAddr == "" {
		return Config{}, fmt.Errorf("CONTROLLER_ADDR is not set")
	}
	if connectorID == "" {
		return Config{}, fmt.Errorf("CONNECTOR_ID is not set")
	}

	privateIP, err := ResolvePrivateIP(controllerAddr)
	if err != nil {
		return Config{}, err
	}

	version := ResolveVersion()

	return Config{
		ControllerAddr: controllerAddr,
		ConnectorID:    connectorID,
		TrustDomain:    trustDomain,
		PrivateIP:      privateIP,
		Version:        version,
	}, nil
}

// Enroll performs enrollment and returns the issued workload certificate.
func Enroll(ctx context.Context, cfg Config) (tls.Certificate, []byte, []byte, string, error) {
	// ---- generate key pair (in-memory only) ----
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, nil, "", fmt.Errorf("failed to generate key pair: %w", err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		return tls.Certificate{}, nil, nil, "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	})

	localCAPEM, err := loadExplicitCA()
	if err != nil {
		return tls.Certificate{}, nil, nil, "", err
	}
	rootPool, err := tlsutil.RootPoolFromPEM(localCAPEM)
	if err != nil {
		return tls.Certificate{}, nil, nil, "", err
	}

	// ---- TLS config for enrollment ----
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    rootPool,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			return tlsutil.VerifyPeerSPIFFE(rawCerts, verifiedChains, cfg.TrustDomain, "controller")
		},
	}

	// ---- connect to controller ----
	conn, err := grpc.DialContext(
		ctx,
		cfg.ControllerAddr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		return tls.Certificate{}, nil, nil, "", fmt.Errorf("failed to connect to controller: %w", err)
	}
	defer conn.Close()

	client := controllerpb.NewEnrollmentServiceClient(conn)

	resp, err := client.EnrollConnector(ctx, &controllerpb.EnrollRequest{
		Id:        cfg.ConnectorID,
		PublicKey: pubPEM,
		Token:     cfg.Token,
		PrivateIp: cfg.PrivateIP,
		Version:   cfg.Version,
	})
	if err != nil {
		return tls.Certificate{}, nil, nil, "", fmt.Errorf("enrollment RPC failed: %w", err)
	}

	if len(resp.Certificate) == 0 {
		return tls.Certificate{}, nil, nil, "", fmt.Errorf("controller returned empty certificate")
	}
	if len(resp.CaCertificate) == 0 {
		return tls.Certificate{}, nil, nil, "", fmt.Errorf("controller returned empty CA certificate")
	}

	if _, err := tlsutil.ParseAndValidateCA(resp.CaCertificate); err != nil {
		return tls.Certificate{}, nil, nil, "", fmt.Errorf("invalid internal CA: %w", err)
	}

	// ---- basic validation of returned cert ----
	block, _ := pem.Decode(resp.Certificate)
	if block == nil || block.Type != "CERTIFICATE" {
		return tls.Certificate{}, nil, nil, "", fmt.Errorf("invalid certificate PEM from controller")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return tls.Certificate{}, nil, nil, "", fmt.Errorf("failed to parse issued certificate: %w", err)
	}

	if len(cert.URIs) != 1 {
		return tls.Certificate{}, nil, nil, "", fmt.Errorf("issued certificate must contain exactly one SPIFFE ID")
	}

	workloadCert := tls.Certificate{
		Certificate: [][]byte{block.Bytes},
		PrivateKey:  privKey,
	}

	return workloadCert, resp.Certificate, resp.CaCertificate, cert.URIs[0].String(), nil
}
