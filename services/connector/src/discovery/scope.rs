use ipnet::IpNet;
use std::net::IpAddr;

#[derive(Debug)]
pub struct ScanScope {
    pub targets: Vec<IpAddr>,
}

pub fn resolve_scope(cidrs: &[String], max_targets: u32) -> Result<ScanScope, String> {
    let mut targets = Vec::new();

    for c in cidrs {
        let net: IpNet = c.parse().map_err(|_| format!("invalid cidr: {}", c))?;

        for ip in net.hosts() {
            if is_invalid_target(&ip) {
                continue;
            }

            targets.push(ip);

            if targets.len() as u32 >= max_targets {
                return Ok(ScanScope { targets });
            }
        }
    }

    Ok(ScanScope { targets })
}

fn is_invalid_target(ip: &IpAddr) -> bool {
    match ip {
        IpAddr::V4(v4) => v4.is_loopback() || v4.is_multicast() || v4.is_unspecified(),
        IpAddr::V6(v6) => v6.is_loopback() || v6.is_multicast() || v6.is_unspecified(),
    }
}
