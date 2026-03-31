use anyhow::Result;
use rustls::{
    client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier},
    pki_types::{CertificateDer, ServerName, UnixTime},
    DigitallySignedStruct, SignatureScheme,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio_rustls::TlsConnector;

/// Shared, pre-built TLS config that avoids rebuilding the rustls ClientConfig
/// (PEM parse → root store → verifier) on every tunnel connection.
#[derive(Clone)]
pub struct SharedTlsConfig {
    inner: Arc<rustls::ClientConfig>,
}

impl SharedTlsConfig {
    /// Build once from CA PEM bytes; reuse across all tunnel connections.
    pub fn new(ca_pem: &[u8]) -> Result<Self> {
        Ok(Self {
            inner: build_client_tls(ca_pem)?,
        })
    }

    fn connector(&self) -> TlsConnector {
        TlsConnector::from(self.inner.clone())
    }
}

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
    /// QUIC endpoint address advertised by the connector (Option C discovery).
    quic_addr: Option<String>,
}

/// Result of opening a TLS tunnel, including any QUIC discovery info.
pub struct TunnelResult {
    pub stream: tokio_rustls::client::TlsStream<tokio::net::TcpStream>,
    /// If set, the connector supports QUIC at this address.
    pub quic_addr: Option<String>,
}

#[derive(Debug)]
struct InternalCaVerifier {
    inner: Arc<dyn ServerCertVerifier>,
}

impl ServerCertVerifier for InternalCaVerifier {
    fn verify_server_cert(
        &self,
        end_entity: &CertificateDer<'_>,
        intermediates: &[CertificateDer<'_>],
        server_name: &ServerName<'_>,
        ocsp_response: &[u8],
        now: UnixTime,
    ) -> Result<ServerCertVerified, rustls::Error> {
        match self.inner.verify_server_cert(
            end_entity,
            intermediates,
            server_name,
            ocsp_response,
            now,
        ) {
            Err(rustls::Error::InvalidCertificate(rustls::CertificateError::NotValidForName)) => {
                Ok(ServerCertVerified::assertion())
            }
            other => other,
        }
    }

    fn verify_tls12_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, rustls::Error> {
        self.inner.verify_tls12_signature(message, cert, dss)
    }

    fn verify_tls13_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, rustls::Error> {
        self.inner.verify_tls13_signature(message, cert, dss)
    }

    fn supported_verify_schemes(&self) -> Vec<SignatureScheme> {
        self.inner.supported_verify_schemes()
    }
}

fn pem_to_der(pem_bytes: &[u8]) -> Result<Vec<u8>> {
    let pem_str = std::str::from_utf8(pem_bytes)?;
    for entry in pem::parse_many(pem_str)? {
        if entry.tag() == "CERTIFICATE" {
            return Ok(entry.into_contents());
        }
    }
    anyhow::bail!("no CERTIFICATE block found in PEM")
}

fn build_client_tls(ca_pem: &[u8]) -> Result<Arc<rustls::ClientConfig>> {
    Ok(Arc::new(build_client_tls_inner(ca_pem)?))
}

/// Build a rustls ClientConfig for QUIC (needs owned config, not Arc).
pub fn build_client_tls_for_quic(ca_pem: &[u8]) -> Result<rustls::ClientConfig> {
    build_client_tls_inner(ca_pem)
}

fn build_client_tls_inner(ca_pem: &[u8]) -> Result<rustls::ClientConfig> {
    let ca_der = pem_to_der(ca_pem)?;
    let mut root_store = rustls::RootCertStore::empty();
    root_store
        .add(CertificateDer::from(ca_der))
        .map_err(|e| anyhow::anyhow!("invalid CA cert: {}", e))?;

    let inner: Arc<dyn ServerCertVerifier> =
        rustls::client::WebPkiServerVerifier::builder(Arc::new(root_store))
            .build()
            .map_err(|e| anyhow::anyhow!("verifier build failed: {}", e))?;

    let config = rustls::ClientConfig::builder()
        .dangerous()
        .with_custom_certificate_verifier(Arc::new(InternalCaVerifier { inner }))
        .with_no_client_auth();

    Ok(config)
}

/// Open a TLS tunnel reusing a pre-built TLS config (fast path — no PEM parsing).
pub async fn open_with_config(
    tls_config: &SharedTlsConfig,
    tunnel_addr: &str,
    token: &str,
    destination: &str,
    port: u16,
    protocol: &str,
) -> Result<TunnelResult> {
    let connector = tls_config.connector();

    let tcp = tokio::net::TcpStream::connect(tunnel_addr)
        .await
        .map_err(|e| anyhow::anyhow!("connect to {}: {}", tunnel_addr, e))?;

    tcp.set_nodelay(true).ok(); // reduce Nagle latency

    let host = tunnel_addr
        .rsplit_once(':')
        .map(|(host, _)| host)
        .unwrap_or(tunnel_addr)
        .trim_matches(['[', ']']);
    let server_name =
        ServerName::try_from(host.to_string()).map_err(|e| anyhow::anyhow!("SNI: {}", e))?;

    let mut stream = connector
        .connect(server_name, tcp)
        .await
        .map_err(|e| anyhow::anyhow!("TLS handshake with {}: {}", tunnel_addr, e))?;

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
            "tunnel rejected: {}",
            resp.error.unwrap_or_else(|| "denied".into())
        );
    }

    Ok(TunnelResult {
        stream,
        quic_addr: resp.quic_addr,
    })
}

