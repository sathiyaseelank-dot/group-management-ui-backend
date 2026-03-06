/// Build a rustls ServerConfig for the connector's :9443 gRPC server.
/// Requires tunneler mTLS with SPIFFE verification.
use anyhow::Result;
use rustls::{
    client::danger::HandshakeSignatureValid,
    server::danger::{ClientCertVerified, ClientCertVerifier},
    pki_types::{CertificateDer, UnixTime},
    DigitallySignedStruct, DistinguishedName, SignatureScheme,
};

use crate::tls::cert_store::CertStore;

/// Extracts the SPIFFE ID from a verified client cert and checks it is a tunneler.
#[derive(Debug)]
pub struct SpiffeTunnelerVerifier {
    pub trust_domain: String,
    pub ca_cert_der: Vec<u8>,
}

impl ClientCertVerifier for SpiffeTunnelerVerifier {
    fn root_hint_subjects(&self) -> &[DistinguishedName] {
        &[]
    }

    fn verify_client_cert(
        &self,
        end_entity: &CertificateDer<'_>,
        intermediates: &[CertificateDer<'_>],
        _now: UnixTime,
    ) -> Result<ClientCertVerified, rustls::Error> {
        crate::tls::client_cfg::verify_chain(end_entity, intermediates, &self.ca_cert_der, false)?;

        // SPIFFE check
        crate::tls::spiffe::extract_spiffe_id(end_entity.as_ref())
            .and_then(|uri| {
                crate::tls::spiffe::verify_spiffe_uri(&uri, &self.trust_domain, "tunneler")
            })
            .map_err(|e| rustls::Error::General(format!("SPIFFE verify: {}", e)))?;

        Ok(ClientCertVerified::assertion())
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

/// A `ResolvesServerCert` that reads the latest cert from the `CertStore`
/// on every TLS handshake, so renewed certs are picked up immediately.
struct DynamicCertResolver {
    store: CertStore,
}

impl std::fmt::Debug for DynamicCertResolver {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("DynamicCertResolver").finish()
    }
}

impl rustls::server::ResolvesServerCert for DynamicCertResolver {
    fn resolve(
        &self,
        _client_hello: rustls::server::ClientHello<'_>,
    ) -> Option<std::sync::Arc<rustls::sign::CertifiedKey>> {
        let (cert_der, key_der) = self.store.snapshot();
        let key = rustls::pki_types::PrivateKeyDer::try_from(key_der).ok()?;
        let signing_key = rustls::crypto::ring::sign::any_supported_type(&key).ok()?;
        Some(std::sync::Arc::new(rustls::sign::CertifiedKey::new(
            vec![CertificateDer::from(cert_der)],
            signing_key,
        )))
    }
}

/// Build the server-side TLS config for the connector's gRPC listener.
pub fn build_server_tls(
    store: &CertStore,
    ca_pem: &[u8],
    trust_domain: &str,
) -> Result<rustls::ServerConfig> {
    let ca_der = pem_to_der(ca_pem)?;

    let verifier = std::sync::Arc::new(SpiffeTunnelerVerifier {
        trust_domain: trust_domain.to_string(),
        ca_cert_der: ca_der,
    });

    let resolver = std::sync::Arc::new(DynamicCertResolver {
        store: store.clone(),
    });

    let mut config = rustls::ServerConfig::builder()
        .with_client_cert_verifier(verifier)
        .with_cert_resolver(resolver);
    config.alpn_protocols = vec![b"h2".to_vec()];

    Ok(config)
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
