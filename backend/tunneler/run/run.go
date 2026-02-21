package run

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	controllerpb "controller/gen/controllerpb"
	"tunneler/enroll"
	"tunneler/internal/tlsutil"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// Run starts the tunneler client.
func Run() error {
	cfg, err := configFromEnv()
	if err != nil {
		return err
	}

	enrollCfg, err := enroll.ConfigFromEnv()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workloadCert, certPEM, caPEM, spiffeID, err := enroll.Enroll(ctx, enrollCfg)
	if err != nil {
		return err
	}

	certInfo, err := parseLeafCert(certPEM)
	if err != nil {
		return err
	}

	store := tlsutil.NewCertStore(workloadCert, certPEM, certInfo.NotAfter)
	totalTTL := certInfo.NotAfter.Sub(certInfo.NotBefore)
	rootPool, err := tlsutil.RootPoolFromPEM(caPEM)
	if err != nil {
		return err
	}

	log.Printf("tunneler enrolled as %s", spiffeID)

	reloadCh := make(chan struct{}, 1)
	go controlPlaneLoop(ctx, cfg.connectorAddr, cfg.trustDomain, store, rootPool, spiffeID, cfg.tunnelerID, reloadCh)
	go renewalLoop(ctx, cfg.controllerAddr, cfg.tunnelerID, cfg.trustDomain, store, rootPool, caPEM, totalTTL, reloadCh)

	<-ctx.Done()
	return ctx.Err()
}

type runtimeConfig struct {
	controllerAddr string
	connectorAddr  string
	tunnelerID     string
	trustDomain    string
}

func configFromEnv() (runtimeConfig, error) {
	controllerAddr := os.Getenv("CONTROLLER_ADDR")
	connectorAddr := os.Getenv("CONNECTOR_ADDR")
	tunnelerID := os.Getenv("TUNNELER_ID")
	trustDomain := os.Getenv("TRUST_DOMAIN")

	if trustDomain == "" {
		trustDomain = "mycorp.internal"
	}
	trustDomain = normalizeTrustDomain(trustDomain)
	if controllerAddr == "" {
		return runtimeConfig{}, fmt.Errorf("CONTROLLER_ADDR is not set")
	}
	if connectorAddr == "" {
		return runtimeConfig{}, fmt.Errorf("CONNECTOR_ADDR is not set")
	}
	if tunnelerID == "" {
		return runtimeConfig{}, fmt.Errorf("TUNNELER_ID is not set")
	}

	return runtimeConfig{
		controllerAddr: controllerAddr,
		connectorAddr:  connectorAddr,
		tunnelerID:     tunnelerID,
		trustDomain:    trustDomain,
	}, nil
}

func controlPlaneLoop(ctx context.Context, connectorAddr, trustDomain string, store *tlsutil.CertStore, roots *x509.CertPool, spiffeID, tunnelerID string, reloadCh <-chan struct{}) {
	backoff := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		sessionCtx, cancel := context.WithCancel(ctx)
		errCh := make(chan error, 1)
		go func() {
			errCh <- connectToConnector(sessionCtx, connectorAddr, trustDomain, store, roots, spiffeID, tunnelerID)
		}()

		select {
		case <-ctx.Done():
			cancel()
			<-errCh
			return
		case <-reloadCh:
			cancel()
			<-errCh
		case err := <-errCh:
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("connector connection ended: %v", err)
			}
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func connectToConnector(ctx context.Context, connectorAddr, trustDomain string, store *tlsutil.CertStore, roots *x509.CertPool, spiffeID, tunnelerID string) error {
	tlsConfig := &tls.Config{
		MinVersion:           tls.VersionTLS13,
		GetClientCertificate: store.GetClientCertificate,
		RootCAs:              roots,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			return tlsutil.VerifyPeerSPIFFE(rawCerts, verifiedChains, trustDomain, "connector")
		},
	}

	conn, err := grpc.DialContext(
		ctx,
		connectorAddr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := controllerpb.NewControlPlaneClient(conn)
	stream, err := client.Connect(ctx)
	if err != nil {
		return err
	}

	if err := stream.Send(&controllerpb.ControlMessage{Type: "tunneler_hello"}); err != nil {
		return err
	}

	recvErr := make(chan error, 1)
	go func() {
		for {
			_, err := stream.Recv()
			if err != nil {
				recvErr <- err
				return
			}
		}
	}()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-recvErr:
			return err
		case <-ticker.C:
			payload, _ := json.Marshal(map[string]string{
				"tunneler_id": tunnelerID,
				"spiffe_id":   spiffeID,
			})
			if err := stream.Send(&controllerpb.ControlMessage{
				Type:    "tunneler_heartbeat",
				Payload: payload,
				Status:  "ONLINE",
			}); err != nil {
				return err
			}
		}
	}
}

