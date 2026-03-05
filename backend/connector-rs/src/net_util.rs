use anyhow::{bail, Result};
use std::net::UdpSocket;

/// Discover the outbound private IP by opening a UDP socket toward the controller host.
pub fn resolve_private_ip(controller_addr: &str) -> Result<String> {
    if let Ok(ip) = std::env::var("CONNECTOR_PRIVATE_IP") {
        let ip = ip.trim().to_string();
        if !ip.is_empty() {
            return Ok(ip);
        }
    }
    discover_private_ip(controller_addr)
}

fn discover_private_ip(controller_addr: &str) -> Result<String> {
    let host = controller_host(controller_addr)?;
    // Use UDP routing trick: connect (no packet sent) to learn the local address.
    let target = format!("{}:53", host);
    let sock = UdpSocket::bind("0.0.0.0:0")
        .map_err(|e| anyhow::anyhow!("failed to bind UDP socket: {}", e))?;
    sock.connect(&target)
        .map_err(|e| anyhow::anyhow!("failed to determine private IP: {}", e))?;
    let local = sock.local_addr()
        .map_err(|e| anyhow::anyhow!("failed to determine private IP: {}", e))?;
    Ok(local.ip().to_string())
}

fn controller_host(controller_addr: &str) -> Result<String> {
    if controller_addr.contains("://") {
        bail!("CONTROLLER_ADDR must be host:port");
    }
    // Split host:port
    match controller_addr.rsplit_once(':') {
        Some((host, _port)) if !host.is_empty() => Ok(host.trim_matches('[').trim_matches(']').to_string()),
        _ => bail!("CONTROLLER_ADDR must be host:port"),
    }
}
