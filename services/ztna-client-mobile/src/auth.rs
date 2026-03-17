use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use rand::RngCore;
use reqwest::Client;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

use crate::ZtnaError;

/// Generate a PKCE code verifier (32 random bytes → URL-safe base64, no padding).
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

#[derive(Debug, Deserialize)]
pub struct AuthorizeResponse {
    pub auth_url: String,
    pub state: String,
}

#[derive(Debug, Deserialize)]
pub struct TokenResponse {
    pub access_token: String,
    pub refresh_token: String,
    pub expires_in: i64,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
pub struct RefreshResponse {
    pub access_token: String,
    pub refresh_token: String,
    pub expires_in: i64,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
pub struct DeviceUserView {
    pub user: DeviceUser,
    pub workspace: DeviceWorkspace,
    pub device: DeviceSummary,
    pub session: DeviceSession,
    #[serde(default)]
    pub resources: Vec<DeviceResource>,
    pub synced_at: i64,
}

#[derive(Debug, Deserialize)]
pub struct DeviceUser {
    pub id: String,
    pub email: String,
    pub role: String,
}

#[derive(Debug, Deserialize)]
pub struct DeviceWorkspace {
    pub id: String,
    pub name: String,
    pub slug: String,
    pub trust_domain: String,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
pub struct DeviceSummary {
    pub id: String,
    pub certificate_issued: bool,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
pub struct DeviceSession {
    pub id: String,
    pub expires_at: i64,
    #[serde(default)]
    pub access_token_expires_at_hint: i64,
}

#[derive(Debug, Deserialize)]
pub struct DeviceResource {
    pub id: String,
    pub name: String,
    #[serde(rename = "type")]
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
}

#[derive(Debug, Deserialize)]
pub struct EnrollCertResponse {
    pub device_id: String,
    pub spiffe_id: String,
    pub certificate_pem: String,
    pub ca_cert_pem: String,
    pub expires_at: i64,
    pub access_token: String,
}

#[derive(Debug, Serialize)]
struct AuthorizeRequest<'a> {
    tenant_slug: &'a str,
    code_challenge: &'a str,
    code_challenge_method: &'a str,
    redirect_uri: &'a str,
}

#[derive(Debug, Serialize)]
struct AuthStartRequest<'a> {
    workspace_name: &'a str,
    code_challenge: &'a str,
    platform: &'a str,
}

#[derive(Debug, Deserialize)]
pub struct AuthStartResponse {
    pub auth_url: String,
}

#[derive(Debug, Serialize)]
struct TokenRequest<'a> {
    code: &'a str,
    code_verifier: &'a str,
    state: &'a str,
}

#[derive(Debug, Serialize)]
struct AuthCompleteRequest<'a> {
    session_code: &'a str,
    code_verifier: &'a str,
}

#[derive(Debug, Serialize)]
struct EnrollCertRequest<'a> {
    device_id: &'a str,
    public_key_pem: &'a str,
    hostname: &'a str,
    os: &'a str,
    client_version: &'a str,
}

pub async fn start_device_auth(
    controller_url: &str,
    tenant_slug: &str,
    code_challenge: &str,
    redirect_uri: &str,
) -> Result<AuthorizeResponse, ZtnaError> {
    let client = Client::new();
    let resp = client
        .post(format!("{}/api/device/authorize", controller_url))
        .json(&AuthorizeRequest {
            tenant_slug,
            code_challenge,
            code_challenge_method: "S256",
            redirect_uri,
        })
        .send()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(ZtnaError::Auth(format!("authorize failed: {}", text)));
    }
    resp.json::<AuthorizeResponse>()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))
}

pub async fn exchange_device_code(
    controller_url: &str,
    code: &str,
    code_verifier: &str,
    state: &str,
) -> Result<TokenResponse, ZtnaError> {
    let client = Client::new();
    let resp = client
        .post(format!("{}/api/device/token", controller_url))
        .json(&TokenRequest {
            code,
            code_verifier,
            state,
        })
        .send()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(ZtnaError::Auth(format!("token exchange failed: {}", text)));
    }
    resp.json::<TokenResponse>()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))
}

