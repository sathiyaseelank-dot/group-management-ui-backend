package admin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"controller/ca"
	controllerpb "controller/gen/controllerpb"
	"controller/state"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// DeviceServiceServer implements controllerpb.DeviceServiceServer using the
// same business logic as the HTTP device handlers, sharing all admin helpers.
type DeviceServiceServer struct {
	controllerpb.UnimplementedDeviceServiceServer
	S *Server // pointer to the admin.Server for access to all dependencies
}

// tokenFromGRPCContext extracts a Bearer token from gRPC incoming metadata.
func tokenFromGRPCContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return ""
	}
	v := vals[0]
	if after, ok := strings.CutPrefix(v, "Bearer "); ok {
		return after
	}
	if after, ok := strings.CutPrefix(v, "bearer "); ok {
		return after
	}
	return ""
}

func (d *DeviceServiceServer) deviceClaimsFromGRPC(ctx context.Context) (allClaims, error) {
	tok := tokenFromGRPCContext(ctx)
	return parseAllClaims(tok, d.S.JWTSecret)
}

func (d *DeviceServiceServer) grpcDB(ctx context.Context) (*sql.DB, error) {
	db := d.S.db()
	if db == nil {
		return nil, status.Error(codes.Internal, "database not available")
	}
	return db, nil
}

// DeviceAuthorize — unauthenticated PKCE start.
func (d *DeviceServiceServer) DeviceAuthorize(ctx context.Context, req *controllerpb.DeviceAuthorizeRequest) (*controllerpb.DeviceAuthorizeResponse, error) {
	if req.TenantSlug == "" || req.CodeChallenge == "" || req.RedirectUri == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_slug, code_challenge, and redirect_uri are required")
	}
	if !isAllowedRedirectURI(req.RedirectUri) {
		return nil, status.Error(codes.InvalidArgument, "redirect_uri must be a loopback address or a custom URI scheme")
	}
	if req.CodeChallengeMethod != "" && req.CodeChallengeMethod != "S256" {
		return nil, status.Error(codes.InvalidArgument, "only S256 code_challenge_method is supported")
	}

	db, err := d.grpcDB(ctx)
	if err != nil {
		return nil, err
	}

	var ws state.Workspace
	queryErr := db.QueryRow(
		state.Rebind(`SELECT id, name, slug FROM workspaces WHERE slug = ? LIMIT 1`),
		req.TenantSlug,
	).Scan(&ws.ID, &ws.Name, &ws.Slug)
	if queryErr == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "workspace not found")
	}
	if queryErr != nil {
		return nil, status.Error(codes.Internal, "database error")
	}

	// Find an enabled IdP for this workspace
	var idpID, idpType string
	if d.S.IdPs != nil {
		for _, pt := range []string{"google", "github", "oidc"} {
			idp, idpErr := d.S.IdPs.GetEnabledByType(ws.ID, pt)
			if idpErr == nil && idp != nil {
				idpID = idp.ID
				idpType = idp.ProviderType
				break
			}
		}
	}

	// Fallback to env-var OAuth
	if idpID == "" {
		if d.S.OAuthConfig != nil {
			idpType = "google"
		} else if d.S.GitHubOAuthConfig != nil {
			idpType = "github"
		} else {
			return nil, status.Error(codes.FailedPrecondition, "no identity provider configured for this workspace")
		}
	}

	csrfState, err := randomHex(16)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	deviceState := "device:" + csrfState

	_, err = db.Exec(
		state.Rebind(`INSERT INTO device_auth_requests (state, workspace_id, code_challenge, redirect_uri, idp_id, platform, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, 'cli', ?, ?)`),
		deviceState, ws.ID, req.CodeChallenge, req.RedirectUri, idpID,
		time.Now().Unix(), time.Now().Add(10*time.Minute).Unix(),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to store auth request")
	}
	storeOAuthState(deviceState)

	baseURL := d.S.InviteBaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8081"
	}
	callbackURI := baseURL + "/api/device/callback"

	var authURL string
	if d.S.IdPs != nil && idpID != "" {
		idp, idpErr := d.S.IdPs.GetEnabledByType(ws.ID, idpType)
		if idpErr == nil {
			secret, _ := d.S.IdPs.DecryptSecret(idp)
			cfg := buildIdPOAuthConfig(idp, secret, callbackURI)
			authURL = cfg.AuthCodeURL(deviceState)
		}
	}

	if authURL == "" {
		if idpType == "github" && d.S.GitHubOAuthConfig != nil {
			cfg := *d.S.GitHubOAuthConfig
			cfg.RedirectURL = callbackURI
			authURL = cfg.AuthCodeURL(deviceState)
		} else if d.S.OAuthConfig != nil {
			// Use the admin OAuth app — its redirect URI (/oauth/google/callback) is
			// already registered in Google Console and routes through handleOAuthCallback
			// which forwards device: states to handleDeviceCallback.
			authURL = d.S.OAuthConfig.AuthCodeURL(deviceState)
		} else {
			clientCfg := d.S.effectiveClientOAuthConfig()
			if clientCfg != nil {
				cfg := *clientCfg
				cfg.RedirectURL = callbackURI
				authURL = cfg.AuthCodeURL(deviceState)
			}
		}
	}

	if authURL == "" {
		return nil, status.Error(codes.Internal, "failed to build auth URL")
	}

	return &controllerpb.DeviceAuthorizeResponse{
		AuthUrl: authURL,
		State:   deviceState,
	}, nil
}

