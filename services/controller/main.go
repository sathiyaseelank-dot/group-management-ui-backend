package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"controller/admin"
	"controller/api"
	"controller/ca"
	controllerpb "controller/gen/controllerpb"
	"controller/mailer"
	"controller/state"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	// ---- required environment variables ----
	caCertPEM, caKeyPEM, err := loadCA()
	if err != nil {
		log.Fatal(err)
	}
	trustDomain := os.Getenv("TRUST_DOMAIN")
	if trustDomain == "" {
		trustDomain = "mycorp.internal"
	}
	trustDomain = normalizeTrustDomain(trustDomain)
	adminAddr := os.Getenv("ADMIN_HTTP_ADDR")
	if adminAddr == "" {
		adminAddr = ":8081"
	}
	adminAuthToken := os.Getenv("ADMIN_AUTH_TOKEN")
	internalAuthToken := os.Getenv("INTERNAL_API_TOKEN")
	policySigningKey := os.Getenv("POLICY_SIGNING_KEY")
	if policySigningKey == "" {
		policySigningKey = internalAuthToken
	}
	policyTTL := 10 * time.Minute
	if v := strings.TrimSpace(os.Getenv("POLICY_SNAPSHOT_TTL_SECONDS")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			policyTTL = time.Duration(secs) * time.Second
		}
	}
	tokenStorePath := os.Getenv("TOKEN_STORE_PATH")
	if tokenStorePath == "" {
		tokenStorePath = "/var/lib/grpccontroller/tokens.json"
	}

	if adminAuthToken == "" {
		log.Fatal("ADMIN_AUTH_TOKEN is not set")
	}
	if internalAuthToken == "" {
		log.Fatal("INTERNAL_API_TOKEN is not set")
	}

	// ---- load internal CA ----
	caInst, err := ca.LoadCA(caCertPEM, caKeyPEM)
	if err != nil {
		log.Fatalf("failed to load internal CA: %v", err)
	}

	// ---- load or issue controller TLS certificate ----
	controllerTLSCert, err := loadOrIssueControllerCert(caInst, trustDomain)
	if err != nil {
		log.Fatalf("failed to prepare controller TLS cert: %v", err)
	}

	// ---- TLS config (mTLS with workspace-CA-aware client cert verification) ----
	// ClientCAs is intentionally not set: workspace-enrolled connectors present a chain
	// [leaf, workspace-CA] where the workspace CA is a sub-CA signed by the global CA.
	// Go's built-in ClientCAs check only knows the global CA and would reject these certs
	// before reaching the gRPC interceptor. Instead we use RequestClientCert + a custom
	// VerifyPeerCertificate that reconstructs the full chain against the global CA root,
	// automatically enforcing name constraints on any intermediate workspace CA.
	tlsConfig := &tls.Config{
		Certificates:          []tls.Certificate{controllerTLSCert},
		ClientAuth:            tls.RequestClientCert,
		MinVersion:            tls.VersionTLS13,
		VerifyPeerCertificate: buildWorkspaceAwarePeerVerifier(caCertPEM),
	}

	creds := credentials.NewTLS(tlsConfig)

	db, err := state.Open(os.Getenv("DATABASE_URL"), os.Getenv("DB_PATH"))
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Token TTL configuration - defaults to 24 hours if not set or 0
	tokenTTLMinutes := 0
	if v := strings.TrimSpace(os.Getenv("TOKEN_TTL_MINUTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			tokenTTLMinutes = n
			log.Printf("using custom token TTL: %d minutes", n)
		}
	}

	registry := state.NewRegistry()
	agentRegistry := state.NewAgentRegistry()
	agentStatus := state.NewAgentStatusRegistry()
	aclStore := state.NewACLStoreWithDB(db)
	tokenStore := state.NewTokenStoreWithDB(tokenTTLMinutes, db)
	userStore := state.NewUserStore(db)
	remoteNetStore := state.NewRemoteNetworkStore(db)
	workspaceStore := state.NewWorkspaceStore(db)

	idpEncKey := []byte(os.Getenv("IDP_ENCRYPTION_KEY"))
	if len(idpEncKey) == 0 {
		idpEncKey = []byte(os.Getenv("JWT_SECRET"))
	}
	// Encrypt workspace CA private keys at rest using the same key as IdP secrets.
	workspaceStore.SetEncryptionKey(idpEncKey)
	idpStore := state.NewIdentityProviderStore(db, idpEncKey)
	sessionStore := state.NewSessionStore(db)

	systemDomain := os.Getenv("SYSTEM_DOMAIN")
	if systemDomain == "" {
		systemDomain = "zerotrust.com"
	}

	// ---- gRPC server ----
	grpcServer := grpc.NewServer(
		grpc.Creds(creds),
		grpc.UnaryInterceptor(api.UnaryAuthInterceptor(trustDomain, map[string]struct{}{
			controllerpb.EnrollmentService_EnrollConnector_FullMethodName:     {},
			controllerpb.EnrollmentService_EnrollTunneler_FullMethodName:      {},
			controllerpb.DeviceService_DeviceAuthorize_FullMethodName:         {},
			controllerpb.DeviceService_DeviceToken_FullMethodName:             {},
			controllerpb.DeviceService_DeviceRefresh_FullMethodName:           {},
			controllerpb.DeviceService_DeviceRevoke_FullMethodName:            {},
			controllerpb.DeviceService_DeviceEnrollCert_FullMethodName:        {},
			controllerpb.DeviceService_DeviceMe_FullMethodName:                {},
			controllerpb.DeviceService_DeviceSync_FullMethodName:              {},
			controllerpb.DeviceService_DeviceReportPosture_FullMethodName:     {},
		}, "connector", "agent")),
		grpc.StreamInterceptor(api.StreamSPIFFEInterceptor(trustDomain, "connector", "agent")),
	)

	scanStore := state.NewScanStore()
	controlPlaneServer := api.NewControlPlaneServer(trustDomain, registry, agentRegistry, agentStatus, aclStore, db, []byte(policySigningKey), policyTTL, scanStore)
	_ = state.LoadConnectorsFromDB(db, registry)
	_ = state.LoadAgentRegistryFromDB(db, agentRegistry)
	_ = state.LoadAgentsFromDB(db, agentStatus)
	_ = state.LoadACLsFromDB(db, aclStore)
	controlPlaneServer.NotifyACLInit()
	controlPlaneServer.StartBatcher()
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			_ = state.PruneAuditLogs(db, time.Now().Add(-24*time.Hour))
		}
	}()
	// Prune stale discovered services periodically.
	go func() {
		retentionHours := 72 // default 3 days
		if v := os.Getenv("DISCOVERY_RETENTION_HOURS"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				retentionHours = n
			}
		}
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-time.Duration(retentionHours) * time.Hour)
			if n, err := state.PruneDiscoveredServices(db, cutoff); err != nil {
				log.Printf("discovery prune error: %v", err)
			} else if n > 0 {
				log.Printf("discovery prune: deleted %d stale rows (retention=%dh)", n, retentionHours)
			}
		}
	}()
	// Mark discovered services stale when their agent is offline.
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if n, err := state.MarkStaleDiscoveredServices(db, 90*time.Second); err != nil {
				log.Printf("discovery staleness check error: %v", err)
			} else if n > 0 {
				log.Printf("discovery staleness: marked %d services stale", n)
			}
		}
	}()
	// Mark connectors and tunnelers offline when their last heartbeat is stale.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-45 * time.Second).Unix()
			_, _ = db.Exec(state.Rebind(`UPDATE connectors SET status='offline' WHERE status='online' AND last_seen < ?`), cutoff)
			_, _ = db.Exec(state.Rebind(`UPDATE agents     SET status='offline' WHERE status='online' AND last_seen < ?`), cutoff)
		}
	}()

	// ---- rotate any workspace CAs minted with the old broken URI constraint ----
	rotateWorkspaceCAsIfNeeded(workspaceStore, caInst)

	// ---- trust domain validator (multi-tenant) ----
	api.SetTrustDomainValidator(api.NewTrustDomainValidator(trustDomain, systemDomain))

	// ---- enrollment service ----
	enrollServer := api.NewEnrollmentServer(
		caInst,
		caCertPEM,
		trustDomain, // SPIFFE trust domain (without scheme)
		tokenStore,
		registry,
		controlPlaneServer,
	)
	crl := ca.NewCRL()
	if err := crl.LoadFromDB(db); err != nil {
		log.Printf("crl: failed to load from DB: %v", err)
	}
	enrollServer.DB = db
	enrollServer.Workspaces = workspaceStore
	enrollServer.SystemDomain = systemDomain
	enrollServer.CRL = crl

	controllerpb.RegisterEnrollmentServiceServer(grpcServer, enrollServer)
	controllerpb.RegisterControlPlaneServer(grpcServer, controlPlaneServer)

	// ---- OAuth + mailer config (optional) ----
	var oauthCfg = admin.BuildGoogleOAuthConfig(
		os.Getenv("GOOGLE_CLIENT_ID"),
		os.Getenv("GOOGLE_CLIENT_SECRET"),
		os.Getenv("OAUTH_REDIRECT_URL"),
	)
	// Client app: used for PKCE flows (device auth v2 + invite registration).
	// Falls back to oauthCfg if unset.
	var clientOAuthCfg = admin.BuildClientGoogleOAuthConfig(
		os.Getenv("CLIENT_GOOGLE_CLIENT_ID"),
		os.Getenv("CLIENT_GOOGLE_CLIENT_SECRET"),
		os.Getenv("CLIENT_OAUTH_REDIRECT_URL"),
	)
	var githubOAuthCfg = admin.BuildGitHubOAuthConfig(
		os.Getenv("GITHUB_CLIENT_ID"),
		os.Getenv("GITHUB_CLIENT_SECRET"),
		os.Getenv("GITHUB_OAUTH_REDIRECT_URL"),
	)

	adminLoginEmails := map[string]struct{}{}
	if raw := os.Getenv("ADMIN_LOGIN_EMAILS"); raw != "" {
		for _, e := range strings.Split(raw, ",") {
			if em := strings.TrimSpace(strings.ToLower(e)); em != "" {
				adminLoginEmails[em] = struct{}{}
			}
		}
	}

	signupAllowedDomains := map[string]struct{}{}
	if raw := os.Getenv("SIGNUP_ALLOWED_DOMAINS"); raw != "" {
		for _, d := range strings.Split(raw, ",") {
			if domain := strings.TrimSpace(strings.ToLower(d)); domain != "" {
				signupAllowedDomains[domain] = struct{}{}
			}
		}
	}

	var m *mailer.Mailer
	if host := os.Getenv("SMTP_HOST"); host != "" {
		m = mailer.New(
			host,
			os.Getenv("SMTP_PORT"),
			os.Getenv("SMTP_USER"),
			os.Getenv("SMTP_PASS"),
			os.Getenv("SMTP_FROM"),
		)
	}

	// ---- admin HTTP server ----
	adminMux := http.NewServeMux()
	adminServer := &admin.Server{
		Tokens:            tokenStore,
		Reg:               registry,
		Agents:            agentStatus,
		ACLs:              aclStore,
		ACLNotify:         controlPlaneServer,
		Users:             userStore,
		RemoteNet:         remoteNetStore,
		ScanStore:         scanStore,
		ControlPlane:      controlPlaneServer,
		StreamChecker:     controlPlaneServer,
		AdminAuthToken:    adminAuthToken,
		InternalAuthToken: internalAuthToken,
		CACertPEM:         caCertPEM,
		OAuthConfig:       oauthCfg,
		ClientOAuthConfig: clientOAuthCfg,
		GitHubOAuthConfig: githubOAuthCfg,
		JWTSecret:         []byte(os.Getenv("JWT_SECRET")),
		AdminLoginEmails:     adminLoginEmails,
		SignupAllowedDomains: signupAllowedDomains,
		DashboardURL:      os.Getenv("DASHBOARD_URL"),
		InviteBaseURL:     os.Getenv("INVITE_BASE_URL"),
		Mailer:            m,
		Workspaces:        workspaceStore,
		IntermediateCA:    caInst,
		SystemDomain:      systemDomain,
		IdPs:              idpStore,
		Sessions:          sessionStore,
		SecureCookies:     os.Getenv("SECURE_COOKIES") == "true",
		AllowedOrigins:    parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS")),
		MaxSessionsPerUser:   parseIntEnv("MAX_SESSIONS_PER_USER", 5),
		StrictSessionBinding: os.Getenv("STRICT_SESSION_BINDING") == "true",
		AccessRequests:    state.NewAccessRequestStore(db),
		AuditKey:          []byte(os.Getenv("JWT_SECRET")),
	}
	admin.SetCORSOrigins(parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS")), os.Getenv("DASHBOARD_URL"))
	adminServer.RegisterRoutes(adminMux)
	adminServer.RegisterOAuthRoutes(adminMux)
	adminMux.HandleFunc("/ca.crl", func(w http.ResponseWriter, r *http.Request) {
		crlPEM, err := crl.Encode(caInst)
		if err != nil {
			http.Error(w, "failed to generate CRL", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Write(crlPEM)
	})

	// ---- device gRPC service ----
	deviceSvcServer := &admin.DeviceServiceServer{S: adminServer}
	controllerpb.RegisterDeviceServiceServer(grpcServer, deviceSvcServer)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := sessionStore.CleanExpired(); err != nil {
				log.Printf("session cleanup: %v", err)
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		arStore := state.NewAccessRequestStore(db)
		for range ticker.C {
			if n, err := arStore.CleanExpiredGrants(); err != nil {
				log.Printf("access request grant cleanup: %v", err)
			} else if n > 0 {
				log.Printf("access request grant cleanup: expired %d grants", n)
			}
		}
	}()
	go func() {
		log.Printf("admin HTTP server listening %s", adminAddr)
		if err := http.ListenAndServe(adminAddr, adminMux); err != nil {
			log.Fatalf("admin HTTP server failed: %v", err)
		}
	}()

	// ---- OAuth callback listener ----
	// Google OAuth apps register specific redirect URIs (e.g. :8080).
	// If OAUTH_CALLBACK_ADDR is set, start an additional listener on that address
	// serving the same mux so the registered callback URIs resolve correctly.
	if oauthCallbackAddr := strings.TrimSpace(os.Getenv("OAUTH_CALLBACK_ADDR")); oauthCallbackAddr != "" && oauthCallbackAddr != adminAddr {
		go func() {
			log.Printf("OAuth callback listener on %s", oauthCallbackAddr)
			if err := http.ListenAndServe(oauthCallbackAddr, adminMux); err != nil {
				log.Fatalf("OAuth callback listener failed: %v", err)
			}
		}()
	}

	// ---- listen ----
	lis, err := net.Listen("tcp", ":8443")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	log.Println("controller gRPC server listening on :8443")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server failed: %v", err)
	}
}

func loadCA() ([]byte, []byte, error) {
	certPEM := []byte(os.Getenv("INTERNAL_CA_CERT"))
	keyPEM := []byte(os.Getenv("INTERNAL_CA_KEY"))
	certPath := envOrDefault("INTERNAL_CA_CERT_PATH", "ca/ca.crt")
	keyPath := envOrDefault("INTERNAL_CA_KEY_PATH", "ca/ca.pkcs8.key")

	if len(certPEM) == 0 {
		b, err := readOptionalFile(certPath)
		if err != nil {
			return nil, nil, err
		}
		certPEM = b
	}
	if len(keyPEM) == 0 {
		b, err := readOptionalFile(keyPath)
		if err != nil {
			return nil, nil, err
		}
		keyPEM = b
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return nil, nil, errors.New("INTERNAL_CA_CERT or INTERNAL_CA_KEY is not set, and CA files were not available at " + certPath + " and " + keyPath)
	}

	return certPEM, keyPEM, nil
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func readOptionalFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		return b, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return nil, errors.New("failed to read " + path + ": " + err.Error())
}

func parseAllowedOrigins(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

func normalizeTrustDomain(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimSuffix(v, ".")
	return v
}

func parseIntEnv(key string, defaultVal int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultVal
}

// buildWorkspaceAwarePeerVerifier returns a VerifyPeerCertificate callback that
// accepts client certs from both global-enrolled and workspace-enrolled connectors/agents.
//
// Global connector:   rawCerts = [leaf]              chain: leaf → global CA (root)
// Workspace connector: rawCerts = [leaf, workspace-CA] chain: leaf → workspace-CA → global CA
//
// Name constraints on any intermediate are enforced by Go's x509 library, so
// workspace CAs minted with the old broken PermittedURIDomains are still rejected.
//
// If rawCerts is empty (initial enrollment — no cert yet) the callback returns nil;
// the gRPC interceptor handles auth via the enrollment token in the request body.
func buildWorkspaceAwarePeerVerifier(globalCAPEM []byte) func([][]byte, [][]*x509.Certificate) error {
	globalPool := x509.NewCertPool()
	if !globalPool.AppendCertsFromPEM(globalCAPEM) {
		log.Fatal("buildWorkspaceAwarePeerVerifier: failed to parse global CA PEM")
	}
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return nil
		}
		leaf, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("parse client cert: %w", err)
		}
		intermediates := x509.NewCertPool()
		for _, raw := range rawCerts[1:] {
			if c, err := x509.ParseCertificate(raw); err == nil {
				intermediates.AddCert(c)
			}
		}
		_, err = leaf.Verify(x509.VerifyOptions{
			Roots:         globalPool,
			Intermediates: intermediates,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		})
		return err
	}
}

