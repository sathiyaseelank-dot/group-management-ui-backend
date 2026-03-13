package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	controllerpb "controller/gen/controllerpb"
	"controller/state"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// ControlPlaneServer implements the controller.v1.ControlPlane service.
type ControlPlaneServer struct {
	controllerpb.UnimplementedControlPlaneServer
	registry       *state.Registry
	agents      *state.AgentRegistry
	agentStatus *state.AgentStatusRegistry
	acls           *state.ACLStore
	db             *sql.DB
	signingKey     []byte
	snapshotTTL    time.Duration
	scanStore      *state.ScanStore
	mu             sync.Mutex
	clients        map[string]*connectorClient
}

// NewControlPlaneServer creates a new control plane server.
func NewControlPlaneServer(trustDomain string, registry *state.Registry, agents *state.AgentRegistry, agentStatus *state.AgentStatusRegistry, acls *state.ACLStore, db *sql.DB, signingKey []byte, snapshotTTL time.Duration, scanStore *state.ScanStore) *ControlPlaneServer {
	_ = trustDomain
	return &ControlPlaneServer{
		registry:       registry,
		agents:      agents,
		agentStatus: agentStatus,
		acls:           acls,
		db:             db,
		signingKey:     signingKey,
		snapshotTTL:    snapshotTTL,
		scanStore:      scanStore,
		clients:        make(map[string]*connectorClient),
	}
}