/// Probe a connector via TLS to discover its QUIC address without opening
/// a full tunnel. Returns the `quic_addr` (if any) even if the tunnel
/// handshake is rejected by the connector.
pub async fn probe_quic_addr(
    tls_config: &SharedTlsConfig,
    tunnel_addr: &str,
    token: &str,
) -> Result<Option<String>> {
    let connector = tls_config.connector();

    let tcp = tokio::net::TcpStream::connect(tunnel_addr)
        .await
        .map_err(|e| anyhow::anyhow!("connect to {}: {}", tunnel_addr, e))?;
    tcp.set_nodelay(true).ok();

    let host = tunnel_addr
        .rsplit_once(':')
        .map(|(host, _)| host)
        .unwrap_or(tunnel_addr)
        .trim_matches(['[', ']']);
    let server_name =
        ServerName::try_from(host.to_string()).map_err(|e| anyhow::anyhow!("SNI: {}", e))?;

    let mut stream = connector
        .connect(server_name, tcp)
        .await
        .map_err(|e| anyhow::anyhow!("TLS handshake with {}: {}", tunnel_addr, e))?;

    // Send a dummy request — connector may reject it, but will still
    // include quic_addr in the response.
    let req = TunnelRequest {
        token,
        destination: "0.0.0.0",
        port: 0,
        protocol: "tcp",
    };
    let mut line = serde_json::to_string(&req)?;
    line.push('\n');
    stream.write_all(line.as_bytes()).await?;

    let resp = read_response_line(&mut stream).await?;
    // We don't care about ok/error — only the quic_addr.
    Ok(resp.quic_addr)
}

/// Open a TLS tunnel (legacy path — rebuilds TLS config from PEM each call).
pub async fn open(
    tunnel_addr: &str,
    ca_pem: &[u8],
    token: &str,
    destination: &str,
    port: u16,
    protocol: &str,
) -> Result<TunnelResult> {
    let cfg = SharedTlsConfig::new(ca_pem)?;
    open_with_config(&cfg, tunnel_addr, token, destination, port, protocol).await
}

async fn read_response_line(
    stream: &mut tokio_rustls::client::TlsStream<tokio::net::TcpStream>,
) -> Result<TunnelResponse> {
    let mut buf = Vec::with_capacity(256);
    let mut byte = [0u8; 1];
    loop {
        let n = stream.read(&mut byte).await?;
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
