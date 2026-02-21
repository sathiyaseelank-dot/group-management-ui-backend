package ca

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"time"
)

// CA represents the internal Certificate Authority used by the controller.
// It holds the parsed CA certificate and a crypto.Signer for the CA private key.
type CA struct {
	Cert *x509.Certificate
	Key  crypto.Signer
}

// GenerateSelfSignedCA creates a standards-compliant CA certificate and key.
// The CA certificate includes critical BasicConstraints and KeyUsage for cert signing.
func GenerateSelfSignedCA(commonName string, ttl time.Duration) (certPEM, keyPEM []byte, err error) {
	if ttl <= 0 {
		return nil, nil, errors.New("invalid CA TTL")
	}

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(ttl),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		Subject:               pkix.Name{CommonName: commonName},
		MaxPathLenZero:        true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}

// LoadCA loads and parses the internal CA certificate and private key.
// certPEM and keyPEM must be PEM-encoded data.
// The private key must implement crypto.Signer (RSA, ECDSA, TPM-backed, etc.).
func LoadCA(certPEM, keyPEM []byte) (*CA, error) {
	if len(certPEM) == 0 {
		return nil, errors.New("CA certificate PEM is empty")
	}
	if len(keyPEM) == 0 {
		return nil, errors.New("CA private key PEM is empty")
	}

	// Decode and parse CA certificate
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return nil, errors.New("failed to decode CA certificate PEM")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}

	// Decode and parse CA private key (PKCS#8)
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, errors.New("failed to decode CA private key PEM")
	}

	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}

	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, errors.New("CA private key does not implement crypto.Signer")
	}

	return &CA{
		Cert: cert,
		Key:  signer,
	}, nil
}
