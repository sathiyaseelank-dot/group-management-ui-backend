use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use anyhow::{anyhow, Result};
use axum::{
    extract::{Query, Request, State},
    http::StatusCode,
    middleware::{self, Next},
    response::{Html, IntoResponse, Response},
    routing::{get, post},
    Json, Router,
};
use p256::ecdsa::SigningKey;
use p256::pkcs8::{DecodePrivateKey, EncodePrivateKey, EncodePublicKey, LineEnding};
use serde::Deserialize;
use tracing::{error, info, warn};
use uuid::Uuid;

use crate::auth::{
    compute_code_challenge, enroll_device_cert, exchange_device_code, fetch_device_view,
    generate_code_verifier, refresh_device_token, report_device_posture, revoke_device_token,
    start_device_auth, sync_device_view, DeviceResource, DeviceUserView,
};
use crate::config::Config;
use crate::product::{CliResourceInfo, CliStatusResponse, CliWorkspaceStatus};
use crate::token_store::{
    clear_workspace_state, list_workspace_states, load_workspace_state, save_workspace_state,
    StoredDevice, StoredResource, StoredSession, StoredUser, StoredWorkspace, StoredWorkspaceState,
};

const CLIENT_VERSION: &str = env!("CARGO_PKG_VERSION");

#[derive(Clone)]
pub struct AppState {
    pub config: Config,
    pub pending: Arc<Mutex<HashMap<String, PendingAuth>>>,
    /// Token the CLI must supply in `X-Service-Token` to reach management
    /// endpoints.  Empty string disables auth (dev mode / token-init failure).
    pub service_token: String,
}

#[derive(Debug, Clone)]
pub struct PendingAuth {
    pub code_verifier: String,
    pub tenant_slug: String,
}

fn sanitize_workspace(state: &StoredWorkspaceState) -> CliWorkspaceStatus {
    CliWorkspaceStatus {
        workspace_slug: state.workspace.slug.clone(),
        workspace_name: state.workspace.name.clone(),
        user_email: state.user.email.clone(),
        user_role: state.user.role.clone(),
        device_id: state.device.id.clone(),
        resources: state
            .resources
            .iter()
            .map(|r| CliResourceInfo {
                name: r.name.clone(),
                address: r.address.clone(),
                protocol: r.protocol.clone(),
                port_from: r.port_from,
                port_to: r.port_to,
                remote_network_name: r.remote_network_name.clone(),
                firewall_status: r.firewall_status.clone(),
            })
            .collect(),
        session_expires_at: state.session.expires_at,
        last_sync_at: state.last_sync_at,
    }
}

fn sanitize_resources(resources: &[StoredResource]) -> Vec<CliResourceInfo> {
    resources
        .iter()
        .map(|r| CliResourceInfo {
            name: r.name.clone(),
            address: r.address.clone(),
            protocol: r.protocol.clone(),
            port_from: r.port_from,
            port_to: r.port_to,
            remote_network_name: r.remote_network_name.clone(),
            firewall_status: r.firewall_status.clone(),
        })
        .collect()
}

#[derive(Debug)]
struct DeviceMaterial {
    device_id: String,
    private_key_pem: String,
    public_key_pem: String,
    hostname: String,
    os: String,
    device_name: String,
    device_model: String,
    device_make: String,
    serial_number: String,
}

async fn report_posture(config: &Config, state: &StoredWorkspaceState) {
    if state.device.id.is_empty() {
        return;
    }
    let posture = crate::posture::collect(&state.device.id, &state.device.spiffe_id);
    if let Err(e) =
        report_device_posture(&config.controller_grpc_addr, &state.session.access_token, &posture).await
    {
        warn!("posture report failed: {}", e);
    }
}

pub async fn run_posture_reporter(config: Config) {
    loop {
        tokio::time::sleep(Duration::from_secs(300)).await;
        let states = match list_workspace_states() {
            Ok(s) => s,
            Err(_) => continue,
        };
        for ws in states {
            if ws.session.expires_at <= now_unix() {
                continue;
            }
            let posture = crate::posture::collect(&ws.device.id, &ws.device.spiffe_id);
            if let Err(e) = report_device_posture(
                &config.controller_grpc_addr,
                &ws.session.access_token,
                &posture,
            )
            .await
            {
                warn!(
                    "background posture report failed for {}: {}",
                    ws.workspace.slug, e
                );
            }
        }
    }
}

