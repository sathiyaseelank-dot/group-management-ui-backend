package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"
)

// CertStore keeps the current workload certificate in memory for rotation.
type CertStore struct {
	mu       sync.RWMutex
	cert     tls.Certificate
	certPEM  []byte
	notAfter time.Time
}

// NewCertStore initializes a new CertStore.
func NewCertStore(cert tls.Certificate, certPEM []byte, notAfter time.Time) *CertStore {
	return &CertStore{cert: cert, certPEM: certPEM, notAfter: notAfter}
}

// Update replaces the certificate in memory.
func (s *CertStore) Update(cert tls.Certificate, certPEM []byte, notAfter time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cert = cert
	s.certPEM = certPEM
	s.notAfter = notAfter
}

// NotAfter returns the current certificate expiry.
func (s *CertStore) NotAfter() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.notAfter
}

// GetClientCertificate returns the current certificate for client-side handshakes.
func (s *CertStore) GetClientCertificate(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &s.cert, nil
}

// RootPoolFromPEM builds a cert pool from PEM bytes.
func RootPoolFromPEM(pemBytes []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New("failed to parse root CA PEM")
	}
	return pool, nil
}

// ParseAndValidateCA ensures the PEM contains a valid CA certificate.
func ParseAndValidateCA(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("invalid CA PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	if !cert.IsCA || !cert.BasicConstraintsValid {
		return nil, errors.New("certificate is not a valid CA")
	}
	if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, errors.New("CA certificate missing key usage")
	}
	return cert, nil
}

// EqualCAPEM compares two CA PEM blocks by their DER bytes.
func EqualCAPEM(a, b []byte) bool {
	ab, _ := pem.Decode(a)
	bb, _ := pem.Decode(b)
	if ab == nil || bb == nil {
		return false
	}
	return string(ab.Bytes) == string(bb.Bytes)
}

// VerifyPeerSPIFFE validates SPIFFE identity using verified chains.
func VerifyPeerSPIFFE(rawCerts [][]byte, verifiedChains [][]*x509.Certificate, trustDomain, expectedRole string) error {
	if len(rawCerts) == 0 {
		return errors.New("no peer certificates")
	}

	if len(verifiedChains) == 0 || len(verifiedChains[0]) == 0 {
		return errors.New("peer verification failed")
	}

	leaf := verifiedChains[0][0]
	if len(leaf.URIs) != 1 {
		return errors.New("exactly one SPIFFE ID is required")
	}

	uri := leaf.URIs[0]
	if err := verifySPIFFEURI(uri, trustDomain, expectedRole); err != nil {
		return err
	}

	return nil
}

func verifySPIFFEURI(uri *url.URL, trustDomain, expectedRole string) error {
	if uri.Scheme != "spiffe" {
		return errors.New("SPIFFE ID must use spiffe:// scheme")
	}
	if uri.Host != trustDomain {
		return errors.New("SPIFFE trust domain mismatch")
	}
	path := strings.TrimPrefix(uri.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		return errors.New("invalid SPIFFE path")
	}
	role := parts[0]
	if expectedRole != "" && role != expectedRole {
		return errors.New("unexpected SPIFFE role")
	}
	return nil
}
