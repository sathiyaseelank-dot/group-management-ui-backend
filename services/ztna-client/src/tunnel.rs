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

#[derive(Serialize)]
struct TunnelRequest<'a> {
    token: &'a str,
    destination: &'a str,
    port: u16,
}

#[derive(Deserialize)]
struct TunnelResponse {
    ok: bool,
    error: Option<String>,
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
            Err(rustls::Error::InvalidCertificate(
                rustls::CertificateError::NotValidForName,
            )) => Ok(ServerCertVerified::assertion()),
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

    Ok(Arc::new(config))
}

pub async fn open(
    tunnel_addr: &str,
    ca_pem: &[u8],
    token: &str,
    destination: &str,
    port: u16,
) -> Result<tokio_rustls::client::TlsStream<tokio::net::TcpStream>> {
    let tls_config = build_client_tls(ca_pem)?;
    let connector = TlsConnector::from(tls_config);

    let tcp = tokio::net::TcpStream::connect(tunnel_addr)
        .await
        .map_err(|e| anyhow::anyhow!("connect to {}: {}", tunnel_addr, e))?;

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

    Ok(stream)
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