// DeviceToken — unauthenticated PKCE code exchange.
func (d *DeviceServiceServer) DeviceToken(ctx context.Context, req *controllerpb.DeviceTokenRequest) (*controllerpb.DeviceTokenResponse, error) {
	if req.Code == "" || req.CodeVerifier == "" {
		return nil, status.Error(codes.InvalidArgument, "code and code_verifier are required")
	}

	entry, ok := consumeDeviceCode(req.Code)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "invalid or expired code")
	}

	// Verify PKCE
	if entry.codeChallenge != "" {
		h := sha256.Sum256([]byte(req.CodeVerifier))
		computed := encodeBase64URL(h[:])
		if computed != entry.codeChallenge {
			return nil, status.Error(codes.InvalidArgument, "pkce verification failed")
		}
	}

	db, err := d.grpcDB(ctx)
	if err != nil {
		return nil, err
	}

	email := entry.email
	var userID string
	if d.S.Users != nil {
		u, lookupErr := d.S.Workspaces.GetUserByEmail(email)
		if lookupErr == nil {
			userID = u.ID
		} else {
			newUser := state.User{
				Name:      email,
				Email:     email,
				Status:    "Active",
				Role:      "Member",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			if createErr := d.S.Users.CreateUser(&newUser); createErr != nil {
				log.Printf("device token grpc: failed to create user %s: %v", email, createErr)
			}
			if u2, lookupErr2 := d.S.Workspaces.GetUserByEmail(email); lookupErr2 == nil {
				userID = u2.ID
			}
		}
	}

	sessionID, err := randomHex(16)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	refreshTokenRaw, err := randomHex(32)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	hashBytes := sha256.Sum256([]byte(refreshTokenRaw))
	refreshTokenHash := hex.EncodeToString(hashBytes[:])

	if d.S.Sessions != nil {
		sess := &state.Session{
			ID:               sessionID,
			UserID:           userID,
			WorkspaceID:      entry.wsID,
			SessionType:      "device",
			RefreshTokenHash: refreshTokenHash,
			CreatedAt:        time.Now().Unix(),
			ExpiresAt:        time.Now().Add(30 * 24 * time.Hour).Unix(),
		}
		if createErr := d.S.Sessions.Create(sess); createErr != nil {
			log.Printf("device token grpc: failed to create session: %v", createErr)
		}
	}

	wsRole := lookupWorkspaceMemberRole(db, entry.wsID, userID)
	accessToken, signErr := d.S.signDeviceJWT(email, userID, entry.wsID, entry.wsSlug, wsRole, "", sessionID)
	if signErr != nil {
		return nil, status.Error(codes.Internal, "failed to create access token")
	}

	return &controllerpb.DeviceTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenRaw,
		ExpiresIn:    900,
	}, nil
}

