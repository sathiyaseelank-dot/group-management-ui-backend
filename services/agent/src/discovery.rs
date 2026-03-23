use anyhow::Result;
use std::collections::HashSet;
use std::hash::{Hash, Hasher};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr};
use tracing::{info, warn};

use crate::enroll::pb::ControlMessage;
use crate::firewall::FirewallEnforcer;

/// Ports to ignore (system noise).
const IGNORED_PORTS: &[(u16, &str)] = &[(5355, "LLMNR"), (631, "IPP"), (5353, "mDNS")];

/// Start of ephemeral port range.
const EPHEMERAL_PORT_START: u16 = 32768;

#[derive(Debug, Clone, serde::Serialize)]
pub struct DiscoveredService {
    pub protocol: &'static str,
    pub port: u16,
    pub bound_ip: String,
    pub service_name: String,
    pub process_name: Option<String>,
}

/// Static lookup of well-known port numbers to service names.
pub fn service_from_port(port: u16) -> &'static str {
    match port {
        21 => "FTP",
        22 => "SSH",
        25 => "SMTP",
        53 => "DNS",
        80 => "HTTP",
        110 => "POP3",
        143 => "IMAP",
        443 => "HTTPS",
        445 => "SMB",
        465 => "SMTPS",
        587 => "SMTP",
        993 => "IMAPS",
        995 => "POP3S",
        1433 => "MSSQL",
        1521 => "Oracle",
        2375 => "Docker",
        2376 => "Docker TLS",
        3000 => "Dev Server",
        3306 => "MySQL",
        3389 => "RDP",
        4222 => "NATS",
        5432 => "PostgreSQL",
        5672 => "RabbitMQ",
        5900 => "VNC",
        6379 => "Redis",
        6443 => "Kubernetes API",
        8080 => "HTTP Proxy",
        8081 => "HTTP Alt",
        8443 => "gRPC/TLS",
        8888 => "HTTP Alt",
        9090 => "Prometheus",
        9200 => "Elasticsearch",
        9443 => "Connector",
        11211 => "Memcached",
        15672 => "RabbitMQ Mgmt",
        27017 => "MongoDB",
        _ => "",
    }
}

fn is_externally_listening(addr: &SocketAddr) -> bool {
    let ip = addr.ip();
    if ip.is_loopback() {
        return false;
    }
    true
}

fn is_ignored_port(port: u16) -> bool {
    IGNORED_PORTS.iter().any(|(p, _)| *p == port)
}

fn include_ephemeral() -> bool {
    std::env::var("DISCOVERY_INCLUDE_EPHEMERAL")
        .map(|v| v == "true" || v == "1")
        .unwrap_or(false)
}

fn should_include_port(port: u16) -> bool {
    if is_ignored_port(port) {
        return false;
    }
    if port >= EPHEMERAL_PORT_START && !include_ephemeral() {
        // Include ephemeral ports only if they map to a well-known service
        return !service_from_port(port).is_empty();
    }
    true
}

// ── Linux: direct /proc/net parser (no CAP_SYS_PTRACE needed) ──────────

#[cfg(target_os = "linux")]
mod platform {
    use super::*;

    struct ProcEntry {
        addr: SocketAddr,
        inode: u64,
    }

    fn parse_proc_ipv4(hex: &str) -> Option<Ipv4Addr> {
        let n = u32::from_str_radix(hex, 16).ok()?;
        Some(Ipv4Addr::from(n.to_be()))
    }

    fn parse_proc_ipv6(hex: &str) -> Option<Ipv6Addr> {
        if hex.len() != 32 {
            return None;
        }
        let mut octets = [0u8; 16];
        for i in 0..4 {
            let word = u32::from_str_radix(&hex[i * 8..(i + 1) * 8], 16).ok()?;
            let bytes = word.to_be().to_le_bytes();
            octets[i * 4..i * 4 + 4].copy_from_slice(&bytes);
        }
        Some(Ipv6Addr::from(octets))
    }

