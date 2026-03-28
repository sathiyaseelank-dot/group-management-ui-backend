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
	registry    *state.Registry
	agents      *state.AgentRegistry
	agentStatus *state.AgentStatusRegistry
	acls        *state.ACLStore
	db          *sql.DB
	snapshotTTL time.Duration
	scanStore   *state.ScanStore
	mu          sync.Mutex
	clients     map[string]*connectorClient
	seqMu       sync.Mutex
	agentSeqs   map[string]uint64
	batcher     *DiscoveryBatcher
}

// NewControlPlaneServer creates a new control plane server.
func NewControlPlaneServer(trustDomain string, registry *state.Registry, agents *state.AgentRegistry, agentStatus *state.AgentStatusRegistry, acls *state.ACLStore, db *sql.DB, snapshotTTL time.Duration, scanStore *state.ScanStore) *ControlPlaneServer {
	_ = trustDomain
	return &ControlPlaneServer{
		registry:    registry,
		agents:      agents,
		agentStatus: agentStatus,
		acls:        acls,
		db:          db,
		snapshotTTL: snapshotTTL,
		scanStore:   scanStore,
		clients:     make(map[string]*connectorClient),
		agentSeqs:   make(map[string]uint64),
		batcher:     NewDiscoveryBatcher(db),
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
	// Look up connector's workspace for RLS enforcement.
	var connectorWsID string
	if s.db != nil && connectorID != "" {
		_ = s.db.QueryRow(state.Rebind(`SELECT workspace_id FROM connectors WHERE id = ?`), connectorID).Scan(&connectorWsID)
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
		workspaceID: connectorWsID,
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
			deviceTunnelAddr := ""
			if len(msg.GetPayload()) > 0 {
				var payload struct {
					Agents []struct {
						AgentID string `json:"agent_id"`
						Status  string `json:"status"`
						IP      string `json:"ip"`
					} `json:"agents"`
					DeviceTunnelAddr string `json:"device_tunnel_addr"`
				}
				if err := json.Unmarshal(msg.GetPayload(), &payload); err == nil {
					deviceTunnelAddr = payload.DeviceTunnelAddr
					if s.agentStatus != nil {
						for _, t := range payload.Agents {
							s.agentStatus.Record(t.AgentID, "", msg.GetConnectorId(), t.IP)
							log.Printf("agent heartbeat: agent_id=%s connector_id=%s status=%s ip=%s", t.AgentID, msg.GetConnectorId(), t.Status, t.IP)
							if s.acls != nil && s.acls.DB() != nil {
								if rec, ok := s.agentStatus.Get(t.AgentID); ok {
									_ = state.SaveAgentToDB(s.acls.DB(), rec)
								}
							}
						}
					}
				} else if s.agentStatus != nil {
					var agents []struct {
						AgentID string `json:"agent_id"`
						Status  string `json:"status"`
						IP      string `json:"ip"`
					}
					if err := json.Unmarshal(msg.GetPayload(), &agents); err == nil {
						for _, t := range agents {
							s.agentStatus.Record(t.AgentID, "", msg.GetConnectorId(), t.IP)
							log.Printf("agent heartbeat: agent_id=%s connector_id=%s status=%s ip=%s", t.AgentID, msg.GetConnectorId(), t.Status, t.IP)
							if s.acls != nil && s.acls.DB() != nil {
								if rec, ok := s.agentStatus.Get(t.AgentID); ok {
									_ = state.SaveAgentToDB(s.acls.DB(), rec)
								}
							}
						}
					}
				}
			}
			if s.registry != nil {
				s.registry.RecordHeartbeat(msg.GetConnectorId(), msg.GetPrivateIp(), deviceTunnelAddr)
				if s.acls != nil && s.acls.DB() != nil {
					if rec, ok := s.registry.Get(msg.GetConnectorId()); ok {
						_ = state.SaveConnectorToDB(s.acls.DB(), rec)
					}
				}
			}
			log.Printf("heartbeat: connector_id=%s private_ip=%s tunnel_addr=%s status=%s", msg.GetConnectorId(), msg.GetPrivateIp(), deviceTunnelAddr, msg.GetStatus())
			// Refresh agent allowlist on every heartbeat for self-healing.
			connectorID := msg.GetConnectorId()
			if s.db != nil && connectorID != "" {
				_ = s.RefreshConnectorAllowlist(connectorID)
			}
		}
		if msg.GetType() == "agent_heartbeat" && s.agentStatus != nil {
			var payload struct {
				AgentID     string `json:"agent_id"`
				SPIFFEID    string `json:"spiffe_id"`
				Status      string `json:"status"`
				ConnectorID string `json:"connector_id"`
				IP          string `json:"ip"`
			}
			if err := json.Unmarshal(msg.GetPayload(), &payload); err == nil {
				s.agentStatus.Record(payload.AgentID, payload.SPIFFEID, payload.ConnectorID, payload.IP)
				if s.acls != nil && s.acls.DB() != nil {
					if rec, ok := s.agentStatus.Get(payload.AgentID); ok {
						_ = state.SaveAgentToDB(s.acls.DB(), rec)
					}
				}
			}
		}
		if msg.GetType() == "agent_posture" && s.db != nil {
			var payload struct {
				AgentID           string `json:"agent_id"`
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
			if err := json.Unmarshal(msg.GetPayload(), &payload); err == nil && payload.AgentID != "" {
				wsID := ""
				if s.registry != nil {
					if rec, ok := s.registry.Get(connectorID); ok {
						wsID = rec.WorkspaceID
					}
				}
				posture := state.DevicePosture{
					DeviceID: payload.AgentID, WorkspaceID: wsID, SPIFFEID: payload.SPIFFEID,
					OSType: payload.OSType, OSVersion: payload.OSVersion, Hostname: payload.Hostname,
					FirewallEnabled: payload.FirewallEnabled, DiskEncrypted: payload.DiskEncrypted,
					ScreenLockEnabled: payload.ScreenLockEnabled, ClientVersion: payload.ClientVersion,
					CollectedAt: payload.CollectedAt,
				}
				if err := state.UpsertDevicePosture(s.db, posture); err != nil {
					log.Printf("device posture upsert failed: %v", err)
				}
				log.Printf("device_posture: agent_id=%s os=%s/%s firewall=%v encrypted=%v",
					payload.AgentID, payload.OSType, payload.OSVersion, payload.FirewallEnabled, payload.DiskEncrypted)
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
					PolicyRuleID    string `json:"policy_rule_id"`
				}
				if err := json.Unmarshal(msg.GetPayload(), &payload); err == nil {
					auditWsID := ""
					if s.registry != nil {
						if rec, ok := s.registry.Get(msg.GetConnectorId()); ok {
							auditWsID = rec.WorkspaceID
						}
					}
					_, _ = s.acls.DB().Exec(
						state.Rebind(`INSERT INTO audit_logs (principal_spiffe, agent_id, resource_id, destination, protocol, port, decision, reason, connection_id, created_at, workspace_id, policy_rule_id, policy_decision, policy_reason)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
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
						payload.PolicyRuleID,
						payload.Decision,
						payload.Reason,
					)
				}
			}
		}
		if msg.GetType() == "agent_log" && s.db != nil {
			var payload struct {
				AgentID string `json:"agent_id"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(msg.GetPayload(), &payload); err == nil &&
				strings.TrimSpace(payload.AgentID) != "" &&
				strings.TrimSpace(payload.Message) != "" {
				nowISO := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
				_, _ = s.db.Exec(
					state.Rebind(`INSERT INTO agent_logs (agent_id, timestamp, message) VALUES (?, ?, ?)`),
					payload.AgentID,
					nowISO,
					payload.Message,
				)
			}
		}
		if msg.GetType() == "agent_discovery_diff" {
			log.Printf("agent_discovery_diff: received payload=%s", string(msg.GetPayload()))
			if s.db != nil {
				var diff struct {
					AgentID string `json:"agent_id"`
					Seq     uint64 `json:"seq"`
					Added   []struct {
						Protocol    string `json:"protocol"`
						Port        int    `json:"port"`
						BoundIP     string `json:"bound_ip"`
						ServiceName string `json:"service_name"`
						ProcessName string `json:"process_name"`
					} `json:"added"`
					Removed []struct {
						Protocol string `json:"protocol"`
						Port     int    `json:"port"`
					} `json:"removed"`
				}
				if err := json.Unmarshal(msg.GetPayload(), &diff); err != nil {
					log.Printf("agent_discovery_diff: parse error: %v", err)
				} else {
					// Sequence check
					s.seqMu.Lock()
					lastSeq := s.agentSeqs[diff.AgentID]
					if diff.Seq > 0 && lastSeq > 0 && diff.Seq != lastSeq+1 {
						log.Printf("agent_discovery_diff: seq gap for agent_id=%s expected=%d got=%d (next full_sync will correct)", diff.AgentID, lastSeq+1, diff.Seq)
					}
					s.agentSeqs[diff.AgentID] = diff.Seq
					s.seqMu.Unlock()

					var wsID string
					_ = s.db.QueryRow(state.Rebind(`SELECT workspace_id FROM agents WHERE id = ?`), diff.AgentID).Scan(&wsID)

					for _, svc := range diff.Added {
						s.batcher.QueueUpsert(state.AgentDiscoveredService{
							AgentID:     diff.AgentID,
							Port:        svc.Port,
							Protocol:    svc.Protocol,
							BoundIP:     svc.BoundIP,
							ServiceName: svc.ServiceName,
							ProcessName: svc.ProcessName,
							WorkspaceID: wsID,
						})
					}

					for _, svc := range diff.Removed {
						s.batcher.QueueGone(diff.AgentID, svc.Port, svc.Protocol)
					}

					log.Printf("agent_discovery_diff: agent_id=%s seq=%d added=%d removed=%d", diff.AgentID, diff.Seq, len(diff.Added), len(diff.Removed))
				}
			}
		}
		if msg.GetType() == "agent_discovery_full_sync" {
			log.Printf("agent_discovery_full_sync: received payload=%s", string(msg.GetPayload()))
			if s.db != nil {
				var sync struct {
					AgentID  string `json:"agent_id"`
					Seq      uint64 `json:"seq"`
					Services []struct {
						Protocol    string `json:"protocol"`
						Port        int    `json:"port"`
						BoundIP     string `json:"bound_ip"`
						ServiceName string `json:"service_name"`
						ProcessName string `json:"process_name"`
					} `json:"services"`
					Fingerprint uint64 `json:"fingerprint"`
				}
				if err := json.Unmarshal(msg.GetPayload(), &sync); err != nil {
					log.Printf("agent_discovery_full_sync: parse error: %v", err)
				} else {
					// Reset seq tracker
					s.seqMu.Lock()
					s.agentSeqs[sync.AgentID] = sync.Seq
					s.seqMu.Unlock()

					// Reactivate stale services first
					if reactivated, err := state.ReactivateStaleServices(s.db, sync.AgentID); err != nil {
						log.Printf("agent_discovery_full_sync: reactivate error: %v", err)
					} else if reactivated > 0 {
						log.Printf("agent_discovery_full_sync: reactivated %d stale services for agent_id=%s", reactivated, sync.AgentID)
					}

					var wsID string
					_ = s.db.QueryRow(state.Rebind(`SELECT workspace_id FROM agents WHERE id = ?`), sync.AgentID).Scan(&wsID)

					reported := make([]state.PortProto, 0, len(sync.Services))
					for _, svc := range sync.Services {
						if err := state.UpsertAgentDiscoveredService(s.db, state.AgentDiscoveredService{
							AgentID:     sync.AgentID,
							Port:        svc.Port,
							Protocol:    svc.Protocol,
							BoundIP:     svc.BoundIP,
							ServiceName: svc.ServiceName,
							ProcessName: svc.ProcessName,
							WorkspaceID: wsID,
						}); err != nil {
							log.Printf("agent_discovery_full_sync: upsert error port=%d: %v", svc.Port, err)
						}
						reported = append(reported, state.PortProto{Port: svc.Port, Protocol: svc.Protocol})
					}

					if gone, err := state.ReconcileDiscoveredServices(s.db, sync.AgentID, reported); err != nil {
						log.Printf("agent_discovery_full_sync: reconcile error: %v", err)
					} else if gone > 0 {
						log.Printf("agent_discovery_full_sync: reconciled %d stale services for agent_id=%s", gone, sync.AgentID)
					}

					log.Printf("agent_discovery_full_sync: agent_id=%s seq=%d services=%d fingerprint=%d", sync.AgentID, sync.Seq, len(sync.Services), sync.Fingerprint)
				}
			}
		}
		if msg.GetType() == "agent_discovery_report" {
			log.Printf("agent_discovery_report: received payload=%s", string(msg.GetPayload()))
			if s.db != nil {
				var report struct {
					AgentID  string `json:"agent_id"`
					Services []struct {
						Protocol string `json:"protocol"`
						Port     int    `json:"port"`
						BoundIP  string `json:"bound_ip"`
					} `json:"services"`
				}
				if err := json.Unmarshal(msg.GetPayload(), &report); err != nil {
					log.Printf("agent_discovery_report: failed to parse payload: %v", err)
				} else {
					// Look up agent's workspace_id once per report
					var wsID string
					_ = s.db.QueryRow(state.Rebind(
						`SELECT workspace_id FROM agents WHERE id = ?`),
						report.AgentID,
					).Scan(&wsID)

					reported := make([]state.PortProto, 0, len(report.Services))
					for _, svc := range report.Services {
						if err := state.UpsertAgentDiscoveredService(s.db, state.AgentDiscoveredService{
							AgentID:     report.AgentID,
							Port:        svc.Port,
							Protocol:    svc.Protocol,
							BoundIP:     svc.BoundIP,
							WorkspaceID: wsID,
						}); err != nil {
							log.Printf("agent_discovery_report: upsert error port=%d: %v", svc.Port, err)
						}
						reported = append(reported, state.PortProto{Port: svc.Port, Protocol: svc.Protocol})
					}
					// Reconcile: mark any previously-active services not in this report as gone.
					// Handles agent restarts where the agent's in-memory sent_services is empty.
					if gone, err := state.ReconcileDiscoveredServices(s.db, report.AgentID, reported); err != nil {
						log.Printf("agent_discovery_report: reconcile error: %v", err)
					} else if gone > 0 {
						log.Printf("agent_discovery_report: reconciled %d stale service(s) as gone for agent_id=%s", gone, report.AgentID)
					}
					log.Printf("agent_discovery_report: upserted agent_id=%s workspace=%s services=%d", report.AgentID, wsID, len(report.Services))
				}
			} else {
				log.Printf("agent_discovery_report: db is nil, cannot persist")
			}
		}
		if msg.GetType() == "agent_discovery_gone" {
			log.Printf("agent_discovery_gone: received payload=%s", string(msg.GetPayload()))
			if s.db != nil {
				var report struct {
					AgentID  string `json:"agent_id"`
					Services []struct {
						Protocol string `json:"protocol"`
						Port     int    `json:"port"`
					} `json:"services"`
				}
				if err := json.Unmarshal(msg.GetPayload(), &report); err != nil {
					log.Printf("agent_discovery_gone: failed to parse payload: %v", err)
				} else {
					ports := make([]int, len(report.Services))
					proto := "tcp"
					for i, svc := range report.Services {
						ports[i] = svc.Port
						if svc.Protocol != "" {
							proto = svc.Protocol
						}
					}
					if err := state.MarkServicesGone(s.db, report.AgentID, ports, proto); err != nil {
						log.Printf("agent_discovery_gone: mark gone error: %v", err)
					} else {
						log.Printf("agent_discovery_gone: marked %d services gone for agent_id=%s", len(ports), report.AgentID)
					}
				}
			}
		}
		if msg.GetType() == "agent_discovery_heartbeat" {
			if s.db != nil {
				var payload struct {
					AgentID string `json:"agent_id"`
				}
				if err := json.Unmarshal(msg.GetPayload(), &payload); err == nil {
					if err := state.TouchAgentDiscoveryLastSeen(s.db, payload.AgentID); err != nil {
						log.Printf("agent_discovery_heartbeat: touch last_seen error: %v", err)
					} else {
						log.Printf("agent_discovery_heartbeat: bumped last_seen for agent_id=%s", payload.AgentID)
					}
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

// NotifyAgentAllowed persists a newly enrolled agent and refreshes allowlists
// for connectors in the same remote network.
func (s *ControlPlaneServer) NotifyAgentAllowed(agentID, spiffeID, version, hostname, ip string) {
	if s.agents != nil {
		s.agents.Add(agentID, spiffeID)
	}
	var remoteNetworkID string
	if s.db != nil {
		workspaceID := ""
		trimmed := strings.TrimPrefix(spiffeID, "spiffe://")
		parts := strings.Split(trimmed, "/")
		if len(parts) >= 3 {
			trustDomain := strings.TrimSpace(parts[0])
			if trustDomain != "" {
				_ = s.db.QueryRow(
					state.Rebind(`SELECT id FROM workspaces WHERE trust_domain = ? LIMIT 1`),
					trustDomain,
				).Scan(&workspaceID)
			}
		}
		if _, err := s.db.Exec(
			state.Rebind(`INSERT INTO agents (id, spiffe_id, connector_id, version, hostname, last_seen, ip, workspace_id)
			VALUES (?, ?, '', ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				spiffe_id=excluded.spiffe_id,
				version=excluded.version,
				hostname=excluded.hostname,
				last_seen=excluded.last_seen,
				ip=excluded.ip,
				workspace_id=CASE WHEN excluded.workspace_id = '' THEN agents.workspace_id ELSE excluded.workspace_id END`),
			agentID, spiffeID, version, hostname, time.Now().UTC().Unix(), ip, workspaceID,
		); err != nil {
			log.Printf("failed to persist enrolled agent %s: %v", agentID, err)
			return
		}
		if err := s.db.QueryRow(
			state.Rebind(`SELECT COALESCE(TRIM(remote_network_id), '') FROM agents WHERE id = ?`),
			agentID,
		).Scan(&remoteNetworkID); err != nil {
			log.Printf("failed to resolve remote_network_id for agent %s: %v", agentID, err)
			return
		}
	}
	if strings.TrimSpace(remoteNetworkID) == "" {
		log.Printf("agent %s enrolled without remote_network_id; agent will not be allowlisted until assigned to a remote network", agentID)
		return
	}
	s.RefreshAllowlistsForRemoteNetwork(remoteNetworkID)
}

type connectorClient struct {
	stream      controllerpb.ControlPlane_ConnectServer
	sendMu      sync.Mutex
	connectorID string
	signingKey  []byte
	workspaceID string // workspace this connector belongs to
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
	if c == nil || c.connectorID == "" {
		return
	}
	list, networkID, err := s.allowlistForConnector(c.connectorID)
	if err != nil {
		log.Printf("failed to build agent allowlist for connector %s: %v", c.connectorID, err)
		return
	}
	payload, err := json.Marshal(list)
	if err != nil {
		return
	}
	log.Printf(
		"agent allowlist pushed: connector_id=%s network_id=%s entries=%d",
		c.connectorID,
		networkID,
		len(list),
	)
	_ = c.send(&controllerpb.ControlMessage{
		Type:    "agent_allowlist",
		Payload: payload,
	})
}

func (s *ControlPlaneServer) allowlistForConnector(connectorID string) ([]state.AgentInfo, string, error) {
	if strings.TrimSpace(connectorID) == "" {
		return nil, "", fmt.Errorf("connector_id required")
	}
	if s.db == nil {
		if s.agents == nil {
			return []state.AgentInfo{}, "", nil
		}
		return s.agents.List(), "", nil
	}

	networkID, err := lookupConnectorNetwork(s.db, connectorID)
	if err != nil {
		if strings.Contains(err.Error(), "has no network") {
			return []state.AgentInfo{}, "", nil
		}
		return nil, "", err
	}

	rows, err := s.db.Query(
		state.Rebind(`SELECT id, spiffe_id
			FROM agents
			WHERE remote_network_id = ?
			  AND revoked = 0
			  AND COALESCE(TRIM(spiffe_id), '') <> ''
			ORDER BY id ASC`),
		networkID,
	)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	list := make([]state.AgentInfo, 0)
	for rows.Next() {
		var item state.AgentInfo
		if err := rows.Scan(&item.ID, &item.SPIFFEID); err != nil {
			return nil, "", err
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return list, networkID, nil
}

func (s *ControlPlaneServer) RefreshConnectorAllowlist(connectorID string) error {
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
	s.sendAllowlist(target)
	return nil
}

func (s *ControlPlaneServer) RefreshAllowlistsForRemoteNetwork(networkID string) {
	if s.db == nil || strings.TrimSpace(networkID) == "" {
		return
	}

	rows, err := s.db.Query(
		state.Rebind(`SELECT connector_id
			FROM (
				SELECT id AS connector_id FROM connectors WHERE remote_network_id = ?
				UNION
				SELECT connector_id FROM remote_network_connectors WHERE network_id = ?
			) connector_scope
			ORDER BY connector_id ASC`),
		networkID,
		networkID,
	)
	if err != nil {
		log.Printf("failed to load connectors for allowlist refresh in network %s: %v", networkID, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var connectorID string
		if err := rows.Scan(&connectorID); err != nil {
			log.Printf("failed to scan connector for allowlist refresh in network %s: %v", networkID, err)
			return
		}
		if err := s.RefreshConnectorAllowlist(connectorID); err != nil && !strings.Contains(err.Error(), "not connected") {
			log.Printf("failed to refresh allowlist for connector %s: %v", connectorID, err)
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("failed during allowlist refresh iteration in network %s: %v", networkID, err)
	}
}

// ACL notifications
func (s *ControlPlaneServer) StartBatcher() {
	if s.batcher != nil {
		go s.batcher.Run()
	}
}

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
	// Set RLS session variable for workspace isolation on policy queries.
	if c.workspaceID != "" {
		_, _ = s.db.Exec("SET LOCAL app.workspace_id = $1", c.workspaceID)
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
	nowISO := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	uiMsg := fmt.Sprintf(
		"policy snapshot pushed: version=%d previous_version=%d resources=%d reason=%s trigger=%s changed_fields=%s network_id=%s protected_hash=%s",
		snap.SnapshotMeta.PolicyVersion,
		previousVersion,
		len(snap.Resources),
		reason,
		trigger,
		changedFields,
		networkID,
		newHash,
	)
	_, _ = s.db.Exec(
		state.Rebind(`INSERT INTO connector_logs (connector_id, timestamp, message) VALUES (?, ?, ?)`),
		c.connectorID,
		nowISO,
		uiMsg,
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

func (s *ControlPlaneServer) lookupConnectorWorkspace(connectorID string) string {
	if s.db == nil || connectorID == "" {
		return ""
	}
	var wsID string
	_ = s.db.QueryRow(state.Rebind(`SELECT workspace_id FROM connectors WHERE id = ?`), connectorID).Scan(&wsID)
	return wsID
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
