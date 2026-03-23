use anyhow::Result;
use std::path::PathBuf;
use tracing::{info, warn};

use crate::enroll::EnrollResult;
use zeroize::Zeroizing;

const CERT_FILE: &str = "cert.pem";
const KEY_FILE: &str = "key.der";
const CA_FILE: &str = "ca.pem";

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