    /// Parse /proc/net/tcp{,6} for LISTEN sockets (state 0A), returning address + inode.
    fn parse_proc_tcp(path: &str, is_v6: bool) -> Vec<ProcEntry> {
        let content = match std::fs::read_to_string(path) {
            Ok(c) => c,
            Err(_) => return vec![],
        };
        let mut results = vec![];
        for line in content.lines().skip(1) {
            let fields: Vec<&str> = line.split_whitespace().collect();
            if fields.len() < 10 {
                continue;
            }
            if fields[3] != "0A" {
                continue;
            }
            let parts: Vec<&str> = fields[1].split(':').collect();
            if parts.len() != 2 {
                continue;
            }
            let port = match u16::from_str_radix(parts[1], 16) {
                Ok(p) => p,
                Err(_) => continue,
            };
            let ip: IpAddr = if is_v6 {
                match parse_proc_ipv6(parts[0]) {
                    Some(v6) => IpAddr::V6(v6),
                    None => continue,
                }
            } else {
                match parse_proc_ipv4(parts[0]) {
                    Some(v4) => IpAddr::V4(v4),
                    None => continue,
                }
            };
            let inode = fields[9].parse::<u64>().unwrap_or(0);
            results.push(ProcEntry {
                addr: SocketAddr::new(ip, port),
                inode,
            });
        }
        results
    }

    /// Check if process name detection is explicitly opted-in.
    /// Disabled by default: scanning /proc/*/fd/* requires root and exposes
    /// the full process inventory, which is an unnecessary privilege escalation
    /// surface if the agent is compromised.
    fn process_names_enabled() -> bool {
        std::env::var("DISCOVERY_PROCESS_NAMES")
            .map(|v| v == "true" || v == "1")
            .unwrap_or(false)
    }

    /// Find the process name that owns the given socket inode by scanning /proc/*/fd/*.
    /// Only called when DISCOVERY_PROCESS_NAMES=true.
    fn process_name_for_inode(inode: u64) -> Option<String> {
        if inode == 0 {
            return None;
        }
        let target = format!("socket:[{}]", inode);
        let proc_dir = match std::fs::read_dir("/proc") {
            Ok(d) => d,
            Err(_) => return None,
        };
        for entry in proc_dir.flatten() {
            let name = entry.file_name();
            let name_str = name.to_string_lossy();
            if !name_str.chars().all(|c| c.is_ascii_digit()) {
                continue;
            }
            let fd_dir = entry.path().join("fd");
            let fds = match std::fs::read_dir(&fd_dir) {
                Ok(d) => d,
                Err(_) => continue,
            };
            for fd_entry in fds.flatten() {
                match std::fs::read_link(fd_entry.path()) {
                    Ok(link) if link.to_string_lossy() == target => {
                        let comm_path = entry.path().join("comm");
                        return std::fs::read_to_string(comm_path)
                            .ok()
                            .map(|s| s.trim().to_string());
                    }
                    _ => continue,
                }
            }
        }
        None
    }

    pub fn discover_sync() -> Result<Vec<DiscoveredService>> {
        let mut entries = parse_proc_tcp("/proc/net/tcp", false);
        entries.extend(parse_proc_tcp("/proc/net/tcp6", true));
        info!("discovery: raw TCP listener count = {}", entries.len());

        let mut exposed = Vec::new();
        for entry in &entries {
            if !is_externally_listening(&entry.addr) {
                continue;
            }
            let port = entry.addr.port();
            if !should_include_port(port) {
                continue;
            }
            let svc_name = service_from_port(port);
            let proc_name = if process_names_enabled() {
                process_name_for_inode(entry.inode)
            } else {
                None
            };
            exposed.push(DiscoveredService {
                protocol: "tcp",
                port,
                bound_ip: entry.addr.ip().to_string(),
                service_name: svc_name.to_string(),
                process_name: proc_name,
            });
        }

        info!(
            "discovery: externally-listening services = {}",
            exposed.len()
        );
        Ok(exposed)
    }
}

// ── Non-Linux: fall back to `listeners` crate ───────────────────────────

#[cfg(not(target_os = "linux"))]
mod platform {
    use super::*;

