mod agent_tunnel;
mod allowlist;
mod buildinfo;
mod config;
mod control_plane;
mod device_tunnel;
mod discovery;
mod enroll;
mod net_util;
mod persistence;
mod policy;
mod quic_listener;
mod renewal;
mod server;
mod tls;
mod watchdog;

use agent_tunnel::AgentTunnelHub;
use allowlist::{AgentAllowlist, AgentInfo};
use anyhow::Result;
use clap::{Parser, Subcommand};
use enroll::pb::ControlMessage;
use persistence::DeleteCleanupRequest;
use policy::{types::PolicyResource, PolicyCache, PolicySnapshot};
use std::collections::HashMap;
use std::path::Path;
use std::sync::{Arc, RwLock};
use std::time::{Duration, SystemTime};
use tls::cert_store::CertStore;
use tokio::sync::{broadcast, mpsc, Notify};
use tracing::{error, info, warn};

#[derive(serde::Deserialize)]
struct AgentShutdownRequest {
    agent_id: String,
    #[serde(default)]
    reason: String,
}

#[derive(serde::Deserialize)]
struct ConnectorShutdownRequest {
    #[serde(default)]
    connector_id: String,
    #[serde(default)]
    reason: String,
}

enum ControlPlaneAction {
    Continue,
    Shutdown,
}

fn is_permission_denied(err: &anyhow::Error) -> bool {
    let msg = format!("{}", err);
    msg.contains("PermissionDenied")
}

/// Stores the latest protected resources so each agent can receive
/// a personalized firewall policy based on its own IP.
#[derive(Clone)]
pub struct LatestFirewallPolicy {
    inner: Arc<std::sync::RwLock<Vec<PolicyResource>>>,
}

impl LatestFirewallPolicy {
    pub fn new() -> Self {
        Self {
            inner: Arc::new(std::sync::RwLock::new(Vec::new())),
        }
    }

    pub fn store(&self, resources: Vec<PolicyResource>) {
        if let Ok(mut w) = self.inner.write() {
            *w = resources;
        }
    }

    pub fn get(&self) -> Vec<PolicyResource> {
        self.inner.read().map(|r| r.clone()).unwrap_or_default()
    }
}

/// Tracks the last-known status and IP of each connected agent.
/// Updated by the agent-facing server; read by the controller heartbeat.
#[derive(Clone)]
pub struct AgentRegistry {
    inner: Arc<RwLock<HashMap<String, (String, String)>>>,
}

impl AgentRegistry {
    pub fn new() -> Self {
        Self {
            inner: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    pub fn update(&self, agent_id: &str, status: &str, ip: &str) {
        if let Ok(mut map) = self.inner.write() {
            map.insert(agent_id.to_string(), (status.to_string(), ip.to_string()));
        }
    }

    pub fn remove(&self, agent_id: &str) {
        if let Ok(mut map) = self.inner.write() {
            map.remove(agent_id);
        }
    }

    pub fn snapshot(&self) -> Vec<AgentStatusEntry> {
        self.inner
            .read()
            .map(|map| {
                map.iter()
                    .map(|(id, (st, ip))| AgentStatusEntry {
                        agent_id: id.clone(),
                        status: st.clone(),
                        ip: ip.clone(),
                    })
                    .collect()
            })
            .unwrap_or_default()
    }

    pub fn get_ip(&self, agent_id: &str) -> Option<String> {
        self.inner
            .read()
            .ok()
            .and_then(|map| map.get(agent_id).map(|(_, ip)| ip.clone()))
    }
}

#[derive(serde::Serialize)]
pub struct AgentStatusEntry {
    pub agent_id: String,
    pub status: String,
    pub ip: String,
}

#[derive(Parser)]
#[command(name = "grpcconnector2", about = "Arise connector (Rust)")]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Enroll this connector with the controller (one-time)
    Enroll,
    /// Run delete-triggered cleanup after the service stops
    CleanupDelete,
    /// Run the connector service
    Run {
        /// Enable systemd watchdog heartbeats
        #[arg(long)]
        systemd_watchdog: bool,
    },
}

#[tokio::main]
async fn main() {
    rustls::crypto::ring::default_provider()
        .install_default()
        .expect("Failed to install rustls crypto provider");

    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env().unwrap_or_else(|_| "info".into()),
        )
        .init();

