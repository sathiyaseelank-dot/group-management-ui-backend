use anyhow::Result;
use std::path::PathBuf;
use tracing::{info, warn};

use crate::enroll::EnrollResult;
use zeroize::Zeroizing;

const CERT_FILE: &str = "cert.pem";
const KEY_FILE: &str = "key.der";
const CA_FILE: &str = "ca.pem";

fn state_dir() -> Option<PathBuf> {
    if let Ok(dir) = std::env::var("STATE_DIRECTORY") {
        let dir = dir.trim().to_string();
        if !dir.is_empty() {
            return Some(PathBuf::from(dir));
        }
    }
    if let Ok(dir) = std::env::var("XDG_STATE_HOME") {
        let dir = dir.trim().to_string();
        if !dir.is_empty() {
            return Some(PathBuf::from(dir).join("ztna-agent"));
        }
    }
    if let Ok(home) = std::env::var("HOME") {
        let home = home.trim().to_string();
        if !home.is_empty() {
            return Some(PathBuf::from(home).join(".local/state/ztna-agent"));
        }
    }
    None
}

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
    let key_der = std::fs::read(&key_path)
        .map_err(|e| anyhow::anyhow!("failed to read saved key: {}", e))?;
    let ca_pem = std::fs::read(&ca_path)
        .map_err(|e| anyhow::anyhow!("failed to read saved CA: {}", e))?;

    let cert_der = crate::enroll::pem_cert_to_der(&cert_pem)?;

    let (_not_before, not_after) = crate::enroll::cert_validity(&cert_der)?;
    let now = std::time::SystemTime::now();
    if now >= not_after {
        info!("saved certificate has expired, will re-enroll");
        return Ok(None);
    }

    let spiffe_id = crate::tls::spiffe::extract_spiffe_id(&cert_der)?;

    info!("loaded saved certificate for {}", spiffe_id);
    Ok(Some(EnrollResult {
        cert_der,
        cert_pem,
        ca_pem,
        key_der: Zeroizing::new(key_der),
        spiffe_id,
    }))
}

// ── Firewall state persistence ──────────────────────────────────────

const FIREWALL_STATE_FILE: &str = "firewall_state.json";

pub fn save_firewall_state(state: &crate::firewall::FirewallState) -> Result<()> {
    let dir = match state_dir() {
        Some(d) => d,
        None => {
            warn!("STATE_DIRECTORY not set, cannot persist firewall state");
            return Ok(());
        }
    };
    std::fs::create_dir_all(&dir)?;
    let data = serde_json::to_vec_pretty(state)
        .map_err(|e| anyhow::anyhow!("failed to serialize firewall state: {}", e))?;
    std::fs::write(dir.join(FIREWALL_STATE_FILE), data)?;
    info!("saved firewall state to {}", dir.display());
    Ok(())
}

pub fn load_firewall_state() -> Result<Option<crate::firewall::FirewallState>> {
    let dir = match state_dir() {
        Some(d) => d,
        None => return Ok(None),
    };
    let path = dir.join(FIREWALL_STATE_FILE);
    if !path.exists() {
        return Ok(None);
    }
    let data = std::fs::read(&path)
        .map_err(|e| anyhow::anyhow!("failed to read firewall state: {}", e))?;
    let state: crate::firewall::FirewallState = serde_json::from_slice(&data)
        .map_err(|e| anyhow::anyhow!("failed to parse firewall state: {}", e))?;
    info!("loaded firewall state from {}", path.display());
    Ok(Some(state))
}

// ── Discovery state persistence ────────────────────────────────────

const DISCOVERY_STATE_FILE: &str = "discovery_state.json";

#[derive(serde::Serialize, serde::Deserialize)]
pub struct DiscoveryState {
    pub services: Vec<DiscoveryServiceEntry>,
    pub fingerprint: u64,
}

#[derive(serde::Serialize, serde::Deserialize)]
pub struct DiscoveryServiceEntry {
    pub port: u16,
    pub protocol: String,
}

pub fn save_discovery_state(state: &DiscoveryState) -> Result<()> {
    let dir = match state_dir() {
        Some(d) => d,
        None => {
            warn!("STATE_DIRECTORY not set, cannot persist discovery state");
            return Ok(());
        }
    };
    std::fs::create_dir_all(&dir)?;
    let data = serde_json::to_vec_pretty(state)
        .map_err(|e| anyhow::anyhow!("failed to serialize discovery state: {}", e))?;
    std::fs::write(dir.join(DISCOVERY_STATE_FILE), data)?;
    info!("saved discovery state to {}", dir.display());
    Ok(())
}

pub fn load_discovery_state() -> Result<Option<DiscoveryState>> {
    let dir = match state_dir() {
        Some(d) => d,
        None => return Ok(None),
    };
    let path = dir.join(DISCOVERY_STATE_FILE);
    if !path.exists() {
        return Ok(None);
    }
    let data = std::fs::read(&path)
        .map_err(|e| anyhow::anyhow!("failed to read discovery state: {}", e))?;
    let state: DiscoveryState = serde_json::from_slice(&data)
        .map_err(|e| anyhow::anyhow!("failed to parse discovery state: {}", e))?;
    info!("loaded discovery state from {}", path.display());
    Ok(Some(state))
}

// ── Enrollment persistence ─────────────────────────────────────────

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

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(dir.join(KEY_FILE), std::fs::Permissions::from_mode(0o600))?;
    }

    info!("saved enrollment artifacts to {}", dir.display());
    Ok(())
}
