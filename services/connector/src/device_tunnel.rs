use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt};
use tokio::net::{TcpListener, TcpStream, UdpSocket};
use tokio_rustls::TlsAcceptor;
use tracing::{info, warn};

use crate::tls::cert_store::CertStore;
use crate::tls::server_cfg::build_device_tunnel_tls;
use crate::ControlMessage;
use crate::{agent_tunnel::AgentTunnelHub, policy::PolicyCache, AgentRegistry};

fn default_tcp() -> String {
    "tcp".to_string()
}

#[derive(Deserialize)]
struct TunnelRequest {
    token: String,
    destination: String,
    port: u16,
    #[serde(default = "default_tcp")]
    protocol: String,
}

#[derive(Serialize)]
struct TunnelResponse {
    ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
    /// QUIC endpoint address for the client to upgrade to on subsequent
    /// connections (Option C discovery).  Omitted when QUIC is not available.
    #[serde(skip_serializing_if = "Option::is_none")]
    quic_addr: Option<String>,
}

/// Global QUIC address advertised to clients.  Set once by `quic_listener`
/// when the QUIC endpoint starts successfully.
static QUIC_ADVERTISE_ADDR: std::sync::OnceLock<String> = std::sync::OnceLock::new();

/// Called by quic_listener after bind to publish the QUIC address.
pub fn set_quic_advertise_addr(addr: String) {
    let _ = QUIC_ADVERTISE_ADDR.set(addr);
}

#[derive(Deserialize)]
struct CheckAccessResponse {
    allowed: bool,
    resource_id: String,
}

pub async fn listen(
    addr: &str,
    controller_http_url: String,
    store: CertStore,
    acl: Arc<PolicyCache>,
    tunnel_hub: AgentTunnelHub,
    agent_registry: Arc<AgentRegistry>,
    connector_id: String,
    control_tx: tokio::sync::mpsc::Sender<ControlMessage>,
) -> Result<()> {
    let tls_config = build_device_tunnel_tls(&store)?;
    let acceptor = TlsAcceptor::from(Arc::new(tls_config));
    let listener = TcpListener::bind(addr).await?;
    info!("device tunnel (TLS) listening on {}", addr);

    loop {
        match listener.accept().await {
            Ok((stream, peer)) => {
                let ctrl = controller_http_url.clone();
                let acl = acl.clone();
                let tunnel_hub = tunnel_hub.clone();
                let agent_registry = agent_registry.clone();
                let connector_id = connector_id.clone();
                let control_tx = control_tx.clone();
                let acc = acceptor.clone();
                tokio::spawn(async move {
                    match acc.accept(stream).await {
                        Ok(tls) => {
                            if let Err(e) =
                                handle_stream(
                                    tls,
                                    &ctrl,
                                    acl,
                                    tunnel_hub,
                                    agent_registry,
                                    &connector_id,
                                    &control_tx,
                                )
                                .await
                            {
                                warn!("device tunnel client error from {}: {}", peer, e);
                            }
                        }
                        Err(e) => warn!("device tunnel TLS accept from {}: {}", peer, e),
                    }
                });
            }
            Err(e) => warn!("device tunnel accept error: {}", e),
        }
    }
}

async fn read_line<S: AsyncRead + Unpin>(stream: &mut S) -> Result<String> {
    let mut buf = Vec::with_capacity(256);
    let mut byte = [0u8; 1];
    loop {
        let n = stream.read(&mut byte).await?;
        if n == 0 {
            anyhow::bail!("EOF before handshake newline");
        }
        if byte[0] == b'\n' {
            break;
        }
        buf.push(byte[0]);
        if buf.len() > 4096 {
            anyhow::bail!("handshake line too long");
        }
    }
    Ok(String::from_utf8(buf)?)
}

async fn send_response<S: AsyncWrite + Unpin>(
    stream: &mut S,
    ok: bool,
    error: Option<&str>,
) -> Result<()> {
    let resp = TunnelResponse {
        ok,
        error: error.map(|s| s.to_string()),
        quic_addr: if ok {
            QUIC_ADVERTISE_ADDR.get().cloned()
        } else {
            None
        },
    };
    let mut line = serde_json::to_string(&resp)?;
    line.push('\n');
    stream.write_all(line.as_bytes()).await?;
    Ok(())
}

