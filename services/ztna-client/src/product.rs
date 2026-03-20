//! Service proxy for product installs.
//!
//! When the ztna-client binary is installed as a product (config file at
//! `/etc/ztna-client/client.conf`), the systemd service owns the state
//! directory (`/var/lib/ztna-client`).  A non-root CLI cannot read or
//! write that directory.
//!
//! Instead, the CLI proxies commands through the service's existing HTTP
//! server at `http://127.0.0.1:{port}`.  All responses use sanitized
//! types that never expose private keys, tokens, or other secrets.

use std::time::Duration;

use anyhow::{anyhow, Result};
use reqwest::{header, Client};
use serde::{Deserialize, Serialize};

// ---------------------------------------------------------------------------
// Sanitized response types — shared between service and CLI
// ---------------------------------------------------------------------------

/// Top-level status response from the service.  Contains service-level
/// information and a list of workspace statuses (without secrets).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CliStatusResponse {
    /// Transport mode the service is running in ("tun" or "socks5").
    pub mode: String,
    /// Whether the service has a usable configuration.
    pub configured: bool,
    /// Active workspace sessions (sanitized — no tokens or keys).
    pub workspaces: Vec<CliWorkspaceStatus>,
    /// Semver version string of the running service binary.
    /// `#[serde(default)]` keeps CLI compatible with older service versions
    /// that pre-date this field.
    #[serde(default)]
    pub client_version: String,
}

/// Per-workspace status without secrets.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CliWorkspaceStatus {
    pub workspace_slug: String,
    pub workspace_name: String,
    pub user_email: String,
    pub user_role: String,
    pub device_id: String,
    pub resources: Vec<CliResourceInfo>,
    pub session_expires_at: i64,
    pub last_sync_at: i64,
}

/// Resource info suitable for display — no internal IDs or secrets.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CliResourceInfo {
    pub name: String,
    pub address: String,
    pub protocol: String,
    pub port_from: Option<i32>,
    pub port_to: Option<i32>,
    pub remote_network_name: String,
    pub firewall_status: String,
}

/// Response from POST /login.
#[derive(Debug, Deserialize)]
struct LoginResponse {
    auth_url: String,
}

// ---------------------------------------------------------------------------
// Service reachability
// ---------------------------------------------------------------------------

const SERVICE_CONNECT_TIMEOUT: Duration = Duration::from_secs(2);
const SERVICE_REQUEST_TIMEOUT: Duration = Duration::from_secs(30);

/// Check whether the service is listening on the expected port.
pub async fn is_service_running(port: u16) -> bool {
    tokio::time::timeout(
        SERVICE_CONNECT_TIMEOUT,
        tokio::net::TcpStream::connect(format!("127.0.0.1:{}", port)),
    )
    .await
    .map(|r| r.is_ok())
    .unwrap_or(false)
}

/// Build an HTTP client that injects `X-Service-Token` on every request when
/// a token is provided.
fn authed_client(token: Option<&str>) -> Client {
    let mut builder = Client::builder()
        .connect_timeout(SERVICE_CONNECT_TIMEOUT)
        .timeout(SERVICE_REQUEST_TIMEOUT);

    if let Some(tok) = token {
        if !tok.is_empty() {
            if let Ok(val) = header::HeaderValue::from_str(tok) {
                let mut headers = header::HeaderMap::new();
                headers.insert("X-Service-Token", val);
                builder = builder.default_headers(headers);
            }
        }
    }

    builder.build().unwrap_or_default()
}

// ---------------------------------------------------------------------------
// Proxy functions
// ---------------------------------------------------------------------------

pub async fn proxy_status(base_url: &str, token: Option<&str>) -> Result<CliStatusResponse> {
    let resp = authed_client(token)
        .get(format!("{}/status", base_url))
        .send()
        .await
        .map_err(|e| anyhow!("cannot reach service: {}", e))?;
    if !resp.status().is_success() {
        anyhow::bail!("service returned HTTP {}", resp.status());
    }
    Ok(resp.json().await?)
}

pub async fn proxy_resources(
    base_url: &str,
    tenant: &str,
    token: Option<&str>,
) -> Result<Vec<CliResourceInfo>> {
    let resp = authed_client(token)
        .get(format!("{}/resources", base_url))
        .query(&[("tenant", tenant)])
        .send()
        .await
        .map_err(|e| anyhow!("cannot reach service: {}", e))?;
    if !resp.status().is_success() {
        let status = resp.status();
        let text = resp.text().await.unwrap_or_default();
        anyhow::bail!("service error (HTTP {}): {}", status, text);
    }
    #[derive(Deserialize)]
    struct Resp {
        resources: Vec<CliResourceInfo>,
    }
    let body: Resp = resp.json().await?;
    Ok(body.resources)
}

pub async fn proxy_sync(
    base_url: &str,
    tenant: &str,
    token: Option<&str>,
) -> Result<CliWorkspaceStatus> {
    let resp = authed_client(token)
        .post(format!("{}/sync", base_url))
        .json(&serde_json::json!({ "tenant": tenant }))
        .send()
        .await
        .map_err(|e| anyhow!("cannot reach service: {}", e))?;
    if !resp.status().is_success() {
        let status = resp.status();
        let text = resp.text().await.unwrap_or_default();
        anyhow::bail!("service error (HTTP {}): {}", status, text);
    }
    Ok(resp.json().await?)
}

pub async fn proxy_disconnect(base_url: &str, tenant: &str, token: Option<&str>) -> Result<()> {
    let resp = authed_client(token)
        .post(format!("{}/disconnect", base_url))
        .json(&serde_json::json!({ "tenant": tenant }))
        .send()
        .await
        .map_err(|e| anyhow!("cannot reach service: {}", e))?;
    if !resp.status().is_success() {
        let status = resp.status();
        let text = resp.text().await.unwrap_or_default();
        anyhow::bail!("service error (HTTP {}): {}", status, text);
    }
    Ok(())
}

pub async fn proxy_login(base_url: &str, tenant: &str, token: Option<&str>) -> Result<String> {
    let resp = authed_client(token)
        .post(format!("{}/login", base_url))
        .json(&serde_json::json!({ "tenant": tenant }))
        .send()
        .await
        .map_err(|e| anyhow!("cannot reach service: {}", e))?;
    if !resp.status().is_success() {
        let status = resp.status();
        let text = resp.text().await.unwrap_or_default();
        anyhow::bail!("service error (HTTP {}): {}", status, text);
    }
    let body: LoginResponse = resp.json().await?;
    Ok(body.auth_url)
}

/// Wait for login to complete by polling /status until the tenant appears.
pub async fn poll_login_complete(
    base_url: &str,
    tenant: &str,
    timeout: Duration,
    token: Option<&str>,
) -> Result<CliWorkspaceStatus> {
    let started = std::time::Instant::now();
    while started.elapsed() < timeout {
        if let Ok(status) = proxy_status(base_url, token).await {
            if let Some(ws) = status
                .workspaces
                .into_iter()
                .find(|w| w.workspace_slug == tenant)
            {
                // Session must have a future expiry to be considered complete.
                let now = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .unwrap_or_default()
                    .as_secs() as i64;
                if ws.session_expires_at > now {
                    return Ok(ws);
                }
            }
        }
        tokio::time::sleep(Duration::from_secs(1)).await;
    }
    Err(anyhow!("login timed out after {:?}", timeout))
}