    let cli = Cli::parse();
    if let Err(e) = run(cli).await {
        error!("{:#}", e);
        std::process::exit(1);
    }
}

async fn run(cli: Cli) -> Result<()> {
    match cli.command {
        Commands::Enroll => cmd_enroll().await,
        Commands::CleanupDelete => cmd_cleanup_delete(),
        Commands::Run { systemd_watchdog } => cmd_run(systemd_watchdog).await,
    }
}

async fn cmd_enroll() -> Result<()> {
    let cfg = config::enroll_config_from_env()?;
    let result = enroll::enroll(&cfg).await?;
    println!("Enrolled connector with SPIFFE ID: {}", result.spiffe_id);
    info!("enrollment completed successfully");
    Ok(())
}

async fn cmd_run(systemd_watchdog: bool) -> Result<()> {
    let cfg = config::run_config_from_env()?;

    if systemd_watchdog {
        tokio::spawn(watchdog::watchdog_loop());
    }

    // Try loading saved enrollment state; fall back to fresh enrollment.
    let result = match persistence::load_saved_enrollment() {
        Ok(Some(saved)) => {
            info!("reusing saved certificate for {}", saved.spiffe_id);
            saved
        }
        _ => {
            let enroll_cfg = config::EnrollConfig {
                controller_addr: cfg.controller_addr.clone(),
                connector_id: cfg.connector_id.clone(),
                controller_trust_domain: cfg.controller_trust_domain.clone(),
                token: cfg.enrollment_token.clone(),
                private_ip: cfg.private_ip.clone(),
                version: buildinfo::version().to_string(),
                ca_pem: cfg.ca_pem.clone(),
            };
            match enroll::enroll(&enroll_cfg).await {
                Ok(enrolled) => {
                    info!("connector enrolled as {}", enrolled.spiffe_id);
                    if let Err(e) = persistence::save_enrollment(&enrolled) {
                        warn!("failed to persist enrollment state: {}", e);
                    }
                    enrolled
                }
                Err(e) => {
                    if is_permission_denied(&e) {
                        error!("enrollment rejected: connector token was revoked or deleted; shutting down");
                        return Ok(());
                    }
                    return Err(e);
                }
            }
        }
    };

    let (not_before, not_after) = enroll::cert_validity(&result.cert_der).unwrap_or((
        SystemTime::now(),
        SystemTime::now() + Duration::from_secs(3600),
    ));
    let total_ttl = not_after
        .duration_since(not_before)
        .unwrap_or(Duration::from_secs(3600));

    let store = CertStore::new(
        result.cert_der.clone(),
        result.cert_chain_der.clone(),
        result.key_der.to_vec(),
        not_after,
        total_ttl,
    );

    // Use the connector ID from the issued SPIFFE cert, not the config value.
    // The controller derives the policy signing key using this ID as the TLS
    // exporter context, so both sides must agree on the same value.
    let enrolled_connector_id = tls::spiffe::connector_id_from_spiffe(&result.spiffe_id)
        .unwrap_or_else(|| cfg.connector_id.clone());

    let allowlist = Arc::new(AgentAllowlist::new());
    let acl = Arc::new(PolicyCache::new(Vec::new(), cfg.stale_grace));
    let (send_ch, recv_ch) = mpsc::channel::<ControlMessage>(16);
    let agent_registry = Arc::new(AgentRegistry::new());
    let agent_tunnel_hub = AgentTunnelHub::new();
    let (firewall_tx, _) = broadcast::channel::<()>(16);
    let latest_fw_policy = LatestFirewallPolicy::new();
    let reload = Arc::new(Notify::new());
    let shutdown = Arc::new(Notify::new());

    if !cfg.device_tunnel_addr.is_empty() && !cfg.controller_http_url.is_empty() {
        // TLS/TCP device tunnel
        let device_tunnel_addr = cfg.device_tunnel_addr.clone();
        let controller_http_url = cfg.controller_http_url.clone();
        let tunnel_store = store.clone();
        let tunnel_acl = acl.clone();
        let tunnel_hub = agent_tunnel_hub.clone();
        let tunnel_agent_registry = agent_registry.clone();
        tokio::spawn(async move {
            if let Err(e) = device_tunnel::listen(
                &device_tunnel_addr,
                controller_http_url,
                tunnel_store,
                tunnel_acl,
                tunnel_hub,
                tunnel_agent_registry,
            )
            .await
            {
                warn!("device tunnel stopped: {}", e);
            }
        });

        // QUIC/UDP device tunnel (same port, different protocol)
        let quic_addr = cfg.device_tunnel_addr.clone();
        let quic_advertise_addr = cfg.device_tunnel_advertise_addr.clone();
        let quic_ctrl = cfg.controller_http_url.clone();
        let quic_store = store.clone();
        let quic_acl = acl.clone();
        let quic_hub = agent_tunnel_hub.clone();
        let quic_agent_registry = agent_registry.clone();
        tokio::spawn(async move {
            if let Err(e) = quic_listener::listen(
                &quic_addr,
                &quic_advertise_addr,
                quic_ctrl,
                quic_store,
                quic_acl,
                quic_hub,
                quic_agent_registry,
            )
            .await
            {
                // Non-fatal: QUIC is an optimization, TLS still works
                warn!("QUIC device tunnel failed to start: {}", e);
            }
        });
    }

    // Start agent-facing gRPC server
    tokio::spawn(server::server_loop(
        cfg.listen_addr.clone(),
        cfg.agent_trust_domain.clone(),
        store.clone(),
        result.ca_pem.clone(),
        allowlist.clone(),
        acl.clone(),
        send_ch.clone(),
        enrolled_connector_id.clone(),
        agent_registry.clone(),
        agent_tunnel_hub.clone(),
        firewall_tx.clone(),
        latest_fw_policy.clone(),
    ));

    // Start certificate renewal loop
    tokio::spawn(renewal::renewal_loop(
        cfg.controller_addr.clone(),
        enrolled_connector_id.clone(),
        cfg.controller_trust_domain.clone(),
        store.clone(),
        cfg.ca_pem.clone(),
        result.ca_pem.clone(),
        reload.clone(),
        shutdown.clone(),
    ));

    // Run control plane loop (blocks until context cancelled)
    control_plane_loop(
        cfg.controller_addr.clone(),
        cfg.controller_trust_domain.clone(),
        enrolled_connector_id.clone(),
        cfg.private_ip.clone(),
        cfg.device_tunnel_advertise_addr.clone(),
        store.clone(),
        cfg.ca_pem.clone(),
        allowlist.clone(),
        acl.clone(),
        send_ch,
        recv_ch,
        reload,
        agent_registry,
        agent_tunnel_hub,
        firewall_tx,
        latest_fw_policy,
        shutdown,
    )
    .await;

    Ok(())
}

