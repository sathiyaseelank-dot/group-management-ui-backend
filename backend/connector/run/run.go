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
	aclCache := newACLCache()
	controllerSendCh := make(chan *controllerpb.ControlMessage, 16)

	reloadCh := make(chan struct{}, 1)
	go controlPlaneLoop(ctx, cfg.controllerAddr, cfg.trustDomain, cfg.connectorID, cfg.privateIP, store, rootPool, allowlist, aclCache, controllerSendCh, reloadCh)
	go renewalLoop(ctx, cfg.controllerAddr, cfg.connectorID, cfg.trustDomain, store, rootPool, caPEM, totalTTL)

	if cfg.listenAddr != "" {
		go serverLoop(ctx, cfg.listenAddr, cfg.trustDomain, store, rootPool, allowlist, aclCache, controllerSendCh, cfg.connectorID)
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
}

func configFromEnv() (runtimeConfig, error) {
	controllerAddr := os.Getenv("CONTROLLER_ADDR")
	connectorID := os.Getenv("CONNECTOR_ID")
	trustDomain := os.Getenv("TRUST_DOMAIN")
	listenAddr := os.Getenv("CONNECTOR_LISTEN_ADDR")

	if trustDomain == "" {
		trustDomain = "mycorp.internal"
	}
	if controllerAddr == "" {
		return runtimeConfig{}, fmt.Errorf("CONTROLLER_ADDR is not set")
	}
	if connectorID == "" {
		return runtimeConfig{}, fmt.Errorf("CONNECTOR_ID is not set")
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
	}, nil
}

func runConnectorServer(addr, trustDomain string, store *tlsutil.CertStore, roots *x509.CertPool, allowlist *tunnelerAllowlist, acl *aclCache, controllerSendCh chan<- *controllerpb.ControlMessage, connectorID string) error {
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

func serverLoop(ctx context.Context, addr, trustDomain string, store *tlsutil.CertStore, roots *x509.CertPool, allowlist *tunnelerAllowlist, acl *aclCache, controllerSendCh chan<- *controllerpb.ControlMessage, connectorID string) {
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

func controlPlaneLoop(ctx context.Context, controllerAddr, trustDomain, connectorID, privateIP string, store *tlsutil.CertStore, roots *x509.CertPool, allowlist *tunnelerAllowlist, acl *aclCache, controllerSendCh <-chan *controllerpb.ControlMessage, reloadCh <-chan struct{}) {
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

func connectControlPlane(ctx context.Context, controllerAddr, trustDomain, connectorID, privateIP string, store *tlsutil.CertStore, roots *x509.CertPool, allowlist *tunnelerAllowlist, acl *aclCache, controllerSendCh <-chan *controllerpb.ControlMessage) error {
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

func handleControlMessage(msg *controllerpb.ControlMessage, allowlist *tunnelerAllowlist, acl *aclCache) {
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
	case "acl_init":
		if acl == nil {
			return
		}
		var state aclState
		if err := json.Unmarshal(msg.GetPayload(), &state); err == nil {
			acl.Replace(state)
		}
	case "resource_updated":
		if acl == nil {
			return
		}
		var res aclResource
		if err := json.Unmarshal(msg.GetPayload(), &res); err == nil {
			acl.UpsertResource(res)
		}
	case "resource_removed":
		if acl == nil {
			return
		}
		var payload struct {
			ResourceID string `json:"resource_id"`
		}
		if err := json.Unmarshal(msg.GetPayload(), &payload); err == nil {
			acl.RemoveResource(payload.ResourceID)
		}
	case "authorization_updated":
		if acl == nil {
			return
		}
		var auth aclAuthorization
		if err := json.Unmarshal(msg.GetPayload(), &auth); err == nil {
			acl.UpsertAuthorization(auth)
		}
	case "authorization_removed":
		if acl == nil {
			return
		}
		var payload struct {
			ResourceID      string `json:"resource_id"`
			PrincipalSPIFFE string `json:"principal_spiffe"`
		}
		if err := json.Unmarshal(msg.GetPayload(), &payload); err == nil {
			acl.RemoveAuthorization(payload.ResourceID, payload.PrincipalSPIFFE)
		}
	}
}

type aclResource struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Address string `json:"address"`
}

type aclFilter struct {
	Protocol       string `json:"protocol"`
	PortRangeStart uint16 `json:"port_range_start"`
	PortRangeEnd   uint16 `json:"port_range_end"`
}

type aclAuthorization struct {
	PrincipalSPIFFE string      `json:"principal_spiffe"`
	ResourceID      string      `json:"resource_id"`
	Filters         []aclFilter `json:"filters,omitempty"`
}

type aclState struct {
	Resources      []aclResource      `json:"resources"`
	Authorizations []aclAuthorization `json:"authorizations"`
}

type aclCache struct {
	mu             sync.RWMutex
	resources      map[string]aclResource
	authorizations map[string]aclAuthorization
}

func newACLCache() *aclCache {
	return &aclCache{
		resources:      make(map[string]aclResource),
		authorizations: make(map[string]aclAuthorization),
	}
}

func (a *aclCache) Replace(state aclState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.resources = make(map[string]aclResource)
	for _, r := range state.Resources {
		a.resources[r.ID] = r
	}
	a.authorizations = make(map[string]aclAuthorization)
	for _, auth := range state.Authorizations {
		key := auth.PrincipalSPIFFE + "|" + auth.ResourceID
		a.authorizations[key] = auth
	}
}

func (a *aclCache) UpsertResource(r aclResource) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.resources[r.ID] = r
}

func (a *aclCache) RemoveResource(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.resources, id)
	for key, auth := range a.authorizations {
		if auth.ResourceID == id {
			delete(a.authorizations, key)
		}
	}
}

func (a *aclCache) UpsertAuthorization(auth aclAuthorization) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := auth.PrincipalSPIFFE + "|" + auth.ResourceID
	a.authorizations[key] = auth
}

func (a *aclCache) RemoveAuthorization(resourceID, principalSPIFFE string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.authorizations, principalSPIFFE+"|"+resourceID)
}

func (a *aclCache) Allowed(principalSPIFFE, dest, protocol string, port uint16) (bool, string, string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, auth := range a.authorizations {
		if auth.PrincipalSPIFFE != principalSPIFFE {
			continue
		}
		res, ok := a.resources[auth.ResourceID]
		if !ok {
			continue
		}
		if !resourceMatches(res, dest) {
			continue
		}
		if len(auth.Filters) == 0 {
			return true, res.ID, "allowed"
		}
		for _, f := range auth.Filters {
			if !filterMatches(f, protocol, port) {
				continue
			}
			return true, res.ID, "allowed"
		}
		return false, res.ID, "filter_denied"
	}
	return false, "", "no_authorization"
}

func resourceMatches(res aclResource, dest string) bool {
	switch res.Type {
	case "internet":
		return true
	case "dns":
		return strings.EqualFold(res.Address, dest)
	case "cidr":
		_, cidr, err := net.ParseCIDR(res.Address)
		if err != nil {
			return false
		}
		ip := net.ParseIP(dest)
		if ip == nil {
			return false
		}
		return cidr.Contains(ip)
	default:
		return false
	}
}

func filterMatches(f aclFilter, protocol string, port uint16) bool {
	if protocol != "" && strings.ToLower(f.Protocol) != strings.ToLower(protocol) {
		return false
	}
	if f.PortRangeStart == 0 && f.PortRangeEnd == 0 {
		return true
	}
	if port == 0 {
		return false
	}
	start := f.PortRangeStart
	end := f.PortRangeEnd
	if end == 0 {
		end = start
	}
	return port >= start && port <= end
}
