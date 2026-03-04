package run

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"connector/enroll"
	"connector/internal/spiffe"
	"connector/internal/tlsutil"
	controllerpb "controller/gen/controllerpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// Run starts the long-running connector service.
func Run() error {
	cfg, err := configFromEnv()
	if err != nil {
		return err
	}

	enrollCfg, err := enroll.ConfigFromEnvRun()
	if err != nil {
		return err
	}
	enrollCfg.Token = os.Getenv("ENROLLMENT_TOKEN")
	if enrollCfg.Token == "" {
		cred, err := enroll.ReadCredential("ENROLLMENT_TOKEN")
		if err != nil {
			return err
		}
		enrollCfg.Token = cred
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if systemdWatchdogEnabled() {
		go systemdWatchdogLoop(ctx)
	}

	if enrollCfg.Token == "" {
		return fmt.Errorf("ENROLLMENT_TOKEN is required for enrollment")
	}
	cert, certPEM, caPEM, spiffeID, err := enroll.Enroll(ctx, enrollCfg)
	if err != nil {
		return err
	}

	certInfo, err := parseLeafCert(certPEM)
	if err != nil {
		return err
	}
	workloadCert := cert
	notAfter := certInfo.NotAfter
	totalTTL := certInfo.NotAfter.Sub(certInfo.NotBefore)

	log.Printf("connector enrolled as %s", spiffeID)

	store := tlsutil.NewCertStore(workloadCert, nil, notAfter)
	rootPool, err := tlsutil.RootPoolFromPEM(caPEM)
	if err != nil {
		return err
	}
	allowlist := newTunnelerAllowlist()
	policyCache := newPolicyCache(cfg.policyKey, cfg.staleGrace)
	controllerSendCh := make(chan *controllerpb.ControlMessage, 16)

	reloadCh := make(chan struct{}, 1)
	go controlPlaneLoop(ctx, cfg.controllerAddr, cfg.trustDomain, cfg.connectorID, cfg.privateIP, store, rootPool, allowlist, policyCache, controllerSendCh, reloadCh)
	go renewalLoop(ctx, cfg.controllerAddr, cfg.connectorID, cfg.trustDomain, store, rootPool, caPEM, totalTTL)

	if cfg.listenAddr != "" {
		go serverLoop(ctx, cfg.listenAddr, cfg.trustDomain, store, rootPool, allowlist, policyCache, controllerSendCh, cfg.connectorID)
	}

	<-ctx.Done()
	return ctx.Err()
}

func systemdWatchdogEnabled() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--systemd-watchdog" {
			return true
		}
	}
	return false
}

func systemdWatchdogLoop(ctx context.Context) {
	socket := os.Getenv("NOTIFY_SOCKET")
	if socket == "" {
		return
	}
	interval := watchdogInterval()
	if interval <= 0 {
		return
	}

	if err := systemdNotify(socket, "READY=1"); err != nil {
		log.Printf("systemd notify failed: %v", err)
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = systemdNotify(socket, "WATCHDOG=1")
		}
	}
}

func watchdogInterval() time.Duration {
	usecStr := strings.TrimSpace(os.Getenv("WATCHDOG_USEC"))
	if usecStr == "" {
		return 0
	}
	usec, err := strconv.ParseInt(usecStr, 10, 64)
	if err != nil || usec <= 0 {
		return 0
	}
	d := time.Duration(usec) * time.Microsecond
	return d / 2
}

func systemdNotify(socket, msg string) error {
	addr := socket
	if strings.HasPrefix(addr, "@") {
		addr = "\x00" + addr[1:]
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: addr, Net: "unixgram"})
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write([]byte(msg))
	return err
}

type runtimeConfig struct {
	controllerAddr string
	connectorID    string
	trustDomain    string
	listenAddr     string
	privateIP      string
	policyKey      []byte
	staleGrace     time.Duration
}

func configFromEnv() (runtimeConfig, error) {
	controllerAddr := os.Getenv("CONTROLLER_ADDR")
	connectorID := os.Getenv("CONNECTOR_ID")
	trustDomain := os.Getenv("TRUST_DOMAIN")
	listenAddr := os.Getenv("CONNECTOR_LISTEN_ADDR")
	policyKey := os.Getenv("POLICY_SIGNING_KEY")
	staleGrace := 10 * time.Minute
	if v := strings.TrimSpace(os.Getenv("POLICY_STALE_GRACE_SECONDS")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			staleGrace = time.Duration(secs) * time.Second
		}
	}

	if trustDomain == "" {
		trustDomain = "mycorp.internal"
	}
	if controllerAddr == "" {
		return runtimeConfig{}, fmt.Errorf("CONTROLLER_ADDR is not set")
	}
	if connectorID == "" {
		return runtimeConfig{}, fmt.Errorf("CONNECTOR_ID is not set")
	}
	if policyKey == "" {
		return runtimeConfig{}, fmt.Errorf("POLICY_SIGNING_KEY is not set")
	}

	privateIP, err := enroll.ResolvePrivateIP(controllerAddr)
	if err != nil {
		return runtimeConfig{}, err
	}
	if listenAddr == "" {
		listenAddr = net.JoinHostPort(privateIP, "9443")
	}

	return runtimeConfig{
		controllerAddr: controllerAddr,
		connectorID:    connectorID,
		trustDomain:    trustDomain,
		listenAddr:     listenAddr,
		privateIP:      privateIP,
		policyKey:      []byte(policyKey),
		staleGrace:     staleGrace,
	}, nil
}

