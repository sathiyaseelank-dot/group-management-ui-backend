use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt};
use tokio::net::{TcpListener, TcpStream};
use tokio_rustls::TlsAcceptor;
use tracing::{info, warn};

use crate::tls::cert_store::CertStore;
use crate::tls::server_cfg::build_device_tunnel_tls;
use crate::{agent_tunnel::AgentTunnelHub, policy::PolicyCache};

#[derive(Deserialize)]
struct TunnelRequest {
    token: String,
    destination: String,
    port: u16,
}

#[derive(Serialize)]
struct TunnelResponse {
    ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
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
                let acc = acceptor.clone();
                tokio::spawn(async move {
                    match acc.accept(stream).await {
                        Ok(tls) => {
                            if let Err(e) = handle(tls, &ctrl, acl, tunnel_hub).await {
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
    };
    let mut line = serde_json::to_string(&resp)?;
    line.push('\n');
    stream.write_all(line.as_bytes()).await?;
    Ok(())
}

async fn handle(
    mut stream: tokio_rustls::server::TlsStream<TcpStream>,
    controller_http_url: &str,
    acl: Arc<PolicyCache>,
    tunnel_hub: AgentTunnelHub,
) -> Result<()> {
    let line = read_line(&mut stream).await?;
    let req: TunnelRequest =
        serde_json::from_str(line.trim()).map_err(|e| anyhow::anyhow!("bad handshake: {}", e))?;

    let allowed = check_access(controller_http_url, &req.token, &req.destination, req.port).await;
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
            send_response(&mut stream, true, None).await?;
            if protected {
                let agent_id = tunnel_hub
                    .first_agent_id()
                    .ok_or_else(|| anyhow::anyhow!("no connected agent for protected resource"))?;
                info!(
                    "routing protected device tunnel {}:{} via agent {}",
                    req.destination, req.port, agent_id
                );
                return crate::agent_tunnel::relay_stream(
                    tunnel_hub,
                    &agent_id,
                    stream,
                    &req.destination,
                    req.port,
                )
                .await;
            }
        }
    }

    let dest = format!("{}:{}", req.destination, req.port);
    let mut resource = match TcpStream::connect(&dest).await {
        Ok(s) => s,
        Err(e) => {
            let msg = format!("connect to {} failed: {}", dest, e);
            let _ = send_response(&mut stream, false, Some(&msg)).await;
            return Err(anyhow::anyhow!("{}", msg));
        }
    };

    match tokio::io::copy_bidirectional(&mut stream, &mut resource).await {
        Ok((sent, recv)) => info!("device tunnel closed {} sent={} recv={}", dest, sent, recv),
        Err(e) => warn!("device tunnel I/O error {}: {}", dest, e),
    }
    Ok(())
}

async fn check_access(
    controller_http_url: &str,
    token: &str,
    destination: &str,
    port: u16,
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
            protocol: "tcp",
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
