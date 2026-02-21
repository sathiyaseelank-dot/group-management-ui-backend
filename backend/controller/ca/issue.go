package ca

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/url"
	"time"
)

// IssueWorkloadCert issues a short-lived X.509 certificate for a workload.
// - spiffeID must be a valid SPIFFE URI (spiffe://...)
// - pubKey is the workload public key
// - ttl controls certificate lifetime
//
// This function does NOT perform authorization.
// It assumes the caller has already validated the workload identity.
func IssueWorkloadCert(
	ca *CA,
	spiffeID string,
	pubKey crypto.PublicKey,
	ttl time.Duration,
	dnsNames []string,
	ipAddrs []net.IP,
) ([]byte, error) {

	if ca == nil || ca.Cert == nil || ca.Key == nil {
		return nil, errors.New("CA is not initialized")
	}

	if ttl <= 0 {
		return nil, errors.New("invalid certificate TTL")
	}

	uri, err := url.Parse(spiffeID)
	if err != nil {
		return nil, err
	}

	if uri.Scheme != "spiffe" {
		return nil, errors.New("SPIFFE ID must use spiffe:// scheme")
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, err
	}

	now := time.Now()

	tmpl := x509.Certificate{
		SerialNumber: serial,

		NotBefore: now.Add(-1 * time.Minute),
		NotAfter:  now.Add(ttl),

		KeyUsage: x509.KeyUsageDigitalSignature,

		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},

		BasicConstraintsValid: true,
		IsCA:                  false,

		// Enforce exactly one URI SAN and no CN.
		URIs:        []*url.URL{uri},
		DNSNames:    dnsNames,
		IPAddresses: ipAddrs,
	}

	der, err := x509.CreateCertificate(
		rand.Reader,
		&tmpl,
		ca.Cert,
		pubKey,
		ca.Key,
	)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	})

	return certPEM, nil
}