fn cmd_cleanup_delete() -> Result<()> {
    let request = match persistence::load_delete_cleanup_request()? {
        Some(request) => request,
        None => {
            info!("no connector delete cleanup request pending");
            return Ok(());
        }
    };

    if !should_remove_config_for_shutdown_reason(&request.reason) {
        info!(
            "connector cleanup skipped: unsupported reason={} connector_id={}",
            request.reason, request.connector_id
        );
        persistence::clear_delete_cleanup_request()?;
        return Ok(());
    }

    let config_dir = Path::new("/etc/connector");
    if config_dir.exists() {
        std::fs::remove_dir_all(config_dir)
            .map_err(|e| anyhow::anyhow!("failed to remove {}: {}", config_dir.display(), e))?;
        info!(
            "removed connector config directory {} for connector_id={}",
            config_dir.display(),
            request.connector_id
        );
    } else {
        info!(
            "connector config directory {} already absent for connector_id={}",
            config_dir.display(),
            request.connector_id
        );
    }

    persistence::clear_delete_cleanup_request()?;
    Ok(())
}

/// Outer reconnect loop around the control plane stream.
#[allow(clippy::too_many_arguments)]
async fn control_plane_loop(
    controller_addr: String,
    controller_trust_domain: String,
    connector_id: String,
    private_ip: String,
    device_tunnel_addr: String,
    store: CertStore,
    ca_pem: Vec<u8>,
    allowlist: Arc<AgentAllowlist>,
    acl: Arc<PolicyCache>,
    send_ch: mpsc::Sender<ControlMessage>,
    mut recv_ch: mpsc::Receiver<ControlMessage>,
    reload: Arc<Notify>,
    agent_registry: Arc<AgentRegistry>,
    agent_tunnel_hub: AgentTunnelHub,
    firewall_tx: broadcast::Sender<()>,
    latest_fw_policy: LatestFirewallPolicy,
    shutdown: Arc<Notify>,
) {
    let mut backoff = Duration::from_secs(2);
    loop {
        let connect = connect_control_plane(
            &controller_addr,
            &controller_trust_domain,
            &connector_id,
            &private_ip,
            &device_tunnel_addr,
            &store,
            &ca_pem,
            &allowlist,
            &acl,
            &send_ch,
            &mut recv_ch,
            &agent_registry,
            &agent_tunnel_hub,
            &firewall_tx,
            &latest_fw_policy,
            &shutdown,
        );
        tokio::pin!(connect);

        let should_sleep = tokio::select! {
            result = &mut connect => {
                match result {
                    Ok(ControlPlaneAction::Continue) => true,
                    Ok(ControlPlaneAction::Shutdown) => return,
                    Err(e) => {
                        warn!("control-plane connection ended: {}", e);
                        if is_permission_denied(&e) {
                            error!("controller rejected connector permanently; shutting down");
                            shutdown.notify_one();
                            return;
                        }
                        true
                    }
                }
            }
            _ = reload.notified() => {
                info!("cert reload signal received, reconnecting");
                backoff = Duration::from_secs(2);
                false
            }
            _ = shutdown.notified() => return,
        };

        if !should_sleep {
            continue;
        }

        tokio::select! {
            _ = tokio::time::sleep(backoff) => {}
            _ = shutdown.notified() => return,
        }
        if backoff < Duration::from_secs(30) {
            backoff *= 2;
        }
    }
}

