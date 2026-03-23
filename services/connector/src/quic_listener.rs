//! QUIC device tunnel listener.
//!
//! Runs alongside the TLS/TCP device tunnel on the same port number but over
//! UDP.  Each QUIC bidirectional stream is handled identically to a TLS/TCP
//! connection via `device_tunnel::handle_stream`.

use anyhow::Result;
use quinn::ConnectionError;
use std::sync::Arc;
use tracing::{info, warn};

use crate::agent_tunnel::AgentTunnelHub;
use crate::device_tunnel;
use crate::policy::PolicyCache;
use crate::tls::cert_store::CertStore;
use crate::tls::server_cfg::build_device_tunnel_tls;

/// Start the QUIC listener for device tunnel connections.
///
/// Binds to the same port as the TLS/TCP listener but on UDP.  On success,
/// advertises the QUIC address so TLS responses include `quic_addr` for
/// client discovery (Option C).
pub async fn listen(
    addr: &str,
    controller_http_url: String,
    store: CertStore,
    acl: Arc<PolicyCache>,
    tunnel_hub: AgentTunnelHub,
) -> Result<()> {
    let tls_config = build_device_tunnel_tls(&store)?;

    let quic_server_config = quinn::crypto::rustls::QuicServerConfig::try_from(tls_config)
        .map_err(|e| anyhow::anyhow!("QUIC server config: {}", e))?;
    let server_config = quinn::ServerConfig::with_crypto(Arc::new(quic_server_config));

    let socket_addr: std::net::SocketAddr = addr
        .parse()
        .map_err(|e| anyhow::anyhow!("bad QUIC listen addr '{}': {}", addr, e))?;
    let endpoint = quinn::Endpoint::server(server_config, socket_addr)?;

    // Advertise this address so TLS tunnel responses include it
    device_tunnel::set_quic_advertise_addr(addr.to_string());
    info!("device tunnel (QUIC) listening on {}", addr);

    loop {
        match endpoint.accept().await {
            Some(incoming) => {
                let ctrl = controller_http_url.clone();
                let acl = acl.clone();
                let hub = tunnel_hub.clone();
                tokio::spawn(async move {
                    match incoming.await {
                        Ok(conn) => {
                            let peer = conn.remote_address();
                            // Accept bidirectional streams — each is one tunnel.
                            loop {
                                match conn.accept_bi().await {
                                    Ok((send, recv)) => {
                                        let ctrl = ctrl.clone();
                                        let acl = acl.clone();
                                        let hub = hub.clone();
                                        tokio::spawn(async move {
                                            let stream = QuicBiStream { send, recv };
                                            if let Err(e) = device_tunnel::handle_stream(
                                                stream, &ctrl, acl, hub,
                                            )
                                            .await
                                            {
                                                warn!(
                                                    "QUIC device tunnel error from {}: {}",
                                                    peer, e
                                                );
                                            }
                                        });
                                    }
                                    Err(ConnectionError::ApplicationClosed(_)) => break,
                                    Err(e) => {
                                        warn!("QUIC accept_bi from {}: {}", peer, e);
                                        break;
                                    }
                                }
                            }
                        }
                        Err(e) => warn!("QUIC connection error: {}", e),
                    }
                });
            }
            None => break,
        }
    }
    Ok(())
}

// ---------------------------------------------------------------------------
// Adapter: wrap quinn (SendStream, RecvStream) as AsyncRead + AsyncWrite
// ---------------------------------------------------------------------------

use tokio::io::{AsyncRead, AsyncWrite, ReadBuf};

struct QuicBiStream {
    send: quinn::SendStream,
    recv: quinn::RecvStream,
}

impl AsyncRead for QuicBiStream {
    fn poll_read(
        mut self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
        buf: &mut ReadBuf<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        std::pin::Pin::new(&mut self.recv).poll_read(cx, buf)
    }
}

impl AsyncWrite for QuicBiStream {
    fn poll_write(
        mut self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
        buf: &[u8],
    ) -> std::task::Poll<std::io::Result<usize>> {
        match std::pin::Pin::new(&mut self.send).poll_write(cx, buf) {
            std::task::Poll::Ready(Ok(n)) => std::task::Poll::Ready(Ok(n)),
            std::task::Poll::Ready(Err(e)) => {
                std::task::Poll::Ready(Err(std::io::Error::new(std::io::ErrorKind::Other, e)))
            }
            std::task::Poll::Pending => std::task::Poll::Pending,
        }
    }

    fn poll_flush(
        mut self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        match std::pin::Pin::new(&mut self.send).poll_flush(cx) {
            std::task::Poll::Ready(Ok(())) => std::task::Poll::Ready(Ok(())),
            std::task::Poll::Ready(Err(e)) => {
                std::task::Poll::Ready(Err(std::io::Error::new(std::io::ErrorKind::Other, e)))
            }
            std::task::Poll::Pending => std::task::Poll::Pending,
        }
    }

    fn poll_shutdown(
        mut self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        match std::pin::Pin::new(&mut self.send).poll_shutdown(cx) {
            std::task::Poll::Ready(Ok(())) => std::task::Poll::Ready(Ok(())),
            std::task::Poll::Ready(Err(e)) => {
                std::task::Poll::Ready(Err(std::io::Error::new(std::io::ErrorKind::Other, e)))
            }
            std::task::Poll::Pending => std::task::Poll::Pending,
        }
    }
}
