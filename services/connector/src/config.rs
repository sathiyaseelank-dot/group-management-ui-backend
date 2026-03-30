use anyhow::{bail, Result};
use std::env;
use std::time::Duration;

#[derive(Debug, Clone)]
pub struct EnrollConfig {
    pub controller_addr: String,
    pub connector_id: String,
    /// Trust domain for verifying the controller's SPIFFE cert.
    pub controller_trust_domain: String,
    pub token: String,
    pub private_ip: String,
    pub version: String,
    pub ca_pem: Vec<u8>,
}

#[derive(Debug, Clone)]
pub struct RunConfig {
    pub controller_addr: String,
    pub connector_id: String,
    /// Trust domain for verifying the controller's SPIFFE cert.
    pub controller_trust_domain: String,
    /// Trust domain for verifying agent SPIFFE certs on the connector listener.
    pub agent_trust_domain: String,
    pub listen_addr: String,
    pub private_ip: String,
    pub stale_grace: Duration,
    pub enrollment_token: String,
    pub ca_pem: Vec<u8>,
    pub controller_http_url: String,
    pub device_tunnel_addr: String,
    pub device_tunnel_advertise_addr: String,
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
        Err(e) => anyhow::bail!("failed to read credential {}: {}", name, e),
    }
}

pub fn load_controller_ca() -> Result<Vec<u8>> {
    // Try env var CONTROLLER_CA first
    if let Ok(ca) = env::var("CONTROLLER_CA") {
        let ca = ca.trim().to_string();
        if !ca.is_empty() {
            return Ok(ca.into_bytes());
        }
    }
    // Try credential file
    if let Some(ca) = read_credential("CONTROLLER_CA")? {
        if !ca.is_empty() {
            return Ok(ca.into_bytes());
        }
    }
    // Fall back to CONTROLLER_CA_PATH
    let ca_path = env::var("CONTROLLER_CA_PATH").unwrap_or_default();
    let ca_path = ca_path.trim().to_string();
    if ca_path.is_empty() {
        bail!("CONTROLLER_CA_PATH is not set (explicit controller trust is required)");
    }
    let pem = std::fs::read(&ca_path)
        .map_err(|e| anyhow::anyhow!("failed to read controller CA at {}: {}", ca_path, e))?;
    Ok(pem)
}

/// Returns `(controller_trust_domain, agent_trust_domain)`.
///
/// - `agent_trust_domain`: `AGENT_TRUST_DOMAIN`, falling back to `TRUST_DOMAIN`,
///   defaulting to `"mycorp.internal"`.
/// - `controller_trust_domain`: `CONTROLLER_TRUST_DOMAIN`, falling back to
///   `TRUST_DOMAIN`, defaulting to `"mycorp.internal"`.
fn load_trust_domains() -> (String, String) {
    let fallback = {
        let v = env::var("TRUST_DOMAIN").unwrap_or_default();
        let v = v.trim().to_string();
        if v.is_empty() {
            "mycorp.internal".to_string()
        } else {
            normalize_trust_domain(&v)
        }
    };
    let agent_trust_domain = {
        let v = env::var("AGENT_TRUST_DOMAIN").unwrap_or_default();
        let v = v.trim().to_string();
        if v.is_empty() {
            fallback.clone()
        } else {
            normalize_trust_domain(&v)
        }
    };
    let controller_trust_domain = {
        let v = env::var("CONTROLLER_TRUST_DOMAIN").unwrap_or_default();
        let v = v.trim().to_string();
        if v.is_empty() {
            fallback
        } else {
            normalize_trust_domain(&v)
        }
    };
    (controller_trust_domain, agent_trust_domain)
}