#[allow(clippy::too_many_arguments)]
async fn connect_control_plane(
    controller_addr: &str,
    controller_trust_domain: &str,
    connector_id: &str,
    private_ip: &str,
    device_tunnel_addr: &str,
    store: &CertStore,
    ca_pem: &[u8],
    allowlist: &Arc<AgentAllowlist>,
    acl: &Arc<PolicyCache>,
    _send_ch: &mpsc::Sender<ControlMessage>,
    recv_ch: &mut mpsc::Receiver<ControlMessage>,
    agent_registry: &Arc<AgentRegistry>,
    agent_tunnel_hub: &AgentTunnelHub,
    firewall_tx: &broadcast::Sender<()>,
    latest_fw_policy: &LatestFirewallPolicy,
    shutdown: &Arc<Notify>,
) -> Result<ControlPlaneAction> {
    let policy_cb = {
        let acl = acl.clone();
        Arc::new(move |key: Vec<u8>| {
            acl.set_signing_key(key);
            tracing::info!("derived policy signing key from mTLS");
        })
    };
    let channel = tls::client_cfg::build_tonic_channel_with_policy_key(
        controller_addr,
        controller_trust_domain,
        store,
        ca_pem,
        connector_id,
        Some(policy_cb),
    )
    .await?;

    let mut client = enroll::pb::control_plane_client::ControlPlaneClient::new(channel);

    let (stream_tx, stream_rx) = mpsc::channel::<ControlMessage>(16);
    let in_stream = tokio_stream::wrappers::ReceiverStream::new(stream_rx);

    let mut stream = client
        .connect(tonic::Request::new(in_stream))
        .await?
        .into_inner();

    // Send initial hello
    stream_tx
        .send(ControlMessage {
            r#type: "connector_hello".to_string(),
            ..Default::default()
        })
        .await?;

    let mut heartbeat = tokio::time::interval(Duration::from_secs(10));
    heartbeat.tick().await; // skip immediate

    loop {
        tokio::select! {
            msg = stream.message() => {
                match msg {
                    Ok(Some(m)) => {
                        if m.r#type == "scan_command" {
                            let tx = stream_tx.clone();
                            let cid = connector_id.to_string();
                            tokio::spawn(async move {
                                match serde_json::from_slice::<discovery::scan::ScanCommand>(&m.payload) {
                                    Ok(cmd) => {
                                        let report = discovery::scan::execute_scan(cmd, &cid).await;
                                        if let Ok(payload) = serde_json::to_vec(&report) {
                                            let _ = tx.send(ControlMessage {
                                                r#type: "scan_report".into(),
                                                payload,
                                                ..Default::default()
                                            }).await;
                                        }
                                    }
                                    Err(e) => tracing::error!("bad scan_command: {}", e),
                                }
                            });
                        } else {
                            let action = handle_control_message(
                                &m,
                                allowlist,
                                acl,
                                firewall_tx,
                                latest_fw_policy,
                                agent_registry,
                                agent_tunnel_hub,
                                shutdown,
                            ).await;
                            if matches!(action, ControlPlaneAction::Shutdown) {
                                return Ok(ControlPlaneAction::Shutdown);
                            }
                        }
                    }
                    Ok(None) => return Ok(ControlPlaneAction::Continue),
                    Err(e) => return Err(anyhow::anyhow!("stream recv: {}", e)),
                }
            }
            Some(out_msg) = recv_ch.recv() => {
                stream_tx.send(out_msg).await?;
            }
            _ = heartbeat.tick() => {
                let agents = agent_registry.snapshot();
                let payload = serde_json::to_vec(&serde_json::json!({
                    "agents": agents,
                    "device_tunnel_addr": device_tunnel_addr,
                })).unwrap_or_default();
                stream_tx.send(ControlMessage {
                    r#type: "heartbeat".to_string(),
                    connector_id: connector_id.to_string(),
                    private_ip: private_ip.to_string(),
                    status: "ONLINE".to_string(),
                    payload,
                    ..Default::default()
                }).await?;
            }
        }
    }
}

