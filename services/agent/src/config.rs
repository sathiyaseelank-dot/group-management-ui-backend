use anyhow::{bail, Result};
use std::env;

#[derive(Debug, Clone)]
pub struct EnrollConfig {
    pub controller_addr: String,
    pub agent_id: String,
    pub trust_domain: String,
    pub token: String,
    pub ca_pem: Vec<u8>,
}

#[derive(Debug, Clone)]
pub struct RunConfig {
    pub controller_addr: String,
    pub connector_addr: String,
    pub agent_id: String,
    pub trust_domain: String,
    pub enrollment_token: String,
    pub ca_pem: Vec<u8>,
    pub tun_name: String,
}

pub fn normalize_trust_domain(v: &str) -> String {
    v.trim().trim_end_matches('.').to_string()
}

pub fn read_credential(name: &str) -> Result<Option<String>> {
    let dir = env::var("CREDENTIALS_DIRECTORY").unwrap_or_default();
    if dir.trim().is_empty() {
        return Ok(None);
    }
    let path = std::path::Path::new(dir.trim()).join(name);
    match std::fs::read_to_string(&path) {
        Ok(s) => Ok(Some(s.trim().to_string())),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(None),
        Err(e) => bail!("failed to read credential {}: {}", name, e),
    }
}

pub fn load_controller_ca() -> Result<Vec<u8>> {
    if let Ok(ca) = env::var("CONTROLLER_CA") {
        let ca = ca.trim().to_string();
        if !ca.is_empty() {
            return Ok(ca.into_bytes());
        }
    }
    if let Some(ca) = read_credential("CONTROLLER_CA")? {
        if !ca.is_empty() {
            return Ok(ca.into_bytes());
        }
    }
    let ca_path = env::var("CONTROLLER_CA_PATH").unwrap_or_default();
    let ca_path = ca_path.trim().to_string();
    if ca_path.is_empty() {
        bail!("CONTROLLER_CA_PATH is not set (explicit controller trust is required)");
    }
    let pem = std::fs::read(&ca_path)
        .map_err(|e| anyhow::anyhow!("failed to read controller CA at {}: {}", ca_path, e))?;
    Ok(pem)
}

pub fn enroll_config_from_env() -> Result<EnrollConfig> {
    let controller_addr = env::var("CONTROLLER_ADDR").unwrap_or_default();
    let mut agent_id = env::var("AGENT_ID").unwrap_or_default();
    if agent_id.trim().is_empty() {
        agent_id = env::var("TUNNELER_ID").unwrap_or_default();
    }
    let mut trust_domain = env::var("TRUST_DOMAIN").unwrap_or_default();
    if trust_domain.is_empty() {
        trust_domain = "mycorp.internal".to_string();
    }
    let trust_domain = normalize_trust_domain(&trust_domain);

    if controller_addr.trim().is_empty() {
        bail!("CONTROLLER_ADDR is not set");
    }
    if agent_id.trim().is_empty() {
        bail!("AGENT_ID is not set (set AGENT_ID or TUNNELER_ID)");
    }

    let mut token = env::var("ENROLLMENT_TOKEN").unwrap_or_default();
    if token.trim().is_empty() {
        token = read_credential("ENROLLMENT_TOKEN")?.unwrap_or_default();
    }
    if token.trim().is_empty() {
        bail!("ENROLLMENT_TOKEN is not set");
    }

    let ca_pem = load_controller_ca()?;

    Ok(EnrollConfig {
        controller_addr: controller_addr.trim().to_string(),
        agent_id: agent_id.trim().to_string(),
        trust_domain,
        token: token.trim().to_string(),
        ca_pem,
    })
}

pub fn run_config_from_env() -> Result<RunConfig> {
    let controller_addr = env::var("CONTROLLER_ADDR").unwrap_or_default();
    let connector_addr = env::var("CONNECTOR_ADDR").unwrap_or_default();
    let mut agent_id = env::var("AGENT_ID").unwrap_or_default();
    if agent_id.trim().is_empty() {
        agent_id = env::var("TUNNELER_ID").unwrap_or_default();
    }
    let mut trust_domain = env::var("TRUST_DOMAIN").unwrap_or_default();
    if trust_domain.is_empty() {
        trust_domain = "mycorp.internal".to_string();
    }
    let trust_domain = normalize_trust_domain(&trust_domain);

    if controller_addr.trim().is_empty() {
        bail!("CONTROLLER_ADDR is not set");
    }
    if connector_addr.trim().is_empty() {
        bail!("CONNECTOR_ADDR is not set");
    }
    if agent_id.trim().is_empty() {
        bail!("AGENT_ID is not set (set AGENT_ID or TUNNELER_ID)");
    }

    let mut enrollment_token = env::var("ENROLLMENT_TOKEN").unwrap_or_default();
    if enrollment_token.trim().is_empty() {
        enrollment_token = read_credential("ENROLLMENT_TOKEN")?.unwrap_or_default();
    }
    if enrollment_token.trim().is_empty() {
        bail!("ENROLLMENT_TOKEN is not set");
    }

    let ca_pem = load_controller_ca()?;

    let tun_name = env::var("TUN_NAME").unwrap_or_else(|_| "tun0".to_string());

    Ok(RunConfig {
        controller_addr: controller_addr.trim().to_string(),
        connector_addr: connector_addr.trim().to_string(),
        agent_id: agent_id.trim().to_string(),
        trust_domain,
        enrollment_token: enrollment_token.trim().to_string(),
        ca_pem,
        tun_name,
    })
}
