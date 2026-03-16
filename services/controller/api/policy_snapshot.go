package api

import (
	"controller/state"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

type PolicySnapshot struct {
	SnapshotMeta SnapshotMeta     `json:"snapshot_meta"`
	Resources    []PolicyResource `json:"resources"`
}

type SnapshotMeta struct {
	ConnectorID   string `json:"connector_id"`
	PolicyVersion int    `json:"policy_version"`
	CompiledAt    string `json:"compiled_at"`
	ValidUntil    string `json:"valid_until"`
	Signature     string `json:"signature"`
}

type PostureRequirements struct {
	RequireFirewall       bool   `json:"require_firewall"`
	RequireDiskEncryption bool   `json:"require_disk_encryption"`
	RequireScreenLock     bool   `json:"require_screen_lock"`
	MinOSVersion          string `json:"min_os_version,omitempty"`
}

type PolicyResource struct {
	ResourceID           string               `json:"resource_id"`
	Type                 string               `json:"type"`
	Address              string               `json:"address"`
	Port                 int                  `json:"port"`
	Protocol             string               `json:"protocol"`
	PortFrom             *int                 `json:"port_from,omitempty"`
	PortTo               *int                 `json:"port_to,omitempty"`
	AllowedIdentities    []string             `json:"allowed_identities"`
	FirewallStatus       string               `json:"firewall_status"`
	PostureRequirements  *PostureRequirements `json:"posture_requirements,omitempty"`
}

// UI helpers (shared with admin UI compile endpoints).
func PolicyResourcesForUI(db *sql.DB, remoteNetworkID string) ([]PolicyResource, error) {
	return policyResources(db, remoteNetworkID)
}

func PolicyHashForUI(resources []PolicyResource) string {
	return policyHash(resources)
}

func PolicyVersionForUI(db *sql.DB, connectorID, policyHash, compiledAt string) int {
	return policyVersion(db, connectorID, policyHash, compiledAt)
}

func CompilePolicySnapshot(db *sql.DB, connectorID string, ttl time.Duration, signingKey []byte) (PolicySnapshot, error) {
	if db == nil {
		return PolicySnapshot{}, errors.New("db not configured")
	}
	if connectorID == "" {
		return PolicySnapshot{}, errors.New("connector_id required")
	}
	networkID, err := lookupConnectorNetwork(db, connectorID)
	if err != nil {
		return PolicySnapshot{}, err
	}
	resources, err := policyResources(db, networkID)
	if err != nil {
		return PolicySnapshot{}, err
	}
	now := time.Now().UTC()
	compiledAt := now.Format(time.RFC3339)
	validUntil := now.Add(ttl).Format(time.RFC3339)
	payloadHash := policyHash(resources)
	version := policyVersion(db, connectorID, payloadHash, compiledAt)

	snap := PolicySnapshot{
		SnapshotMeta: SnapshotMeta{
			ConnectorID:   connectorID,
			PolicyVersion: version,
			CompiledAt:    compiledAt,
			ValidUntil:    validUntil,
			Signature:     "",
		},
		Resources: resources,
	}

	snap = normalizeSnapshot(snap)
	sig, err := signSnapshot(signingKey, snap)
	if err != nil {
		return PolicySnapshot{}, err
	}
	snap.SnapshotMeta.Signature = sig
	return snap, nil
}

func signSnapshot(key []byte, snap PolicySnapshot) (string, error) {
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

func normalizeSnapshot(snap PolicySnapshot) PolicySnapshot {
	sort.Slice(snap.Resources, func(i, j int) bool {
		return snap.Resources[i].ResourceID < snap.Resources[j].ResourceID
	})
	for i := range snap.Resources {
		sort.Strings(snap.Resources[i].AllowedIdentities)
	}
	return snap
}

func lookupConnectorNetwork(db *sql.DB, connectorID string) (string, error) {
	var networkID sql.NullString
	if err := db.QueryRow(state.Rebind(`SELECT remote_network_id FROM connectors WHERE id = ?`), connectorID).Scan(&networkID); err != nil {
		return "", err
	}
	if networkID.Valid && strings.TrimSpace(networkID.String) != "" {
		return networkID.String, nil
	}
	// Fallback: check the remote_network_connectors junction table
	var assigned sql.NullString
	if err := db.QueryRow(state.Rebind(`SELECT network_id FROM remote_network_connectors WHERE connector_id = ? LIMIT 1`), connectorID).Scan(&assigned); err != nil {
		return "", fmt.Errorf("connector %s has no network", connectorID)
	}
	if assigned.Valid && strings.TrimSpace(assigned.String) != "" {
		return assigned.String, nil
	}
	return "", fmt.Errorf("connector %s has no network", connectorID)
}

func policyResources(db *sql.DB, remoteNetworkID string) ([]PolicyResource, error) {
	rows, err := db.Query(state.Rebind(`SELECT id, type, address, protocol, port_from, port_to, firewall_status FROM resources WHERE remote_network_id = ? ORDER BY id ASC`), remoteNetworkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	identityStmt, _ := db.Prepare(state.Rebind(`
		SELECT DISTINCT u.certificate_identity
		FROM access_rules ar
		JOIN access_rule_groups arg ON arg.rule_id = ar.id
		JOIN user_group_members gm ON gm.group_id = arg.group_id
		JOIN users u ON u.id = gm.user_id
		WHERE ar.resource_id = ? AND ar.enabled = 1
		  AND u.certificate_identity IS NOT NULL
		  AND LOWER(TRIM(u.status)) = 'active'
		  AND NOT EXISTS (
		      SELECT 1
		      FROM user_groups ug
		      JOIN user_group_members ugm2 ON ugm2.group_id = ug.id AND ugm2.user_id = u.id
		      JOIN device_trusted_profiles dtp ON dtp.id = ug.trusted_profile_id
		      WHERE ug.trusted_profile_id != ''
		        AND EXISTS (
		            SELECT 1 FROM sessions s
		            LEFT JOIN device_posture dp ON dp.device_id = s.device_id
		            WHERE s.user_id = u.id AND s.revoked = 0
		              AND (
		                (dtp.require_firewall = 1 AND (dp.device_id IS NULL OR dp.firewall_enabled = 0))
		                OR (dtp.require_disk_encryption = 1 AND (dp.device_id IS NULL OR dp.disk_encrypted = 0))
		                OR (dtp.require_screen_lock = 1 AND (dp.device_id IS NULL OR dp.screen_lock_enabled = 0))
		              )
		        )
		  )
		ORDER BY u.certificate_identity ASC`))
	postureStmt, _ := db.Prepare(state.Rebind(`
		SELECT dtp.require_firewall, dtp.require_disk_encryption, dtp.require_screen_lock, dtp.min_os_version
		FROM access_rules ar
		JOIN access_rule_groups arg ON arg.rule_id = ar.id
		JOIN user_groups ug ON ug.id = arg.group_id
		JOIN device_trusted_profiles dtp ON dtp.id = ug.trusted_profile_id
		WHERE ar.resource_id = ? AND ar.enabled = 1 AND ug.trusted_profile_id != ''
		LIMIT 1`))
	resources := []PolicyResource{}
	for rows.Next() {
		var id, resType, address string
		var protocol sql.NullString
		var portFrom sql.NullInt64
		var portTo sql.NullInt64
		var firewallStatus sql.NullString
		if err := rows.Scan(&id, &resType, &address, &protocol, &portFrom, &portTo, &firewallStatus); err != nil {
			return nil, err
		}
		identities := []string{}
		if identityStmt != nil {
			idRows, _ := identityStmt.Query(id)
			for idRows != nil && idRows.Next() {
				var identity sql.NullString
				if err := idRows.Scan(&identity); err == nil && identity.Valid && identity.String != "" {
					identities = append(identities, identity.String)
				}
			}
			if idRows != nil {
				idRows.Close()
			}
		}
		fwStatus := "unprotected"
		if firewallStatus.Valid && firewallStatus.String != "" {
			fwStatus = firewallStatus.String
		}
		res := PolicyResource{
			ResourceID:        id,
			Type:              normalizeResourceType(resType, address),
			Address:           address,
			Port:              0,
			Protocol:          "TCP",
			AllowedIdentities: identities,
			FirewallStatus:    fwStatus,
		}
		if postureStmt != nil {
			var fw, de, sl int
			var minOS string
			if err := postureStmt.QueryRow(id).Scan(&fw, &de, &sl, &minOS); err == nil {
				res.PostureRequirements = &PostureRequirements{
					RequireFirewall:       fw != 0,
					RequireDiskEncryption: de != 0,
					RequireScreenLock:     sl != 0,
					MinOSVersion:          minOS,
				}
			}
		}
		if protocol.Valid && protocol.String != "" {
			res.Protocol = protocol.String
		}
		if portFrom.Valid {
			v := int(portFrom.Int64)
			res.PortFrom = &v
			res.Port = v
		}
		if portTo.Valid {
			v := int(portTo.Int64)
			res.PortTo = &v
			if res.Port == 0 || res.Port != v {
				res.Port = 0
			}
		}
		resources = append(resources, res)
	}
	if identityStmt != nil {
		identityStmt.Close()
	}
	if postureStmt != nil {
		postureStmt.Close()
	}
	return resources, nil
}

func policyHash(resources []PolicyResource) string {
	payload := struct {
		Resources []PolicyResource `json:"resources"`
	}{Resources: resources}
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func policyVersion(db *sql.DB, connectorID, policyHash, compiledAt string) int {
	var version int
	var existingHash sql.NullString
	_ = db.QueryRow(state.Rebind(`SELECT version, policy_hash FROM connector_policy_versions WHERE connector_id = ?`), connectorID).Scan(&version, &existingHash)
	if version == 0 || !existingHash.Valid || existingHash.String != policyHash {
		version = version + 1
	}
	_, _ = db.Exec(state.Rebind(`INSERT INTO connector_policy_versions (connector_id, version, compiled_at, policy_hash)
    VALUES (?, ?, ?, ?)
    ON CONFLICT(connector_id) DO UPDATE SET version=excluded.version, compiled_at=excluded.compiled_at, policy_hash=excluded.policy_hash`), connectorID, version, compiledAt, policyHash)
	return version
}

func normalizeResourceType(resType, address string) string {
	switch strings.ToLower(strings.TrimSpace(resType)) {
	case "cidr", "dns", "internet":
		return strings.ToLower(strings.TrimSpace(resType))
	}
	if address == "" {
		return "dns"
	}
	addr := strings.ToLower(strings.TrimSpace(address))
	if addr == "*" || addr == "internet" {
		return "internet"
	}
	if strings.Contains(addr, "/") {
		if _, _, err := net.ParseCIDR(addr); err == nil {
			return "cidr"
		}
	}
	return "dns"
}