async fn handle_control_message(
    msg: &ControlMessage,
    allowlist: &Arc<AgentAllowlist>,
    acl: &Arc<PolicyCache>,
    firewall_tx: &broadcast::Sender<()>,
    latest_fw_policy: &LatestFirewallPolicy,
    agent_registry: &Arc<AgentRegistry>,
    agent_tunnel_hub: &AgentTunnelHub,
    shutdown: &Arc<Notify>,
) -> ControlPlaneAction {
    match msg.r#type.as_str() {
        "agent_allowlist" => {
            if let Ok(items) = serde_json::from_slice::<Vec<AgentInfo>>(&msg.payload) {
                let count = items.len();
                // Preserve controller ordering: connector-bound agents first.
                let preferred: Vec<String> = items.iter().map(|i| i.agent_id.clone()).collect();
                agent_tunnel_hub.set_preferred_order(preferred);
                allowlist.replace(items);
                info!("agent allowlist replaced: entries={}", count);
            }
        }
        "agent_shutdown" => {
            if let Ok(req) = serde_json::from_slice::<AgentShutdownRequest>(&msg.payload) {
                let payload = serde_json::json!({
                    "reason": req.reason,
                });
                if let Err(e) = agent_tunnel_hub
                    .send_message(&req.agent_id, "shutdown", &payload)
                    .await
                {
                    warn!(
                        "failed to forward shutdown to agent {}: {}",
                        req.agent_id, e
                    );
                } else {
                    info!(
                        "forwarded shutdown to agent {} reason={}",
                        req.agent_id, req.reason
                    );
                }
            }
        }
        "connector_shutdown" => {
            let req = serde_json::from_slice::<ConnectorShutdownRequest>(&msg.payload).unwrap_or(
                ConnectorShutdownRequest {
                    connector_id: String::new(),
                    reason: "deleted".to_string(),
                },
            );
            if should_remove_config_for_shutdown_reason(&req.reason) {
                let request = DeleteCleanupRequest {
                    connector_id: req.connector_id.clone(),
                    reason: req.reason.clone(),
                };
                if let Err(e) = persistence::save_delete_cleanup_request(&request) {
                    warn!(
                        "failed to persist connector delete cleanup request: connector_id={} err={}",
                        request.connector_id, e
                    );
                }
            }
            error!(
                "received shutdown from controller: connector was {}",
                req.reason
            );
            shutdown.notify_one();
            return ControlPlaneAction::Shutdown;
        }
        "agent_allow" => {
            if let Ok(item) = serde_json::from_slice::<AgentInfo>(&msg.payload) {
                allowlist.add(&item.spiffe_id);
                info!(
                    "agent allowlist add: agent_id={} spiffe_id={}",
                    item.agent_id, item.spiffe_id
                );
            }
        }
        "policy_snapshot" => {
            if let Ok(snap) = serde_json::from_slice::<PolicySnapshot>(&msg.payload) {
                let version = snap.snapshot_meta.policy_version;
                let resource_count = snap.resources.len();
                if acl.replace_snapshot(snap.clone()) {
                    info!(
                        "policy snapshot applied: version={} resources={}",
                        version, resource_count
                    );

                    let protected_resources: Vec<PolicyResource> = snap
                        .resources
                        .iter()
                        .filter(|r| r.firewall_status == "protected")
                        .cloned()
                        .collect();
                    let protected_resource_count = protected_resources.len();
                    let summary = summarize_firewall_distribution(
                        &protected_resources,
                        &agent_registry.snapshot(),
                    );
                    info!(
                        "firewall policy prepared for agents: action=sync reason=\"policy snapshot applied for protected resources\" version={} protected_resources={} matched_resources={} unmatched_resources={} ambiguous_resources={}",
                        version,
                        protected_resource_count,
                        summary.matched_resources,
                        summary.unmatched_resources,
                        summary.ambiguous_resources,
                    );
                    for warning in summary.warnings {
                        warn!("{}", warning);
                    }
                    latest_fw_policy.store(protected_resources);
                    let _ = firewall_tx.send(());
                } else {
                    warn!(
                        "policy snapshot rejected: version={} resources={}",
                        version, resource_count
                    );
                }
            }
        }
        _ => {}
    }
    ControlPlaneAction::Continue
}

