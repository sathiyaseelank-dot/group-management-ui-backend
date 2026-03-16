use anyhow::Result;
use aes_gcm::{
    aead::{Aead, AeadCore, KeyInit},
    Aes256Gcm, Key, Nonce,
};
use base64::engine::general_purpose::STANDARD as B64;
use base64::Engine;
use directories::ProjectDirs;
use rand::RngCore;
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
    /// Private key PEM, encrypted at rest.  Value is either:
    ///   - `"enc1:<base64(12-byte-nonce || AES-256-GCM-ciphertext)>"` (current), or
    ///   - a raw PEM string (legacy, auto-migrated to enc1 on next save).
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

/// Key file path used as fallback when the OS keychain is unavailable.
fn key_file_path(tenant_slug: &str) -> Option<PathBuf> {
    state_dir().map(|dir| dir.join(format!(".{}.key", tenant_slug)))
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

// ---------------------------------------------------------------------------
// Per-workspace AES-256 key management
// ---------------------------------------------------------------------------

const ENC_PREFIX: &str = "enc1:";

/// Retrieve (or create) the 32-byte AES-256 key used to protect the private
/// key for `tenant_slug`.
///
/// Strategy:
/// 1. Try the OS keychain (via the `keyring` crate).
/// 2. Fall back to a per-tenant key file (`.<tenant>.key`, 0600) so the tool
///    continues to work on headless/server systems without a D-Bus keychain.
fn get_or_create_workspace_key(tenant_slug: &str) -> Result<[u8; 32]> {
    let service = "ztna-client";
    let account = format!("device-key/{}", tenant_slug);

    if let Ok(entry) = keyring::Entry::new(service, &account) {
        match entry.get_password() {
            Ok(b64) => {
                let bytes = B64.decode(b64.trim())?;
                if bytes.len() == 32 {
                    let mut key = [0u8; 32];
                    key.copy_from_slice(&bytes);
                    return Ok(key);
                }
                // Corrupt entry — regenerate below.
            }
            Err(keyring::Error::NoEntry) => {
                // First use — generate, store, and return.
                let mut key = [0u8; 32];
                rand::thread_rng().fill_bytes(&mut key);
                let b64 = B64.encode(key);
                if entry.set_password(&b64).is_ok() {
                    return Ok(key);
                }
                // set_password failed (e.g. no unlock daemon) — fall through.
            }
            Err(_) => {
                // Keychain not accessible — fall through to key file.
            }
        }
    }

    get_or_create_keyfile_key(tenant_slug)
}

/// Fall-back: store the key in a separate file (`.<tenant>.key`) with 0600
/// permissions, keeping the key separate from the main JSON state file.
fn get_or_create_keyfile_key(tenant_slug: &str) -> Result<[u8; 32]> {
    let path = key_file_path(tenant_slug)
        .ok_or_else(|| anyhow::anyhow!("no config dir for key file"))?;

    if path.exists() {
        let b64 = fs::read_to_string(&path)?;
        let bytes = B64.decode(b64.trim())?;
        if bytes.len() == 32 {
            let mut key = [0u8; 32];
            key.copy_from_slice(&bytes);
            return Ok(key);
        }
        // Corrupt file — regenerate.
    }

    let mut key = [0u8; 32];
    rand::thread_rng().fill_bytes(&mut key);
    write_secure(&path, &B64.encode(key))?;
    Ok(key)
}

/// Encrypt `pem` using AES-256-GCM.  Returns `"enc1:<base64>"`.
fn encrypt_private_key(key_bytes: &[u8; 32], pem: &str) -> Result<String> {
    let key = Key::<Aes256Gcm>::from_slice(key_bytes);
    let cipher = Aes256Gcm::new(key);
    let nonce = Aes256Gcm::generate_nonce(&mut rand::rngs::OsRng);
    let mut blob = nonce.to_vec();
    blob.extend_from_slice(
        &cipher
            .encrypt(&nonce, pem.as_bytes())
            .map_err(|_| anyhow::anyhow!("failed to encrypt private key"))?,
    );
    Ok(format!("{}{}", ENC_PREFIX, B64.encode(&blob)))
}

/// Decrypt a value previously produced by `encrypt_private_key`.
fn decrypt_private_key(key_bytes: &[u8; 32], encrypted: &str) -> Result<String> {
    let b64 = encrypted
        .strip_prefix(ENC_PREFIX)
        .ok_or_else(|| anyhow::anyhow!("private_key_pem: unexpected format (not enc1:)"))?;
    let blob = B64.decode(b64.trim())?;
    if blob.len() < 12 {
        return Err(anyhow::anyhow!("private_key_pem: ciphertext too short"));
    }
    let (nonce_bytes, ciphertext) = blob.split_at(12);
    let key = Key::<Aes256Gcm>::from_slice(key_bytes);
    let cipher = Aes256Gcm::new(key);
    let nonce = Nonce::from_slice(nonce_bytes);
    let plaintext = cipher
        .decrypt(nonce, ciphertext)
        .map_err(|_| anyhow::anyhow!("private_key_pem: decryption authentication failed"))?;
    Ok(String::from_utf8(plaintext)?)
}

// ---------------------------------------------------------------------------
// Public persistence API
// ---------------------------------------------------------------------------

pub fn save_workspace_state(tenant_slug: &str, state: &StoredWorkspaceState) -> Result<()> {
    let path = state_path(tenant_slug).ok_or_else(|| anyhow::anyhow!("no config dir"))?;
    let mut state = state.clone();

    // Encrypt the private key if it is currently stored as plain text.
    if !state.device.private_key_pem.is_empty()
        && !state.device.private_key_pem.starts_with(ENC_PREFIX)
    {
        let key = get_or_create_workspace_key(tenant_slug)?;
        state.device.private_key_pem = encrypt_private_key(&key, &state.device.private_key_pem)?;
    }

    let json = serde_json::to_string_pretty(&state)?;
    write_secure(&path, &json)
}

pub fn load_workspace_state(tenant_slug: &str) -> Option<StoredWorkspaceState> {
    let path = state_path(tenant_slug)?;
    let data = fs::read_to_string(path).ok()?;
    let mut state: StoredWorkspaceState = serde_json::from_str(&data).ok()?;

    // Decrypt private key if it was stored in enc1 format.
    if state.device.private_key_pem.starts_with(ENC_PREFIX) {
        let key = get_or_create_workspace_key(tenant_slug).ok()?;
        state.device.private_key_pem = decrypt_private_key(&key, &state.device.private_key_pem).ok()?;
    }
    // Legacy plain-text keys are left as-is here; they will be encrypted on
    // the next call to save_workspace_state.

    Some(state)
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
        // Derive tenant slug from the filename to look up the decryption key.
        let tenant_slug = path
            .file_stem()
            .and_then(|s| s.to_str())
            .unwrap_or("")
            .to_string();
        let data = fs::read_to_string(&path)?;
        if let Ok(mut state) = serde_json::from_str::<StoredWorkspaceState>(&data) {
            if state.device.private_key_pem.starts_with(ENC_PREFIX) && !tenant_slug.is_empty() {
                if let Ok(key) = get_or_create_workspace_key(&tenant_slug) {
                    if let Ok(pem) = decrypt_private_key(&key, &state.device.private_key_pem) {
                        state.device.private_key_pem = pem;
                    }
                }
            }
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
    // Also remove the fallback key file if present.
    if let Some(path) = key_file_path(tenant_slug) {
        let _ = fs::remove_file(path);
    }
}