/// Axum middleware that validates the `X-Service-Token` header.
///
/// Rejected requests receive `401 Unauthorized` with no body.  If the service
/// token is empty (dev mode or token initialisation failure) all requests are
/// allowed through so that development workflows are unaffected.
async fn require_service_token(
    State(state): State<AppState>,
    request: Request,
    next: Next,
) -> Response {
    if state.service_token.is_empty() {
        return next.run(request).await;
    }
    let provided = request
        .headers()
        .get("X-Service-Token")
        .and_then(|v| v.to_str().ok());
    if provided == Some(state.service_token.as_str()) {
        next.run(request).await
    } else {
        StatusCode::UNAUTHORIZED.into_response()
    }
}

/// Management API router — bound to 127.0.0.1 only.
///
/// All routes require a valid `X-Service-Token` header (written at service
/// startup to `<state_dir>/.service.token`, mode 0644).  The loopback binding
/// provides network-level isolation; the token provides process-level isolation
/// so that other local processes cannot call management endpoints.
pub fn management_router(state: AppState) -> Router {
    Router::new()
        .route("/status", get(handle_status))
        .route("/resources", get(handle_resources))
        .route("/sync", post(handle_sync))
        .route("/disconnect", post(handle_disconnect))
        .route("/login", post(handle_login))
        .layer(middleware::from_fn_with_state(
            state.clone(),
            require_service_token,
        ))
        .with_state(state)
}

/// OAuth callback router — may be bound to a LAN address for testing.
///
/// Exposes only the `/callback` route; no management endpoints here so
/// LAN exposure does not affect the control surface.
pub fn callback_router(state: AppState) -> Router {
    Router::new()
        .route("/callback", get(handle_callback))
        .with_state(state)
}

fn now_unix() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs() as i64
}

fn local_os() -> String {
    std::env::consts::OS.to_string()
}

fn local_hostname() -> String {
    hostname::get()
        .ok()
        .and_then(|v| v.into_string().ok())
        .unwrap_or_else(|| "unknown-host".to_string())
}

fn read_dmi_file(path: &str) -> String {
    std::fs::read_to_string(path)
        .unwrap_or_default()
        .trim()
        .to_string()
}

fn collect_device_info() -> (String, String, String, String) {
    // Returns (device_name, device_model, device_make, serial_number)
    #[cfg(target_os = "linux")]
    {
        let model = read_dmi_file("/sys/class/dmi/id/product_name");
        let make = read_dmi_file("/sys/class/dmi/id/sys_vendor");
        let serial = read_dmi_file("/sys/class/dmi/id/product_serial");
        let name = local_hostname();
        return (name, model, make, serial);
    }

    #[cfg(target_os = "macos")]
    {
        fn run_cmd(cmd: &str, args: &[&str]) -> String {
            std::process::Command::new(cmd)
                .args(args)
                .output()
                .ok()
                .and_then(|o| String::from_utf8(o.stdout).ok())
                .unwrap_or_default()
        }
        let sp = run_cmd("system_profiler", &["SPHardwareDataType"]);
        let model = sp.lines()
            .find(|l| l.trim_start().starts_with("Model Name:"))
            .map(|l| l.split(':').nth(1).unwrap_or("").trim().to_string())
            .unwrap_or_default();
        let serial = sp.lines()
            .find(|l| l.trim_start().starts_with("Serial Number"))
            .map(|l| l.split(':').nth(1).unwrap_or("").trim().to_string())
            .unwrap_or_default();
        let name = local_hostname();
        return (name, model, "Apple Inc.".to_string(), serial);
    }

    #[cfg(target_os = "windows")]
    {
        fn wmic(query: &str) -> String {
            std::process::Command::new("wmic")
                .args(query.split_whitespace())
                .output()
                .ok()
                .and_then(|o| String::from_utf8(o.stdout).ok())
                .map(|s| s.lines().nth(1).unwrap_or("").trim().to_string())
                .unwrap_or_default()
        }
        let model = wmic("computersystem get Model");
        let make = wmic("computersystem get Manufacturer");
        let serial = wmic("bios get SerialNumber");
        let name = local_hostname();
        return (name, model, make, serial);
    }

    #[allow(unreachable_code)]
    (local_hostname(), String::new(), String::new(), String::new())
}