#[derive(Default)]
struct FirewallDistributionSummary {
    matched_resources: usize,
    unmatched_resources: usize,
    ambiguous_resources: usize,
    warnings: Vec<String>,
}

pub(crate) fn build_agent_firewall_payload(
    agent_id: &str,
    agent_ip: &str,
    protected_resources: &[PolicyResource],
    agent_snapshot: &[AgentStatusEntry],
) -> Vec<u8> {
    let protected_ports: Vec<serde_json::Value> = if agent_ip.trim().is_empty() {
        Vec::new()
    } else {
        let ip_owners = collect_ip_owners(agent_snapshot);
        protected_resources
            .iter()
            .filter(|resource| resource_owner(resource, &ip_owners).as_deref() == Some(agent_id))
            .flat_map(extract_port_rules)
            .collect()
    };

    serde_json::to_vec(&serde_json::json!({
        "action": "sync",
        "protected_ports": protected_ports
    }))
    .unwrap_or_else(|_| b"{\"action\":\"sync\",\"protected_ports\":[]}".to_vec())
}

fn summarize_firewall_distribution(
    protected_resources: &[PolicyResource],
    agent_snapshot: &[AgentStatusEntry],
) -> FirewallDistributionSummary {
    let ip_owners = collect_ip_owners(agent_snapshot);
    let mut summary = FirewallDistributionSummary::default();

    for resource in protected_resources {
        match resource_owner(resource, &ip_owners) {
            Some(_) => summary.matched_resources += 1,
            None => {
                let address = resource.address.trim();
                let owners = ip_owners.get(address).cloned().unwrap_or_default();
                if owners.is_empty() {
                    summary.unmatched_resources += 1;
                    summary.warnings.push(format!(
                        "protected resource skipped for firewall distribution: resource_id={} address={} reason=\"no owning agent matched\"",
                        resource.resource_id,
                        if address.is_empty() { "\"\"" } else { address }
                    ));
                } else {
                    summary.ambiguous_resources += 1;
                    summary.warnings.push(format!(
                        "protected resource skipped for firewall distribution: resource_id={} address={} reason=\"multiple owning agents matched\" agents={}",
                        resource.resource_id,
                        address,
                        owners.join(","),
                    ));
                }
            }
        }
    }

    summary
}

fn collect_ip_owners(agent_snapshot: &[AgentStatusEntry]) -> HashMap<String, Vec<String>> {
    let mut ip_owners: HashMap<String, Vec<String>> = HashMap::new();
    for agent in agent_snapshot {
        let ip = agent.ip.trim();
        if ip.is_empty() {
            continue;
        }
        ip_owners
            .entry(ip.to_string())
            .or_default()
            .push(agent.agent_id.clone());
    }
    ip_owners
}

fn resource_owner(
    resource: &PolicyResource,
    ip_owners: &HashMap<String, Vec<String>>,
) -> Option<String> {
    let owners = ip_owners.get(resource.address.trim())?;
    if owners.len() == 1 {
        owners.first().cloned()
    } else {
        None
    }
}

fn should_remove_config_for_shutdown_reason(reason: &str) -> bool {
    reason.trim().eq_ignore_ascii_case("deleted")
}