pub fn enroll_config_from_env() -> Result<EnrollConfig> {
    let controller_addr = env::var("CONTROLLER_ADDR").unwrap_or_default();
    let connector_id = env::var("CONNECTOR_ID").unwrap_or_default();
    let (controller_trust_domain, _) = load_trust_domains();

    if controller_addr.trim().is_empty() {
        bail!("CONTROLLER_ADDR is not set");
    }
    if connector_id.trim().is_empty() {
        bail!("CONNECTOR_ID is not set");
    }

    let mut token = env::var("ENROLLMENT_TOKEN").unwrap_or_default();
    if token.trim().is_empty() {
        token = read_credential("ENROLLMENT_TOKEN")?.unwrap_or_default();
    }
    if token.trim().is_empty() {
        bail!("ENROLLMENT_TOKEN is not set");
    }

    let private_ip = crate::net_util::resolve_private_ip(&controller_addr)?;
    let version = resolve_version();
    let ca_pem = load_controller_ca()?;

    Ok(EnrollConfig {
        controller_addr: controller_addr.trim().to_string(),
        connector_id: connector_id.trim().to_string(),
        controller_trust_domain,
        token: token.trim().to_string(),
        private_ip,
        version,
        ca_pem,
    })
}

pub fn run_config_from_env() -> Result<RunConfig> {
    let controller_addr = env::var("CONTROLLER_ADDR").unwrap_or_default();
    let connector_id = env::var("CONNECTOR_ID").unwrap_or_default();
    let (controller_trust_domain, agent_trust_domain) = load_trust_domains();
    let listen_addr_env = env::var("CONNECTOR_LISTEN_ADDR").unwrap_or_default();

    let stale_grace = {
        let v = env::var("POLICY_STALE_GRACE_SECONDS").unwrap_or_default();
        if let Ok(secs) = v.trim().parse::<u64>() {
            if secs > 0 {
                Duration::from_secs(secs)
            } else {
                Duration::from_secs(600)
            }
        } else {
            Duration::from_secs(600)
        }
    };

    if controller_addr.trim().is_empty() {
        bail!("CONTROLLER_ADDR is not set");
    }
    if connector_id.trim().is_empty() {
        bail!("CONNECTOR_ID is not set");
    }
    let mut enrollment_token = env::var("ENROLLMENT_TOKEN").unwrap_or_default();
    if enrollment_token.trim().is_empty() {
        enrollment_token = read_credential("ENROLLMENT_TOKEN")?.unwrap_or_default();
    }
    if enrollment_token.trim().is_empty() {
        bail!("ENROLLMENT_TOKEN is required for enrollment");
    }

    let private_ip = crate::net_util::resolve_private_ip(&controller_addr)?;
    let listen_addr = if listen_addr_env.trim().is_empty() {
        format!("{}:9443", private_ip)
    } else {
        listen_addr_env.trim().to_string()
    };
    let controller_http_url = env::var("CONTROLLER_HTTP_URL").unwrap_or_default();
    let controller_http_url = controller_http_url.trim().to_string();
    let device_tunnel_addr =
        env::var("DEVICE_TUNNEL_ADDR").unwrap_or_else(|_| format!("{}:9444", private_ip));
    let device_tunnel_addr = device_tunnel_addr.trim().to_string();
    let device_tunnel_advertise_addr = env::var("DEVICE_TUNNEL_ADVERTISE_ADDR")
        .unwrap_or_else(|_| default_device_tunnel_advertise_addr(&device_tunnel_addr, &private_ip));
    let device_tunnel_advertise_addr = device_tunnel_advertise_addr.trim().to_string();

    let ca_pem = load_controller_ca()?;

    Ok(RunConfig {
        controller_addr: controller_addr.trim().to_string(),
        connector_id: connector_id.trim().to_string(),
        controller_trust_domain,
        agent_trust_domain,
        listen_addr,
        private_ip,
        stale_grace,
        enrollment_token: enrollment_token.trim().to_string(),
        ca_pem,
        controller_http_url,
        device_tunnel_addr,
        device_tunnel_advertise_addr,
    })
}

fn resolve_version() -> String {
    if let Ok(v) = env::var("CONNECTOR_VERSION") {
        let v = v.trim().to_string();
        if !v.is_empty() {
            return v;
        }
    }
    crate::buildinfo::version().to_string()
}

fn default_device_tunnel_advertise_addr(bind_addr: &str, private_ip: &str) -> String {
    let bind_addr = bind_addr.trim();
    if let Some((host, port)) = bind_addr.rsplit_once(':') {
        let host = host.trim_matches(['[', ']']).trim();
        if host.is_empty() || host == "0.0.0.0" || host == "::" {
            return format!("{}:{}", private_ip.trim(), port);
        }
    }
    bind_addr.to_string()
}
