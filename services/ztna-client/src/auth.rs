use anyhow::{anyhow, Result};
use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use rand::RngCore;
use sha2::{Digest, Sha256};

pub mod pb {
    tonic::include_proto!("controller.v1");
}

/// Generate a PKCE code verifier (43 random URL-safe chars).
pub fn generate_code_verifier() -> String {
    let mut buf = [0u8; 32];
    rand::thread_rng().fill_bytes(&mut buf);
    URL_SAFE_NO_PAD.encode(buf)
}

/// Compute PKCE code challenge: BASE64URL(SHA256(verifier)).
pub fn compute_code_challenge(verifier: &str) -> String {
    let hash = Sha256::digest(verifier.as_bytes());
    URL_SAFE_NO_PAD.encode(hash)
}

#[derive(Debug, Clone)]
pub struct AuthorizeResponse {
    pub auth_url: String,
    pub state: String,
}

#[derive(Debug, Clone)]
pub struct TokenResponse {
    pub access_token: String,
    pub refresh_token: String,
    pub expires_in: i64,
}

#[allow(dead_code)]
#[derive(Debug, Clone)]
pub struct RefreshResponse {
    pub access_token: String,
    pub refresh_token: String,
    pub expires_in: i64,
}

#[allow(dead_code)]
#[derive(Debug, Clone)]
pub struct DeviceUserView {
    pub user: DeviceUser,
    pub workspace: DeviceWorkspace,
    pub device: DeviceSummary,
    pub session: DeviceSession,
    pub resources: Vec<DeviceResource>,
    pub synced_at: i64,
}

#[derive(Debug, Clone)]
pub struct DeviceUser {
    pub id: String,
    pub email: String,
    pub role: String,
}

#[derive(Debug, Clone)]
pub struct DeviceWorkspace {
    pub id: String,
    pub name: String,
    pub slug: String,
    pub trust_domain: String,
}

#[allow(dead_code)]
#[derive(Debug, Clone)]
pub struct DeviceSummary {
    pub id: String,
    pub certificate_issued: bool,
}

#[allow(dead_code)]
#[derive(Debug, Clone)]
pub struct DeviceSession {
    pub id: String,
    pub expires_at: i64,
    pub access_token_expires_at_hint: i64,
}

#[derive(Debug, Clone)]
pub struct DeviceResource {
    pub id: String,
    pub name: String,
    pub r#type: String,
    pub address: String,
    pub protocol: String,
    pub port_from: Option<i32>,
    pub port_to: Option<i32>,
    pub alias: Option<String>,
    pub description: String,
    pub remote_network_id: String,
    pub remote_network_name: String,
    pub firewall_status: String,
    pub connector_tunnel_addr: String,
}

#[derive(Debug, Clone)]
pub struct EnrollCertResponse {
    pub device_id: String,
    pub spiffe_id: String,
    pub certificate_pem: String,
    pub ca_cert_pem: String,
    pub expires_at: i64,
    pub access_token: String,
}

/// Decode a DER length prefix at `bytes[0..]`.
/// Returns `(value, header_octets_consumed)`.
fn der_length(bytes: &[u8]) -> (usize, usize) {
    if bytes.is_empty() {
        return (0, 0);
    }
    if bytes[0] & 0x80 == 0 {
        return (bytes[0] as usize, 1);
    }
    let n = (bytes[0] & 0x7f) as usize;
    if n == 0 || n > 4 || n + 1 > bytes.len() {
        return (0, 1);
    }
    let mut len = 0usize;
    for k in 0..n {
        len = (len << 8) | bytes[1 + k] as usize;
    }
    (len, 1 + n)
}

/// Scan raw DER certificate bytes for a URI-type Subject Alternative Name
/// (ASN.1 context tag `[6]` = `0x86`) whose value starts with `spiffe://`
/// and contains the path segment `/controller/`.
///
/// This is intentionally a byte-level scan rather than a full ASN.1 parse so
/// that it can be used without additional dependencies during TLS bootstrap.
fn has_spiffe_controller_san(cert_der: &[u8]) -> bool {
    const CONTROLLER_SEGMENT: &[u8] = b"/controller/";
    let mut i = 0;
    while i < cert_der.len() {
        if cert_der[i] == 0x86 && i + 1 < cert_der.len() {
            let (data_len, hdr) = der_length(&cert_der[i + 1..]);
            let start = i + 1 + hdr;
            let end = start + data_len;
            if end <= cert_der.len() {
                let val = &cert_der[start..end];
                if val.starts_with(b"spiffe://")
                    && val
                        .windows(CONTROLLER_SEGMENT.len())
                        .any(|w| w == CONTROLLER_SEGMENT)
                {
                    return true;
                }
            }
        }
        i += 1;
    }
    false
}

