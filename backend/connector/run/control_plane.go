package run

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"connector/internal/spiffe"
	controllerpb "controller/gen/controllerpb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type controlPlaneServer struct {
	controllerpb.UnimplementedControlPlaneServer
	connectorID string
	sendCh      chan<- *controllerpb.ControlMessage
	acls        *policyCache
}

func (s *controlPlaneServer) Connect(stream controllerpb.ControlPlane_ConnectServer) error {
	role, ok := spiffe.RoleFromContext(stream.Context())
	if !ok || role != "tunneler" {
		return status.Error(codes.PermissionDenied, "tunneler role required")
	}

	spiffeID, _ := spiffe.SPIFFEIDFromContext(stream.Context())
	log.Printf("tunneler connected: %s", spiffeID)
	tunnelerID := parseTunnelerID(spiffeID)
	connectionID := fmt.Sprintf("conn-%d", time.Now().UnixNano())

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
		if msg.GetType() == "tunneler_heartbeat" && s.sendCh != nil {
			payload := struct {
				TunnelerID  string `json:"tunneler_id"`
				SPIFFEID    string `json:"spiffe_id"`
				Status      string `json:"status"`
				ConnectorID string `json:"connector_id"`
			}{
				TunnelerID:  tunnelerID,
				SPIFFEID:    spiffeID,
				Status:      msg.GetStatus(),
				ConnectorID: s.connectorID,
			}
			if data, err := json.Marshal(payload); err == nil {
				s.sendCh <- &controllerpb.ControlMessage{
					Type:    "tunneler_heartbeat",
					Payload: data,
				}
			}
		}
		if msg.GetType() == "tunneler_request" && s.acls != nil {
			var req struct {
				Destination string `json:"destination"`
				Protocol    string `json:"protocol"`
				Port        uint16 `json:"port"`
			}
			if err := json.Unmarshal(msg.GetPayload(), &req); err != nil {
				s.sendDecision(spiffeID, tunnelerID, req.Destination, req.Protocol, req.Port, false, "", "invalid_request", connectionID)
				continue
			}
			allowed, resourceID, reason := s.acls.Allowed(spiffeID, req.Destination, req.Protocol, req.Port)
			s.sendDecision(spiffeID, tunnelerID, req.Destination, req.Protocol, req.Port, allowed, resourceID, reason, connectionID)
		}
	}
}

func (s *controlPlaneServer) sendDecision(spiffeID, tunnelerID, dest, protocol string, port uint16, allowed bool, resourceID, reason, connectionID string) {
	decision := "deny"
	if allowed {
		decision = "allow"
	}
	log.Printf("acl decision: principal=%s tunneler_id=%s resource_id=%s dest=%s protocol=%s port=%d decision=%s reason=%s",
		spiffeID, tunnelerID, resourceID, dest, protocol, port, decision, reason)

	if s.sendCh == nil {
		return
	}
	payload := struct {
		TunnelerID  string `json:"tunneler_id"`
		SPIFFEID    string `json:"spiffe_id"`
		ResourceID  string `json:"resource_id"`
		Destination string `json:"destination"`
		Protocol    string `json:"protocol"`
		Port        uint16 `json:"port"`
		Decision    string `json:"decision"`
		Reason      string `json:"reason"`
		ConnectorID string `json:"connector_id"`
	}{
		TunnelerID:  tunnelerID,
		SPIFFEID:    spiffeID,
		ResourceID:  resourceID,
		Destination: dest,
		Protocol:    protocol,
		Port:        port,
		Decision:    decision,
		Reason:      reason,
		ConnectorID: s.connectorID,
		// ConnectionID: connectionID,
	}
	if data, err := json.Marshal(payload); err == nil {
		s.sendCh <- &controllerpb.ControlMessage{
			Type:    "acl_decision",
			Payload: data,
		}
	}
}

func parseTunnelerID(spiffeID string) string {
	if spiffeID == "" {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(spiffeID, "spiffe://"), "/")
	if len(parts) < 3 {
		return ""
	}
	if parts[1] != "tunneler" {
		return ""
	}
	return parts[2]
}