fn extract_port_rules(r: &policy::types::PolicyResource) -> Vec<serde_json::Value> {
    match (r.port_from, r.port_to) {
        (Some(from), Some(to)) => (from..=to)
            .map(|p| {
                serde_json::json!({
                    "port": p,
                    "protocol": &r.protocol
                })
            })
            .collect(),
        _ if r.port > 0 => vec![serde_json::json!({
            "port": r.port,
            "protocol": &r.protocol
        })],
        _ => vec![],
    }
}

fn format_port_rules(rules: &[serde_json::Value]) -> String {
    let ports: Vec<String> = rules
        .iter()
        .map(|rule| {
            let port = rule
                .get("port")
                .and_then(|v| v.as_u64())
                .unwrap_or_default();
            let protocol = rule
                .get("protocol")
                .and_then(|v| v.as_str())
                .unwrap_or("unknown");
            format!("{}/{}", port, protocol)
        })
        .collect();
    if ports.is_empty() {
        "none".to_string()
    } else {
        ports.join(",")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn policy_resource(id: &str, address: &str, protocol: &str, port: u16) -> PolicyResource {
        PolicyResource {
            resource_id: id.to_string(),
            resource_type: "dns".to_string(),
            address: address.to_string(),
            port,
            protocol: protocol.to_string(),
            port_from: None,
            port_to: None,
            allowed_identities: Vec::new(),
            firewall_status: "protected".to_string(),
        }
    }

    fn agent_entry(agent_id: &str, ip: &str) -> AgentStatusEntry {
        AgentStatusEntry {
            agent_id: agent_id.to_string(),
            status: "ONLINE".to_string(),
            ip: ip.to_string(),
        }
    }

    #[test]
    fn delete_cleanup_is_only_requested_for_deleted_reason() {
        assert!(should_remove_config_for_shutdown_reason("deleted"));
        assert!(should_remove_config_for_shutdown_reason(" deleted "));
        assert!(!should_remove_config_for_shutdown_reason("revoked"));
        assert!(!should_remove_config_for_shutdown_reason(""));
    }

    #[test]
    fn build_agent_firewall_payload_scopes_rules_to_matching_agent() {
        let resources = vec![
            policy_resource("res-a", "10.0.0.10", "TCP", 22),
            policy_resource("res-b", "10.0.0.20", "TCP", 443),
        ];
        let agents = vec![
            agent_entry("agent-a", "10.0.0.10"),
            agent_entry("agent-b", "10.0.0.20"),
        ];

        let payload = build_agent_firewall_payload("agent-a", "10.0.0.10", &resources, &agents);
        let parsed: serde_json::Value = serde_json::from_slice(&payload).unwrap();
        let ports = parsed["protected_ports"].as_array().unwrap();
        assert_eq!(ports.len(), 1);
        assert_eq!(ports[0]["port"].as_u64(), Some(22));
    }

    #[test]
    fn build_agent_firewall_payload_returns_empty_for_non_owner() {
        let resources = vec![policy_resource("res-a", "10.0.0.10", "TCP", 22)];
        let agents = vec![
            agent_entry("agent-a", "10.0.0.10"),
            agent_entry("agent-b", "10.0.0.20"),
        ];

        let payload = build_agent_firewall_payload("agent-b", "10.0.0.20", &resources, &agents);
        let parsed: serde_json::Value = serde_json::from_slice(&payload).unwrap();
        assert!(parsed["protected_ports"].as_array().unwrap().is_empty());
    }

    #[test]
    fn summarize_firewall_distribution_marks_unmatched_and_ambiguous_resources() {
        let resources = vec![
            policy_resource("res-matched", "10.0.0.10", "TCP", 22),
            policy_resource("res-unmatched", "10.0.0.30", "TCP", 443),
            policy_resource("res-ambiguous", "10.0.0.40", "TCP", 8080),
        ];
        let agents = vec![
            agent_entry("agent-a", "10.0.0.10"),
            agent_entry("agent-b1", "10.0.0.40"),
            agent_entry("agent-b2", "10.0.0.40"),
        ];

        let summary = summarize_firewall_distribution(&resources, &agents);
        assert_eq!(summary.matched_resources, 1);
        assert_eq!(summary.unmatched_resources, 1);
        assert_eq!(summary.ambiguous_resources, 1);
        assert_eq!(summary.warnings.len(), 2);
    }
}