fn map_resources(resources: Vec<DeviceResource>) -> Vec<StoredResource> {
    resources
        .into_iter()
        .map(|res| StoredResource {
            id: res.id,
            name: res.name,
            r#type: res.r#type,
            address: res.address,
            protocol: res.protocol,
            port_from: res.port_from,
            port_to: res.port_to,
            alias: res.alias,
            description: res.description,
            remote_network_id: res.remote_network_id,
            remote_network_name: res.remote_network_name,
            firewall_status: res.firewall_status,
        })
        .collect()
}

fn build_workspace_state(
    view: DeviceUserView,
    access_token: String,
    refresh_token: String,
    token_expires_at: i64,
    device: StoredDevice,
) -> StoredWorkspaceState {
    StoredWorkspaceState {
        schema_version: crate::token_store::CURRENT_SCHEMA_VERSION,
        workspace: StoredWorkspace {
            id: view.workspace.id,
            name: view.workspace.name,
            slug: view.workspace.slug,
            trust_domain: view.workspace.trust_domain,
        },
        user: StoredUser {
            id: view.user.id,
            email: view.user.email,
            role: view.user.role,
        },
        device,
        session: StoredSession {
            id: view.session.id,
            access_token,
            refresh_token,
            expires_at: token_expires_at,
        },
        resources: map_resources(view.resources),
        last_sync_at: view.synced_at,
    }
}

fn existing_device_material(existing: Option<&StoredWorkspaceState>) -> Result<DeviceMaterial> {
    let (device_name, device_model, device_make, serial_number) = collect_device_info();

    if let Some(existing) = existing {
        if !existing.device.id.is_empty() && !existing.device.private_key_pem.is_empty() {
            let signing_key = SigningKey::from_pkcs8_pem(&existing.device.private_key_pem)?;
            let verifying_key = signing_key.verifying_key();
            return Ok(DeviceMaterial {
                device_id: existing.device.id.clone(),
                private_key_pem: existing.device.private_key_pem.clone(),
                public_key_pem: verifying_key.to_public_key_pem(LineEnding::LF)?,
                hostname: existing.device.hostname.clone(),
                os: existing.device.os.clone(),
                device_name,
                device_model,
                device_make,
                serial_number,
            });
        }
    }

    let signing_key = SigningKey::from(&p256::SecretKey::random(&mut rand::rngs::OsRng));
    let private_key_pem = signing_key.to_pkcs8_pem(LineEnding::LF)?.to_string();
    let public_key_pem = signing_key
        .verifying_key()
        .to_public_key_pem(LineEnding::LF)?;
    Ok(DeviceMaterial {
        device_id: Uuid::new_v4().to_string(),
        private_key_pem,
        public_key_pem,
        hostname: local_hostname(),
        os: local_os(),
        device_name,
        device_model,
        device_make,
        serial_number,
    })
}

async fn enroll_and_sync(
    config: &Config,
    _tenant_slug: &str,
    access_token: String,
    refresh_token: String,
    token_expires_at: i64,
    existing: Option<&StoredWorkspaceState>,
) -> Result<StoredWorkspaceState> {
    let material = existing_device_material(existing)?;
    let enroll = enroll_device_cert(
        &config.controller_grpc_addr,
        &access_token,
        &material.device_id,
        &material.public_key_pem,
        &material.hostname,
        &material.os,
        CLIENT_VERSION,
        &material.device_name,
        &material.device_model,
        &material.device_make,
        &material.serial_number,
    )
    .await?;
    let view = fetch_device_view(&config.controller_grpc_addr, &enroll.access_token).await?;
    Ok(build_workspace_state(
        view,
        enroll.access_token,
        refresh_token,
        token_expires_at,
        StoredDevice {
            id: enroll.device_id,
            spiffe_id: enroll.spiffe_id,
            certificate_pem: enroll.certificate_pem,
            private_key_pem: material.private_key_pem,
            ca_cert_pem: enroll.ca_cert_pem,
            cert_expires_at: enroll.expires_at,
            hostname: material.hostname,
            os: material.os,
            client_version: CLIENT_VERSION.to_string(),
        },
    ))
}