func runConnectorServer(addr, trustDomain string, store *tlsutil.CertStore, roots *x509.CertPool, allowlist *tunnelerAllowlist, acl *policyCache, controllerSendCh chan<- *controllerpb.ControlMessage, connectorID string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	tlsConfig := &tls.Config{
		MinVersion:     tls.VersionTLS13,
		ClientAuth:     tls.RequireAndVerifyClientCert,
		ClientCAs:      roots,
		GetCertificate: store.GetCertificate,
	}

	grpcServer := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.UnaryInterceptor(spiffe.UnaryInterceptorWithAllowlist(trustDomain, allowlist, "tunneler")),
		grpc.StreamInterceptor(spiffe.StreamInterceptorWithAllowlist(trustDomain, allowlist, "tunneler")),
	)

	controllerpb.RegisterControlPlaneServer(grpcServer, &controlPlaneServer{
		connectorID: connectorID,
		sendCh:      controllerSendCh,
		acls:        acl,
	})

	log.Printf("connector server listening on %s", addr)
	return grpcServer.Serve(lis)
}

func serverLoop(ctx context.Context, addr, trustDomain string, store *tlsutil.CertStore, roots *x509.CertPool, allowlist *tunnelerAllowlist, acl *policyCache, controllerSendCh chan<- *controllerpb.ControlMessage, connectorID string) {
	backoff := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := runConnectorServer(addr, trustDomain, store, roots, allowlist, acl, controllerSendCh, connectorID); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("connector server stopped: %v", err)
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

func controlPlaneLoop(ctx context.Context, controllerAddr, trustDomain, connectorID, privateIP string, store *tlsutil.CertStore, roots *x509.CertPool, allowlist *tunnelerAllowlist, acl *policyCache, controllerSendCh <-chan *controllerpb.ControlMessage, reloadCh <-chan struct{}) {
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
			errCh <- connectControlPlane(sessionCtx, controllerAddr, trustDomain, connectorID, privateIP, store, roots, allowlist, acl, controllerSendCh)
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
				log.Printf("control-plane connection ended: %v", err)
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

func connectControlPlane(ctx context.Context, controllerAddr, trustDomain, connectorID, privateIP string, store *tlsutil.CertStore, roots *x509.CertPool, allowlist *tunnelerAllowlist, acl *policyCache, controllerSendCh <-chan *controllerpb.ControlMessage) error {
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

	if err := stream.Send(&controllerpb.ControlMessage{Type: "connector_hello"}); err != nil {
		return err
	}

	recvCh := make(chan *controllerpb.ControlMessage, 1)
	recvErr := make(chan error, 1)
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				recvErr <- err
				return
			}
			recvCh <- msg
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
		case msg := <-recvCh:
			handleControlMessage(msg, allowlist, acl)
		case msg := <-controllerSendCh:
			if msg != nil {
				if err := stream.Send(msg); err != nil {
					return err
				}
			}
		case <-ticker.C:
			if err := stream.Send(&controllerpb.ControlMessage{
				Type:        "heartbeat",
				ConnectorId: connectorID,
				PrivateIp:   privateIP,
				Status:      "ONLINE",
			}); err != nil {
				return err
			}
		}
	}
}

func renewalLoop(ctx context.Context, controllerAddr, connectorID, trustDomain string, store *tlsutil.CertStore, roots *x509.CertPool, caPEM []byte, totalTTL time.Duration) {
	for {
		next := nextRenewal(store.NotAfter(), totalTTL)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		cert, certPEM, notAfter, notBefore, err := renewOnce(ctx, controllerAddr, connectorID, trustDomain, store, roots, caPEM)
		if err != nil {
			log.Printf("certificate renewal failed: %v", err)
			continue
		}

		store.Update(cert, certPEM, notAfter)
		totalTTL = notAfter.Sub(notBefore)
	}
}

