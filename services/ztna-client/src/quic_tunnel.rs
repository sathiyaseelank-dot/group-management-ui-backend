//! QUIC connection pool for the device tunnel.
//!
//! Maintains one QUIC connection per connector endpoint.  Each resource
//! connection opens a new bidirectional QUIC stream over the existing
//! connection — eliminating per-connection TLS handshake overhead and
//! enabling 0-RTT resumption on reconnects.
//!
//! Discovery: the client learns the QUIC address from the `quic_addr` field
//! in the TLS tunnel handshake response (Option C).

use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::Arc;
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt, ReadBuf};
use tokio::sync::Mutex;
use tracing::info;

/// Cache of discovered QUIC addresses, keyed by TLS tunnel address.
/// Populated from `quic_addr` in TLS handshake responses.
#[derive(Clone, Default)]
pub struct QuicAddrCache {
    inner: Arc<Mutex<HashMap<String, String>>>,
}

impl QuicAddrCache {
    pub fn new() -> Self {
        Self::default()
    }

    pub async fn set(&self, tls_addr: &str, quic_addr: String) {
        self.inner
            .lock()
            .await
            .insert(tls_addr.to_string(), quic_addr);
    }

    pub async fn get(&self, tls_addr: &str) -> Option<String> {
        self.inner.lock().await.get(tls_addr).cloned()
    }

    pub async fn remove(&self, tls_addr: &str) {
        self.inner.lock().await.remove(tls_addr);
    }
}

/// Pool of QUIC connections, one per connector endpoint.
#[derive(Clone)]
pub struct QuicPool {
    connections: Arc<Mutex<HashMap<String, quinn::Connection>>>,
    client_config: Arc<quinn::ClientConfig>,
}

impl QuicPool {
    pub fn new(ca_pem: &[u8]) -> Result<Self> {
        let client_config = build_quic_client_config(ca_pem)?;
        Ok(Self {
            connections: Arc::new(Mutex::new(HashMap::new())),
            client_config: Arc::new(client_config),
        })
    }

    /// Open a bidirectional QUIC stream for a tunnel connection.
    ///
    /// Reuses an existing QUIC connection to the endpoint, or creates a new one.
    /// Performs the JSON handshake on the stream and returns it ready for relay.
    pub async fn open_stream(
        &self,
        quic_addr: &str,
        token: &str,
        destination: &str,
        port: u16,
        protocol: &str,
    ) -> Result<QuicBiStream> {
        let conn = self.get_or_connect(quic_addr).await?;

        let (send, recv) = conn
            .open_bi()
            .await
            .map_err(|e| anyhow::anyhow!("QUIC open_bi: {}", e))?;

        let mut stream = QuicBiStream { send, recv };

        // JSON handshake (same format as TLS tunnel)
        let req = TunnelRequest {
            token,
            destination,
            port,
            protocol,
        };
        let mut line = serde_json::to_string(&req)?;
        line.push('\n');
        stream.write_all(line.as_bytes()).await?;

        let resp = read_response_line(&mut stream).await?;
        if !resp.ok {
            anyhow::bail!(
                "QUIC tunnel rejected: {}",
                resp.error.unwrap_or_else(|| "denied".into())
            );
        }

        Ok(stream)
    }

    async fn get_or_connect(&self, quic_addr: &str) -> Result<quinn::Connection> {
        let mut conns = self.connections.lock().await;

        // Reuse existing connection if still alive
        if let Some(conn) = conns.get(quic_addr) {
            if conn.close_reason().is_none() {
                return Ok(conn.clone());
            }
        }
        // Remove stale connection (if any) before creating a new one
        conns.remove(quic_addr);

        // Create new QUIC connection
        let addr: std::net::SocketAddr = quic_addr
            .parse()
            .map_err(|e| anyhow::anyhow!("bad QUIC addr '{}': {}", quic_addr, e))?;

        let mut endpoint =
            quinn::Endpoint::client("0.0.0.0:0".parse().unwrap())?;
        endpoint.set_default_client_config((*self.client_config).clone());

        let host = quic_addr
            .rsplit_once(':')
            .map(|(h, _)| h)
            .unwrap_or(quic_addr)
            .trim_matches(['[', ']']);

        info!("[quic] connecting to {} (server_name={})", quic_addr, host);
        let conn = endpoint
            .connect(addr, host)?
            .await
            .map_err(|e| anyhow::anyhow!("QUIC connect to {}: {}", quic_addr, e))?;

        info!("[quic] connected to {}", quic_addr);
        conns.insert(quic_addr.to_string(), conn.clone());
        Ok(conn)
    }
}

// ---------------------------------------------------------------------------
// Handshake types (same wire format as TLS tunnel)
// ---------------------------------------------------------------------------

#[derive(Serialize)]
struct TunnelRequest<'a> {
    token: &'a str,
    destination: &'a str,
    port: u16,
    protocol: &'a str,
}

#[derive(Deserialize)]
struct TunnelResponse {
    ok: bool,
    error: Option<String>,
}

async fn read_response_line<R: AsyncRead + Unpin>(r: &mut R) -> Result<TunnelResponse> {
    let mut buf = Vec::with_capacity(256);
    let mut byte = [0u8; 1];
    loop {
        let n = r.read(&mut byte).await?;
        if n == 0 {
            anyhow::bail!("EOF before tunnel response");
        }
        if byte[0] == b'\n' {
            break;
        }
        buf.push(byte[0]);
        if buf.len() > 4096 {
            anyhow::bail!("tunnel response too long");
        }
    }
    Ok(serde_json::from_slice(&buf)?)
}

// ---------------------------------------------------------------------------
// TLS config for QUIC client
// ---------------------------------------------------------------------------

fn build_quic_client_config(ca_pem: &[u8]) -> Result<quinn::ClientConfig> {
    let tls_config = crate::tunnel::build_client_tls_for_quic(ca_pem)?;
    let quic_config = quinn::crypto::rustls::QuicClientConfig::try_from(tls_config)
        .map_err(|e| anyhow::anyhow!("QUIC client config: {}", e))?;
    Ok(quinn::ClientConfig::new(Arc::new(quic_config)))
}

// ---------------------------------------------------------------------------
// QuicBiStream: AsyncRead + AsyncWrite wrapper for QUIC streams
// ---------------------------------------------------------------------------

pub struct QuicBiStream {
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
