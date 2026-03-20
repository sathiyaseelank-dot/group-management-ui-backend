use aes_gcm::{
    aead::{Aead, AeadCore, KeyInit},
    Aes256Gcm, Key, Nonce,
};
use anyhow::Result;
use base64::engine::general_purpose::STANDARD as B64;
use base64::Engine;
use rand::RngCore;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;
use std::sync::OnceLock;

/// Current schema version written on every save.
///
/// Increment this when making breaking changes to `StoredWorkspaceState` or
/// its nested types.  Old state files that predate this field will deserialize
/// `schema_version` as 0 (via `#[serde(default)]`) and can be migrated.
pub const CURRENT_SCHEMA_VERSION: u32 = 1;

/// `#[serde(default)]` on sub-structs lets new optional fields be added to
/// any nested type without breaking deserialization of existing state files.

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct StoredUser {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub email: String,
    #[serde(default)]
    pub role: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct StoredWorkspace {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub slug: String,
    #[serde(default)]
    pub trust_domain: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct StoredDevice {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub spiffe_id: String,
    #[serde(default)]
    pub certificate_pem: String,
    /// Private key PEM, encrypted at rest.  Value is either:
    ///   - `"enc1:<base64(12-byte-nonce || AES-256-GCM-ciphertext)>"` (current), or
    ///   - a raw PEM string (legacy, auto-migrated to enc1 on next save).
    #[serde(default)]
    pub private_key_pem: String,
    #[serde(default)]
    pub ca_cert_pem: String,
    #[serde(default)]
    pub cert_expires_at: i64,
    #[serde(default)]
    pub hostname: String,
    #[serde(default)]
    pub os: String,
    #[serde(default)]
    pub client_version: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct StoredSession {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub access_token: String,
    #[serde(default)]
    pub refresh_token: String,
    #[serde(default)]
    pub expires_at: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct StoredResource {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub r#type: String,
    #[serde(default)]
    pub address: String,
    #[serde(default)]
    pub protocol: String,
    pub port_from: Option<i32>,
    pub port_to: Option<i32>,
    pub alias: Option<String>,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub remote_network_id: String,
    #[serde(default)]
    pub remote_network_name: String,
    #[serde(default)]
    pub firewall_status: String,
    #[serde(default)]
    pub connector_tunnel_addr: String,
}

pub fn connector_tunnel_addr_for_resource(
    resources: &[StoredResource],
    resource_id: &str,
    fallback: &str,
) -> String {
    let resource_id = resource_id.trim();
    if !resource_id.is_empty() {
        if let Some(resource) = resources.iter().find(|resource| resource.id == resource_id) {
            let addr = resource.connector_tunnel_addr.trim();
            if !addr.is_empty() {
                return addr.to_string();
            }
        }
    }

    fallback.trim().to_string()
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StoredWorkspaceState {
    /// Incremented when the persisted format changes in a breaking way.
    /// Files written before this field was introduced deserialize as 0.
    #[serde(default)]
    pub schema_version: u32,
    pub workspace: StoredWorkspace,
    pub user: StoredUser,
    pub device: StoredDevice,
    pub session: StoredSession,
    #[serde(default)]
    pub resources: Vec<StoredResource>,
    #[serde(default)]
    pub last_sync_at: i64,
}

// ---------------------------------------------------------------------------
// Configurable state directory
// ---------------------------------------------------------------------------

/// Global state directory, set once at startup via `init_state_dir`.
static STATE_DIR: OnceLock<PathBuf> = OnceLock::new();

/// Initialize the state directory.  Must be called once at startup before
/// any load/save operations.  Safe to call multiple times — second call
/// is a no-op.
pub fn init_state_dir(dir: PathBuf) {
    let _ = STATE_DIR.set(dir);
}

/// Return the active state directory.  Falls back to XDG data dir
/// if `init_state_dir` was never called (e.g. in tests or dev one-liners).
fn state_dir() -> PathBuf {
    if let Some(dir) = STATE_DIR.get() {
        return dir.clone();
    }
    // Fallback — uses XDG data directory (not config, since this is
    // runtime/session state).  Only reached when init_state_dir was
    // not called, e.g. running via `cargo run` without the full
    // Config::load() → init_state_dir() path.
    directories::ProjectDirs::from("com", "zerotrust", "ztna-client")
        .map(|dirs| dirs.data_local_dir().to_path_buf())
        .unwrap_or_else(|| PathBuf::from("/tmp/ztna-client"))
}

fn state_path(tenant_slug: &str) -> PathBuf {
    state_dir().join(format!("{}.json", tenant_slug))
}

/// Key file path used as fallback when the OS keychain is unavailable.
fn key_file_path(tenant_slug: &str) -> PathBuf {
    state_dir().join(format!(".{}.key", tenant_slug))
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
/// Uses a per-tenant key file (`.<tenant>.key`, 0600).  The OS keychain is not
/// used because it is unreliable on headless/server Linux systems (no D-Bus /
/// keyring daemon), which causes inconsistent key retrieval between save and
/// load and ultimately breaks decryption.
fn get_or_create_workspace_key(tenant_slug: &str) -> Result<[u8; 32]> {
    get_or_create_keyfile_key(tenant_slug)
}

/// Store the key in a separate file (`.<tenant>.key`) with 0600
/// permissions, keeping the key separate from the main JSON state file.
fn get_or_create_keyfile_key(tenant_slug: &str) -> Result<[u8; 32]> {
    let path = key_file_path(tenant_slug);

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
    let path = state_path(tenant_slug);
    let mut state = state.clone();

    // Always stamp the current schema version on save so we can detect
    // future migration needs on load.
    state.schema_version = CURRENT_SCHEMA_VERSION;

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
    let path = state_path(tenant_slug);
    let data = fs::read_to_string(path).ok()?;
    let mut state: StoredWorkspaceState = serde_json::from_str(&data).ok()?;

    // Decrypt private key if it was stored in enc1 format.
    if state.device.private_key_pem.starts_with(ENC_PREFIX) {
        let key = get_or_create_workspace_key(tenant_slug).ok()?;
        state.device.private_key_pem =
            decrypt_private_key(&key, &state.device.private_key_pem).ok()?;
    }
    // Legacy plain-text keys are left as-is here; they will be encrypted on
    // the next call to save_workspace_state.

    Some(state)
}

pub fn list_workspace_states() -> Result<Vec<StoredWorkspaceState>> {
    let mut out = Vec::new();
    let dir = state_dir();
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
    let _ = fs::remove_file(state_path(tenant_slug));
    // Also remove the fallback key file if present.
    let _ = fs::remove_file(key_file_path(tenant_slug));
}