// rotateWorkspaceCAsIfNeeded inspects every active workspace CA and re-issues any
// that were minted with the pre-fix PermittedURIDomains (entries prefixed "spiffe://").
// After rotation, connectors/agents enrolled under the old CA must be restarted to
// re-enroll; their existing short-lived certs (5-min TTL) will fail chain verification
// until they pick up a cert signed by the new CA.
func rotateWorkspaceCAsIfNeeded(workspaceStore *state.WorkspaceStore, parentCA *ca.CA) {
	if workspaceStore == nil {
		return
	}
	workspaces, err := workspaceStore.ListAllWorkspaces()
	if err != nil {
		log.Printf("ca-rotation: failed to list workspaces: %v", err)
		return
	}
	rotated := 0
	for _, ws := range workspaces {
		if !ca.HasBrokenURIConstraint([]byte(ws.CACertPEM)) {
			continue
		}
		certPEM, keyPEM, err := ca.IssueWorkspaceCA(parentCA, ws.TrustDomain, 365*24*time.Hour)
		if err != nil {
			log.Printf("ca-rotation: failed to reissue CA for workspace %s (%s): %v", ws.ID, ws.TrustDomain, err)
			continue
		}
		if err := workspaceStore.UpdateWorkspaceCA(ws.ID, string(certPEM), string(keyPEM)); err != nil {
			log.Printf("ca-rotation: failed to store rotated CA for workspace %s (%s): %v", ws.ID, ws.TrustDomain, err)
			continue
		}
		log.Printf("ca-rotation: rotated workspace CA %s (%s)", ws.ID, ws.TrustDomain)
		rotated++
	}
	if rotated > 0 {
		log.Printf("ca-rotation: rotated %d workspace CA(s) with broken URI constraints — restart affected connectors/agents to re-enroll", rotated)
	}
}