pub async fn begin_login(state: &AppState, tenant_slug: &str) -> Result<String> {
    let code_verifier = generate_code_verifier();
    let code_challenge = compute_code_challenge(&code_verifier);
    let callback_host = state.config.effective_callback_host();
    let redirect_uri = format!("http://{}:{}/callback", callback_host, state.config.port);

    let auth = start_device_auth(
        &state.config.controller_grpc_addr,
        tenant_slug,
        &code_challenge,
        &redirect_uri,
    )
    .await?;

    {
        let mut pending = state.pending.lock().unwrap();
        pending.insert(
            auth.state.clone(),
            PendingAuth {
                code_verifier,
                tenant_slug: tenant_slug.to_string(),
            },
        );
    }

    Ok(auth.auth_url)
}

pub async fn wait_for_login(tenant_slug: &str, timeout: Duration) -> Result<StoredWorkspaceState> {
    let started = std::time::Instant::now();
    while started.elapsed() < timeout {
        if let Some(state) = load_workspace_state(tenant_slug) {
            return Ok(state);
        }
        tokio::time::sleep(Duration::from_secs(1)).await;
    }
    Err(anyhow!("login timed out after {:?}", timeout))
}

pub async fn ensure_workspace_state(
    config: &Config,
    tenant_slug: &str,
    force_sync: bool,
) -> Result<StoredWorkspaceState> {
    let mut state = load_workspace_state(tenant_slug)
        .ok_or_else(|| anyhow!("no saved workspace state for {}", tenant_slug))?;
    let now = now_unix();

    if state.session.expires_at <= now + 60 {
        let refreshed =
            refresh_device_token(&config.controller_grpc_addr, &state.session.refresh_token).await?;
        state.session.access_token = refreshed.access_token;
        state.session.refresh_token = refreshed.refresh_token;
        state.session.expires_at = now + refreshed.expires_in;
    }

    if state.device.certificate_pem.trim().is_empty() || state.device.cert_expires_at <= now + 300 {
        state = enroll_and_sync(
            config,
            tenant_slug,
            state.session.access_token.clone(),
            state.session.refresh_token.clone(),
            state.session.expires_at,
            Some(&state),
        )
        .await?;
        save_workspace_state(tenant_slug, &state)?;
        report_posture(config, &state).await;
        return Ok(state);
    }

    if force_sync || state.last_sync_at <= now - 60 || state.resources.is_empty() {
        let view = sync_device_view(&config.controller_grpc_addr, &state.session.access_token).await?;
        state.workspace = StoredWorkspace {
            id: view.workspace.id,
            name: view.workspace.name,
            slug: view.workspace.slug,
            trust_domain: view.workspace.trust_domain,
        };
        state.user = StoredUser {
            id: view.user.id,
            email: view.user.email,
            role: view.user.role,
        };
        state.session.id = view.session.id;
        state.resources = map_resources(view.resources);
        state.last_sync_at = view.synced_at;
    }

    save_workspace_state(tenant_slug, &state)?;
    report_posture(config, &state).await;
    Ok(state)
}

pub async fn disconnect_workspace(config: &Config, tenant_slug: &str) -> Result<()> {
    if let Some(state) = load_workspace_state(tenant_slug) {
        revoke_device_token(&config.controller_grpc_addr, &state.session.refresh_token).await?;
    }
    clear_workspace_state(tenant_slug);
    Ok(())
}

#[derive(Deserialize)]
struct CallbackQuery {
    code: Option<String>,
    state: Option<String>,
    error: Option<String>,
}