func renewOnce(ctx context.Context, controllerAddr, connectorID, trustDomain string, store *tlsutil.CertStore, roots *x509.CertPool, caPEM []byte) (tls.Certificate, []byte, time.Time, time.Time, error) {
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
	resp, err := client.Renew(ctx, &controllerpb.EnrollRequest{Id: connectorID, PublicKey: pubPEM})
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

type tunnelerAllowlist struct {
	mu       sync.RWMutex
	bySPIFFE map[string]struct{}
}

func newTunnelerAllowlist() *tunnelerAllowlist {
	return &tunnelerAllowlist{bySPIFFE: make(map[string]struct{})}
}

func (a *tunnelerAllowlist) Allowed(spiffeID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.bySPIFFE[spiffeID]
	return ok
}

func (a *tunnelerAllowlist) Replace(items []tunnelerInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.bySPIFFE = make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.SPIFFEID == "" {
			continue
		}
		a.bySPIFFE[item.SPIFFEID] = struct{}{}
	}
}

func (a *tunnelerAllowlist) Add(spiffeID string) {
	if spiffeID == "" {
		return
	}
	a.mu.Lock()
	a.bySPIFFE[spiffeID] = struct{}{}
	a.mu.Unlock()
}

type tunnelerInfo struct {
	TunnelerID string `json:"tunneler_id"`
	SPIFFEID   string `json:"spiffe_id"`
}

func handleControlMessage(msg *controllerpb.ControlMessage, allowlist *tunnelerAllowlist, acl *policyCache) {
	if msg == nil || allowlist == nil {
		return
	}
	switch msg.GetType() {
	case "tunneler_allowlist":
		var items []tunnelerInfo
		if err := json.Unmarshal(msg.GetPayload(), &items); err == nil {
			allowlist.Replace(items)
		}
	case "tunneler_allow":
		var item tunnelerInfo
		if err := json.Unmarshal(msg.GetPayload(), &item); err == nil {
			allowlist.Add(item.SPIFFEID)
		}
	case "policy_snapshot":
		if acl == nil {
			return
		}
		var snap policySnapshot
		if err := json.Unmarshal(msg.GetPayload(), &snap); err == nil {
			_ = acl.ReplaceSnapshot(snap)
		}
	}
}

// Policy snapshot enforcement (O(1) ACL lookup)
type policySnapshot struct {
	SnapshotMeta snapshotMeta     `json:"snapshot_meta"`
	Resources    []policyResource `json:"resources"`
}

type snapshotMeta struct {
	ConnectorID   string `json:"connector_id"`
	PolicyVersion int    `json:"policy_version"`
	CompiledAt    string `json:"compiled_at"`
	ValidUntil    string `json:"valid_until"`
	Signature     string `json:"signature"`
}

type policyResource struct {
	ResourceID        string   `json:"resource_id"`
	Type              string   `json:"type"`
	Address           string   `json:"address"`
	Port              int      `json:"port"`
	Protocol          string   `json:"protocol"`
	PortFrom          *int     `json:"port_from,omitempty"`
	PortTo            *int     `json:"port_to,omitempty"`
	AllowedIdentities []string `json:"allowed_identities"`
}

type policyCache struct {
	mu          sync.RWMutex
	byID        map[string]policyResource
	byDNS       map[string][]string
	byIP        map[string][]string
	byCIDR      []cidrEntry
	internetIDs []string
	aclTable    map[string]struct{}
	meta        snapshotMeta
	validUntil  time.Time
	signingKey  []byte
	staleGrace  time.Duration
	hasSnapshot bool
}

type cidrEntry struct {
	Net        *net.IPNet
	ResourceID string
}

func newPolicyCache(signingKey []byte, staleGrace time.Duration) *policyCache {
	return &policyCache{
		byID:       make(map[string]policyResource),
		byDNS:      make(map[string][]string),
		byIP:       make(map[string][]string),
		aclTable:   make(map[string]struct{}),
		signingKey: signingKey,
		staleGrace: staleGrace,
	}
}

