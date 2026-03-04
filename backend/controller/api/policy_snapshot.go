package api

import (
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

type PolicyResource struct {
	ResourceID        string   `json:"resource_id"`
	Type              string   `json:"type"`
	Address           string   `json:"address"`
	Port              int      `json:"port"`
	Protocol          string   `json:"protocol"`
	PortFrom          *int     `json:"port_from,omitempty"`
	PortTo            *int     `json:"port_to,omitempty"`
	AllowedIdentities []string `json:"allowed_identities"`
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
	if err := db.QueryRow(`SELECT remote_network_id FROM connectors WHERE id = ?`, connectorID).Scan(&networkID); err != nil {
		return "", err
	}
	if !networkID.Valid || strings.TrimSpace(networkID.String) == "" {
		return "", fmt.Errorf("connector %s has no network", connectorID)
	}
	return networkID.String, nil
}

func policyResources(db *sql.DB, remoteNetworkID string) ([]PolicyResource, error) {
	rows, err := db.Query(`SELECT id, type, address, protocol, port_from, port_to FROM resources WHERE remote_network_id = ? ORDER BY id ASC`, remoteNetworkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	identityStmt, _ := db.Prepare(`SELECT DISTINCT u.certificate_identity as identity
    FROM access_rules ar
    JOIN access_rule_groups arg ON arg.rule_id = ar.id
    JOIN user_group_members gm ON gm.group_id = arg.group_id
    JOIN users u ON u.id = gm.user_id
    WHERE ar.resource_id = ? AND ar.enabled = 1 AND u.certificate_identity IS NOT NULL
    ORDER BY u.certificate_identity ASC`)
	resources := []PolicyResource{}
	for rows.Next() {
		var id, resType, address string
		var protocol sql.NullString
		var portFrom sql.NullInt64
		var portTo sql.NullInt64
		if err := rows.Scan(&id, &resType, &address, &protocol, &portFrom, &portTo); err != nil {
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
		res := PolicyResource{
			ResourceID:        id,
			Type:              normalizeResourceType(resType, address),
			Address:           address,
			Port:              0,
			Protocol:          "TCP",
			AllowedIdentities: identities,
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
	_ = db.QueryRow(`SELECT version, policy_hash FROM connector_policy_versions WHERE connector_id = ?`, connectorID).Scan(&version, &existingHash)
	if version == 0 || !existingHash.Valid || existingHash.String != policyHash {
		version = version + 1
	}
	_, _ = db.Exec(`INSERT INTO connector_policy_versions (connector_id, version, compiled_at, policy_hash)
    VALUES (?, ?, ?, ?)
    ON CONFLICT(connector_id) DO UPDATE SET version=excluded.version, compiled_at=excluded.compiled_at, policy_hash=excluded.policy_hash`, connectorID, version, compiledAt, policyHash)
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
