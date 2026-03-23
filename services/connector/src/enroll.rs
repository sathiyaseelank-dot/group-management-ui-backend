use anyhow::{bail, Result};
use p256::{pkcs8::EncodePublicKey, SecretKey};
use pem::Pem;
use tracing::info;
use zeroize::Zeroizing;

use crate::config::EnrollConfig;
use crate::tls::spiffe::extract_spiffe_id;

// Include the generated protobuf code
pub mod pb {
    tonic::include_proto!("controller.v1");
}

#[allow(dead_code)]
pub struct EnrollResult {
    pub cert_der: Vec<u8>,
    pub cert_pem: Vec<u8>,
    pub ca_pem: Vec<u8>,
    pub key_der: Zeroizing<Vec<u8>>,
    pub spiffe_id: String,
}

/// Perform enrollment: generate key pair, call EnrollConnector RPC, return certs.
pub async fn enroll(cfg: &EnrollConfig) -> Result<EnrollResult> {
    let (key_der, pub_pem) = generate_key_pair()?;

    let channel = build_enroll_channel(cfg).await?;
    let mut client = pb::enrollment_service_client::EnrollmentServiceClient::new(channel);

    let resp = client
        .enroll_connector(pb::EnrollRequest {
            id: cfg.connector_id.clone(),
            public_key: pub_pem,
            token: cfg.token.clone(),
            private_ip: cfg.private_ip.clone(),
            version: cfg.version.clone(),
        })
        .await
        .map_err(|e| anyhow::anyhow!("enrollment RPC failed: {}", e))?
        .into_inner();

    if resp.certificate.is_empty() {
        bail!("controller returned empty certificate");
    }
    if resp.ca_certificate.is_empty() {
        bail!("controller returned empty CA certificate");
    }

    let cert_der = pem_cert_to_der(&resp.certificate)?;
    let spiffe_id = extract_spiffe_id(&cert_der)?;

    info!("enrolled connector as {}", spiffe_id);

    Ok(EnrollResult {
        cert_der,
        cert_pem: resp.certificate,
        ca_pem: resp.ca_certificate,
        key_der,
        spiffe_id,
    })
}

/// Perform renewal: call Renew RPC using existing cert for mTLS.
pub async fn renew(
    controller_addr: &str,
    connector_id: &str,
    trust_domain: &str,
    store: &crate::tls::cert_store::CertStore,
    controller_ca_pem: &[u8],
    workload_ca_pem: &[u8],
) -> Result<EnrollResult> {
    let (key_der, pub_pem) = generate_key_pair()?;

    let channel = crate::tls::client_cfg::build_tonic_channel(
        controller_addr,
        trust_domain,
        store,
        controller_ca_pem,
    )
    .await?;

    let mut client = pb::enrollment_service_client::EnrollmentServiceClient::new(channel);

    let resp = client
        .renew(pb::EnrollRequest {
            id: connector_id.to_string(),
            public_key: pub_pem,
            token: String::new(),
            private_ip: String::new(),
            version: String::new(),
        })
        .await
        .map_err(|e| anyhow::anyhow!("renewal RPC failed: {}", e))?
        .into_inner();

    if resp.ca_certificate.is_empty() {
        bail!("empty CA certificate in renewal response");
    }
    if !ca_pem_equal(workload_ca_pem, &resp.ca_certificate) {
        bail!("internal CA mismatch during renewal");
    }

    let cert_der = pem_cert_to_der(&resp.certificate)?;
    let spiffe_id = extract_spiffe_id(&cert_der)?;

    Ok(EnrollResult {
        cert_der,
        cert_pem: resp.certificate,
        ca_pem: resp.ca_certificate,
        key_der,
        spiffe_id,
    })
}

fn generate_key_pair() -> Result<(Zeroizing<Vec<u8>>, Vec<u8>)> {
    let mut rng = rand::thread_rng();
    let secret = SecretKey::random(&mut rng);
    let public = secret.public_key();

    // Encode public key as PKIX DER, then PEM
    let pub_der = public
        .to_public_key_der()
        .map_err(|e| anyhow::anyhow!("failed to encode public key: {}", e))?;

    let pub_pem = pem::encode(&Pem::new("PUBLIC KEY", pub_der.as_bytes().to_vec())).into_bytes();

    // Encode private key as PKCS#8 DER
    use p256::pkcs8::EncodePrivateKey;
    let key_der = Zeroizing::new(
        secret
            .to_pkcs8_der()
            .map_err(|e| anyhow::anyhow!("failed to encode private key: {}", e))?
            .as_bytes()
            .to_vec(),
    );

    Ok((key_der, pub_pem))
}

