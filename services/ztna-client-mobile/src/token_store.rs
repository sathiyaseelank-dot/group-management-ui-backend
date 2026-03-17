use serde::{Deserialize, Serialize};
use std::fs;
use std::path::{Path, PathBuf};

use crate::ZtnaError;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StoredUser {
    pub id: String,
    pub email: String,
    pub role: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StoredWorkspace {
    pub id: String,
    pub name: String,
    pub slug: String,
    pub trust_domain: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StoredDevice {
    pub id: String,
    pub spiffe_id: String,
    pub certificate_pem: String,
    pub private_key_pem: String,
    pub ca_cert_pem: String,
    pub cert_expires_at: i64,
    pub hostname: String,
    pub os: String,
    pub client_version: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StoredSession {
    pub id: String,
    pub access_token: String,
    pub refresh_token: String,
    pub expires_at: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StoredResource {
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
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StoredWorkspaceState {
    pub workspace: StoredWorkspace,
    pub user: StoredUser,
    pub device: StoredDevice,
    pub session: StoredSession,
    pub resources: Vec<StoredResource>,
    pub last_sync_at: i64,
}

/// PKCE pending state persisted between begin_login / complete_login.
#[derive(Debug, Serialize, Deserialize)]
pub struct PendingPkce {
    pub code_verifier: String,
    pub tenant_slug: String,
    pub controller_url: String,
}

fn state_path(data_dir: &str, tenant_slug: &str) -> PathBuf {
    Path::new(data_dir).join(format!("{}.json", tenant_slug))
}

fn pkce_path(data_dir: &str, state: &str) -> PathBuf {
    Path::new(data_dir).join(format!("pkce_{}.json", state))
}

fn write_secure(path: &PathBuf, data: &str) -> Result<(), ZtnaError> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|e| ZtnaError::Io(e.to_string()))?;
    }
    fs::write(path, data).map_err(|e| ZtnaError::Io(e.to_string()))?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(path, fs::Permissions::from_mode(0o600))
            .map_err(|e| ZtnaError::Io(e.to_string()))?;
    }
    Ok(())
}

pub fn save_workspace_state(
    data_dir: &str,
    tenant_slug: &str,
    state: &StoredWorkspaceState,
) -> Result<(), ZtnaError> {
    let path = state_path(data_dir, tenant_slug);
    let json = serde_json::to_string_pretty(state)
        .map_err(|e| ZtnaError::Io(e.to_string()))?;
    write_secure(&path, &json)
}

pub fn load_workspace_state(
    data_dir: &str,
    tenant_slug: &str,
) -> Option<StoredWorkspaceState> {
    let path = state_path(data_dir, tenant_slug);
    let data = fs::read_to_string(path).ok()?;
    serde_json::from_str(&data).ok()
}

pub fn list_workspace_states(data_dir: &str) -> Result<Vec<StoredWorkspaceState>, ZtnaError> {
    let mut out = Vec::new();
    let dir = Path::new(data_dir);
    if !dir.exists() {
        return Ok(out);
    }
    for entry in fs::read_dir(dir).map_err(|e| ZtnaError::Io(e.to_string()))? {
        let entry = entry.map_err(|e| ZtnaError::Io(e.to_string()))?;
        let path = entry.path();
        let name = path
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or_default();
        // Skip pkce_*.json files and only read workspace state files
        if name.starts_with("pkce_") {
            continue;
        }
        if path.extension().and_then(|ext| ext.to_str()) != Some("json") {
            continue;
        }
        let data = fs::read_to_string(&path).map_err(|e| ZtnaError::Io(e.to_string()))?;
        if let Ok(state) = serde_json::from_str::<StoredWorkspaceState>(&data) {
            out.push(state);
        }
    }
    out.sort_by(|a, b| a.workspace.slug.cmp(&b.workspace.slug));
    Ok(out)
}

pub fn clear_workspace_state(data_dir: &str, tenant_slug: &str) {
    let path = state_path(data_dir, tenant_slug);
    let _ = fs::remove_file(path);
}

pub fn save_pending_pkce(
    data_dir: &str,
    state: &str,
    pending: &PendingPkce,
) -> Result<(), ZtnaError> {
    let path = pkce_path(data_dir, state);
    let json = serde_json::to_string(pending).map_err(|e| ZtnaError::Io(e.to_string()))?;
    write_secure(&path, &json)
}

pub fn load_pending_pkce(data_dir: &str, state: &str) -> Option<PendingPkce> {
    let path = pkce_path(data_dir, state);
    let data = fs::read_to_string(path).ok()?;
    serde_json::from_str(&data).ok()
}

pub fn clear_pending_pkce(data_dir: &str, state: &str) {
    let path = pkce_path(data_dir, state);
    let _ = fs::remove_file(path);
}
