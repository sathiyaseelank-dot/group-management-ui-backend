use anyhow::Result;
use rustls::{
    client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier},
    pki_types::{CertificateDer, ServerName, UnixTime},
    ClientConfig, DigitallySignedStruct, SignatureScheme,
};
use std::sync::Arc;
use tokio::net::TcpStream;
use tonic::transport::{Channel, Endpoint};

use crate::tls::cert_store::CertStore;

#[derive(Debug)]
struct SpiffeVerifier {
    ca_cert_der: Vec<u8>,
    trust_domain: String,
    expected_role: String,
}

impl ServerCertVerifier for SpiffeVerifier {
    fn verify_server_cert(
        &self,
        end_entity: &CertificateDer<'_>,
        intermediates: &[CertificateDer<'_>],
        _server_name: &ServerName<'_>,
        _ocsp_response: &[u8],
        _now: UnixTime,
    ) -> Result<ServerCertVerified, rustls::Error> {
        verify_chain(end_entity, intermediates, &self.ca_cert_der, true)?;

        crate::tls::spiffe::extract_spiffe_id(end_entity.as_ref())
            .and_then(|uri| {
                crate::tls::spiffe::verify_spiffe_uri(&uri, &self.trust_domain, &self.expected_role)
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
/// peer's SPIFFE ID using the CA in `ca_pem`.
///
/// The tunneler connects to both the controller (for enrollment/renewal) and
/// the connector (for the control plane stream). The `expected_role` defaults
/// to "controller" here; for connector connections the caller passes the
/// connector address and the verifier checks the "connector" role via the
/// overload below.
pub async fn build_tonic_channel(
    addr: &str,
    trust_domain: &str,
    store: &CertStore,
    ca_pem: &[u8],
) -> Result<Channel> {
    // Detect expected role from usage context: if the address is the controller
    // we expect "controller", but build_tonic_channel_with_role is more explicit.
    build_tonic_channel_with_role(addr, trust_domain, store, ca_pem, "controller").await
}

pub async fn build_tonic_channel_with_role(
    addr: &str,
    trust_domain: &str,
    store: &CertStore,
    ca_pem: &[u8],
    expected_role: &str,
) -> Result<Channel> {
    let (cert_der, key_der) = store.snapshot();
    let ca_der = pem_to_der(ca_pem)?;

    let verifier = Arc::new(SpiffeVerifier {
        ca_cert_der: ca_der,
        trust_domain: trust_domain.to_string(),
        expected_role: expected_role.to_string(),
    });

    let mut client_config = ClientConfig::builder()
        .dangerous()
        .with_custom_certificate_verifier(verifier)
        .with_client_auth_cert(
            vec![CertificateDer::from(cert_der)],
            rustls::pki_types::PrivateKeyDer::try_from(key_der)
                .map_err(|e| anyhow::anyhow!("invalid private key: {}", e))?,
        )?;
    client_config.alpn_protocols = vec![b"h2".to_vec()];
    let client_config = Arc::new(client_config);

    let tls_connector = tokio_rustls::TlsConnector::from(client_config);
    let addr_owned = addr.to_string();
    let sni = expected_role.to_string();

    let connector = tower::service_fn(move |_uri: http::Uri| {
        let tls = tls_connector.clone();
        let addr = addr_owned.clone();
        let sni = sni.clone();
        async move {
            let tcp = TcpStream::connect(&addr).await?;
            let domain = ServerName::try_from(sni.as_str().to_owned()).map_err(|e| {
                std::io::Error::new(std::io::ErrorKind::InvalidInput, format!("{}", e))
            })?;
            let tls_stream = tls.connect(domain, tcp).await?;
            Ok::<_, std::io::Error>(hyper_util::rt::TokioIo::new(tls_stream))
        }
    });

    let url = format!("http://{}", addr);
    let channel = Endpoint::from_shared(url)?.connect_with_connector_lazy(connector);

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
