use anyhow::{anyhow, Result};
use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use rand::RngCore;
use reqwest::Client;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

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
    #[serde(default)]
    pub acl: serde_json::Value,
}

#[derive(Debug, Deserialize)]
pub struct RefreshResponse {
    pub access_token: String,
    pub refresh_token: String,
    pub expires_in: i64,
}

#[derive(Debug, Serialize)]
struct AuthorizeRequest<'a> {
    tenant_slug: &'a str,
    code_challenge: &'a str,
    code_challenge_method: &'a str,
    redirect_uri: &'a str,
}

#[derive(Debug, Serialize)]
struct TokenRequest<'a> {
    code: &'a str,
    code_verifier: &'a str,
    state: &'a str,
}

pub async fn start_device_auth(
    controller_url: &str,
    tenant_slug: &str,
    code_challenge: &str,
    redirect_uri: &str,
) -> Result<AuthorizeResponse> {
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
        .await?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(anyhow!("authorize failed: {}", text));
    }
    Ok(resp.json::<AuthorizeResponse>().await?)
}

pub async fn exchange_device_code(
    controller_url: &str,
    code: &str,
    code_verifier: &str,
    state: &str,
) -> Result<TokenResponse> {
    let client = Client::new();
    let resp = client
        .post(format!("{}/api/device/token", controller_url))
        .json(&TokenRequest {
            code,
            code_verifier,
            state,
        })
        .send()
        .await?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(anyhow!("token exchange failed: {}", text));
    }
    Ok(resp.json::<TokenResponse>().await?)
}

pub async fn refresh_device_token(
    controller_url: &str,
    refresh_token: &str,
) -> Result<RefreshResponse> {
    let client = Client::new();
    let resp = client
        .post(format!("{}/api/device/refresh", controller_url))
        .json(&serde_json::json!({ "refresh_token": refresh_token }))
        .send()
        .await?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(anyhow!("refresh failed: {}", text));
    }
    Ok(resp.json::<RefreshResponse>().await?)
}

pub async fn revoke_device_token(
    controller_url: &str,
    refresh_token: &str,
) -> Result<()> {
    let client = Client::new();
    let _ = client
        .post(format!("{}/api/device/revoke", controller_url))
        .json(&serde_json::json!({ "refresh_token": refresh_token }))
        .send()
        .await?;
    Ok(())
}
