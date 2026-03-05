/// Tunneler-facing gRPC server on :9443
use crate::allowlist::TunnelerAllowlist;
use crate::control_plane::{ConnectorControlPlane, ControlPlaneServer};
use crate::policy::PolicyCache;
use crate::tls::cert_store::CertStore;
use anyhow::Result;
use std::pin::Pin;
use std::sync::Arc;
use std::task::{Context, Poll};
use tokio::io::{AsyncRead, AsyncWrite, ReadBuf};
use tokio::sync::mpsc;
use tonic::transport::server::Connected;
use tonic::transport::Server;
use tracing::{info, warn};

use crate::enroll::pb::ControlMessage;

/// Peer certificate info extracted from the TLS session, injected into
/// tonic request extensions via the `Connected` trait.
#[derive(Clone, Debug)]
pub struct PeerCertInfo {
    pub peer_certs: Vec<Vec<u8>>,
}

/// Wrapper around a TLS stream that implements `Connected` so tonic
/// can populate request extensions with peer certificate info.
pub struct TlsConnStream {
    inner: tokio_rustls::server::TlsStream<tokio::net::TcpStream>,
    peer_info: PeerCertInfo,
}

impl TlsConnStream {
    fn new(tls: tokio_rustls::server::TlsStream<tokio::net::TcpStream>) -> Self {
        let peer_certs = tls
            .get_ref()
            .1
            .peer_certificates()
            .map(|certs| certs.iter().map(|c| c.as_ref().to_vec()).collect())
            .unwrap_or_default();
        Self {
            inner: tls,
            peer_info: PeerCertInfo { peer_certs },
        }
    }
}

impl Connected for TlsConnStream {
    type ConnectInfo = PeerCertInfo;
    fn connect_info(&self) -> Self::ConnectInfo {
        self.peer_info.clone()
    }
}

impl AsyncRead for TlsConnStream {
    fn poll_read(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &mut ReadBuf<'_>) -> Poll<std::io::Result<()>> {
        Pin::new(&mut self.get_mut().inner).poll_read(cx, buf)
    }
}

impl AsyncWrite for TlsConnStream {
    fn poll_write(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &[u8]) -> Poll<std::io::Result<usize>> {
        Pin::new(&mut self.get_mut().inner).poll_write(cx, buf)
    }
    fn poll_flush(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<std::io::Result<()>> {
        Pin::new(&mut self.get_mut().inner).poll_flush(cx)
    }
    fn poll_shutdown(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<std::io::Result<()>> {
        Pin::new(&mut self.get_mut().inner).poll_shutdown(cx)
    }
}

#[allow(clippy::too_many_arguments)]
pub async fn server_loop(
    listen_addr: String,
    trust_domain: String,
    store: CertStore,
    ca_pem: Vec<u8>,
    allowlist: Arc<TunnelerAllowlist>,
    acl: Arc<PolicyCache>,
    send_ch: mpsc::Sender<ControlMessage>,
    connector_id: String,
) {
    let mut backoff = std::time::Duration::from_secs(2);
    loop {
        match run_server(
            &listen_addr,
            &trust_domain,
            &store,
            &ca_pem,
            allowlist.clone(),
            acl.clone(),
            send_ch.clone(),
            connector_id.clone(),
        )
        .await
        {
            Ok(()) => {}
            Err(e) => warn!("connector server stopped: {}", e),
        }

        tokio::time::sleep(backoff).await;
        if backoff < std::time::Duration::from_secs(30) {
            backoff *= 2;
        }
    }
}

#[allow(clippy::too_many_arguments)]
async fn run_server(
    listen_addr: &str,
    trust_domain: &str,
    store: &CertStore,
    ca_pem: &[u8],
    allowlist: Arc<TunnelerAllowlist>,
    acl: Arc<PolicyCache>,
    send_ch: mpsc::Sender<ControlMessage>,
    connector_id: String,
) -> Result<()> {
    let server_tls = crate::tls::server_cfg::build_server_tls(store, ca_pem, trust_domain)?;
    let tls_acceptor = tokio_rustls::TlsAcceptor::from(Arc::new(server_tls));

    let addr: std::net::SocketAddr = listen_addr.parse()?;
    let listener = tokio::net::TcpListener::bind(addr).await?;

    let svc = ConnectorControlPlane {
        connector_id,
        send_ch,
        allowlist,
        acl,
        trust_domain: trust_domain.to_string(),
    };

    info!("connector server listening on {}", listen_addr);

    // Accept TCP connections, upgrade to TLS, wrap with peer cert info, feed into tonic
    let (conn_tx, conn_rx) = mpsc::channel::<Result<TlsConnStream, std::io::Error>>(16);
    let incoming = tokio_stream::wrappers::ReceiverStream::new(conn_rx);

    tokio::spawn(async move {
        loop {
            match listener.accept().await {
                Ok((tcp, _peer)) => {
                    let acceptor = tls_acceptor.clone();
                    let tx = conn_tx.clone();
                    tokio::spawn(async move {
                        match acceptor.accept(tcp).await {
                            Ok(tls) => { let _ = tx.send(Ok(TlsConnStream::new(tls))).await; }
                            Err(e) => {
                                tracing::debug!("TLS accept failed: {}", e);
                            }
                        }
                    });
                }
                Err(e) => {
                    warn!("TCP accept failed: {}", e);
                    break;
                }
            }
        }
    });

    Server::builder()
        .add_service(ControlPlaneServer::new(svc))
        .serve_with_incoming(incoming)
        .await?;

    Ok(())
}