func renewalLoop(ctx context.Context, controllerAddr, tunnelerID, trustDomain string, store *tlsutil.CertStore, roots *x509.CertPool, caPEM []byte, totalTTL time.Duration, reloadCh chan<- struct{}) {
	for {
		next := nextRenewal(store.NotAfter(), totalTTL)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		cert, certPEM, notAfter, notBefore, err := renewOnce(ctx, controllerAddr, tunnelerID, trustDomain, store, roots, caPEM)
		if err != nil {
			log.Printf("certificate renewal failed: %v", err)
			continue
		}

		store.Update(cert, certPEM, notAfter)
		totalTTL = notAfter.Sub(notBefore)
	}
}

func renewOnce(ctx context.Context, controllerAddr, tunnelerID, trustDomain string, store *tlsutil.CertStore, roots *x509.CertPool, caPEM []byte) (tls.Certificate, []byte, time.Time, time.Time, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, time.Time{}, time.Time{}, err
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		return tls.Certificate{}, nil, time.Time{}, time.Time{}, err
	}

	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	tlsConfig := &tls.Config{
		MinVersion:           tls.VersionTLS13,
		GetClientCertificate: store.GetClientCertificate,
		RootCAs:              roots,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			return tlsutil.VerifyPeerSPIFFE(rawCerts, verifiedChains, trustDomain, "controller")
		},
	}

	conn, err := grpc.DialContext(
		ctx,
		controllerAddr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		return tls.Certificate{}, nil, time.Time{}, time.Time{}, err
	}
	defer conn.Close()

	client := controllerpb.NewEnrollmentServiceClient(conn)
	resp, err := client.Renew(ctx, &controllerpb.EnrollRequest{Id: tunnelerID, PublicKey: pubPEM})
	if err != nil {
		return tls.Certificate{}, nil, time.Time{}, time.Time{}, err
	}
	if len(resp.CaCertificate) == 0 {
		return tls.Certificate{}, nil, time.Time{}, time.Time{}, errors.New("empty CA certificate in renewal response")
	}
	if !tlsutil.EqualCAPEM(caPEM, resp.CaCertificate) {
		return tls.Certificate{}, nil, time.Time{}, time.Time{}, errors.New("internal CA mismatch during renewal")
	}

	block, _ := pem.Decode(resp.Certificate)
	if block == nil || block.Type != "CERTIFICATE" {
		return tls.Certificate{}, nil, time.Time{}, time.Time{}, errors.New("invalid certificate PEM")
	}

	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return tls.Certificate{}, nil, time.Time{}, time.Time{}, err
	}

	workloadCert := tls.Certificate{Certificate: [][]byte{block.Bytes}, PrivateKey: privKey}
	return workloadCert, resp.Certificate, leaf.NotAfter, leaf.NotBefore, nil
}

func nextRenewal(notAfter time.Time, totalTTL time.Duration) time.Time {
	remaining := time.Until(notAfter)
	if remaining <= 0 {
		return time.Now().Add(10 * time.Second)
	}
	if totalTTL <= 0 {
		totalTTL = remaining
	}
	renewAt := totalTTL * 30 / 100
	next := notAfter.Add(-renewAt)
	if next.Before(time.Now().Add(10 * time.Second)) {
		return time.Now().Add(10 * time.Second)
	}
	return next
}

func parseLeafCert(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("invalid certificate PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

func normalizeTrustDomain(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimSuffix(v, ".")
	return v
}