pub async fn handle_stream<S: AsyncRead + AsyncWrite + Unpin + Send + 'static>(
    mut stream: S,
    controller_http_url: &str,
    acl: Arc<PolicyCache>,
    tunnel_hub: AgentTunnelHub,
    agent_registry: Arc<AgentRegistry>,
    connector_id: &str,
    control_tx: &tokio::sync::mpsc::Sender<ControlMessage>,
) -> Result<()> {
    let line = read_line(&mut stream).await?;
    let req: TunnelRequest =
        serde_json::from_str(line.trim()).map_err(|e| anyhow::anyhow!("bad handshake: {}", e))?;

    let allowed = check_access(
        controller_http_url,
        &req.token,
        &req.destination,
        req.port,
        &req.protocol,
    )
    .await;
    match allowed {
        Err(e) => {
            let _ = send_response(&mut stream, false, Some("check-access error")).await;
            return Err(e);
        }
        Ok(resp) if !resp.allowed => {
            send_response(&mut stream, false, Some("access denied")).await?;
            return Ok(());
        }
        Ok(resp) => {
            let protected = acl
                .resource_by_id(&resp.resource_id)
                .map(|resource| resource.firewall_status.eq_ignore_ascii_case("protected"))
                .unwrap_or(false);
            if protected {
                let agent_id = resolve_protected_resource_owner(&req.destination, &agent_registry)
                    .ok_or_else(|| {
                        anyhow::anyhow!(
                            "no uniquely connected owning agent for protected resource {}",
                            req.destination
                        )
                    })?;
                let relay_session = crate::agent_tunnel::open_relay_session(
                    tunnel_hub,
                    &agent_id,
                    &req.destination,
                    req.port,
                    &req.protocol,
                )
                .await?;
                send_response(&mut stream, true, None).await?;
                emit_connector_access_log(
                    control_tx,
                    connector_id,
                    &format!(
                        "client access opened: destination={} port={} protocol={} path=agent_relay agent_id={}",
                        req.destination, req.port, req.protocol, agent_id
                    ),
                )
                .await;
                info!(
                    "routing protected device tunnel {}:{} via agent {}",
                    req.destination, req.port, agent_id
                );
                return relay_session.relay_stream(stream).await;
            }
            send_response(&mut stream, true, None).await?;
        }
    }

    let dest = format!("{}:{}", req.destination, req.port);

    if req.protocol == "udp" {
        return relay_udp_direct(&mut stream, &dest).await;
    }

    let mut resource = match TcpStream::connect(&dest).await {
        Ok(s) => s,
        Err(e) => {
            let msg = format!("connect to {} failed: {}", dest, e);
            let _ = send_response(&mut stream, false, Some(&msg)).await;
            return Err(anyhow::anyhow!("{}", msg));
        }
    };
    emit_connector_access_log(
        control_tx,
        connector_id,
        &format!(
            "client access opened: destination={} port={} protocol={} path=connector_direct",
            req.destination, req.port, req.protocol
        ),
    )
    .await;

    match tokio::io::copy_bidirectional(&mut stream, &mut resource).await {
        Ok((sent, recv)) => info!("device tunnel closed {} sent={} recv={}", dest, sent, recv),
        Err(e) => warn!("device tunnel I/O error {}: {}", dest, e),
    }
    Ok(())
}

async fn emit_connector_access_log(
    control_tx: &tokio::sync::mpsc::Sender<ControlMessage>,
    connector_id: &str,
    message: &str,
) {
    let payload = serde_json::json!({
        "connector_id": connector_id,
        "message": message,
    });
    let _ = control_tx
        .send(ControlMessage {
            r#type: "connector_log".to_string(),
            connector_id: connector_id.to_string(),
            payload: serde_json::to_vec(&payload).unwrap_or_default(),
            ..Default::default()
        })
        .await;
}

async fn check_access(
    controller_http_url: &str,
    token: &str,
    destination: &str,
    port: u16,
    protocol: &str,
) -> Result<CheckAccessResponse> {
    #[derive(Serialize)]
    struct Req<'a> {
        destination: &'a str,
        protocol: &'a str,
        port: u16,
    }

    let resp = reqwest::Client::new()
        .post(format!("{}/api/device/check-access", controller_http_url))
        .bearer_auth(token)
        .json(&Req {
            destination,
            protocol,
            port,
        })
        .send()
        .await?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        anyhow::bail!("check-access: {}", text);
    }

    Ok(resp.json::<CheckAccessResponse>().await?)
}

/// Relay length-prefixed UDP datagrams between a TLS stream and a UDP socket.
async fn relay_udp_direct<S: AsyncRead + AsyncWrite + Unpin>(
    stream: &mut S,
    dest: &str,
) -> Result<()> {
    let udp = UdpSocket::bind("0.0.0.0:0").await?;
    udp.connect(dest).await?;

    let mut udp_buf = [0u8; 65535];
    let mut len_buf = [0u8; 4];

    loop {
        tokio::select! {
            // TLS stream → UDP socket (client sending to resource)
            result = stream.read_exact(&mut len_buf) => {
                match result {
                    Ok(_) => {}
                    Err(_) => break,
                }
                let len = u32::from_be_bytes(len_buf) as usize;
                if len > 65535 { break; }
                let mut buf = vec![0u8; len];
                if stream.read_exact(&mut buf).await.is_err() { break; }
                if udp.send(&buf).await.is_err() { break; }
            }
            // UDP socket → TLS stream (resource responding to client)
            result = udp.recv(&mut udp_buf) => {
                match result {
                    Ok(n) => {
                        let len = (n as u32).to_be_bytes();
                        if stream.write_all(&len).await.is_err() { break; }
                        if stream.write_all(&udp_buf[..n]).await.is_err() { break; }
                    }
                    Err(_) => break,
                }
            }
        }
    }
    info!("UDP relay closed {}", dest);
    Ok(())
}

fn resolve_protected_resource_owner(
    destination: &str,
    agent_registry: &AgentRegistry,
) -> Option<String> {
    let destination = destination.trim();
    if destination.is_empty() {
        return None;
    }

    let mut matches = agent_registry
        .snapshot()
        .into_iter()
        .filter(|agent| agent.ip.trim() == destination)
        .map(|agent| agent.agent_id);

    let owner = matches.next()?;
    if matches.next().is_some() {
        return None;
    }
    Some(owner)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resolve_protected_resource_owner_requires_unique_ip_match() {
        let registry = AgentRegistry::new();
        registry.update("alpha-1", "ONLINE", "192.168.1.85");
        registry.update("beta-1", "ONLINE", "192.168.1.86");

        assert_eq!(
            resolve_protected_resource_owner("192.168.1.85", &registry).as_deref(),
            Some("alpha-1")
        );
        assert_eq!(
            resolve_protected_resource_owner("192.168.1.99", &registry),
            None
        );

        registry.update("gamma-1", "ONLINE", "192.168.1.85");
        assert_eq!(
            resolve_protected_resource_owner("192.168.1.85", &registry),
            None
        );
    }
}
