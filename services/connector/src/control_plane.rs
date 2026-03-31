/// ControlPlane gRPC server implementation (agent-facing).
use crate::allowlist::AgentAllowlist;
use crate::policy::PolicyCache;
use crate::tls::spiffe::agent_id_from_spiffe;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tokio::sync::mpsc;
use tonic::{Request, Response, Status, Streaming};
use tracing::{info, warn};

// Re-export generated types
pub use crate::enroll::pb::{
    control_plane_server::{ControlPlane, ControlPlaneServer},
    ControlMessage,
};

pub struct ConnectorControlPlane {
    pub connector_id: String,
    pub send_ch: mpsc::Sender<ControlMessage>,
    pub allowlist: Arc<AgentAllowlist>,
    pub acl: Arc<PolicyCache>,
    pub trust_domain: String,
    pub agent_registry: Arc<crate::AgentRegistry>,
    pub agent_tunnel_hub: crate::agent_tunnel::AgentTunnelHub,
    pub firewall_tx: tokio::sync::broadcast::Sender<()>,
    pub latest_fw_policy: crate::LatestFirewallPolicy,
}

#[tonic::async_trait]
impl ControlPlane for ConnectorControlPlane {
    type ConnectStream = tokio_stream::wrappers::ReceiverStream<Result<ControlMessage, Status>>;

    async fn connect(
        &self,
        request: Request<Streaming<ControlMessage>>,
    ) -> Result<Response<Self::ConnectStream>, Status> {
        // Extract SPIFFE ID from TLS peer cert via metadata / extensions
        // In tonic, the peer cert is available via request extensions
        let spiffe_id = extract_spiffe_id_from_request(&request, &self.trust_domain)?;

        // Verify it's an agent and is allowed
        crate::tls::spiffe::verify_spiffe_uri(&spiffe_id, &self.trust_domain, "agent")
            .map_err(|e| Status::permission_denied(format!("SPIFFE verify: {}", e)))?;

        if !self.allowlist.allowed(&spiffe_id) {
            warn!(
                "agent rejected by allowlist: spiffe_id={} trust_domain={} allowlist_size={}",
                spiffe_id,
                self.trust_domain,
                self.allowlist.len()
            );
            return Err(Status::permission_denied("agent not in allowlist"));
        }

        let agent_id = agent_id_from_spiffe(&spiffe_id).unwrap_or_else(|| "unknown".to_string());

        // Capture the TCP peer IP at connection time — most reliable source.
        let peer_ip = {
            use crate::server::PeerCertInfo;
            request
                .extensions()
                .get::<PeerCertInfo>()
                .map(|p| p.peer_ip.clone())
                .unwrap_or_default()
        };
        info!("agent connected: {} ip={}", spiffe_id, peer_ip);

        let mut in_stream = request.into_inner();
        let (tx, rx) = mpsc::channel::<Result<ControlMessage, Status>>(16);

        let send_ch = self.send_ch.clone();
        let acl = self.acl.clone();
        let connector_id = self.connector_id.clone();
        let agent_registry = self.agent_registry.clone();
        let agent_tunnel_hub = self.agent_tunnel_hub.clone();
        let mut firewall_rx = self.firewall_tx.subscribe();
        let latest_fw_policy = self.latest_fw_policy.clone();
        agent_tunnel_hub.register_agent(&agent_id, tx.clone());

        // Register with the peer IP immediately so it's available before any heartbeat.
        agent_registry.update(&agent_id, "ONLINE", &peer_ip);

        tokio::spawn(async move {
            let _ =
                send_current_firewall_policy(&tx, &agent_id, &agent_registry, &latest_fw_policy)
                    .await;

            loop {
                tokio::select! {
                    msg = in_stream.message() => {
                        match msg {
                            Ok(None) => break,
                            Ok(Some(msg)) => {
                                handle_agent_message(
                                    &msg,
                                    &spiffe_id,
                                    &agent_id,
                                    &connector_id,
                                    &tx,
                                    &send_ch,
                                    &acl,
                                    &agent_registry,
                                    &peer_ip,
                                    &agent_tunnel_hub,
                                    &latest_fw_policy,
                                )
                                .await;
                            }
                            Err(e) => {
                                warn!("agent stream error: {}", e);
                                break;
                            }
                        }
                    }
                    Ok(()) = firewall_rx.recv() => {
                        let _ = send_current_firewall_policy(&tx, &agent_id, &agent_registry, &latest_fw_policy).await;
                    }
                }
            }
            agent_tunnel_hub.unregister_agent(&agent_id);
            agent_registry.remove(&agent_id);
            info!("agent disconnected: {}", spiffe_id);
        });

        Ok(Response::new(tokio_stream::wrappers::ReceiverStream::new(
            rx,
        )))
    }
}

