use serde::{Deserialize, Serialize};
use std::net::IpAddr;
use std::sync::Arc;
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use tokio::sync::Semaphore;
use tracing::{error, info, warn};

use super::scope;
use super::tcp_ping::tcp_connect_ping;
use super::tcp_probe::probe_tcp;

const MAX_TARGETS: u32 = 512;
const MAX_PORTS: usize = 16;
const MAX_TIMEOUT_SEC: u64 = 60;
const MAX_CONCURRENCY: usize = 32;

#[derive(Debug, Deserialize)]
pub struct ScanCommand {
    pub request_id: String,
    pub targets: Vec<String>,
    pub ports: Vec<u16>,
    #[serde(default = "default_max_targets")]
    pub max_targets: u32,
    #[serde(default = "default_timeout")]
    pub timeout_sec: u64,
}

fn default_max_targets() -> u32 {
    MAX_TARGETS
}

fn default_timeout() -> u64 {
    5
}

#[derive(Debug, Serialize)]
pub struct DiscoveredResource {
    pub id: String,
    pub ip: String,
    pub port: u16,
    pub protocol: String,
    pub reachable_from: String,
    pub first_seen: u64,
}

#[derive(Debug, Serialize)]
pub struct ScanReport {
    pub request_id: String,
    pub results: Vec<DiscoveredResource>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

pub async fn execute_scan(cmd: ScanCommand, connector_id: &str) -> ScanReport {
    let request_id = cmd.request_id.clone();

    // Validate limits
    if cmd.targets.is_empty() {
        return ScanReport {
            request_id,
            results: vec![],
            error: Some("no targets specified".into()),
        };
    }
    if cmd.ports.is_empty() {
        return ScanReport {
            request_id,
            results: vec![],
            error: Some("no ports specified".into()),
        };
    }
    if cmd.ports.len() > MAX_PORTS {
        return ScanReport {
            request_id,
            results: vec![],
            error: Some(format!("too many ports (max {})", MAX_PORTS)),
        };
    }
    let max_targets = cmd.max_targets.min(MAX_TARGETS);
    let timeout_sec = cmd.timeout_sec.min(MAX_TIMEOUT_SEC);
    let timeout = Duration::from_secs(timeout_sec);

    // Resolve scope
    let scan_scope = match scope::resolve_scope(&cmd.targets, max_targets) {
        Ok(s) => s,
        Err(e) => {
            error!("failed to resolve scope: {}", e);
            return ScanReport {
                request_id,
                results: vec![],
                error: Some(e),
            };
        }
    };

    info!(
        "scan {}: {} targets, {} ports",
        request_id,
        scan_scope.targets.len(),
        cmd.ports.len()
    );

    // Host discovery via TCP connect-ping
    let ping_port = cmd.ports[0];
    let ping_timeout = Duration::from_millis(500);
    let sem = Arc::new(Semaphore::new(MAX_CONCURRENCY));
    let mut ping_set = tokio::task::JoinSet::new();

    for ip in &scan_scope.targets {
        let ip = *ip;
        let permit = sem.clone();
        ping_set.spawn(async move {
            let _permit = permit.acquire().await.unwrap();
            let alive = tcp_connect_ping(ip, ping_port, ping_timeout).await;
            (ip, alive)
        });
    }

    let mut alive_hosts: Vec<IpAddr> = Vec::new();
    while let Some(result) = ping_set.join_next().await {
        if let Ok((ip, true)) = result {
            info!("alive: {}", ip);
            alive_hosts.push(ip);
        }
    }

    if alive_hosts.is_empty() {
        warn!("scan {}: no alive hosts found", request_id);
        return ScanReport {
            request_id,
            results: vec![],
            error: None,
        };
    }

    // TCP port probing on alive hosts
    let sem = Arc::new(Semaphore::new(MAX_CONCURRENCY));
    let mut probe_set = tokio::task::JoinSet::new();

    for ip in &alive_hosts {
        for port in &cmd.ports {
            let ip = *ip;
            let port = *port;
            let permit = sem.clone();
            let t = timeout;
            probe_set.spawn(async move {
                let _permit = permit.acquire().await.unwrap();
                let open = probe_tcp(ip, port, t).await;
                (ip, port, open)
            });
        }
    }

    let mut results = Vec::new();
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs();

    while let Some(result) = probe_set.join_next().await {
        if let Ok((ip, port, true)) = result {
            info!("open: {}:{}", ip, port);
            results.push(DiscoveredResource {
                id: format!("tcp:{}:{}", ip, port),
                ip: ip.to_string(),
                port,
                protocol: "tcp".into(),
                reachable_from: connector_id.to_string(),
                first_seen: now,
            });
        }
    }

    info!(
        "scan {}: completed, {} resources discovered",
        request_id,
        results.len()
    );

    ScanReport {
        request_id,
        results,
        error: None,
    }
}
