uniffi::include_scaffolding!("ztna");

mod auth;
mod token_store;

use auth::{
    complete_device_auth_v2, compute_code_challenge, enroll_device_cert, fetch_device_view,
    generate_code_verifier, refresh_device_token, revoke_device_token, start_device_auth_v2,
    sync_device_view,
};
use token_store::{
    clear_pending_pkce, clear_workspace_state, list_workspace_states, load_pending_pkce,
    load_workspace_state, save_pending_pkce, save_workspace_state, PendingPkce, StoredDevice,
    StoredResource, StoredSession, StoredUser, StoredWorkspace, StoredWorkspaceState,
};

use p256::{
    ecdsa::SigningKey,
    pkcs8::{EncodePrivateKey, EncodePublicKey},
    SecretKey,
};
use rand::thread_rng;
use uuid::Uuid;

// ── UniFFI exported types ──────────────────────────────────────────────────

#[derive(Debug, Clone)]
pub struct WorkspaceState {
    pub workspace_id: String,
    pub workspace_name: String,
    pub tenant_slug: String,
    pub trust_domain: String,
    pub user_email: String,
    pub user_role: String,
    pub access_token: String,
    pub refresh_token: String,
    pub session_expires_at: i64,
    pub spiffe_id: String,
    pub cert_expires_at: i64,
    pub resources: Vec<ResourceItem>,
}

#[derive(Debug, Clone)]
pub struct ResourceItem {
    pub id: String,
    pub name: String,
    pub address: String,
    pub protocol: String,
    pub port_from: Option<i32>,
    pub port_to: Option<i32>,
    pub firewall_status: String,
}

#[derive(Debug, thiserror::Error)]
pub enum ZtnaError {
    #[error("network error: {0}")]
    Network(String),
    #[error("auth error: {0}")]
    Auth(String),
    #[error("not found: {0}")]
    NotFound(String),
    #[error("invalid state: {0}")]
    InvalidState(String),
    #[error("io error: {0}")]
    Io(String),
}

// ── Helpers ────────────────────────────────────────────────────────────────

fn runtime() -> tokio::runtime::Runtime {
    tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .expect("Failed to build Tokio runtime")
}

fn stored_to_exported(s: &StoredWorkspaceState) -> WorkspaceState {
    WorkspaceState {
        workspace_id: s.workspace.id.clone(),
        workspace_name: s.workspace.name.clone(),
        tenant_slug: s.workspace.slug.clone(),
        trust_domain: s.workspace.trust_domain.clone(),
        user_email: s.user.email.clone(),
        user_role: s.user.role.clone(),
        access_token: s.session.access_token.clone(),
        refresh_token: s.session.refresh_token.clone(),
        session_expires_at: s.session.expires_at,
        spiffe_id: s.device.spiffe_id.clone(),
        cert_expires_at: s.device.cert_expires_at,
        resources: s
            .resources
            .iter()
            .map(|r| ResourceItem {
                id: r.id.clone(),
                name: r.name.clone(),
                address: r.address.clone(),
                protocol: r.protocol.clone(),
                port_from: r.port_from,
                port_to: r.port_to,
                firewall_status: r.firewall_status.clone(),
            })
            .collect(),
    }
}

// ── Exported UniFFI functions ──────────────────────────────────────────────

/// Step 1 of login: generate PKCE verifier, request auth URL, persist pending state.
/// Returns the auth URL to open in a Chrome Custom Tab.
/// Uses the v2 endpoint (/api/device/auth/start); redirect_uri is handled server-side.
pub fn begin_login(
    controller_url: String,
    tenant_slug: String,
    data_dir: String,
) -> Result<String, ZtnaError> {
    let verifier = generate_code_verifier();
    let challenge = compute_code_challenge(&verifier);

    let resp = runtime().block_on(start_device_auth_v2(
        &controller_url,
        &tenant_slug,
        &challenge,
    ))?;

    // Persist PKCE verifier with a fixed key ("pending") since only one auth
    // is in-flight at a time per device. complete_login loads it by the same key.
    save_pending_pkce(
        &data_dir,
        "pending",
        &PendingPkce {
            code_verifier: verifier,
            tenant_slug: tenant_slug.clone(),
            controller_url: controller_url.clone(),
        },
    )?;

    Ok(resp.auth_url)
}