// DeviceRefresh — unauthenticated token refresh.
func (d *DeviceServiceServer) DeviceRefresh(ctx context.Context, req *controllerpb.DeviceRefreshRequest) (*controllerpb.DeviceRefreshResponse, error) {
	if req.RefreshToken == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh_token is required")
	}
	if d.S.Sessions == nil {
		return nil, status.Error(codes.Unavailable, "session store not configured")
	}

	hashBytes := sha256.Sum256([]byte(req.RefreshToken))
	tokenHash := hex.EncodeToString(hashBytes[:])

	sess, err := d.S.Sessions.GetByRefreshTokenHash(tokenHash)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid refresh token")
	}
	if sess.Revoked || time.Now().Unix() > sess.ExpiresAt {
		return nil, status.Error(codes.Unauthenticated, "refresh token expired or revoked")
	}

	var email, wsSlug string
	db := d.S.db()
	if db != nil && sess.UserID != "" {
		_ = db.QueryRow(state.Rebind(`SELECT email FROM users WHERE id = ?`), sess.UserID).Scan(&email)
		_ = db.QueryRow(state.Rebind(`SELECT slug FROM workspaces WHERE id = ?`), sess.WorkspaceID).Scan(&wsSlug)
	}

	newRefreshRaw, err := randomHex(32)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	newHashBytes := sha256.Sum256([]byte(newRefreshRaw))
	newHash := hex.EncodeToString(newHashBytes[:])
	if err := d.S.Sessions.UpdateRefreshToken(sess.ID, newHash); err != nil {
		return nil, status.Error(codes.Internal, "failed to rotate refresh token")
	}

	wsRole := lookupWorkspaceMemberRole(db, sess.WorkspaceID, sess.UserID)
	accessToken, signErr := d.S.signDeviceJWT(email, sess.UserID, sess.WorkspaceID, wsSlug, wsRole, sess.DeviceID, sess.ID)
	if signErr != nil {
		return nil, status.Error(codes.Internal, "failed to create access token")
	}

	return &controllerpb.DeviceRefreshResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshRaw,
		ExpiresIn:    900,
	}, nil
}

// DeviceRevoke — unauthenticated token revocation.
func (d *DeviceServiceServer) DeviceRevoke(ctx context.Context, req *controllerpb.DeviceRevokeRequest) (*controllerpb.DeviceRevokeResponse, error) {
	if d.S.Sessions == nil {
		return nil, status.Error(codes.Unavailable, "session store not configured")
	}

	if req.RefreshToken != "" {
		hashBytes := sha256.Sum256([]byte(req.RefreshToken))
		tokenHash := hex.EncodeToString(hashBytes[:])
		sess, err := d.S.Sessions.GetByRefreshTokenHash(tokenHash)
		if err == nil {
			_ = d.S.Sessions.Revoke(sess.ID)
		}
	}

	return &controllerpb.DeviceRevokeResponse{Status: "revoked"}, nil
}