// Connect handles a persistent control-plane stream from connectors.
func (s *ControlPlaneServer) Connect(stream controllerpb.ControlPlane_ConnectServer) error {
	role, ok := RoleFromContext(stream.Context())
	if !ok || role != "connector" {
		return status.Error(codes.PermissionDenied, "connector role required")
	}

	spiffeID, _ := SPIFFEIDFromContext(stream.Context())
	log.Printf("control-plane stream connected: %s", spiffeID)
	connectorID := parseConnectorID(spiffeID)
	if s.db != nil && connectorID != "" {
		var revoked int
		if err := s.db.QueryRow(`SELECT revoked FROM connectors WHERE id = ?`, connectorID).Scan(&revoked); err == nil {
			if revoked != 0 {
				return status.Error(codes.PermissionDenied, "connector revoked")
			}
		}
	}
	signingKey := derivePolicyKey(stream.Context(), connectorID)
	if len(signingKey) == 0 {
		log.Printf("policy key derivation failed for connector %s: no mTLS client cert, policy snapshot will not be sent", connectorID)
	} else {
		log.Printf("mTLS verified for connector %s: policy signing key derived, policy snapshot will be sent", connectorID)
	}
	client := &connectorClient{
		stream:      stream,
		connectorID: connectorID,
		signingKey:  signingKey,
	}
	s.addClient(spiffeID, client)
	defer s.removeClient(spiffeID)
	s.sendAllowlist(client)
	s.sendPolicySnapshot(client, "initial_connect", "control-plane connected", "full_snapshot")

	// Log connection event and mark connector online.
	connectTime := time.Now().UTC()
	connectISO := connectTime.Format("2006-01-02T15:04:05.000Z")
	if s.db != nil && connectorID != "" {
		connMsg := "control-plane connected · no client cert · policy snapshot skipped"
		if len(signingKey) > 0 {
			connMsg = "control-plane connected · mTLS verified · policy snapshot sent"
		}
		_, _ = s.db.Exec(
			state.Rebind(`INSERT INTO connector_logs (connector_id, timestamp, message) VALUES (?, ?, ?)`),
			connectorID, connectISO, connMsg,
		)
		_, _ = s.db.Exec(
			state.Rebind(`UPDATE connectors SET status = 'online', installed = 1, last_seen = ?, last_seen_at = ? WHERE id = ?`),
			connectTime.Unix(), connectISO, connectorID,
		)
	}
	defer func() {
		if s.db != nil && connectorID != "" {
			offISO := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			_, _ = s.db.Exec(
				state.Rebind(`INSERT INTO connector_logs (connector_id, timestamp, message) VALUES (?, ?, ?)`),
				connectorID, offISO, "control-plane disconnected",
			)
			_, _ = s.db.Exec(
				state.Rebind(`UPDATE connectors SET status = 'offline' WHERE id = ?`),
				connectorID,
			)
		}
	}()

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if msg.GetType() == "ping" {
			if err := client.send(&controllerpb.ControlMessage{Type: "pong"}); err != nil {
				return err
			}
		}
		if msg.GetType() == "heartbeat" {
			if s.registry != nil {
				s.registry.RecordHeartbeat(msg.GetConnectorId(), msg.GetPrivateIp())
				if s.acls != nil && s.acls.DB() != nil {
					if rec, ok := s.registry.Get(msg.GetConnectorId()); ok {
						_ = state.SaveConnectorToDB(s.acls.DB(), rec)
					}
				}
			}
			log.Printf("heartbeat: connector_id=%s private_ip=%s status=%s", msg.GetConnectorId(), msg.GetPrivateIp(), msg.GetStatus())

			// Connector embeds agent statuses in the heartbeat payload.
			if s.agentStatus != nil && len(msg.GetPayload()) > 0 {
				var agents []struct {
					AgentID string `json:"agent_id"`
					Status  string `json:"status"`
				}
				if err := json.Unmarshal(msg.GetPayload(), &agents); err == nil {
					for _, t := range agents {
						s.agentStatus.Record(t.AgentID, "", msg.GetConnectorId())
						log.Printf("agent heartbeat: agent_id=%s connector_id=%s status=%s", t.AgentID, msg.GetConnectorId(), t.Status)
						if s.acls != nil && s.acls.DB() != nil {
							if rec, ok := s.agentStatus.Get(t.AgentID); ok {
								_ = state.SaveAgentToDB(s.acls.DB(), rec)
							}
						}
					}
				}
			}
		}
		if msg.GetType() == "agent_heartbeat" && s.agentStatus != nil {
			var payload struct {
				AgentID     string `json:"agent_id"`
				SPIFFEID    string `json:"spiffe_id"`
				Status      string `json:"status"`
				ConnectorID string `json:"connector_id"`
			}
			if err := json.Unmarshal(msg.GetPayload(), &payload); err == nil {
				s.agentStatus.Record(payload.AgentID, payload.SPIFFEID, payload.ConnectorID)
				if s.acls != nil && s.acls.DB() != nil {
					if rec, ok := s.agentStatus.Get(payload.AgentID); ok {
						_ = state.SaveAgentToDB(s.acls.DB(), rec)
					}
				}
			}
		}
		if msg.GetType() == "acl_decision" {
			log.Printf("acl decision: %s", string(msg.GetPayload()))
			if s.acls != nil && s.acls.DB() != nil {
				var payload struct {
					PrincipalSPIFFE string `json:"spiffe_id"`
					AgentID         string `json:"agent_id"`
					ResourceID      string `json:"resource_id"`
					Destination     string `json:"destination"`
					Protocol        string `json:"protocol"`
					Port            uint16 `json:"port"`
					Decision        string `json:"decision"`
					Reason          string `json:"reason"`
					ConnectionID    string `json:"connection_id"`
				}
				if err := json.Unmarshal(msg.GetPayload(), &payload); err == nil {
					auditWsID := ""
					if s.registry != nil {
						if rec, ok := s.registry.Get(msg.GetConnectorId()); ok {
							auditWsID = rec.WorkspaceID
						}
					}
					_, _ = s.acls.DB().Exec(
						state.Rebind(`INSERT INTO audit_logs (principal_spiffe, tunneler_id, resource_id, destination, protocol, port, decision, reason, connection_id, created_at, workspace_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
						payload.PrincipalSPIFFE,
						payload.AgentID,
						payload.ResourceID,
						payload.Destination,
						payload.Protocol,
						payload.Port,
						payload.Decision,
						payload.Reason,
						payload.ConnectionID,
						time.Now().UTC().Unix(),
						auditWsID,
					)
				}
			}
		}
		if msg.GetType() == "scan_report" && s.scanStore != nil {
			var report struct {
				RequestID string                     `json:"request_id"`
				Results   []state.DiscoveredResource `json:"results"`
				Error     *string                    `json:"error"`
			}
			if err := json.Unmarshal(msg.GetPayload(), &report); err == nil {
				if report.Error != nil && *report.Error != "" {
					s.scanStore.Fail(report.RequestID, *report.Error)
				} else {
					s.scanStore.Complete(report.RequestID, report.Results)
				}
				log.Printf("scan_report: request_id=%s results=%d", report.RequestID, len(report.Results))
			}
		}
	}
}

// SendToConnector sends a message to a specific connected connector by its connector ID.
func (s *ControlPlaneServer) SendToConnector(connectorID string, msgType string, payload []byte) error {
	s.mu.Lock()
	var target *connectorClient
	for _, c := range s.clients {
		if c.connectorID == connectorID {
			target = c
			break
		}
	}
	s.mu.Unlock()

	if target == nil {
		return fmt.Errorf("connector %s not connected", connectorID)
	}

	return target.send(&controllerpb.ControlMessage{
		Type:    msgType,
		Payload: payload,
	})
}

// NotifyAgentAllowed broadcasts a newly enrolled agent to all connectors
// and persists to DB so the allowlist survives controller restarts.
func (s *ControlPlaneServer) NotifyAgentAllowed(agentID, spiffeID, version, hostname string) {
	if s.agents != nil {
		s.agents.Add(agentID, spiffeID)
	}
	// Persist to DB so LoadAgentRegistryFromDB restores the allowlist on restart.
	if s.db != nil {
		_, _ = s.db.Exec(
			state.Rebind(`INSERT INTO tunnelers (id, spiffe_id, connector_id, version, hostname, last_seen)
			VALUES (?, ?, '', ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET spiffe_id=excluded.spiffe_id, version=excluded.version, hostname=excluded.hostname, last_seen=excluded.last_seen`),
			agentID, spiffeID, version, hostname, time.Now().UTC().Unix(),
		)
	}
	info := state.AgentInfo{ID: agentID, SPIFFEID: spiffeID}
	payload, err := json.Marshal(info)
	if err != nil {
		return
	}
	s.broadcast(&controllerpb.ControlMessage{
		Type:    "agent_allow",
		Payload: payload,
	})
}

type connectorClient struct {
	stream      controllerpb.ControlPlane_ConnectServer
	sendMu      sync.Mutex
	connectorID string
	signingKey  []byte
}

func (c *connectorClient) send(msg *controllerpb.ControlMessage) error {
	if c == nil || msg == nil {
		return nil
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.stream.Send(msg)
}

// IsStreamActive returns true if a connector with the given ID currently has
// an active gRPC control-plane stream. Both the raw connector ID and its SPIFFE
// ID key are checked.
func (s *ControlPlaneServer) IsStreamActive(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, c := range s.clients {
		if key == id || c.connectorID == id {
			return true
		}
	}
	return false
}

func (s *ControlPlaneServer) addClient(id string, c *connectorClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[id] = c
}

func (s *ControlPlaneServer) removeClient(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, id)
}

func (s *ControlPlaneServer) broadcast(msg *controllerpb.ControlMessage) {
	s.mu.Lock()
	clients := make([]*connectorClient, 0, len(s.clients))
	for _, c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.Unlock()

	for _, c := range clients {
		_ = c.send(msg)
	}
}

func (s *ControlPlaneServer) sendAllowlist(c *connectorClient) {
	if s.agents == nil {
		return
	}
	list := s.agents.List()
	payload, err := json.Marshal(list)
	if err != nil {
		return
	}
	_ = c.send(&controllerpb.ControlMessage{
		Type:    "agent_allowlist",
		Payload: payload,
	})
}

// ACL notifications
func (s *ControlPlaneServer) NotifyACLInit() {
	s.broadcastPolicySnapshots("acl_init", "acl store initialized", "full_snapshot")
}

func (s *ControlPlaneServer) NotifyResourceUpsert(res state.Resource) {
	s.broadcastPolicySnapshots("resource_upsert", fmt.Sprintf("resource updated: resource_id=%s", res.ID), "resources")
}

func (s *ControlPlaneServer) NotifyResourceRemoved(resourceID string) {
	s.broadcastPolicySnapshots("resource_removed", fmt.Sprintf("resource removed: resource_id=%s", resourceID), "resources")
}

func (s *ControlPlaneServer) NotifyAuthorizationUpsert(auth state.Authorization) {
	s.broadcastPolicySnapshots("authorization_upsert", fmt.Sprintf("authorization updated: resource_id=%s principal=%s", auth.ResourceID, auth.PrincipalSPIFFE), "allowed_identities")
}

func (s *ControlPlaneServer) NotifyAuthorizationRemoved(resourceID, principalSPIFFE string) {
	s.broadcastPolicySnapshots("authorization_removed", fmt.Sprintf("authorization removed: resource_id=%s principal=%s", resourceID, principalSPIFFE), "allowed_identities")
}

func (s *ControlPlaneServer) NotifyPolicyChange() {
	s.broadcastPolicySnapshots("policy_change", "policy change notification received", "policy")
}

func (s *ControlPlaneServer) broadcastPolicySnapshots(trigger, reason, changedFields string) {
	if s.db == nil {
		return
	}
	s.mu.Lock()
	clients := make([]*connectorClient, 0, len(s.clients))
	for _, c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.Unlock()

	for _, c := range clients {
		s.sendPolicySnapshot(c, trigger, reason, changedFields)
	}
}

func (s *ControlPlaneServer) sendPolicySnapshot(c *connectorClient, trigger, reason, changedFields string) {
	if s.db == nil || c == nil || c.connectorID == "" {
		return
	}
	if len(c.signingKey) == 0 {
		log.Printf("skipping policy snapshot for %s: no policy signing key", c.connectorID)
		return
	}
	networkID, err := lookupConnectorNetwork(s.db, c.connectorID)
	if err != nil {
		log.Printf("failed to resolve connector network for %s: %v", c.connectorID, err)
		return
	}
	var previousVersion int
	var previousHash sql.NullString
	_ = s.db.QueryRow(
		state.Rebind(`SELECT version, policy_hash FROM connector_policy_versions WHERE connector_id = ?`),
		c.connectorID,
	).Scan(&previousVersion, &previousHash)
	snap, err := CompilePolicySnapshot(s.db, c.connectorID, s.snapshotTTL, c.signingKey)
	if err != nil {
		log.Printf("failed to compile snapshot for %s: %v", c.connectorID, err)
		return
	}
	newHash := PolicyHashForUI(snap.Resources)
	if strings.TrimSpace(reason) == "" {
		if !previousHash.Valid || previousHash.String != newHash {
			reason = "policy payload changed"
		} else {
			reason = "policy broadcast requested with unchanged payload"
		}
	}
	if strings.TrimSpace(changedFields) == "" {
		changedFields = "unknown"
	}
	if strings.TrimSpace(trigger) == "" {
		trigger = "unspecified"
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		return
	}
	err = c.send(&controllerpb.ControlMessage{
		Type:    "policy_snapshot",
		Payload: payload,
	})
	if err != nil {
		log.Printf("failed to send policy snapshot to connector %s: %v", c.connectorID, err)
		return
	}
	prevHash := ""
	if previousHash.Valid {
		prevHash = previousHash.String
	}
	log.Printf(
		"policy snapshot pushed: connector_id=%s version=%d previous_version=%d resources=%d reason=%q trigger=%q changed_fields=%q network_id=%s previous_hash=%s new_hash=%s",
		c.connectorID,
		snap.SnapshotMeta.PolicyVersion,
		previousVersion,
		len(snap.Resources),
		reason,
		trigger,
		changedFields,
		networkID,
		prevHash,
		newHash,
	)
}

func parseConnectorID(spiffeID string) string {
	if spiffeID == "" {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(spiffeID, "spiffe://"), "/")
	if len(parts) < 3 {
		return ""
	}
	if parts[1] != "connector" {
		return ""
	}
	return parts[2]
}

const policyKeyLabel = "ztna-policy-signing-v1"

func derivePolicyKey(ctx context.Context, connectorID string) []byte {
	if connectorID == "" {
		return nil
	}
	p, ok := peer.FromContext(ctx)
	if !ok || p.AuthInfo == nil {
		return nil
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil
	}
	key, err := tlsInfo.State.ExportKeyingMaterial(policyKeyLabel, []byte(connectorID), 32)
	if err != nil {
		return nil
	}
	return key
}