    pub fn discover_sync() -> Result<Vec<DiscoveredService>> {
        let all = listeners::get_all()?;
        info!("discovery: listeners crate returned {} entries", all.len());
        let mut exposed = Vec::new();
        for l in all {
            let addr: SocketAddr = match l.socket.parse() {
                Ok(a) => a,
                Err(_) => continue,
            };
            if !is_externally_listening(&addr) {
                continue;
            }
            let port = addr.port();
            if !should_include_port(port) {
                continue;
            }
            let svc_name = service_from_port(port);
            exposed.push(DiscoveredService {
                protocol: "tcp",
                port,
                bound_ip: addr.ip().to_string(),
                service_name: svc_name.to_string(),
                process_name: None,
            });
        }
        info!(
            "discovery: externally-listening services = {}",
            exposed.len()
        );
        Ok(exposed)
    }
}

async fn discover_exposed_services() -> Result<Vec<DiscoveredService>> {
    tokio::task::spawn_blocking(platform::discover_sync).await?
}

/// Compute a fingerprint over the current set of (port, protocol) tuples.
fn compute_fingerprint(ports: &HashSet<(u16, String)>) -> u64 {
    let mut sorted: Vec<_> = ports.iter().collect();
    sorted.sort();
    let mut hasher = std::collections::hash_map::DefaultHasher::new();
    for entry in sorted {
        entry.hash(&mut hasher);
    }
    hasher.finish()
}

/// Shared scan logic: scan ports, filter protected ports, return (services, current_port_set).
async fn scan_services(
    enforcer: &FirewallEnforcer,
) -> Result<(Vec<DiscoveredService>, HashSet<(u16, String)>)> {
    let services = discover_exposed_services().await?;

    let fw_state = enforcer.get_state().await;
    let protected: HashSet<(u16, String)> = fw_state
        .protected_ports
        .iter()
        .map(|r| (r.port, r.protocol.clone()))
        .collect();

    let filtered: Vec<DiscoveredService> = services
        .into_iter()
        .filter(|svc| !protected.contains(&(svc.port, svc.protocol.to_string())))
        .collect();

    let current_ports: HashSet<(u16, String)> = filtered
        .iter()
        .map(|svc| (svc.port, svc.protocol.to_string()))
        .collect();

    Ok((filtered, current_ports))
}

/// Run a differential discovery scan. Sends only added/removed services.
/// Returns `Ok(true)` if a diff was sent, `Ok(false)` if nothing changed.
pub async fn run_discovery_diff(
    agent_id: &str,
    enforcer: &FirewallEnforcer,
    stream_tx: &tokio::sync::mpsc::Sender<ControlMessage>,
    sent_services: &mut HashSet<(u16, String)>,
    last_fingerprint: &mut u64,
    seq: &mut u64,
    dirty: &mut bool,
) -> Result<bool> {
    info!("discovery: starting diff scan for agent_id={}", agent_id);

    let (filtered, current_ports) = scan_services(enforcer).await?;

    let fingerprint = compute_fingerprint(&current_ports);
    if fingerprint == *last_fingerprint {
        return Ok(false);
    }

    // Compute added
    let added: Vec<&DiscoveredService> = filtered
        .iter()
        .filter(|svc| !sent_services.contains(&(svc.port, svc.protocol.to_string())))
        .collect();

    // Compute removed
    let removed: Vec<(u16, String)> = sent_services
        .iter()
        .filter(|p| !current_ports.contains(p))
        .cloned()
        .collect();

    if added.is_empty() && removed.is_empty() {
        *last_fingerprint = fingerprint;
        return Ok(false);
    }

    *seq += 1;

    #[derive(serde::Serialize)]
    struct AddedEntry<'a> {
        protocol: &'static str,
        port: u16,
        bound_ip: &'a str,
        service_name: &'a str,
        #[serde(skip_serializing_if = "Option::is_none")]
        process_name: Option<&'a str>,
    }

    #[derive(serde::Serialize)]
    struct RemovedEntry {
        protocol: String,
        port: u16,
    }

    #[derive(serde::Serialize)]
    struct DiffPayload<'a> {
        agent_id: &'a str,
        seq: u64,
        added: Vec<AddedEntry<'a>>,
        removed: Vec<RemovedEntry>,
    }

    let payload = DiffPayload {
        agent_id,
        seq: *seq,
        added: added
            .iter()
            .map(|svc| AddedEntry {
                protocol: svc.protocol,
                port: svc.port,
                bound_ip: &svc.bound_ip,
                service_name: &svc.service_name,
                process_name: svc.process_name.as_deref(),
            })
            .collect(),
        removed: removed
            .iter()
            .map(|(port, proto)| RemovedEntry {
                protocol: proto.clone(),
                port: *port,
            })
            .collect(),
    };

    info!(
        "discovery: diff seq={} added={} removed={}",
        *seq,
        added.len(),
        removed.len()
    );

    let payload_bytes = serde_json::to_vec(&payload)?;
    if let Err(e) = stream_tx
        .send(ControlMessage {
            r#type: "agent_discovery_diff".to_string(),
            payload: payload_bytes,
            ..Default::default()
        })
        .await
    {
        warn!("discovery: failed to send diff: {}", e);
        return Ok(false);
    }

    // Update sent_services
    for svc in &added {
        sent_services.insert((svc.port, svc.protocol.to_string()));
    }
    for key in &removed {
        sent_services.remove(key);
    }

    *last_fingerprint = fingerprint;
    *dirty = true;
    Ok(true)
}

