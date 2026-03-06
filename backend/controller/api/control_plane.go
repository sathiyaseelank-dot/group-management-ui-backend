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
	tunnelers      *state.TunnelerRegistry
	tunnelerStatus *state.TunnelerStatusRegistry
	acls           *state.ACLStore
	db             *sql.DB
	signingKey     []byte
	snapshotTTL    time.Duration
	mu             sync.Mutex
	clients        map[string]*connectorClient
}

// NewControlPlaneServer creates a new control plane server.
func NewControlPlaneServer(trustDomain string, registry *state.Registry, tunnelers *state.TunnelerRegistry, tunnelerStatus *state.TunnelerStatusRegistry, acls *state.ACLStore, db *sql.DB, signingKey []byte, snapshotTTL time.Duration) *ControlPlaneServer {
	_ = trustDomain
	return &ControlPlaneServer{
		registry:       registry,
		tunnelers:      tunnelers,
		tunnelerStatus: tunnelerStatus,
		acls:           acls,
		db:             db,
		signingKey:     signingKey,
		snapshotTTL:    snapshotTTL,
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
	client := &connectorClient{
		stream:      stream,
		connectorID: connectorID,
		signingKey:  derivePolicyKey(stream.Context(), connectorID),
	}
	s.logConnectorEvent(connectorID, "control-plane stream connected")
	if len(client.signingKey) == 0 {
		log.Printf("policy key derivation failed for connector %s", connectorID)
	}
	s.addClient(spiffeID, client)
	defer s.removeClient(spiffeID)
	s.sendAllowlist(client)
	s.sendPolicySnapshot(client)

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if msg.GetType() == "ping" {
			if err := stream.Send(&controllerpb.ControlMessage{Type: "pong"}); err != nil {
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
		}
		if msg.GetType() == "tunneler_heartbeat" && s.tunnelerStatus != nil {
			var payload struct {
				TunnelerID  string `json:"tunneler_id"`
				SPIFFEID    string `json:"spiffe_id"`
				Status      string `json:"status"`
				ConnectorID string `json:"connector_id"`
			}
			if err := json.Unmarshal(msg.GetPayload(), &payload); err == nil {
				s.tunnelerStatus.Record(payload.TunnelerID, payload.SPIFFEID, payload.ConnectorID)
				if s.acls != nil && s.acls.DB() != nil {
					if rec, ok := s.tunnelerStatus.Get(payload.TunnelerID); ok {
						_ = state.SaveTunnelerToDB(s.acls.DB(), rec)
					}
				}
			}
		}
		if msg.GetType() == "acl_decision" {
			log.Printf("acl decision: %s", string(msg.GetPayload()))
			if s.acls != nil && s.acls.DB() != nil {
				var payload struct {
					PrincipalSPIFFE string `json:"spiffe_id"`
					TunnelerID      string `json:"tunneler_id"`
					ResourceID      string `json:"resource_id"`
					Destination     string `json:"destination"`
					Protocol        string `json:"protocol"`
					Port            uint16 `json:"port"`
					Decision        string `json:"decision"`
					Reason          string `json:"reason"`
					ConnectionID    string `json:"connection_id"`
				}
				if err := json.Unmarshal(msg.GetPayload(), &payload); err == nil {
					_, _ = s.acls.DB().Exec(
						`INSERT INTO audit_logs (principal_spiffe, tunneler_id, resource_id, destination, protocol, port, decision, reason, connection_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
						payload.PrincipalSPIFFE,
						payload.TunnelerID,
						payload.ResourceID,
						payload.Destination,
						payload.Protocol,
						payload.Port,
						payload.Decision,
						payload.Reason,
						payload.ConnectionID,
						time.Now().UTC().Unix(),
					)
				}
			}
		}
	}
}

// NotifyTunnelerAllowed broadcasts a newly enrolled tunneler to all connectors.
func (s *ControlPlaneServer) NotifyTunnelerAllowed(tunnelerID, spiffeID string) {
	if s.tunnelers != nil {
		s.tunnelers.Add(tunnelerID, spiffeID)
	}
	info := state.TunnelerInfo{ID: tunnelerID, SPIFFEID: spiffeID}
	payload, err := json.Marshal(info)
	if err != nil {
		return
	}
	s.broadcast(&controllerpb.ControlMessage{
		Type:    "tunneler_allow",
		Payload: payload,
	})
}

type connectorClient struct {
	stream      controllerpb.ControlPlane_ConnectServer
	sendMu      sync.Mutex
	connectorID string
	signingKey  []byte
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
		c.sendMu.Lock()
		_ = c.stream.Send(msg)
		c.sendMu.Unlock()
	}
}

func (s *ControlPlaneServer) sendAllowlist(c *connectorClient) {
	if s.tunnelers == nil {
		return
	}
	list := s.tunnelers.List()
	payload, err := json.Marshal(list)
	if err != nil {
		return
	}
	c.sendMu.Lock()
	_ = c.stream.Send(&controllerpb.ControlMessage{
		Type:    "tunneler_allowlist",
		Payload: payload,
	})
	c.sendMu.Unlock()
}

// ACL notifications
func (s *ControlPlaneServer) NotifyACLInit() {
	s.broadcastPolicySnapshots()
}

func (s *ControlPlaneServer) NotifyResourceUpsert(res state.Resource) {
	s.broadcastPolicySnapshots()
}

func (s *ControlPlaneServer) NotifyResourceRemoved(resourceID string) {
	s.broadcastPolicySnapshots()
}

func (s *ControlPlaneServer) NotifyAuthorizationUpsert(auth state.Authorization) {
	s.broadcastPolicySnapshots()
}

func (s *ControlPlaneServer) NotifyAuthorizationRemoved(resourceID, principalSPIFFE string) {
	s.broadcastPolicySnapshots()
}

func (s *ControlPlaneServer) NotifyPolicyChange() {
	s.broadcastPolicySnapshots()
}

func (s *ControlPlaneServer) broadcastPolicySnapshots() {
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
		s.sendPolicySnapshot(c)
	}
}

func (s *ControlPlaneServer) sendPolicySnapshot(c *connectorClient) {
	if s.db == nil || c == nil || c.connectorID == "" {
		return
	}
	if len(c.signingKey) == 0 {
		log.Printf("skipping policy snapshot for %s: no derived policy key", c.connectorID)
		return
	}
	snap, err := CompilePolicySnapshot(s.db, c.connectorID, s.snapshotTTL, c.signingKey)
	if err != nil {
		log.Printf("failed to compile snapshot for %s: %v", c.connectorID, err)
		s.logConnectorEvent(c.connectorID, fmt.Sprintf("policy snapshot failed: %v", err))
		return
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		return
	}
	c.sendMu.Lock()
	_ = c.stream.Send(&controllerpb.ControlMessage{
		Type:    "policy_snapshot",
		Payload: payload,
	})
	c.sendMu.Unlock()
	s.logConnectorEvent(c.connectorID, fmt.Sprintf("policy snapshot pushed: version=%d resources=%d", snap.SnapshotMeta.PolicyVersion, len(snap.Resources)))
}

// IsStreamActive returns true if a connector with the given ID currently has
// an active gRPC control-plane stream. The id can be either the raw connector
// ID or its full SPIFFE ID (both forms are checked).
func (s *ControlPlaneServer) IsStreamActive(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Check by SPIFFE ID key first, then by connector ID embedded in key.
	for key, c := range s.clients {
		if key == id || c.connectorID == id {
			return true
		}
	}
	return false
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

func (s *ControlPlaneServer) logConnectorEvent(connectorID, msg string) {
	if s == nil || s.db == nil || connectorID == "" || msg == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(`INSERT INTO connector_logs (connector_id, timestamp, message) VALUES (?, ?, ?)`, connectorID, now, msg)
}