async fn handle_agent_message(
    msg: &ControlMessage,
    spiffe_id: &str,
    agent_id: &str,
    connector_id: &str,
    tx: &mpsc::Sender<Result<ControlMessage, Status>>,
    send_ch: &mpsc::Sender<ControlMessage>,
    acl: &Arc<PolicyCache>,
    agent_registry: &Arc<crate::AgentRegistry>,
    peer_ip: &str,
    agent_tunnel_hub: &crate::agent_tunnel::AgentTunnelHub,
    latest_fw_policy: &crate::LatestFirewallPolicy,
) {
    if agent_tunnel_hub.handle_incoming(msg) {
        return;
    }
    match msg.r#type.as_str() {
        "ping" => {
            let _ = tx
                .send(Ok(ControlMessage {
                    r#type: "pong".to_string(),
                    ..Default::default()
                }))
                .await;
        }
        "agent_heartbeat" => {
            // Prefer TCP peer IP unless it is loopback (agent on same host as connector),
            // in which case use the self-reported IP from the heartbeat payload.
            let status = if msg.status.is_empty() {
                "ONLINE"
            } else {
                &msg.status
            };
            let payload_ip = if !msg.payload.is_empty() {
                #[derive(Deserialize)]
                struct HbPayload {
                    #[serde(default)]
                    ip: String,
                }
                serde_json::from_slice::<HbPayload>(&msg.payload)
                    .map(|p| p.ip)
                    .unwrap_or_default()
            } else {
                String::new()
            };
            let is_loopback = peer_ip == "127.0.0.1" || peer_ip == "::1";
            let ip = if !peer_ip.is_empty() && !is_loopback {
                peer_ip.to_string()
            } else if !payload_ip.is_empty() {
                payload_ip
            } else {
                peer_ip.to_string()
            };
            agent_registry.update(agent_id, status, &ip);
            let _ =
                send_current_firewall_policy(tx, agent_id, agent_registry, latest_fw_policy).await;
            info!(
                "agent heartbeat: agent_id={} spiffe_id={} status={} ip={}",
                agent_id, spiffe_id, status, ip
            );
        }
        "agent_request" => {
            #[derive(Deserialize)]
            struct AgentRequest {
                destination: String,
                protocol: String,
                port: u16,
            }
            let req: AgentRequest = match serde_json::from_slice(&msg.payload) {
                Ok(r) => r,
                Err(_) => {
                    send_decision(
                        spiffe_id,
                        agent_id,
                        "",
                        "",
                        "",
                        0,
                        false,
                        "",
                        "invalid_request",
                        connector_id,
                        send_ch,
                    )
                    .await;
                    return;
                }
            };
            let (allowed, resource_id, reason) =
                acl.allowed(spiffe_id, &req.destination, &req.protocol, req.port);
            send_decision(
                spiffe_id,
                agent_id,
                &req.destination,
                &req.protocol,
                &resource_id,
                req.port,
                allowed,
                &resource_id,
                reason,
                connector_id,
                send_ch,
            )
            .await;
        }
        "agent_log" => {
            let _ = send_ch.send(msg.clone()).await;
        }
        "agent_posture" => {
            let _ = send_ch.send(msg.clone()).await;
        }
        "agent_discovery_diff" => {
            let _ = send_ch.send(msg.clone()).await;
        }
        "agent_discovery_full_sync" => {
            let _ = send_ch.send(msg.clone()).await;
        }
        "agent_discovery_report" => {
            let _ = send_ch.send(msg.clone()).await;
        }
        "agent_discovery_gone" => {
            let _ = send_ch.send(msg.clone()).await;
        }
        "agent_discovery_heartbeat" => {
            let _ = send_ch.send(msg.clone()).await;
        }
        _ => {}
    }
}

