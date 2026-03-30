use anyhow::Result;
use std::path::PathBuf;
use tracing::{info, warn};

use crate::enroll::EnrollResult;
use zeroize::Zeroizing;

const CERT_FILE: &str = "cert.pem";
const KEY_FILE: &str = "key.der";
const CA_FILE: &str = "ca.pem";
const DELETE_REQUEST_FILE: &str = "delete-request.json";

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize, PartialEq, Eq)]
pub struct DeleteCleanupRequest {
    pub connector_id: String,
    pub reason: String,
}

/// Returns the state directory for persisting enrollment artifacts.
/// Uses $STATE_DIRECTORY (set by systemd StateDirectory=) or falls back to /var/lib/connector.
fn state_dir() -> Option<PathBuf> {
    if let Ok(dir) = std::env::var("STATE_DIRECTORY") {
        let dir = dir.trim().to_string();
        if !dir.is_empty() {
            return Some(PathBuf::from(dir));
        }
    }
    None
}

fn require_state_dir() -> Result<PathBuf> {
    state_dir().ok_or_else(|| anyhow::anyhow!("STATE_DIRECTORY not set"))
}

/// Try to load a previously saved enrollment result from disk.
pub fn load_saved_enrollment() -> Result<Option<EnrollResult>> {
    let dir = match state_dir() {
        Some(d) => d,
        None => return Ok(None),
    };

    let cert_path = dir.join(CERT_FILE);
    let key_path = dir.join(KEY_FILE);
    let ca_path = dir.join(CA_FILE);

    if !cert_path.exists() || !key_path.exists() || !ca_path.exists() {
        return Ok(None);
    }

    let cert_pem = std::fs::read(&cert_path)
        .map_err(|e| anyhow::anyhow!("failed to read saved cert: {}", e))?;
    let key_der =
        std::fs::read(&key_path).map_err(|e| anyhow::anyhow!("failed to read saved key: {}", e))?;
    let ca_pem =
        std::fs::read(&ca_path).map_err(|e| anyhow::anyhow!("failed to read saved CA: {}", e))?;

    let cert_der = crate::enroll::pem_cert_to_der(&cert_pem)?;

    // Check if cert is still valid (not expired)
    let (_not_before, not_after) = crate::enroll::cert_validity(&cert_der)?;
    let now = std::time::SystemTime::now();
    if now >= not_after {
        info!("saved certificate has expired, will re-enroll");
        return Ok(None);
    }

    let spiffe_id = crate::tls::spiffe::extract_spiffe_id(&cert_der)?;
    let cert_chain_der = crate::enroll::cert_chain_to_der(&cert_pem, &ca_pem)?;

    info!("loaded saved certificate for {}", spiffe_id);
    Ok(Some(EnrollResult {
        cert_der,
        cert_chain_der,
        cert_pem,
        ca_pem,
        key_der: Zeroizing::new(key_der),
        spiffe_id,
    }))
}

/// Save enrollment artifacts to disk for reuse across restarts.
pub fn save_enrollment(result: &EnrollResult) -> Result<()> {
    let dir = match state_dir() {
        Some(d) => d,
        None => {
            warn!("STATE_DIRECTORY not set, cannot persist enrollment state");
            return Ok(());
        }
    };

    std::fs::create_dir_all(&dir)?;

    std::fs::write(dir.join(CERT_FILE), &result.cert_pem)?;
    std::fs::write(dir.join(KEY_FILE), result.key_der.as_slice())?;
    std::fs::write(dir.join(CA_FILE), &result.ca_pem)?;

    // Restrict permissions on the key file
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(dir.join(KEY_FILE), std::fs::Permissions::from_mode(0o600))?;
    }

    info!("saved enrollment artifacts to {}", dir.display());
    Ok(())
}

pub fn save_delete_cleanup_request(request: &DeleteCleanupRequest) -> Result<()> {
    let dir = require_state_dir()?;
    std::fs::create_dir_all(&dir)?;
    let path = dir.join(DELETE_REQUEST_FILE);
    std::fs::write(&path, serde_json::to_vec(request)?)?;
    info!("saved delete cleanup request to {}", path.display());
    Ok(())
}

pub fn load_delete_cleanup_request() -> Result<Option<DeleteCleanupRequest>> {
    let dir = match state_dir() {
        Some(d) => d,
        None => return Ok(None),
    };
    let path = dir.join(DELETE_REQUEST_FILE);
    if !path.exists() {
        return Ok(None);
    }

    let data = std::fs::read(&path)
        .map_err(|e| anyhow::anyhow!("failed to read delete cleanup request: {}", e))?;
    let request = serde_json::from_slice::<DeleteCleanupRequest>(&data)
        .map_err(|e| anyhow::anyhow!("failed to parse delete cleanup request: {}", e))?;
    Ok(Some(request))
}

pub fn clear_delete_cleanup_request() -> Result<()> {
    let dir = match state_dir() {
        Some(d) => d,
        None => return Ok(()),
    };
    let path = dir.join(DELETE_REQUEST_FILE);
    match std::fs::remove_file(&path) {
        Ok(()) => {
            info!("cleared delete cleanup request at {}", path.display());
            Ok(())
        }
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(e) => Err(anyhow::anyhow!(
            "failed to clear delete cleanup request: {}",
            e
        )),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{Mutex, OnceLock};

    fn env_lock() -> &'static Mutex<()> {
        static LOCK: OnceLock<Mutex<()>> = OnceLock::new();
        LOCK.get_or_init(|| Mutex::new(()))
    }

    #[test]
    fn delete_cleanup_request_round_trip() {
        let _guard = env_lock().lock().unwrap();
        let temp_root =
            std::env::temp_dir().join(format!("connector-persistence-test-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&temp_root);
        std::fs::create_dir_all(&temp_root).unwrap();
        std::env::set_var("STATE_DIRECTORY", &temp_root);

        let request = DeleteCleanupRequest {
            connector_id: "con-test".to_string(),
            reason: "deleted".to_string(),
        };
        save_delete_cleanup_request(&request).unwrap();
        assert_eq!(load_delete_cleanup_request().unwrap(), Some(request));

        clear_delete_cleanup_request().unwrap();
        assert_eq!(load_delete_cleanup_request().unwrap(), None);

        std::env::remove_var("STATE_DIRECTORY");
        let _ = std::fs::remove_dir_all(&temp_root);
    }
}