/// Build a tonic channel to the controller gRPC endpoint using a permissive
/// TLS verifier (accepts any cert from the controller host). This is used for
/// initial unauthenticated calls (Authorize, Token, Refresh, Revoke) and
/// post-enrollment authenticated calls (Me, Sync, Posture, EnrollCert).
///
/// For production use, pass `ca_pem_bytes = Some(...)` to verify against the
/// workspace CA cert once it is available from enrollment.
async fn build_channel(
    controller_addr: &str,
    ca_pem_bytes: Option<&[u8]>,
) -> Result<tonic::transport::Channel> {
    use rustls::{
        client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier},
        pki_types::{CertificateDer, ServerName, UnixTime},
        DigitallySignedStruct, SignatureScheme,
    };
    use std::sync::Arc;
    use tokio::net::TcpStream;
    use tonic::transport::Endpoint;

    // Parse optional CA cert for verification.
    let ca_der_opt: Option<Vec<u8>> = if let Some(pem_bytes) = ca_pem_bytes {
        if let Ok(pem_str) = std::str::from_utf8(pem_bytes) {
            let mut found = None;
            if let Ok(items) = pem::parse_many(pem_str) {
                for p in items {
                    if p.tag() == "CERTIFICATE" {
                        found = Some(p.into_contents());
                        break;
                    }
                }
            }
            found
        } else {
            None
        }
    } else {
        None
    };

    #[derive(Debug)]
    struct ClientVerifier {
        ca_der: Option<Vec<u8>>,
    }

    impl ServerCertVerifier for ClientVerifier {
        fn verify_server_cert(
            &self,
            end_entity: &CertificateDer<'_>,
            _intermediates: &[CertificateDer<'_>],
            _server_name: &ServerName<'_>,
            _ocsp: &[u8],
            _now: UnixTime,
        ) -> std::result::Result<ServerCertVerified, rustls::Error> {
            // If we have a CA cert, verify the chain.
            if let Some(ref ca_der) = self.ca_der {
                use rustls::pki_types::CertificateDer as CDer;
                let ca_cert = CDer::from(ca_der.clone());
                let mut roots = rustls::RootCertStore::empty();
                roots
                    .add(ca_cert)
                    .map_err(|e| rustls::Error::General(format!("add root cert: {}", e)))?;
                let verifier = rustls::client::WebPkiServerVerifier::builder(Arc::new(roots))
                    .build()
                    .map_err(|e| rustls::Error::General(format!("build verifier: {}", e)))?;
                // We verify against the "controller" server name used in our connector.
                let server_name = ServerName::try_from("controller")
                    .map_err(|e| rustls::Error::General(format!("{}", e)))?;
                return verifier.verify_server_cert(
                    end_entity,
                    _intermediates,
                    &server_name,
                    _ocsp,
                    _now,
                );
            }
            // No CA cert available (initial bootstrap) — chain verification is not
            // possible yet, but we still require the server cert to carry a SPIFFE
            // URI SAN of the form  spiffe://<any_trust_domain>/controller/<id>.
            // This prevents an arbitrary MITM from impersonating the controller
            // without also possessing a cert with the right SPIFFE path.
            if !has_spiffe_controller_san(end_entity.as_ref()) {
                return Err(rustls::Error::General(
                    "bootstrap TLS: server cert does not contain a SPIFFE controller URI SAN \
                     (expected spiffe://<trust_domain>/controller/<id>)"
                        .into(),
                ));
            }
            Ok(ServerCertVerified::assertion())
        }

        fn verify_tls12_signature(
            &self,
            msg: &[u8],
            cert: &CertificateDer<'_>,
            dss: &DigitallySignedStruct,
        ) -> std::result::Result<HandshakeSignatureValid, rustls::Error> {
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
        ) -> std::result::Result<HandshakeSignatureValid, rustls::Error> {
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

    let verifier = Arc::new(ClientVerifier { ca_der: ca_der_opt });
    let mut client_config = rustls::ClientConfig::builder()
        .dangerous()
        .with_custom_certificate_verifier(verifier)
        .with_no_client_auth();
    client_config.alpn_protocols = vec![b"h2".to_vec()];
    let client_config = Arc::new(client_config);

    let tls_connector = tokio_rustls::TlsConnector::from(client_config);
    let addr = controller_addr.to_string();

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

    let url = format!("http://{}", controller_addr);
    let channel = Endpoint::from_shared(url)?.connect_with_connector_lazy(connector);
    Ok(channel)
}

/// Add a Bearer token to a gRPC request.
fn with_token<T>(req: T, token: &str) -> tonic::Request<T> {
    let mut request = tonic::Request::new(req);
    let header_value = format!("Bearer {}", token)
        .parse()
        .expect("valid header value");
    request.metadata_mut().insert("authorization", header_value);
    request
}

/// Convert a DeviceViewResponse proto into our local DeviceUserView struct.
fn proto_view_to_local(view: pb::DeviceViewResponse) -> DeviceUserView {
    let user = view.user.unwrap_or_default();
    let workspace = view.workspace.unwrap_or_default();
    let device = view.device.unwrap_or_default();
    let session = view.session.unwrap_or_default();
    let resources = view
        .resources
        .into_iter()
        .map(|r| DeviceResource {
            id: r.id,
            name: r.name,
            r#type: r.r#type,
            address: r.address,
            protocol: r.protocol,
            port_from: if r.has_port_from {
                Some(r.port_from)
            } else {
                None
            },
            port_to: if r.has_port_to { Some(r.port_to) } else { None },
            alias: if r.has_alias { Some(r.alias) } else { None },
            description: r.description,
            remote_network_id: r.remote_network_id,
            remote_network_name: r.remote_network_name,
            firewall_status: r.firewall_status,
            connector_tunnel_addr: r.connector_tunnel_addr,
        })
        .collect();
    DeviceUserView {
        user: DeviceUser {
            id: user.id,
            email: user.email,
            role: user.role,
        },
        workspace: DeviceWorkspace {
            id: workspace.id,
            name: workspace.name,
            slug: workspace.slug,
            trust_domain: workspace.trust_domain,
        },
        device: DeviceSummary {
            id: device.id,
            certificate_issued: device.certificate_issued,
        },
        session: DeviceSession {
            id: session.id,
            expires_at: session.expires_at,
            access_token_expires_at_hint: session.access_token_expires_at_hint,
        },
        resources,
        synced_at: view.synced_at,
    }
}

pub async fn start_device_auth(
    controller_grpc_addr: &str,
    tenant_slug: &str,
    code_challenge: &str,
    redirect_uri: &str,
) -> Result<AuthorizeResponse> {
    let channel = build_channel(controller_grpc_addr, None).await?;
    let mut client = pb::device_service_client::DeviceServiceClient::new(channel);
    let resp = client
        .device_authorize(pb::DeviceAuthorizeRequest {
            tenant_slug: tenant_slug.to_string(),
            code_challenge: code_challenge.to_string(),
            code_challenge_method: "S256".to_string(),
            redirect_uri: redirect_uri.to_string(),
        })
        .await
        .map_err(|e| anyhow!("authorize failed: {}", e))?
        .into_inner();
    Ok(AuthorizeResponse {
        auth_url: resp.auth_url,
        state: resp.state,
    })
}

pub async fn exchange_device_code(
    controller_grpc_addr: &str,
    code: &str,
    code_verifier: &str,
    state: &str,
) -> Result<TokenResponse> {
    let channel = build_channel(controller_grpc_addr, None).await?;
    let mut client = pb::device_service_client::DeviceServiceClient::new(channel);
    let resp = client
        .device_token(pb::DeviceTokenRequest {
            code: code.to_string(),
            code_verifier: code_verifier.to_string(),
            state: state.to_string(),
        })
        .await
        .map_err(|e| anyhow!("token exchange failed: {}", e))?
        .into_inner();
    Ok(TokenResponse {
        access_token: resp.access_token,
        refresh_token: resp.refresh_token,
        expires_in: resp.expires_in,
    })
}

pub async fn refresh_device_token(
    controller_grpc_addr: &str,
    refresh_token: &str,
) -> Result<RefreshResponse> {
    let channel = build_channel(controller_grpc_addr, None).await?;
    let mut client = pb::device_service_client::DeviceServiceClient::new(channel);
    let resp = client
        .device_refresh(pb::DeviceRefreshRequest {
            refresh_token: refresh_token.to_string(),
        })
        .await
        .map_err(|e| anyhow!("refresh failed: {}", e))?
        .into_inner();
    Ok(RefreshResponse {
        access_token: resp.access_token,
        refresh_token: resp.refresh_token,
        expires_in: resp.expires_in,
    })
}

pub async fn revoke_device_token(controller_grpc_addr: &str, refresh_token: &str) -> Result<()> {
    let channel = build_channel(controller_grpc_addr, None).await?;
    let mut client = pb::device_service_client::DeviceServiceClient::new(channel);
    client
        .device_revoke(pb::DeviceRevokeRequest {
            refresh_token: refresh_token.to_string(),
        })
        .await
        .map_err(|e| anyhow!("revoke failed: {}", e))?;
    Ok(())
}

pub async fn fetch_device_view(
    controller_grpc_addr: &str,
    access_token: &str,
) -> Result<DeviceUserView> {
    let channel = build_channel(controller_grpc_addr, None).await?;
    let mut client = pb::device_service_client::DeviceServiceClient::new(channel);
    let resp = client
        .device_me(with_token(pb::DeviceViewRequest {}, access_token))
        .await
        .map_err(|e| anyhow!("fetch device view failed: {}", e))?
        .into_inner();
    Ok(proto_view_to_local(resp))
}

pub async fn sync_device_view(
    controller_grpc_addr: &str,
    access_token: &str,
) -> Result<DeviceUserView> {
    let channel = build_channel(controller_grpc_addr, None).await?;
    let mut client = pb::device_service_client::DeviceServiceClient::new(channel);
    let resp = client
        .device_sync(with_token(pb::DeviceViewRequest {}, access_token))
        .await
        .map_err(|e| anyhow!("sync device view failed: {}", e))?
        .into_inner();
    Ok(proto_view_to_local(resp))
}

pub async fn report_device_posture(
    controller_grpc_addr: &str,
    access_token: &str,
    posture: &crate::posture::DevicePostureReport,
) -> Result<()> {
    let channel = build_channel(controller_grpc_addr, None).await?;
    let mut client = pb::device_service_client::DeviceServiceClient::new(channel);
    let req = pb::DevicePostureRequest {
        device_id: posture.device_id.clone(),
        spiffe_id: posture.spiffe_id.clone(),
        os_type: posture.os_type.clone(),
        os_version: posture.os_version.clone(),
        hostname: posture.hostname.clone(),
        firewall_enabled: posture.firewall_enabled,
        disk_encrypted: posture.disk_encrypted,
        screen_lock_enabled: posture.screen_lock_enabled,
        client_version: posture.client_version.clone(),
        collected_at: posture.collected_at.clone(),
    };
    client
        .device_report_posture(with_token(req, access_token))
        .await
        .map_err(|e| anyhow!("posture report failed: {}", e))?;
    Ok(())
}

pub async fn enroll_device_cert(
    controller_grpc_addr: &str,
    access_token: &str,
    device_id: &str,
    public_key_pem: &str,
    hostname: &str,
    os: &str,
    client_version: &str,
    device_name: &str,
    device_model: &str,
    device_make: &str,
    serial_number: &str,
) -> Result<EnrollCertResponse> {
    let channel = build_channel(controller_grpc_addr, None).await?;
    let mut client = pb::device_service_client::DeviceServiceClient::new(channel);
    let req = pb::DeviceEnrollCertRequest {
        device_id: device_id.to_string(),
        public_key_pem: public_key_pem.to_string(),
        hostname: hostname.to_string(),
        os: os.to_string(),
        client_version: client_version.to_string(),
        device_name: device_name.to_string(),
        device_model: device_model.to_string(),
        device_make: device_make.to_string(),
        serial_number: serial_number.to_string(),
    };
    let resp = client
        .device_enroll_cert(with_token(req, access_token))
        .await
        .map_err(|e| anyhow!("device cert enrollment failed: {}", e))?
        .into_inner();
    Ok(EnrollCertResponse {
        device_id: resp.device_id,
        spiffe_id: resp.spiffe_id,
        certificate_pem: resp.certificate_pem,
        ca_cert_pem: resp.ca_cert_pem,
        expires_at: resp.expires_at,
        access_token: resp.access_token,
    })
}
