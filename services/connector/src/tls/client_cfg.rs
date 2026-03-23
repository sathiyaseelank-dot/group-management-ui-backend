/// Build a tonic Channel with mTLS + SPIFFE controller verification.
/// The connector presents its own cert and verifies the controller's SPIFFE ID.
use anyhow::Result;
use rustls::{
    client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier},
    pki_types::{CertificateDer, ServerName, UnixTime},
    ClientConfig, DigitallySignedStruct, SignatureScheme,
};
use std::sync::Arc;
use tokio::net::TcpStream;
use tonic::transport::{Channel, Endpoint};
use tracing::{info, warn};

use crate::tls::cert_store::CertStore;

/// A `ServerCertVerifier` that checks:
/// 1. The cert chains to our internal CA
/// 2. The SPIFFE URI SAN is `spiffe://<trust_domain>/controller`
#[derive(Debug)]
struct SpiffeControllerVerifier {
    ca_cert_der: Vec<u8>,
    trust_domain: String,
}

impl ServerCertVerifier for SpiffeControllerVerifier {
    fn verify_server_cert(
        &self,
        end_entity: &CertificateDer<'_>,
        intermediates: &[CertificateDer<'_>],
        _server_name: &ServerName<'_>,
        _ocsp_response: &[u8],
        _now: UnixTime,
    ) -> Result<ServerCertVerified, rustls::Error> {
        verify_chain(end_entity, intermediates, &self.ca_cert_der, true)?;

        // Verify SPIFFE ID
        crate::tls::spiffe::extract_spiffe_id(end_entity.as_ref())
            .and_then(|uri| {
                crate::tls::spiffe::verify_spiffe_uri(&uri, &self.trust_domain, "controller")
            })
            .map_err(|e| rustls::Error::General(format!("SPIFFE verify failed: {}", e)))?;

        Ok(ServerCertVerified::assertion())
    }

    fn verify_tls12_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, rustls::Error> {
        rustls::crypto::verify_tls12_signature(
            message,
            cert,
            dss,
            &rustls::crypto::ring::default_provider().signature_verification_algorithms,
        )
    }

    fn verify_tls13_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, rustls::Error> {
        rustls::crypto::verify_tls13_signature(
            message,
            cert,
            dss,
            &rustls::crypto::ring::default_provider().signature_verification_algorithms,
        )
    }

    fn supported_verify_schemes(&self) -> Vec<SignatureScheme> {
        rustls::crypto::ring::default_provider()
            .signature_verification_algorithms
            .supported_schemes()
    }
}

/// Build a tonic `Channel` that presents `store`'s cert and verifies the
/// controller's SPIFFE ID using the CA in `ca_pem`.
pub async fn build_tonic_channel(
    controller_addr: &str,
    trust_domain: &str,
    store: &CertStore,
    ca_pem: &[u8],
) -> Result<Channel> {
    build_tonic_channel_with_policy_key(controller_addr, trust_domain, store, ca_pem, "", None)
        .await
}

const POLICY_KEY_LABEL: &str = "ztna-policy-signing-v1";

fn export_policy_signing_key(
    conn: &rustls::ClientConnection,
    connector_id: &str,
    on_policy_key: Option<&Arc<dyn Fn(Vec<u8>) + Send + Sync>>,
) {
    let Some(cb) = on_policy_key else {
        return;
    };

    if connector_id.trim().is_empty() {
        warn!("policy key derivation skipped: connector_id is empty");
        return;
    }

    let out = vec![0u8; 32];
    match conn.export_keying_material(
        out,
        POLICY_KEY_LABEL.as_bytes(),
        Some(connector_id.as_bytes()),
    ) {
        Ok(key) => {
            info!(
                "derived policy signing key from mTLS (label={}, connector={})",
                POLICY_KEY_LABEL, connector_id
            );
            cb(key);
        }
        Err(e) => warn!("policy key derivation failed: {e}"),
    }
}

pub async fn build_tonic_channel_with_policy_key(
    controller_addr: &str,
    trust_domain: &str,
    store: &CertStore,
    ca_pem: &[u8],
    connector_id: &str,
    on_policy_key: Option<Arc<dyn Fn(Vec<u8>) + Send + Sync>>,
) -> Result<Channel> {
    let (_cert_der, cert_chain_der, key_der) = store.snapshot();

    // Parse CA cert DER from PEM
    let ca_der = pem_to_der(ca_pem)?;

    let verifier = Arc::new(SpiffeControllerVerifier {
        ca_cert_der: ca_der,
        trust_domain: trust_domain.to_string(),
    });

    let mut client_config = ClientConfig::builder()
        .dangerous()
        .with_custom_certificate_verifier(verifier)
        .with_client_auth_cert(
            cert_chain_der
                .into_iter()
                .map(CertificateDer::from)
                .collect(),
            rustls::pki_types::PrivateKeyDer::try_from(key_der)
                .map_err(|e| anyhow::anyhow!("invalid private key: {}", e))?,
        )?;
    client_config.alpn_protocols = vec![b"h2".to_vec()];
    let client_config = Arc::new(client_config);

    let tls_connector = tokio_rustls::TlsConnector::from(client_config);
    let addr = controller_addr.to_string();
    let connector_id = connector_id.to_string();
    let on_policy_key = on_policy_key.clone();

    let connector = tower::service_fn(move |_uri: http::Uri| {
        let tls = tls_connector.clone();
        let addr = addr.clone();
        let connector_id = connector_id.clone();
        let on_policy_key = on_policy_key.clone();
        async move {
            let tcp = TcpStream::connect(&addr).await?;
            let domain = ServerName::try_from("controller").map_err(|e| {
                std::io::Error::new(std::io::ErrorKind::InvalidInput, format!("{}", e))
            })?;
            let tls_stream = tls.connect(domain, tcp).await?;
            export_policy_signing_key(
                tls_stream.get_ref().1,
                &connector_id,
                on_policy_key.as_ref(),
            );
            Ok::<_, std::io::Error>(hyper_util::rt::TokioIo::new(tls_stream))
        }
    });

    let url = format!("http://{}", controller_addr);
    let channel = Endpoint::from_shared(url)?
        .connect_with_connector(connector)
        .await?;

    Ok(channel)
}

fn pem_to_der(pem_bytes: &[u8]) -> Result<Vec<u8>> {
    let pem_str = std::str::from_utf8(pem_bytes)?;
    for pem in ::pem::parse_many(pem_str)? {
        if pem.tag() == "CERTIFICATE" {
            return Ok(pem.into_contents());
        }
    }
    anyhow::bail!("no CERTIFICATE block found in PEM")
}

/// Verify cert chain against our CA using rustls-webpki.
pub(crate) fn verify_chain(
    end_entity: &CertificateDer<'_>,
    intermediates: &[CertificateDer<'_>],
    ca_der: &[u8],
    is_server: bool,
) -> Result<(), rustls::Error> {
    let ca_cert = CertificateDer::from(ca_der);
    let anchor = webpki::anchor_from_trusted_cert(&ca_cert)
        .map_err(|e| rustls::Error::General(format!("CA anchor error: {:?}", e)))?;

    let cert = webpki::EndEntityCert::try_from(end_entity)
        .map_err(|e| rustls::Error::General(format!("cert parse error: {:?}", e)))?;

    let now = UnixTime::now();
    let usage = if is_server {
        webpki::KeyUsage::server_auth()
    } else {
        webpki::KeyUsage::client_auth()
    };

    cert.verify_for_usage(
        webpki::ALL_VERIFICATION_ALGS,
        &[anchor],
        intermediates,
        now,
        usage,
        None,
        None,
    )
    .map_err(|e| rustls::Error::General(format!("chain verify failed: {:?}", e)))?;

    Ok(())
}
