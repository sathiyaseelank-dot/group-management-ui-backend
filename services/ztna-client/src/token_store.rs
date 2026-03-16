use anyhow::Result;
use directories::ProjectDirs;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;

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

fn state_dir() -> Option<PathBuf> {
    ProjectDirs::from("com", "zerotrust", "ztna-client").map(|dirs| dirs.config_dir().to_path_buf())
}

fn state_path(tenant_slug: &str) -> Option<PathBuf> {
    state_dir().map(|dir| dir.join(format!("{}.json", tenant_slug)))
}

fn write_secure(path: &PathBuf, data: &str) -> Result<()> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)?;
    }
    fs::write(path, data)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(path, fs::Permissions::from_mode(0o600))?;
    }
    Ok(())
}

pub fn save_workspace_state(tenant_slug: &str, state: &StoredWorkspaceState) -> Result<()> {
    let path = state_path(tenant_slug).ok_or_else(|| anyhow::anyhow!("no config dir"))?;
    let json = serde_json::to_string_pretty(state)?;
    write_secure(&path, &json)
}

pub fn load_workspace_state(tenant_slug: &str) -> Option<StoredWorkspaceState> {
    let path = state_path(tenant_slug)?;
    let data = fs::read_to_string(path).ok()?;
    serde_json::from_str(&data).ok()
}

pub fn list_workspace_states() -> Result<Vec<StoredWorkspaceState>> {
    let mut out = Vec::new();
    let Some(dir) = state_dir() else {
        return Ok(out);
    };
    if !dir.exists() {
        return Ok(out);
    }
    for entry in fs::read_dir(dir)? {
        let entry = entry?;
        let path = entry.path();
        if path.extension().and_then(|ext| ext.to_str()) != Some("json") {
            continue;
        }
        let data = fs::read_to_string(path)?;
        if let Ok(state) = serde_json::from_str::<StoredWorkspaceState>(&data) {
            out.push(state);
        }
    }
    out.sort_by(|a, b| a.workspace.slug.cmp(&b.workspace.slug));
    Ok(out)
}

pub fn clear_workspace_state(tenant_slug: &str) {
    if let Some(path) = state_path(tenant_slug) {
        let _ = fs::remove_file(path);
    }
}