async fn build_enroll_channel(cfg: &EnrollConfig) -> Result<tonic::transport::Channel> {
    // For enrollment we have no client cert yet -- use a one-shot channel with
    // a custom ServerCertVerifier that only checks the controller SPIFFE ID.
    use rustls::{
        client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier},
        pki_types::{CertificateDer, ServerName, UnixTime},
        DigitallySignedStruct, SignatureScheme,
    };
    use std::sync::Arc;
    use tokio::net::TcpStream;
    use tonic::transport::Endpoint;

    let ca_der = {
        let pem_str = std::str::from_utf8(&cfg.ca_pem)?;
        let mut der = None;
        for p in pem::parse_many(pem_str)? {
            if p.tag() == "CERTIFICATE" {
                der = Some(p.into_contents());
                break;
            }
        }
        der.ok_or_else(|| anyhow::anyhow!("no CERTIFICATE in CA PEM"))?
    };

    #[derive(Debug)]
    struct EnrollVerifier {
        ca_der: Vec<u8>,
        trust_domain: String,
    }

    impl ServerCertVerifier for EnrollVerifier {
        fn verify_server_cert(
            &self,
            end_entity: &CertificateDer<'_>,
            intermediates: &[CertificateDer<'_>],
            _server_name: &ServerName<'_>,
            _ocsp: &[u8],
            _now: UnixTime,
        ) -> Result<ServerCertVerified, rustls::Error> {
            crate::tls::client_cfg::verify_chain(end_entity, intermediates, &self.ca_der, true)?;

            crate::tls::spiffe::extract_spiffe_id(end_entity.as_ref())
                .and_then(|uri| {
                    crate::tls::spiffe::verify_spiffe_uri(&uri, &self.trust_domain, "controller")
                })
                .map_err(|e| rustls::Error::General(format!("{}", e)))?;

            Ok(ServerCertVerified::assertion())
        }

        fn verify_tls12_signature(
            &self,
            msg: &[u8],
            cert: &CertificateDer<'_>,
            dss: &DigitallySignedStruct,
        ) -> Result<HandshakeSignatureValid, rustls::Error> {
            rustls::crypto::verify_tls12_signature(
                msg,
                cert,
                dss,
                &rustls::crypto::ring::default_provider().signature_verification_algorithms,
            )
        }
        fn verify_tls13_signature(
            &self,
            msg: &[u8],
            cert: &CertificateDer<'_>,
            dss: &DigitallySignedStruct,
        ) -> Result<HandshakeSignatureValid, rustls::Error> {
            rustls::crypto::verify_tls13_signature(
                msg,
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

    let verifier = Arc::new(EnrollVerifier {
        ca_der,
        trust_domain: cfg.trust_domain.clone(),
    });

    let mut client_config = rustls::ClientConfig::builder()
        .dangerous()
        .with_custom_certificate_verifier(verifier)
        .with_no_client_auth();
    client_config.alpn_protocols = vec![b"h2".to_vec()];
    let client_config = Arc::new(client_config);

    let tls_connector = tokio_rustls::TlsConnector::from(client_config);
    let addr = cfg.controller_addr.clone();

    let connector = tower::service_fn(move |_uri: http::Uri| {
        let tls = tls_connector.clone();
        let addr = addr.clone();
        async move {
            let tcp = TcpStream::connect(&addr).await?;
            let domain = ServerName::try_from("controller").map_err(|e| {
                std::io::Error::new(std::io::ErrorKind::InvalidInput, format!("{}", e))
            })?;
            let tls_stream = tls.connect(domain, tcp).await?;
            Ok::<_, std::io::Error>(hyper_util::rt::TokioIo::new(tls_stream))
        }
    });

    let url = format!("http://{}", cfg.controller_addr);
    let channel = Endpoint::from_shared(url)?.connect_with_connector_lazy(connector);

    Ok(channel)
}

pub fn pem_cert_to_der(pem_bytes: &[u8]) -> Result<Vec<u8>> {
    let pem_str = std::str::from_utf8(pem_bytes)?;
    for p in pem::parse_many(pem_str)? {
        if p.tag() == "CERTIFICATE" {
            return Ok(p.into_contents());
        }
    }
    bail!("invalid certificate PEM from controller")
}

fn ca_pem_equal(a: &[u8], b: &[u8]) -> bool {
    let a_der = pem_cert_to_der(a).ok();
    let b_der = pem_cert_to_der(b).ok();
    match (a_der, b_der) {
        (Some(a), Some(b)) => a == b,
        _ => false,
    }
}

/// Parse certificate validity times from DER bytes.
pub fn cert_validity(cert_der: &[u8]) -> Result<(std::time::SystemTime, std::time::SystemTime)> {
    use x509_parser::prelude::*;
    let (_, cert) = X509Certificate::from_der(cert_der)
        .map_err(|e| anyhow::anyhow!("failed to parse cert: {}", e))?;
    let not_before = cert.validity().not_before.to_datetime();
    let not_after = cert.validity().not_after.to_datetime();

    let epoch = std::time::UNIX_EPOCH;
    let nb = epoch + std::time::Duration::from_secs(not_before.unix_timestamp() as u64);
    let na = epoch + std::time::Duration::from_secs(not_after.unix_timestamp() as u64);
    Ok((nb, na))
}
