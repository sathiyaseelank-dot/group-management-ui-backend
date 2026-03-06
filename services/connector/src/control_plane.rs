/// ControlPlane gRPC server implementation (tunneler-facing).
use crate::allowlist::TunnelerAllowlist;
use crate::policy::PolicyCache;
use crate::tls::spiffe::tunneler_id_from_spiffe;
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
    pub allowlist: Arc<TunnelerAllowlist>,
    pub acl: Arc<PolicyCache>,
    pub trust_domain: String,
}

#[tonic::async_trait]
impl ControlPlane for ConnectorControlPlane {
    type ConnectStream =
        tokio_stream::wrappers::ReceiverStream<Result<ControlMessage, Status>>;

    async fn connect(
        &self,
        request: Request<Streaming<ControlMessage>>,
    ) -> Result<Response<Self::ConnectStream>, Status> {
        // Extract SPIFFE ID from TLS peer cert via metadata / extensions
        // In tonic, the peer cert is available via request extensions
        let spiffe_id = extract_spiffe_id_from_request(&request, &self.trust_domain)?;

        // Verify it's a tunneler and is allowed
        crate::tls::spiffe::verify_spiffe_uri(&spiffe_id, &self.trust_domain, "tunneler")
            .map_err(|e| Status::permission_denied(format!("SPIFFE verify: {}", e)))?;

        if !self.allowlist.allowed(&spiffe_id) {
            return Err(Status::permission_denied("tunneler not in allowlist"));
        }

        let tunneler_id = tunneler_id_from_spiffe(&spiffe_id)
            .unwrap_or_else(|| "unknown".to_string());
    info!("tunneler connected: {}", spiffe_id);

        let mut in_stream = request.into_inner();
        let (tx, rx) = mpsc::channel::<Result<ControlMessage, Status>>(16);

        let send_ch = self.send_ch.clone();
        let acl = self.acl.clone();
        let connector_id = self.connector_id.clone();

        tokio::spawn(async move {
            loop {
                match in_stream.message().await {
                    Ok(None) => break,
                    Ok(Some(msg)) => {
                        handle_tunneler_message(
                            &msg,
                            &spiffe_id,
                            &tunneler_id,
                            &connector_id,
                            &tx,
                            &send_ch,
                            &acl,
                        )
                        .await;
                    }
                    Err(e) => {
                        warn!("tunneler stream error: {}", e);
                        break;
                    }
                }
            }
        });

        Ok(Response::new(tokio_stream::wrappers::ReceiverStream::new(rx)))
    }
}

async fn handle_tunneler_message(
    msg: &ControlMessage,
    spiffe_id: &str,
    tunneler_id: &str,
    connector_id: &str,
    tx: &mpsc::Sender<Result<ControlMessage, Status>>,
    send_ch: &mpsc::Sender<ControlMessage>,
    acl: &Arc<PolicyCache>,
) {
    match msg.r#type.as_str() {
        "ping" => {
            let _ = tx
                .send(Ok(ControlMessage {
                    r#type: "pong".to_string(),
                    ..Default::default()
                }))
                .await;
        }
        "tunneler_heartbeat" => {
            #[derive(Serialize)]
            struct HeartbeatPayload<'a> {
                tunneler_id: &'a str,
                spiffe_id: &'a str,
                status: &'a str,
                connector_id: &'a str,
            }
            let payload = HeartbeatPayload {
                tunneler_id,
                spiffe_id,
                status: &msg.status,
                connector_id,
            };
            if let Ok(data) = serde_json::to_vec(&payload) {
                let _ = send_ch
                    .send(ControlMessage {
                        r#type: "tunneler_heartbeat".to_string(),
                        payload: data,
                        ..Default::default()
                    })
                    .await;
            }
        }
        "tunneler_request" => {
            #[derive(Deserialize)]
            struct TunnelerRequest {
                destination: String,
                protocol: String,
                port: u16,
            }
            let req: TunnelerRequest = match serde_json::from_slice(&msg.payload) {
                Ok(r) => r,
                Err(_) => {
                    send_decision(
                        spiffe_id,
                        tunneler_id,
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
                tunneler_id,
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
        _ => {}
    }
}

#[allow(clippy::too_many_arguments)]
async fn send_decision(
    spiffe_id: &str,
    tunneler_id: &str,
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
        "acl decision: principal={} tunneler_id={} resource_id={} dest={} protocol={} port={} decision={} reason={}",
        spiffe_id, tunneler_id, resource_id, dest, protocol, port, decision, reason
    );

    #[derive(Serialize)]
    struct DecisionPayload<'a> {
        tunneler_id: &'a str,
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
        tunneler_id,
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


