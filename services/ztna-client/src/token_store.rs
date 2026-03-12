use anyhow::Result;
use directories::ProjectDirs;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StoredTokens {
    pub access_token: String,
    pub refresh_token: String,
    pub workspace_name: String,
    pub workspace_slug: String,
    pub expires_at: i64,
}

fn token_path(tenant_slug: &str) -> Option<PathBuf> {
    ProjectDirs::from("com", "zerotrust", "ztna-client").map(|dirs| {
        dirs.config_dir().join(format!("{}.json", tenant_slug))
    })
}

pub fn save_tokens(tenant_slug: &str, tokens: &StoredTokens) -> Result<()> {
    let path = token_path(tenant_slug).ok_or_else(|| anyhow::anyhow!("no config dir"))?;
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)?;
    }
    let json = serde_json::to_string_pretty(tokens)?;
    fs::write(&path, &json)?;
    // Set file permissions to 0600 on Unix
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&path, fs::Permissions::from_mode(0o600))?;
    }
    Ok(())
}

pub fn load_tokens(tenant_slug: &str) -> Option<StoredTokens> {
    let path = token_path(tenant_slug)?;
    let data = fs::read_to_string(path).ok()?;
    serde_json::from_str(&data).ok()
}

pub fn clear_tokens(tenant_slug: &str) {
    if let Some(path) = token_path(tenant_slug) {
        let _ = fs::remove_file(path);
    }
}