// DeviceEnrollCert — JWT authenticated device certificate enrollment.
func (d *DeviceServiceServer) DeviceEnrollCert(ctx context.Context, req *controllerpb.DeviceEnrollCertRequest) (*controllerpb.DeviceEnrollCertResponse, error) {
	if d.S.Workspaces == nil || d.S.Sessions == nil {
		return nil, status.Error(codes.Unavailable, "device certificate enrollment not configured")
	}

	claims, err := d.deviceClaimsFromGRPC(ctx)
	if err != nil || claims.aud != "device" {
		return nil, status.Error(codes.Unauthenticated, "unauthorized: device token required")
	}

	req.DeviceId = strings.TrimSpace(req.DeviceId)
	req.PublicKeyPem = strings.TrimSpace(req.PublicKeyPem)
	if req.DeviceId == "" || req.PublicKeyPem == "" {
		return nil, status.Error(codes.InvalidArgument, "device_id and public_key_pem are required")
	}

	workspace, wsErr := d.S.Workspaces.GetWorkspace(claims.wsID)
	if wsErr != nil {
		return nil, status.Error(codes.NotFound, "workspace not found")
	}
	if strings.TrimSpace(workspace.CACertPEM) == "" || strings.TrimSpace(workspace.CAKeyPEM) == "" {
		return nil, status.Error(codes.Unavailable, "workspace ca not configured")
	}

	pubKey, pkErr := parsePEMPublicKey(req.PublicKeyPem)
	if pkErr != nil {
		return nil, status.Error(codes.InvalidArgument, pkErr.Error())
	}

	issuerCA, caErr := ca.LoadCA([]byte(workspace.CACertPEM), []byte(workspace.CAKeyPEM))
	if caErr != nil {
		return nil, status.Error(codes.Internal, "failed to load workspace ca")
	}

	deviceID := req.DeviceId
	spiffeID := fmt.Sprintf("spiffe://%s/device/%s/%s", workspace.TrustDomain, claims.userID, deviceID)
	certTTL := 24 * time.Hour
	certPEM, certErr := ca.IssueWorkloadCert(issuerCA, spiffeID, pubKey, certTTL, nil, nil)
	if certErr != nil {
		return nil, status.Error(codes.Internal, "failed to issue certificate")
	}

	if err := d.S.Sessions.UpdateDeviceID(claims.jti, deviceID); err != nil {
		return nil, status.Error(codes.Internal, "failed to update session device id")
	}

	db := d.S.db()
	if db != nil {
		now := isoStringNow()
		_ = state.UpsertDevicePosture(db, state.DevicePosture{
			DeviceID:      deviceID,
			WorkspaceID:   claims.wsID,
			SPIFFEID:      spiffeID,
			OSType:        req.Os,
			OSVersion:     "",
			Hostname:      req.Hostname,
			ClientVersion: req.ClientVersion,
			CollectedAt:   now,
			UserID:        claims.userID,
			DeviceName:    req.DeviceName,
			DeviceModel:   req.DeviceModel,
			DeviceMake:    req.DeviceMake,
			SerialNumber:  req.SerialNumber,
		})
	}

	role := lookupWorkspaceMemberRole(db, claims.wsID, claims.userID)
	accessToken, signErr := d.S.signDeviceJWT(claims.email, claims.userID, claims.wsID, claims.wsSlug, role, deviceID, claims.jti)
	if signErr != nil {
		return nil, status.Error(codes.Internal, "failed to issue access token")
	}

	return &controllerpb.DeviceEnrollCertResponse{
		DeviceId:       deviceID,
		SpiffeId:       spiffeID,
		CertificatePem: string(certPEM),
		CaCertPem:      workspace.CACertPEM,
		ExpiresAt:      time.Now().Add(certTTL).Unix(),
		AccessToken:    accessToken,
	}, nil
}

// deviceViewResponse builds a DeviceViewResponse from claims.
func (d *DeviceServiceServer) deviceViewResponse(ctx context.Context, claims allClaims) (*controllerpb.DeviceViewResponse, error) {
	db, err := d.grpcDB(ctx)
	if err != nil {
		return nil, err
	}

	role := lookupWorkspaceMemberRole(db, claims.wsID, claims.userID)
	resources, resErr := loadAuthorizedResources(db, claims.wsID, claims.userID)
	if resErr != nil {
		return nil, status.Error(codes.Internal, "failed to load resources")
	}

	workspace, wsErr := d.S.Workspaces.GetWorkspace(claims.wsID)
	if wsErr != nil {
		return nil, status.Error(codes.NotFound, "workspace not found")
	}

	sess, sessErr := d.S.Sessions.Get(claims.jti)
	if sessErr != nil {
		return nil, status.Error(codes.Unauthenticated, "session not found")
	}

	protoResources := make([]*controllerpb.DeviceResourceProto, 0, len(resources))
	for _, r := range resources {
		pr := &controllerpb.DeviceResourceProto{
			Id:                r.ID,
			Name:              r.Name,
			Type:              r.Type,
			Address:           r.Address,
			Protocol:          r.Protocol,
			Description:       r.Description,
			RemoteNetworkId:   r.RemoteNetworkID,
			RemoteNetworkName: r.RemoteNetworkName,
			FirewallStatus:    r.FirewallStatus,
		}
		if r.PortFrom != nil {
			pr.PortFrom = int32(*r.PortFrom)
			pr.HasPortFrom = true
		}
		if r.PortTo != nil {
			pr.PortTo = int32(*r.PortTo)
			pr.HasPortTo = true
		}
		if r.Alias != nil {
			pr.Alias = *r.Alias
			pr.HasAlias = true
		}
		protoResources = append(protoResources, pr)
	}

	return &controllerpb.DeviceViewResponse{
		User: &controllerpb.DeviceUserProto{
			Id:    claims.userID,
			Email: claims.email,
			Role:  role,
		},
		Workspace: &controllerpb.DeviceWorkspaceProto{
			Id:          workspace.ID,
			Name:        workspace.Name,
			Slug:        workspace.Slug,
			TrustDomain: workspace.TrustDomain,
		},
		Device: &controllerpb.DeviceSummaryProto{
			Id:                sess.DeviceID,
			CertificateIssued: strings.TrimSpace(sess.DeviceID) != "",
		},
		Session: &controllerpb.DeviceSessionProto{
			Id:                        sess.ID,
			ExpiresAt:                 sess.ExpiresAt,
			AccessTokenExpiresAtHint:  time.Now().Add(15 * time.Minute).Unix(),
		},
		Resources: protoResources,
		SyncedAt:  time.Now().Unix(),
	}, nil
}