/// Run a full discovery sync. Sends complete snapshot for controller-side reconciliation.
/// Returns `Ok(true)` always (a full sync is always sent).
pub async fn run_discovery_full_sync(
    agent_id: &str,
    enforcer: &FirewallEnforcer,
    stream_tx: &tokio::sync::mpsc::Sender<ControlMessage>,
    sent_services: &mut HashSet<(u16, String)>,
    last_fingerprint: &mut u64,
    seq: &mut u64,
    dirty: &mut bool,
) -> Result<bool> {
    info!("discovery: starting full sync for agent_id={}", agent_id);

    let (filtered, current_ports) = scan_services(enforcer).await?;

    *seq += 1;
    let fingerprint = compute_fingerprint(&current_ports);

    #[derive(serde::Serialize)]
    struct ServiceEntry<'a> {
        protocol: &'static str,
        port: u16,
        bound_ip: &'a str,
        service_name: &'a str,
        #[serde(skip_serializing_if = "Option::is_none")]
        process_name: Option<&'a str>,
    }

    #[derive(serde::Serialize)]
    struct FullSyncPayload<'a> {
        agent_id: &'a str,
        seq: u64,
        services: Vec<ServiceEntry<'a>>,
        fingerprint: u64,
    }

    let payload = FullSyncPayload {
        agent_id,
        seq: *seq,
        services: filtered
            .iter()
            .map(|svc| ServiceEntry {
                protocol: svc.protocol,
                port: svc.port,
                bound_ip: &svc.bound_ip,
                service_name: &svc.service_name,
                process_name: svc.process_name.as_deref(),
            })
            .collect(),
        fingerprint,
    };

    info!(
        "discovery: full_sync seq={} services={}",
        *seq,
        filtered.len()
    );

    let payload_bytes = serde_json::to_vec(&payload)?;
    stream_tx
        .send(ControlMessage {
            r#type: "agent_discovery_full_sync".to_string(),
            payload: payload_bytes,
            ..Default::default()
        })
        .await
        .map_err(|e| anyhow::anyhow!("send error: {}", e))?;

    // Update sent_services to match exactly what we just reported
    sent_services.clear();
    for svc in &filtered {
        sent_services.insert((svc.port, svc.protocol.to_string()));
    }
    *last_fingerprint = fingerprint;
    *dirty = true;
    Ok(true)
}