async fn send_current_firewall_policy(
    tx: &mpsc::Sender<Result<ControlMessage, Status>>,
    agent_id: &str,
    agent_registry: &Arc<crate::AgentRegistry>,
    latest_fw_policy: &crate::LatestFirewallPolicy,
) -> Result<(), mpsc::error::SendError<Result<ControlMessage, Status>>> {
    let agent_ip = agent_registry.get_ip(agent_id).unwrap_or_default();
    let protected_resources = latest_fw_policy.get();
    let agent_snapshot = agent_registry.snapshot();
    info!(
        "building firewall payload: agent_id={} agent_ip={} protected_resources={} agents={}",
        agent_id,
        agent_ip,
        protected_resources.len(),
        agent_snapshot.len(),
    );
    let payload = crate::build_agent_firewall_payload(
        agent_id,
        &agent_ip,
        &protected_resources,
        &agent_snapshot,
    );
    if let Ok(parsed) = serde_json::from_slice::<serde_json::Value>(&payload) {
        let port_count = parsed["protected_ports"]
            .as_array()
            .map(|a| a.len())
            .unwrap_or(0);
        info!(
            "firewall payload for agent {}: protected_ports={} payload={}",
            agent_id,
            port_count,
            if port_count == 0 {
                format!("EMPTY (agent_ip={}, resource addresses: {})",
                    agent_ip,
                    protected_resources.iter()
                        .map(|r| format!("{}[agent_ids={:?}]", r.address.trim(), r.agent_ids))
                        .collect::<Vec<_>>()
                        .join(", ")
                )
            } else {
                String::from("ok")
            }
        );
    }
    tx.send(Ok(ControlMessage {
        r#type: "firewall_policy".to_string(),
        payload,
        ..Default::default()
    }))
    .await
}

#[allow(clippy::too_many_arguments)]
async fn send_decision(
    spiffe_id: &str,
    agent_id: &str,
    dest: &str,
    protocol: &str,
    _resource_id: &str,
    port: u16,
    allowed: bool,
    resource_id: &str,
    reason: &str,
    connector_id: &str,
    send_ch: &mpsc::Sender<ControlMessage>,
) {
    let decision = if allowed { "allow" } else { "deny" };
    tracing::info!(
        "acl decision: principal={} agent_id={} resource_id={} dest={} protocol={} port={} decision={} reason={}",
        spiffe_id, agent_id, resource_id, dest, protocol, port, decision, reason
    );

    #[derive(Serialize)]
    struct DecisionPayload<'a> {
        agent_id: &'a str,
        spiffe_id: &'a str,
        resource_id: &'a str,
        destination: &'a str,
        protocol: &'a str,
        port: u16,
        decision: &'a str,
        reason: &'a str,
        connector_id: &'a str,
    }
    let payload = DecisionPayload {
        agent_id,
        spiffe_id,
        resource_id,
        destination: dest,
        protocol,
        port,
        decision,
        reason,
        connector_id,
    };
    if let Ok(data) = serde_json::to_vec(&payload) {
        let _ = send_ch
            .send(ControlMessage {
                r#type: "acl_decision".to_string(),
                payload: data,
                ..Default::default()
            })
            .await;
    }
}

#[allow(clippy::result_large_err)]
fn extract_spiffe_id_from_request<T>(
    request: &Request<T>,
    _trust_domain: &str,
) -> Result<String, Status> {
    use crate::server::PeerCertInfo;

    let peer_info = request
        .extensions()
        .get::<PeerCertInfo>()
        .ok_or_else(|| Status::unauthenticated("no TLS connection info"))?;

    let cert = peer_info
        .peer_certs
        .first()
        .ok_or_else(|| Status::unauthenticated("no peer certificates"))?;

    crate::tls::spiffe::extract_spiffe_id(cert)
        .map_err(|e| Status::unauthenticated(format!("SPIFFE extract failed: {}", e)))
}
