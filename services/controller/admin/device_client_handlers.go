package admin

import (
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"

	"controller/ca"
	"controller/state"
)

type deviceUserResource struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	Address           string  `json:"address"`
	Protocol          string  `json:"protocol"`
	PortFrom          *int    `json:"port_from,omitempty"`
	PortTo            *int    `json:"port_to,omitempty"`
	Alias             *string `json:"alias,omitempty"`
	Description       string  `json:"description"`
	RemoteNetworkID   string  `json:"remote_network_id"`
	RemoteNetworkName string  `json:"remote_network_name"`
	FirewallStatus    string  `json:"firewall_status"`
}

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
			r.description, r.remote_network_id, COALESCE(rn.name, ''), r.firewall_status
		FROM resources r
		JOIN access_rules ar ON ar.resource_id = r.id AND ar.enabled = 1
		JOIN access_rule_groups arg ON arg.rule_id = ar.id
		JOIN user_group_members ugm ON ugm.group_id = arg.group_id
		LEFT JOIN remote_networks rn ON rn.id = r.remote_network_id
		WHERE r.workspace_id = ? AND ugm.user_id = ?
		ORDER BY r.name ASC, r.id ASC`),
		workspaceID,
		userID,
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
		out = append(out, res)
	}
	if out == nil {
		out = []deviceUserResource{}
	}
	return out, rows.Err()
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
		"resources": resources,
		"synced_at": time.Now().Unix(),
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
		http.Error(w, err.Error(), http.StatusBadRequest)
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

	role := lookupWorkspaceMemberRole(s.db(), claims.wsID, claims.userID)
	accessToken, err := s.signDeviceJWT(claims.email, claims.userID, claims.wsID, claims.wsSlug, role, deviceID, claims.jti)
	if err != nil {
		http.Error(w, "failed to issue access token", http.StatusInternalServerError)
		return
	}

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