async fn handle_callback(
    State(state): State<AppState>,
    Query(q): Query<CallbackQuery>,
) -> impl IntoResponse {
    if let Some(err) = q.error {
        return Html(format!(
            "<html><body><p>Authentication error: {}</p></body></html>",
            err
        ));
    }

    let code = match q.code {
        Some(c) => c,
        None => {
            return Html("<html><body><p>Missing authorization code.</p></body></html>".to_string())
        }
    };
    let oauth_state = match q.state {
        Some(s) => s,
        None => {
            return Html("<html><body><p>Missing state parameter.</p></body></html>".to_string())
        }
    };

    let pending = {
        let mut pending = state.pending.lock().unwrap();
        pending.remove(&oauth_state)
    };
    let Some(pending) = pending else {
        return Html(
            "<html><body><p>Unknown or expired state. Please try again.</p></body></html>"
                .to_string(),
        );
    };

    match exchange_device_code(
        &state.config.controller_grpc_addr,
        &code,
        &pending.code_verifier,
        &oauth_state,
    )
    .await
    {
        Ok(tokens) => {
            let token_expires_at = now_unix() + tokens.expires_in;
            let existing = load_workspace_state(&pending.tenant_slug);
            match enroll_and_sync(
                &state.config,
                &pending.tenant_slug,
                tokens.access_token,
                tokens.refresh_token,
                token_expires_at,
                existing.as_ref(),
            )
            .await
            {
                Ok(stored) => {
                    if let Err(err) = save_workspace_state(&pending.tenant_slug, &stored) {
                        error!("failed to save workspace state: {}", err);
                    }
                    report_posture(&state.config, &stored).await;
                    info!("authenticated to workspace: {}", pending.tenant_slug);
                    Html(format!(
                        "<html><body><h2>Connected to {}.</h2><p>You can close this tab and return to the terminal.</p></body></html>",
                        pending.tenant_slug
                    ))
                }
                Err(err) => {
                    error!("post-login device setup failed: {}", err);
                    Html(format!(
                        "<html><body><p>Authentication succeeded, but device setup failed: {}</p></body></html>",
                        err
                    ))
                }
            }
        }
        Err(err) => {
            error!("token exchange failed: {}", err);
            Html(format!(
                "<html><body><p>Authentication failed: {}</p></body></html>",
                err
            ))
        }
    }
}

async fn handle_status(State(state): State<AppState>) -> Json<CliStatusResponse> {
    let workspaces = list_workspace_states()
        .unwrap_or_default()
        .iter()
        .map(sanitize_workspace)
        .collect();
    Json(CliStatusResponse {
        mode: state.config.mode.clone(),
        configured: state.config.is_configured(),
        client_version: CLIENT_VERSION.to_string(),
        workspaces,
    })
}

#[derive(Deserialize)]
struct TenantQuery {
    tenant: Option<String>,
}

async fn handle_resources(Query(q): Query<TenantQuery>) -> impl IntoResponse {
    let Some(tenant) = q.tenant else {
        return (
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({ "error": "tenant query parameter required" })),
        )
            .into_response();
    };
    match load_workspace_state(&tenant) {
        Some(state) => {
            Json(serde_json::json!({ "resources": sanitize_resources(&state.resources) }))
                .into_response()
        }
        None => (
            StatusCode::NOT_FOUND,
            Json(serde_json::json!({ "error": "workspace state not found" })),
        )
            .into_response(),
    }
}

#[derive(Deserialize)]
struct DisconnectRequest {
    tenant: String,
}

async fn handle_sync(
    State(state): State<AppState>,
    Json(body): Json<DisconnectRequest>,
) -> impl IntoResponse {
    match ensure_workspace_state(&state.config, &body.tenant, true).await {
        Ok(synced) => Json(sanitize_workspace(&synced)).into_response(),
        Err(err) => (
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({ "error": err.to_string() })),
        )
            .into_response(),
    }
}

async fn handle_disconnect(
    State(state): State<AppState>,
    Json(body): Json<DisconnectRequest>,
) -> impl IntoResponse {
    match disconnect_workspace(&state.config, &body.tenant).await {
        Ok(()) => Json(serde_json::json!({ "status": "disconnected" })).into_response(),
        Err(err) => (
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({ "error": err.to_string() })),
        )
            .into_response(),
    }
}

#[derive(Deserialize)]
struct LoginRequest {
    tenant: String,
}

async fn handle_login(
    State(state): State<AppState>,
    Json(body): Json<LoginRequest>,
) -> impl IntoResponse {
    match begin_login(&state, &body.tenant).await {
        Ok(auth_url) => {
            Json(serde_json::json!({ "auth_url": auth_url })).into_response()
        }
        Err(err) => (
            StatusCode::BAD_REQUEST,
            Json(serde_json::json!({ "error": err.to_string() })),
        )
            .into_response(),
    }
}