func loadOrIssueControllerCert(caInst *ca.CA, trustDomain string) (tls.Certificate, error) {
	controllerCertPEM := []byte(os.Getenv("CONTROLLER_CERT"))
	controllerKeyPEM := []byte(os.Getenv("CONTROLLER_KEY"))
	if len(controllerCertPEM) > 0 && len(controllerKeyPEM) > 0 {
		return tls.X509KeyPair(controllerCertPEM, controllerKeyPEM)
	}

	controllerID := os.Getenv("CONTROLLER_ID")
	if controllerID == "" {
		controllerID = "default"
	}
	spiffeID := "spiffe://" + trustDomain + "/controller/" + controllerID

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	dnsNames := []string{"localhost"}
	ipAddrs := []net.IP{net.ParseIP("127.0.0.1")}

	// Add SANs based on CONTROLLER_ADDR if provided (host:port or host).
	if addr := strings.TrimSpace(os.Getenv("CONTROLLER_ADDR")); addr != "" {
		host := addr
		if h, _, err := net.SplitHostPort(addr); err == nil {
			host = h
		}
		if ip := net.ParseIP(host); ip != nil {
			ipAddrs = append(ipAddrs, ip)
		} else if host != "" && host != "localhost" {
			dnsNames = append(dnsNames, host)
		}
	}

	// Add all non-loopback interface IPs (LAN addresses).
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip == nil || ip.IsLoopback() {
					continue
				}
				ipAddrs = append(ipAddrs, ip)
			}
		}
	}

	certPEM, err := ca.IssueWorkloadCert(caInst, spiffeID, &privKey.PublicKey, 12*time.Hour, dnsNames, ipAddrs)
	if err != nil {
		return tls.Certificate{}, err
	}

	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return tls.Certificate{}, errors.New("failed to decode controller certificate")
	}

	return tls.Certificate{
		Certificate: [][]byte{block.Bytes},
		PrivateKey:  privKey,
	}, nil
}
