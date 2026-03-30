package admin

import (
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"controller/ca"
	"controller/state"
)

type deviceUserResource struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Type                string  `json:"type"`
	Address             string  `json:"address"`
	Protocol            string  `json:"protocol"`
	PortFrom            *int    `json:"port_from,omitempty"`
	PortTo              *int    `json:"port_to,omitempty"`
	Alias               *string `json:"alias,omitempty"`
	Description         string  `json:"description"`
	ConnectorID         string  `json:"-"`
	RemoteNetworkID     string  `json:"remote_network_id"`
	RemoteNetworkName   string  `json:"remote_network_name"`
	FirewallStatus      string  `json:"firewall_status"`
	ConnectorTunnelAddr string  `json:"connector_tunnel_addr,omitempty"`
	AvailabilityStatus  string  `json:"availability_status"`
}

const (
	resourceAvailabilityOnline  = "online"
	resourceAvailabilityOffline = "offline"
)

func parsePEMPublicKey(publicKeyPEM string) (interface{}, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("invalid public key pem")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	return pub, nil
}

func deviceClaimsFromRequest(s *Server, r *http.Request) (allClaims, error) {
	return parseAllClaims(s.getTokenFromRequest(r), s.JWTSecret)
}

func loadAuthorizedResources(db *sql.DB, workspaceID, userID string) ([]deviceUserResource, error) {
	rows, err := db.Query(
		state.Rebind(`SELECT DISTINCT
			r.id, r.name, r.type, r.address, r.protocol, r.port_from, r.port_to, r.alias,
			r.description, r.connector_id, r.remote_network_id, COALESCE(rn.name, ''), r.firewall_status
		FROM resources r
		JOIN access_rules ar ON ar.resource_id = r.id AND ar.enabled = 1
		JOIN access_rule_groups arg ON arg.rule_id = ar.id
		JOIN user_groups ug ON ug.id = arg.group_id
		JOIN user_group_members ugm ON ugm.group_id = arg.group_id
		LEFT JOIN remote_networks rn ON rn.id = r.remote_network_id
		WHERE ugm.user_id = ?
		  AND (
			r.workspace_id = ?
			OR ar.workspace_id = ?
			OR ug.workspace_id = ?
			OR (
				COALESCE(TRIM(r.workspace_id), '') = ''
				AND COALESCE(TRIM(ar.workspace_id), '') = ''
				AND COALESCE(TRIM(ug.workspace_id), '') = ''
			)
		  )
		ORDER BY r.name ASC, r.id ASC`),
		userID,
		workspaceID,
		workspaceID,
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []deviceUserResource{}
	for rows.Next() {
		var res deviceUserResource
		var protocol sql.NullString
		var portFrom sql.NullInt64
		var portTo sql.NullInt64
		var alias sql.NullString
		if err := rows.Scan(
			&res.ID,
			&res.Name,
			&res.Type,
			&res.Address,
			&protocol,
			&portFrom,
			&portTo,
			&alias,
			&res.Description,
			&res.ConnectorID,
			&res.RemoteNetworkID,
			&res.RemoteNetworkName,
			&res.FirewallStatus,
		); err != nil {
			return nil, err
		}
		res.Protocol = "TCP"
		if protocol.Valid && strings.TrimSpace(protocol.String) != "" {
			res.Protocol = protocol.String
		}
		if portFrom.Valid {
			v := int(portFrom.Int64)
			res.PortFrom = &v
		}
		if portTo.Valid {
			v := int(portTo.Int64)
			res.PortTo = &v
		}
		if alias.Valid && strings.TrimSpace(alias.String) != "" {
			res.Alias = &alias.String
		}
		if strings.TrimSpace(res.FirewallStatus) == "" {
			res.FirewallStatus = "unprotected"
		}
		res.ConnectorTunnelAddr, res.AvailabilityStatus = lookupAuthorizedConnectorTunnelAddr(db, res.ID)
		out = append(out, res)
	}
	if out == nil {
		out = []deviceUserResource{}
	}
	return out, rows.Err()
}

func lookupAuthorizedConnectorTunnelAddr(db *sql.DB, resourceID string) (string, string) {
	if addr, err := lookupOnlineConnectorTunnelAddrByResource(db, resourceID); err == nil && addr != "" {
		return addr, resourceAvailabilityOnline
	}
	return "", resourceAvailabilityOffline
}

func lookupOnlineConnectorTunnelAddrByResource(db *sql.DB, resourceID string) (string, error) {
	if strings.TrimSpace(resourceID) == "" {
		return "", sql.ErrNoRows
	}

	cutoffUnix := time.Now().Add(-connectorStaleThreshold).Unix()
	rows, err := db.Query(state.Rebind(`SELECT c.connector_tunnel_addr, c.private_ip, c.last_seen
		FROM resource_agents ra
		JOIN agents a ON a.id = ra.agent_id
		JOIN connectors c ON c.id = a.connector_id
		WHERE ra.resource_id = ?
		  AND a.revoked = 0
		  AND a.status = 'online'
		  AND a.last_seen >= ?
		  AND c.revoked = 0
		  AND c.status = 'online'
		  AND (
			COALESCE(TRIM(c.connector_tunnel_addr), '') <> ''
			OR COALESCE(TRIM(c.private_ip), '') <> ''
		  )
		GROUP BY c.id, c.connector_tunnel_addr, c.private_ip, c.last_seen
		ORDER BY c.last_seen DESC, c.id ASC`), resourceID, cutoffUnix)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var tunnelAddr sql.NullString
		var privateIP sql.NullString
		var lastSeen sql.NullInt64
		if err := rows.Scan(&tunnelAddr, &privateIP, &lastSeen); err != nil {
			return "", err
		}
		addr := connectorTunnelAddrFromRecord(tunnelAddr.String, privateIP.String, lastSeen)
		if addr != "" {
			return addr, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", sql.ErrNoRows
}

func connectorTunnelAddrFromRecord(tunnelAddr, privateIP string, lastSeen sql.NullInt64) string {
	if !lastSeen.Valid || lastSeen.Int64 <= 0 {
		return ""
	}
	if time.Since(time.Unix(lastSeen.Int64, 0)) > connectorStaleThreshold {
		return ""
	}
	tunnelAddr = strings.TrimSpace(tunnelAddr)
	if tunnelAddr != "" {
		return tunnelAddr
	}
	privateIP = strings.TrimSpace(privateIP)
	if privateIP == "" {
		return ""
	}
	return formatTunnelAddr(privateIP, 9444)
}

func formatTunnelAddr(host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return fmt.Sprintf("[%s]:%d", host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func (s *Server) writeDeviceView(w http.ResponseWriter, r *http.Request) {
	claims, err := deviceClaimsFromRequest(s, r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	db := s.db()
	if db == nil {
		http.Error(w, "database not available", http.StatusInternalServerError)
		return
	}

	role := lookupWorkspaceMemberRole(db, claims.wsID, claims.userID)
	resources, err := loadAuthorizedResources(db, claims.wsID, claims.userID)
	if err != nil {
		http.Error(w, "failed to load resources", http.StatusInternalServerError)
		return
	}

	// Also include resources from active JIT access grants.
	if s.AccessRequests != nil {
		jitResources, jitErr := loadJITGrantResources(db, claims.wsID, claims.userID)
		if jitErr == nil {
			// Merge JIT resources with group-based resources (deduplicate by ID)
			seen := make(map[string]bool)
			for _, r := range resources {
				seen[r.ID] = true
			}
			for _, r := range jitResources {
				if !seen[r.ID] {
					resources = append(resources, r)
					seen[r.ID] = true
				}
			}
		}
	}

	workspace, err := s.Workspaces.GetWorkspace(claims.wsID)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}

	session, err := s.Sessions.Get(claims.jti)
	if err != nil {
		http.Error(w, "session not found", http.StatusUnauthorized)
		return
	}

	// Posture compliance check: if the device has reported posture and the
	// user's groups have trusted profiles, enforce posture requirements.
	deviceCompliant := true
	var postureViolations []string
	deviceID := claims.deviceID
	if deviceID == "" {
		deviceID = session.DeviceID
	}
	if deviceID != "" {
		posture, postureErr := getDevicePosture(db, deviceID, claims.wsID)
		if postureErr == nil && posture != nil {
			profiles, profErr := getTrustedProfilesForUser(db, claims.wsID, claims.userID)
			if profErr == nil && len(profiles) > 0 {
				compliant, violations := meetsPostureRequirements(posture, profiles)
				deviceCompliant = compliant
				postureViolations = violations
				if !compliant {
					// Non-compliant devices get an empty resource list and session is revoked.
					resources = []deviceUserResource{}
					if s.Sessions != nil {
						_ = s.Sessions.Revoke(claims.jti)
					}
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]string{
			"id":    claims.userID,
			"email": claims.email,
			"role":  role,
		},
		"workspace": map[string]string{
			"id":           workspace.ID,
			"name":         workspace.Name,
			"slug":         workspace.Slug,
			"trust_domain": workspace.TrustDomain,
		},
		"device": map[string]interface{}{
			"id":                 session.DeviceID,
			"certificate_issued": strings.TrimSpace(session.DeviceID) != "",
		},
		"session": map[string]interface{}{
			"id":                           session.ID,
			"expires_at":                   session.ExpiresAt,
			"access_token_expires_at_hint": time.Now().Add(15 * time.Minute).Unix(),
		},
		"resources":          resources,
		"device_compliant":   deviceCompliant,
		"posture_violations": postureViolations,
		"synced_at":          time.Now().Unix(),
	})
}

func (s *Server) handleDevicePostureReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, err := deviceClaimsFromRequest(s, r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	db := s.db()
	if db == nil {
		http.Error(w, "database not available", http.StatusInternalServerError)
		return
	}

	var req struct {
		DeviceID          string `json:"device_id"`
		SPIFFEID          string `json:"spiffe_id"`
		OSType            string `json:"os_type"`
		OSVersion         string `json:"os_version"`
		Hostname          string `json:"hostname"`
		FirewallEnabled   bool   `json:"firewall_enabled"`
		DiskEncrypted     bool   `json:"disk_encrypted"`
		ScreenLockEnabled bool   `json:"screen_lock_enabled"`
		ClientVersion     string `json:"client_version"`
		CollectedAt       string `json:"collected_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.DeviceID) == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	collectedAt := req.CollectedAt
	if collectedAt == "" {
		collectedAt = isoStringNow()
	}

	// Validate posture report for suspicious activity
	suspicious := false
	var validationReasons []string
	valid, reasons := ValidatePostureReport(req.OSType, req.OSVersion, req.ClientVersion, req.Hostname, req.FirewallEnabled, req.DiskEncrypted, req.ScreenLockEnabled)
	if !valid {
		suspicious = true
		validationReasons = reasons
		log.Printf("WARNING: Suspicious posture report from device %s (user %s): reasons=%v", req.DeviceID, claims.userID, reasons)
	}

	suspiciousReasons := ""
	if len(validationReasons) > 0 {
		suspiciousReasons = strings.Join(validationReasons, ",")
	}

	if err := state.UpsertDevicePosture(db, state.DevicePosture{
		DeviceID:          req.DeviceID,
		WorkspaceID:       claims.wsID,
		SPIFFEID:          req.SPIFFEID,
		OSType:            req.OSType,
		OSVersion:         req.OSVersion,
		Hostname:          req.Hostname,
		FirewallEnabled:   req.FirewallEnabled,
		DiskEncrypted:     req.DiskEncrypted,
		ScreenLockEnabled: req.ScreenLockEnabled,
		ClientVersion:     req.ClientVersion,
		CollectedAt:       collectedAt,
		UserID:            claims.userID,
		Suspicious:        suspicious,
		SuspiciousReasons: suspiciousReasons,
	}); err != nil {
		http.Error(w, "failed to save posture", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "ok",
		"suspicious": suspicious,
		"reasons":    validationReasons,
	})
}

func (s *Server) handleDeviceMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeDeviceView(w, r)
}

func (s *Server) handleDeviceSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeDeviceView(w, r)
}

func (s *Server) handleDeviceEnrollCert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Workspaces == nil || s.Sessions == nil {
		http.Error(w, "device certificate enrollment not configured", http.StatusServiceUnavailable)
		return
	}

	limitBody(r)
	claims, err := deviceClaimsFromRequest(s, r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		DeviceID      string `json:"device_id"`
		PublicKeyPEM  string `json:"public_key_pem"`
		Hostname      string `json:"hostname"`
		OS            string `json:"os"`
		ClientVersion string `json:"client_version"`
		DeviceName    string `json:"device_name"`
		DeviceModel   string `json:"device_model"`
		DeviceMake    string `json:"device_make"`
		SerialNumber  string `json:"serial_number"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.PublicKeyPEM = strings.TrimSpace(req.PublicKeyPEM)
	if req.DeviceID == "" || req.PublicKeyPEM == "" {
		http.Error(w, "device_id and public_key_pem are required", http.StatusBadRequest)
		return
	}

	// Sanitize device_id for SPIFFE path safety.
	if strings.Contains(req.DeviceID, "/") || strings.Contains(req.DeviceID, "\\") || len(req.DeviceID) > 255 {
		http.Error(w, "invalid device_id", http.StatusBadRequest)
		return
	}

	// Enforce device enrollment limit per user.
	const maxDevicesPerUser = 10
	if db := s.db(); db != nil {
		count, countErr := state.CountDevicesForUser(db, claims.wsID, claims.userID)
		if countErr == nil && count >= maxDevicesPerUser {
			http.Error(w, "device enrollment limit reached", http.StatusForbidden)
			return
		}
	}

	workspace, err := s.Workspaces.GetWorkspace(claims.wsID)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	if strings.TrimSpace(workspace.CACertPEM) == "" || strings.TrimSpace(workspace.CAKeyPEM) == "" {
		http.Error(w, "workspace ca not configured", http.StatusServiceUnavailable)
		return
	}

	pubKey, err := parsePEMPublicKey(req.PublicKeyPEM)
	if err != nil {
		http.Error(w, "invalid public key", http.StatusBadRequest)
		return
	}

	issuerCA, err := ca.LoadCA([]byte(workspace.CACertPEM), []byte(workspace.CAKeyPEM))
	if err != nil {
		http.Error(w, "failed to load workspace ca", http.StatusInternalServerError)
		return
	}

	deviceID := req.DeviceID
	spiffeID := fmt.Sprintf("spiffe://%s/device/%s/%s", workspace.TrustDomain, claims.userID, deviceID)
	certTTL := 24 * time.Hour
	certPEM, err := ca.IssueWorkloadCert(issuerCA, spiffeID, pubKey, certTTL, nil, nil)
	if err != nil {
		http.Error(w, "failed to issue certificate", http.StatusInternalServerError)
		return
	}
	if err := s.Sessions.UpdateDeviceID(claims.jti, deviceID); err != nil {
		http.Error(w, "failed to update session device id", http.StatusInternalServerError)
		return
	}

	if db := s.db(); db != nil {
		now := isoStringNow()
		_ = state.UpsertDevicePosture(db, state.DevicePosture{
			DeviceID:      deviceID,
			WorkspaceID:   claims.wsID,
			SPIFFEID:      spiffeID,
			OSType:        req.OS,
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

	role := lookupWorkspaceMemberRole(s.db(), claims.wsID, claims.userID)
	accessToken, err := s.signDeviceJWT(claims.email, claims.userID, claims.wsID, claims.wsSlug, role, deviceID, claims.jti)
	if err != nil {
		http.Error(w, "failed to issue access token", http.StatusInternalServerError)
		return
	}

	s.audit(r, "device.cert_enrolled", deviceID, "ok")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id":       deviceID,
		"spiffe_id":       spiffeID,
		"certificate_pem": string(certPEM),
		"ca_cert_pem":     workspace.CACertPEM,
		"expires_at":      time.Now().Add(certTTL).Unix(),
		"access_token":    accessToken,
		"metadata": map[string]string{
			"hostname":       req.Hostname,
			"os":             req.OS,
			"client_version": req.ClientVersion,
		},
	})
}

func loadJITGrantResources(db *sql.DB, workspaceID, userID string) ([]deviceUserResource, error) {
	rows, err := db.Query(
		state.Rebind(`SELECT DISTINCT r.id, r.name, r.type, r.address, r.protocol, r.port_from, r.port_to, r.alias,
            r.description, r.connector_id, r.remote_network_id, COALESCE(rn.name, ''), r.firewall_status
        FROM access_request_grants g
        JOIN resources r ON r.id = g.resource_id
        LEFT JOIN remote_networks rn ON rn.id = r.remote_network_id
        WHERE g.workspace_id = ? AND g.user_id = ? AND g.revoked = 0 AND g.expires_at > ?
        ORDER BY r.name ASC`),
		workspaceID, userID, time.Now().Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []deviceUserResource
	for rows.Next() {
		var res deviceUserResource
		var protocol sql.NullString
		var portFrom sql.NullInt64
		var portTo sql.NullInt64
		var alias sql.NullString
		if err := rows.Scan(&res.ID, &res.Name, &res.Type, &res.Address, &protocol, &portFrom, &portTo, &alias,
			&res.Description, &res.ConnectorID, &res.RemoteNetworkID, &res.RemoteNetworkName, &res.FirewallStatus); err != nil {
			return nil, err
		}
		res.Protocol = "TCP"
		if protocol.Valid && strings.TrimSpace(protocol.String) != "" {
			res.Protocol = protocol.String
		}
		if portFrom.Valid {
			v := int(portFrom.Int64)
			res.PortFrom = &v
		}
		if portTo.Valid {
			v := int(portTo.Int64)
			res.PortTo = &v
		}
		if alias.Valid && strings.TrimSpace(alias.String) != "" {
			res.Alias = &alias.String
		}
		if strings.TrimSpace(res.FirewallStatus) == "" {
			res.FirewallStatus = "unprotected"
		}
		res.ConnectorTunnelAddr, res.AvailabilityStatus = lookupAuthorizedConnectorTunnelAddr(db, res.ID)
		out = append(out, res)
	}
	return out, rows.Err()
}