/// Backward-compatible: Run a discovery scan (old protocol).
/// Returns `Ok(true)` if a report was sent.
#[allow(dead_code)]
pub async fn run_discovery_scan(
    agent_id: &str,
    enforcer: &FirewallEnforcer,
    stream_tx: &tokio::sync::mpsc::Sender<ControlMessage>,
    sent_services: &mut HashSet<(u16, String)>,
    last_fingerprint: &mut u64,
) -> Result<bool> {
    info!("discovery: starting scan for agent_id={}", agent_id);

    let (filtered, current_ports) = scan_services(enforcer).await?;

    info!(
        "discovery: protected ports filtered, remaining={}",
        filtered.len()
    );
    info!("discovery: already-sent ports = {:?}", sent_services);

    // ── Gone detection ──
    let gone: Vec<(u16, String)> = sent_services
        .iter()
        .filter(|p| !current_ports.contains(p))
        .cloned()
        .collect();

    let mut had_changes = false;

    if !gone.is_empty() {
        info!(
            "discovery: {} service(s) gone: {:?}",
            gone.len(),
            gone.iter()
                .map(|(p, proto)| format!("{}:{}", proto, p))
                .collect::<Vec<_>>()
        );

        #[derive(serde::Serialize)]
        struct GoneEntry {
            protocol: String,
            port: u16,
        }

        #[derive(serde::Serialize)]
        struct GonePayload<'a> {
            agent_id: &'a str,
            services: Vec<GoneEntry>,
        }

        let payload = GonePayload {
            agent_id,
            services: gone
                .iter()
                .map(|(port, proto)| GoneEntry {
                    protocol: proto.clone(),
                    port: *port,
                })
                .collect(),
        };

        let payload_bytes = serde_json::to_vec(&payload)?;
        if let Err(e) = stream_tx
            .send(ControlMessage {
                r#type: "agent_discovery_gone".to_string(),
                payload: payload_bytes,
                ..Default::default()
            })
            .await
        {
            warn!("discovery: failed to send gone message: {}", e);
        }

        for key in &gone {
            sent_services.remove(key);
        }
        had_changes = true;
    }

    // ── Fingerprint check ─
    let fingerprint = compute_fingerprint(&current_ports);
    if fingerprint == *last_fingerprint && !had_changes {
        info!("discovery: fingerprint unchanged, skipping");
        return Ok(false);
    }

    let all_services: Vec<&DiscoveredService> = filtered.iter().collect();

    let new_count = all_services
        .iter()
        .filter(|svc| !sent_services.contains(&(svc.port, svc.protocol.to_string())))
        .count();

    if new_count == 0 && !had_changes {
        info!("discovery: no new services to report");
        *last_fingerprint = fingerprint;
        return Ok(false);
    }

    if !all_services.is_empty() || had_changes {
        info!(
            "discovery: reporting {} service(s) ({} new)",
            all_services.len(),
            new_count,
        );

        #[derive(serde::Serialize)]
        struct ServiceEntry<'a> {
            protocol: &'static str,
            port: u16,
            bound_ip: &'a str,
            service_name: &'a str,
            #[serde(skip_serializing_if = "Option::is_none")]
            process_name: Option<&'a str>,
        }

        #[derive(serde::Serialize)]
        struct Payload<'a> {
            agent_id: &'a str,
            services: Vec<ServiceEntry<'a>>,
        }

        let payload = Payload {
            agent_id,
            services: all_services
                .iter()
                .map(|svc| ServiceEntry {
                    protocol: svc.protocol,
                    port: svc.port,
                    bound_ip: &svc.bound_ip,
                    service_name: &svc.service_name,
                    process_name: svc.process_name.as_deref(),
                })
                .collect(),
        };

        let payload_bytes = serde_json::to_vec(&payload)?;
        stream_tx
            .send(ControlMessage {
                r#type: "agent_discovery_report".to_string(),
                payload: payload_bytes,
                ..Default::default()
            })
            .await
            .map_err(|e| anyhow::anyhow!("send error: {}", e))?;

        for svc in &all_services {
            sent_services.insert((svc.port, svc.protocol.to_string()));
        }
        had_changes = true;
    }

    *last_fingerprint = fingerprint;
    Ok(had_changes)
}