pub async fn refresh_device_token(
    controller_url: &str,
    refresh_token: &str,
) -> Result<RefreshResponse, ZtnaError> {
    let client = Client::new();
    let resp = client
        .post(format!("{}/api/device/refresh", controller_url))
        .json(&serde_json::json!({ "refresh_token": refresh_token }))
        .send()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(ZtnaError::Auth(format!("refresh failed: {}", text)));
    }
    resp.json::<RefreshResponse>()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))
}

pub async fn revoke_device_token(
    controller_url: &str,
    refresh_token: &str,
) -> Result<(), ZtnaError> {
    let client = Client::new();
    let resp = client
        .post(format!("{}/api/device/revoke", controller_url))
        .json(&serde_json::json!({ "refresh_token": refresh_token }))
        .send()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(ZtnaError::Auth(format!("revoke failed: {}", text)));
    }
    Ok(())
}

pub async fn fetch_device_view(
    controller_url: &str,
    access_token: &str,
) -> Result<DeviceUserView, ZtnaError> {
    let client = Client::new();
    let resp = client
        .get(format!("{}/api/device/me", controller_url))
        .bearer_auth(access_token)
        .send()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(ZtnaError::Auth(format!("fetch device view failed: {}", text)));
    }
    resp.json::<DeviceUserView>()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))
}

pub async fn sync_device_view(
    controller_url: &str,
    access_token: &str,
) -> Result<DeviceUserView, ZtnaError> {
    let client = Client::new();
    let resp = client
        .post(format!("{}/api/device/sync", controller_url))
        .bearer_auth(access_token)
        .send()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(ZtnaError::Auth(format!("sync failed: {}", text)));
    }
    resp.json::<DeviceUserView>()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))
}

/// v2: Start device auth — POST /api/device/auth/start
/// No redirect_uri: server hardcodes it based on platform.
/// Returns auth_url only (state is not needed by client).
pub async fn start_device_auth_v2(
    controller_url: &str,
    workspace_name: &str,
    code_challenge: &str,
) -> Result<AuthStartResponse, ZtnaError> {
    let client = Client::new();
    let resp = client
        .post(format!("{}/api/device/auth/start", controller_url))
        .json(&AuthStartRequest {
            workspace_name,
            code_challenge,
            platform: "mobile",
        })
        .send()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(ZtnaError::Auth(format!("auth start failed: {}", text)));
    }
    resp.json::<AuthStartResponse>()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))
}

/// v2: Complete device auth — POST /api/device/auth/complete
/// Uses session_code from deep link instead of code+state.
pub async fn complete_device_auth_v2(
    controller_url: &str,
    session_code: &str,
    code_verifier: &str,
) -> Result<TokenResponse, ZtnaError> {
    let client = Client::new();
    let resp = client
        .post(format!("{}/api/device/auth/complete", controller_url))
        .json(&AuthCompleteRequest {
            session_code,
            code_verifier,
        })
        .send()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(ZtnaError::Auth(format!("auth complete failed: {}", text)));
    }
    resp.json::<TokenResponse>()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))
}

pub async fn enroll_device_cert(
    controller_url: &str,
    access_token: &str,
    device_id: &str,
    public_key_pem: &str,
    hostname: &str,
    client_version: &str,
) -> Result<EnrollCertResponse, ZtnaError> {
    let client = Client::new();
    let resp = client
        .post(format!("{}/api/device/enroll-cert", controller_url))
        .bearer_auth(access_token)
        .json(&EnrollCertRequest {
            device_id,
            public_key_pem,
            hostname,
            os: "android",
            client_version,
        })
        .send()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(ZtnaError::Auth(format!("enroll cert failed: {}", text)));
    }
    resp.json::<EnrollCertResponse>()
        .await
        .map_err(|e| ZtnaError::Network(e.to_string()))
}
