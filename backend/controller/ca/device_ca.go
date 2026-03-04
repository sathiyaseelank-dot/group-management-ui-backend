package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// LoadOrGenerateDeviceCA ensures a device CA key-pair exists in certDir.
// Returns the parsed CA certificate and private key.
func LoadOrGenerateDeviceCA(certDir string) (*x509.Certificate, *rsa.PrivateKey, error) {
	if certDir == "" {
		certDir = "./mtls"
	}
	caCertPath := filepath.Join(certDir, "device-ca.crt")
	caKeyPath := filepath.Join(certDir, "device-ca.key")

	if _, err := os.Stat(caCertPath); err == nil {
		cert, key, err := loadDeviceCA(caCertPath, caKeyPath)
		if err == nil {
			log.Printf("[device_ca] Loaded existing device CA from %s", certDir)
			return cert, key, nil
		}
		log.Printf("[device_ca] Existing device CA invalid (%v), regenerating...", err)
	}

	log.Printf("[device_ca] Generating device CA in %s...", certDir)
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("create cert dir: %w", err)
	}
	return generateDeviceCA(caCertPath, caKeyPath)
}

func loadDeviceCA(certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, err
	}
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, nil, fmt.Errorf("invalid cert PEM at %s", certPath)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, err
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("invalid key PEM at %s", keyPath)
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func generateDeviceCA(certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, fmt.Errorf("generate device CA key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "ZTNA-Device-CA",
			Organization: []string{"ZTNA"},
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create device CA cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, err
	}

	cf, err := os.Create(certPath)
	if err != nil {
		return nil, nil, fmt.Errorf("write device CA cert: %w", err)
	}
	_ = pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	cf.Close()

	kf, err := os.OpenFile(keyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, nil, fmt.Errorf("write device CA key: %w", err)
	}
	_ = pem.Encode(kf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	kf.Close()

	log.Printf("[device_ca] Generated: %s, %s", certPath, keyPath)
	return cert, key, nil
}

// SignDeviceCSR signs a device CSR with the device CA.
// Returns the PEM-encoded certificate and SHA256 fingerprint of the public key.
func SignDeviceCSR(csrPEM []byte, caCert *x509.Certificate, caKey *rsa.PrivateKey) (certPEM []byte, fingerprint string, err error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil {
		return nil, "", fmt.Errorf("invalid CSR PEM")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("parse CSR: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, "", fmt.Errorf("CSR signature invalid: %w", err)
	}

	serialBytes := make([]byte, 8)
	if _, err := rand.Read(serialBytes); err != nil {
		return nil, "", err
	}
	serial := new(big.Int).SetBytes(serialBytes)

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      csr.Subject,
		DNSNames:     csr.DNSNames,
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, csr.PublicKey, caKey)
	if err != nil {
		return nil, "", fmt.Errorf("sign device cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	// Fingerprint: SHA256 of the DER-encoded public key
	pubDER, err := x509.MarshalPKIXPublicKey(csr.PublicKey)
	if err != nil {
		return nil, "", fmt.Errorf("marshal public key: %w", err)
	}
	sum := sha256.Sum256(pubDER)
	fingerprint = hex.EncodeToString(sum[:])

	return certPEM, fingerprint, nil
}