func (p *policyCache) ReplaceSnapshot(snap policySnapshot) bool {
	if !verifySnapshot(p.signingKey, snap) {
		p.clear()
		log.Printf("policy snapshot rejected: invalid signature")
		return false
	}
	validUntil, err := time.Parse(time.RFC3339, snap.SnapshotMeta.ValidUntil)
	if err != nil {
		p.clear()
		log.Printf("policy snapshot rejected: invalid valid_until")
		return false
	}
	if time.Now().UTC().After(validUntil.Add(p.staleGrace)) {
		p.clear()
		log.Printf("policy snapshot rejected: expired beyond grace")
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.byID = make(map[string]policyResource)
	p.byDNS = make(map[string][]string)
	p.byIP = make(map[string][]string)
	p.byCIDR = nil
	p.internetIDs = nil
	p.aclTable = make(map[string]struct{})
	for _, res := range snap.Resources {
		p.byID[res.ResourceID] = res
		addr := strings.ToLower(strings.TrimSpace(res.Address))
		switch strings.ToLower(strings.TrimSpace(res.Type)) {
		case "internet":
			p.internetIDs = append(p.internetIDs, res.ResourceID)
		case "cidr":
			if _, netCIDR, err := net.ParseCIDR(addr); err == nil && netCIDR != nil {
				p.byCIDR = append(p.byCIDR, cidrEntry{Net: netCIDR, ResourceID: res.ResourceID})
			}
		default:
			if addr != "" {
				p.byDNS[addr] = append(p.byDNS[addr], res.ResourceID)
				if ip := net.ParseIP(addr); ip != nil {
					p.byIP[ip.String()] = append(p.byIP[ip.String()], res.ResourceID)
				}
			}
		}
		for _, identity := range res.AllowedIdentities {
			if identity == "" {
				continue
			}
			p.aclTable[identity+"::"+res.ResourceID] = struct{}{}
		}
	}
	p.meta = snap.SnapshotMeta
	p.validUntil = validUntil
	p.hasSnapshot = true
	return true
}

func (p *policyCache) Allowed(identityID, dest, protocol string, port uint16) (bool, string, string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.hasSnapshot {
		return false, "", "no_snapshot"
	}
	if time.Now().UTC().After(p.validUntil.Add(p.staleGrace)) {
		log.Printf("policy snapshot expired beyond grace; denying all")
		return false, "", "snapshot_expired"
	}
	key := strings.ToLower(strings.TrimSpace(dest))
	// NOTE: CIDR resources are matched only when `dest` is an IP literal.
	// If `dest` is a hostname, we DO NOT resolve DNS here (avoids split-horizon/DNS poisoning and keeps enforcement deterministic).
	// To allow hostnames, define DNS resources explicitly in policy.
	resourceIDs := []string{}

	if ip := net.ParseIP(key); ip != nil {
		resourceIDs = append(resourceIDs, p.byIP[ip.String()]...)
		for _, entry := range p.byCIDR {
			if entry.Net != nil && entry.Net.Contains(ip) {
				resourceIDs = append(resourceIDs, entry.ResourceID)
			}
		}
	}
	if len(resourceIDs) == 0 && key != "" {
		resourceIDs = append(resourceIDs, p.byDNS[key]...)
	}
	if len(resourceIDs) == 0 && len(p.internetIDs) > 0 {
		resourceIDs = append(resourceIDs, p.internetIDs...)
	}
	if len(resourceIDs) == 0 {
		return false, "", "resource_not_found"
	}
	seen := make(map[string]struct{}, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		if _, ok := seen[resourceID]; ok {
			continue
		}
		seen[resourceID] = struct{}{}
		res, ok := p.byID[resourceID]
		if !ok {
			continue
		}
		if res.Protocol != "" && protocol != "" && !strings.EqualFold(res.Protocol, protocol) {
			continue
		}
		if !portMatches(res, port) {
			continue
		}
		if _, ok := p.aclTable[identityID+"::"+res.ResourceID]; ok {
			return true, res.ResourceID, "allowed"
		}
	}
	return false, "", "not_allowed"
}

func (p *policyCache) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.byID = make(map[string]policyResource)
	p.byDNS = make(map[string][]string)
	p.byIP = make(map[string][]string)
	p.byCIDR = nil
	p.internetIDs = nil
	p.aclTable = make(map[string]struct{})
	p.hasSnapshot = false
	var zero snapshotMeta
	p.meta = zero
	p.validUntil = time.Time{}
}

func verifySnapshot(key []byte, snap policySnapshot) bool {
	if len(key) == 0 {
		return false
	}
	expected, err := signSnapshot(key, snap)
	if err != nil {
		return false
	}
	sig := strings.TrimSpace(snap.SnapshotMeta.Signature)
	sig = strings.TrimPrefix(sig, "sha256:")
	provided, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	want, _ := hex.DecodeString(expected)
	return hmac.Equal(want, provided)
}

func signSnapshot(key []byte, snap policySnapshot) (string, error) {
	if len(key) == 0 {
		return "", errors.New("signing key not configured")
	}
	snap.SnapshotMeta.Signature = ""
	data, err := json.Marshal(snap)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func portMatches(res policyResource, port uint16) bool {
	if res.PortFrom == nil && res.PortTo == nil {
		if res.Port == 0 {
			return true
		}
		return port == uint16(res.Port)
	}
	start := 0
	end := 0
	if res.PortFrom != nil {
		start = *res.PortFrom
	}
	if res.PortTo != nil {
		end = *res.PortTo
	}
	if start == 0 && end == 0 {
		return true
	}
	if end == 0 {
		end = start
	}
	return int(port) >= start && int(port) <= end
}
