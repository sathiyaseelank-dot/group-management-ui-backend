package enroll

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	controllerpb "controller/gen/controllerpb"
	"tunneler/internal/tlsutil"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Config controls enrollment behavior.
type Config struct {
	ControllerAddr string
	TunnelerID     string
	TrustDomain    string
	RootCAPEM      []byte
	Token          string
}

// Run performs one-time tunneler enrollment with the controller.
func Run() error {
	cfg, err := ConfigFromEnv()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cert, _, _, spiffeID, err := Enroll(ctx, cfg)
	if err != nil {
		return err
	}

	_ = cert
	fmt.Printf("Enrolled tunneler with SPIFFE ID: %s\n", spiffeID)
	return nil
}

// ConfigFromEnv builds Config from environment variables.
func ConfigFromEnv() (Config, error) {
	controllerAddr := os.Getenv("CONTROLLER_ADDR")
	tunnelerID := os.Getenv("TUNNELER_ID")
	trustDomain := os.Getenv("TRUST_DOMAIN")
	token := os.Getenv("ENROLLMENT_TOKEN")
	if trustDomain == "" {
		trustDomain = "mycorp.internal"
	}
	trustDomain = normalizeTrustDomain(trustDomain)

	rootCAPEM, err := loadExplicitCA()
	if err != nil {
		return Config{}, err
	}

	if controllerAddr == "" {
		return Config{}, fmt.Errorf("CONTROLLER_ADDR is not set")
	}
	if tunnelerID == "" {
		return Config{}, fmt.Errorf("TUNNELER_ID is not set")
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

	return Config{
		ControllerAddr: controllerAddr,
		TunnelerID:     tunnelerID,
		TrustDomain:    trustDomain,
		RootCAPEM:      rootCAPEM,
		Token:          token,
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

	rootPool, err := tlsutil.RootPoolFromPEM(cfg.RootCAPEM)
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

	resp, err := client.EnrollTunneler(ctx, &controllerpb.EnrollRequest{
		Id:        cfg.TunnelerID,
		PublicKey: pubPEM,
		Token:     cfg.Token,
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

func normalizeTrustDomain(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimSuffix(v, ".")
	return v
}

func loadExplicitCA() ([]byte, error) {
	if cred, err := ReadCredential("CONTROLLER_CA"); err != nil {
		return nil, err
	} else if cred != "" {
		return []byte(cred), nil
	}

	caPath := strings.TrimSpace(os.Getenv("CONTROLLER_CA_PATH"))
	if caPath == "" {
		return nil, fmt.Errorf("CONTROLLER_CA_PATH is not set (explicit controller trust is required)")
	}

	pemBytes, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read controller CA at %s: %w", caPath, err)
	}

	return pemBytes, nil
}

func ReadCredential(name string) (string, error) {
	dir := strings.TrimSpace(os.Getenv("CREDENTIALS_DIRECTORY"))
	if dir == "" {
		return "", nil
	}
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read credential %s: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}