/// Step 2 of login: exchange session_code for tokens, enroll device cert, fetch resources.
/// Uses the v2 endpoint (/api/device/auth/complete) with session_code from deep link.
pub fn complete_login(
    controller_url: String,
    session_code: String,
    data_dir: String,
) -> Result<WorkspaceState, ZtnaError> {
    let pending = load_pending_pkce(&data_dir, "pending")
        .ok_or_else(|| ZtnaError::InvalidState("no pending PKCE found".into()))?;
    clear_pending_pkce(&data_dir, "pending");

    runtime().block_on(async {
        // Exchange session_code for tokens using v2 endpoint.
        let tokens = complete_device_auth_v2(
            &controller_url,
            &session_code,
            &pending.code_verifier,
        )
        .await?;

        // Fetch device view to get workspace + user + session info.
        let view = fetch_device_view(&controller_url, &tokens.access_token).await?;

        // Generate a P-256 key pair for device certificate enrollment.
        let secret_key = SecretKey::random(&mut thread_rng());
        let signing_key = SigningKey::from(&secret_key);
        let public_key_pem = signing_key
            .verifying_key()
            .to_public_key_pem(p256::pkcs8::LineEnding::LF)
            .map_err(|e| ZtnaError::Auth(format!("key pem error: {e}")))?;
        let private_key_pem = secret_key
            .to_pkcs8_pem(p256::pkcs8::LineEnding::LF)
            .map_err(|e| ZtnaError::Auth(format!("private key pem error: {e}")))?;
        let device_id = Uuid::new_v4().to_string();

        let cert_resp = enroll_device_cert(
            &controller_url,
            &tokens.access_token,
            &device_id,
            &public_key_pem,
            "android-device",
            "0.1.0",
        )
        .await?;

        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs() as i64;
        let session_expires_at = now + view.session.expires_at;

        let stored = StoredWorkspaceState {
            workspace: StoredWorkspace {
                id: view.workspace.id.clone(),
                name: view.workspace.name.clone(),
                slug: view.workspace.slug.clone(),
                trust_domain: view.workspace.trust_domain.clone(),
            },
            user: StoredUser {
                id: view.user.id.clone(),
                email: view.user.email.clone(),
                role: view.user.role.clone(),
            },
            device: StoredDevice {
                id: cert_resp.device_id.clone(),
                spiffe_id: cert_resp.spiffe_id.clone(),
                certificate_pem: cert_resp.certificate_pem.clone(),
                private_key_pem: private_key_pem.to_string(),
                ca_cert_pem: cert_resp.ca_cert_pem.clone(),
                cert_expires_at: cert_resp.expires_at,
                hostname: "android-device".into(),
                os: "android".into(),
                client_version: "0.1.0".into(),
            },
            session: StoredSession {
                id: view.session.id.clone(),
                access_token: cert_resp.access_token.clone(),
                refresh_token: tokens.refresh_token.clone(),
                expires_at: session_expires_at,
            },
            resources: view
                .resources
                .iter()
                .map(|r| StoredResource {
                    id: r.id.clone(),
                    name: r.name.clone(),
                    r#type: r.r#type.clone(),
                    address: r.address.clone(),
                    protocol: r.protocol.clone(),
                    port_from: r.port_from,
                    port_to: r.port_to,
                    alias: r.alias.clone(),
                    description: r.description.clone(),
                    remote_network_id: r.remote_network_id.clone(),
                    remote_network_name: r.remote_network_name.clone(),
                    firewall_status: r.firewall_status.clone(),
                })
                .collect(),
            last_sync_at: now,
        };

        save_workspace_state(&data_dir, &view.workspace.slug, &stored)?;
        Ok(stored_to_exported(&stored))
    })
}

/// Load persisted workspace state for a given tenant slug.
pub fn load_state(
    tenant_slug: String,
    data_dir: String,
) -> Result<Option<WorkspaceState>, ZtnaError> {
    Ok(load_workspace_state(&data_dir, &tenant_slug).map(|s| stored_to_exported(&s)))
}

/// List all persisted workspaces.
pub fn list_workspaces(data_dir: String) -> Result<Vec<WorkspaceState>, ZtnaError> {
    let states = list_workspace_states(&data_dir)?;
    Ok(states.iter().map(stored_to_exported).collect())
}

/// Sync resources from controller, refreshing token if needed.
pub fn sync(
    tenant_slug: String,
    controller_url: String,
    data_dir: String,
) -> Result<WorkspaceState, ZtnaError> {
    let mut stored = load_workspace_state(&data_dir, &tenant_slug)
        .ok_or_else(|| ZtnaError::NotFound(format!("no state for {}", tenant_slug)))?;

    runtime().block_on(async {
        // Attempt token refresh if close to expiry (within 5 min).
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs() as i64;
        if stored.session.expires_at - now < 300 {
            match refresh_device_token(&controller_url, &stored.session.refresh_token).await {
                Ok(refreshed) => {
                    stored.session.access_token = refreshed.access_token;
                    stored.session.refresh_token = refreshed.refresh_token;
                    stored.session.expires_at = now + refreshed.expires_in;
                }
                Err(e) => return Err(ZtnaError::Auth(format!("token refresh failed: {e}"))),
            }
        }

        let view = sync_device_view(&controller_url, &stored.session.access_token).await?;

        stored.resources = view
            .resources
            .iter()
            .map(|r| StoredResource {
                id: r.id.clone(),
                name: r.name.clone(),
                r#type: r.r#type.clone(),
                address: r.address.clone(),
                protocol: r.protocol.clone(),
                port_from: r.port_from,
                port_to: r.port_to,
                alias: r.alias.clone(),
                description: r.description.clone(),
                remote_network_id: r.remote_network_id.clone(),
                remote_network_name: r.remote_network_name.clone(),
                firewall_status: r.firewall_status.clone(),
            })
            .collect();
        stored.last_sync_at = now;

        save_workspace_state(&data_dir, &tenant_slug, &stored)?;
        Ok(stored_to_exported(&stored))
    })
}

/// Revoke session token and wipe local state.
pub fn disconnect(
    tenant_slug: String,
    controller_url: String,
    data_dir: String,
) -> Result<(), ZtnaError> {
    if let Some(stored) = load_workspace_state(&data_dir, &tenant_slug) {
        let _ = runtime()
            .block_on(revoke_device_token(&controller_url, &stored.session.refresh_token));
    }
    clear_workspace_state(&data_dir, &tenant_slug);
    Ok(())
}
