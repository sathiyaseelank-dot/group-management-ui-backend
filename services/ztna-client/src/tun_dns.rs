#[allow(unused_imports)]
use std::collections::HashMap;
use std::net::IpAddr;

use tracing::warn;

use crate::token_store::StoredResource;

/// Resolved resource: an IP address mapped back to the original address string
/// (domain or IP) and resource metadata needed for ACL + tunnel.
#[derive(Debug, Clone)]
#[allow(dead_code)]
pub struct ResolvedResource {
    pub ip: IpAddr,
    pub original_address: String,
    pub name: String,
    pub port_from: Option<i32>,
    pub port_to: Option<i32>,
    pub protocol: String,
}

/// Resolve a list of `StoredResource` entries to IP addresses.
/// Domain-based addresses are resolved via the system DNS resolver.
/// Returns one `ResolvedResource` per IP (multiple IPs possible per domain).
pub async fn resolve_resources(resources: &[StoredResource]) -> Vec<ResolvedResource> {
    let mut out = Vec::new();
    for r in resources {
        let addr = r.address.trim();
        if addr.is_empty() {
            continue;
        }

        // Try parsing as an IP first
        if let Ok(ip) = addr.parse::<IpAddr>() {
            out.push(ResolvedResource {
                ip,
                original_address: addr.to_string(),
                name: r.name.clone(),
                port_from: r.port_from,
                port_to: r.port_to,
                protocol: r.protocol.clone(),
            });
            continue;
        }

        // Domain name — resolve via system DNS
        let lookup_addr = format!("{}:0", addr);
        let resolved = tokio::net::lookup_host(lookup_addr).await;
        match resolved {
            Ok(addrs) => {
                let mut seen = std::collections::HashSet::new();
                for sock_addr in addrs {
                    let ip = sock_addr.ip();
                    if seen.insert(ip) {
                        out.push(ResolvedResource {
                            ip,
                            original_address: addr.to_string(),
                            name: r.name.clone(),
                            port_from: r.port_from,
                            port_to: r.port_to,
                            protocol: r.protocol.clone(),
                        });
                    }
                }
            }
            Err(e) => {
                warn!("[tun-dns] failed to resolve {}: {}", addr, e);
            }
        }
    }
    out
}

/// Build a reverse lookup map: IP -> original domain/address.
#[allow(dead_code)]
pub fn build_reverse_map(resolved: &[ResolvedResource]) -> HashMap<IpAddr, String> {
    let mut map = HashMap::new();
    for r in resolved {
        map.insert(r.ip, r.original_address.clone());
    }
    map
}