// DeviceMe — JWT authenticated device view.
func (d *DeviceServiceServer) DeviceMe(ctx context.Context, req *controllerpb.DeviceViewRequest) (*controllerpb.DeviceViewResponse, error) {
	claims, err := d.deviceClaimsFromGRPC(ctx)
	if err != nil || claims.aud != "device" {
		return nil, status.Error(codes.Unauthenticated, "unauthorized: device token required")
	}
	if d.S.Sessions != nil && claims.jti != "" {
		if valid, err := d.S.Sessions.IsValid(claims.jti); err == nil && !valid {
			return nil, status.Error(codes.Unauthenticated, "session revoked or expired")
		}
	}
	return d.deviceViewResponse(ctx, claims)
}

// DeviceSync — JWT authenticated device sync.
func (d *DeviceServiceServer) DeviceSync(ctx context.Context, req *controllerpb.DeviceViewRequest) (*controllerpb.DeviceViewResponse, error) {
	claims, err := d.deviceClaimsFromGRPC(ctx)
	if err != nil || claims.aud != "device" {
		return nil, status.Error(codes.Unauthenticated, "unauthorized: device token required")
	}
	if d.S.Sessions != nil && claims.jti != "" {
		if valid, err := d.S.Sessions.IsValid(claims.jti); err == nil && !valid {
			return nil, status.Error(codes.Unauthenticated, "session revoked or expired")
		}
	}
	return d.deviceViewResponse(ctx, claims)
}

// DeviceReportPosture — JWT authenticated posture reporting.
func (d *DeviceServiceServer) DeviceReportPosture(ctx context.Context, req *controllerpb.DevicePostureRequest) (*controllerpb.DevicePostureResponse, error) {
	claims, err := d.deviceClaimsFromGRPC(ctx)
	if err != nil || claims.aud != "device" {
		return nil, status.Error(codes.Unauthenticated, "unauthorized: device token required")
	}
	if d.S.Sessions != nil && claims.jti != "" {
		if valid, err := d.S.Sessions.IsValid(claims.jti); err == nil && !valid {
			return nil, status.Error(codes.Unauthenticated, "session revoked or expired")
		}
	}

	db, err := d.grpcDB(ctx)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.DeviceId) == "" {
		return nil, status.Error(codes.InvalidArgument, "device_id is required")
	}

	collectedAt := req.CollectedAt
	if collectedAt == "" {
		collectedAt = isoStringNow()
	}

	if err := state.UpsertDevicePosture(db, state.DevicePosture{
		DeviceID:          req.DeviceId,
		WorkspaceID:       claims.wsID,
		SPIFFEID:          req.SpiffeId,
		OSType:            req.OsType,
		OSVersion:         req.OsVersion,
		Hostname:          req.Hostname,
		FirewallEnabled:   req.FirewallEnabled,
		DiskEncrypted:     req.DiskEncrypted,
		ScreenLockEnabled: req.ScreenLockEnabled,
		ClientVersion:     req.ClientVersion,
		CollectedAt:       collectedAt,
		UserID:            claims.userID,
	}); err != nil {
		return nil, status.Error(codes.Internal, "failed to save posture")
	}

	return &controllerpb.DevicePostureResponse{Status: "ok"}, nil
}
