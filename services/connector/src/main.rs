mod allowlist;
mod buildinfo;
mod config;
mod control_plane;
mod discovery;
mod enroll;
mod net_util;
mod persistence;
mod policy;
mod renewal;
mod server;
mod tls;
mod watchdog;

use allowlist::{TunnelerAllowlist, TunnelerInfo};
use anyhow::Result;
use clap::{Parser, Subcommand};
use enroll::pb::ControlMessage;
use policy::{PolicyCache, PolicySnapshot};
use std::collections::HashMap;
use std::sync::{Arc, RwLock};
use std::time::{Duration, SystemTime};
use tls::cert_store::CertStore;
use tokio::sync::mpsc;
use tracing::{error, info, warn};

/// Tracks the last-known status of each connected tunneler.
/// Updated by the tunneler-facing server; read by the controller heartbeat.
#[derive(Clone)]
pub struct TunnelerRegistry {
    inner: Arc<RwLock<HashMap<String, String>>>,
}

impl TunnelerRegistry {
    pub fn new() -> Self {
        Self {
            inner: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    pub fn update(&self, tunneler_id: &str, status: &str) {
        if let Ok(mut map) = self.inner.write() {
            map.insert(tunneler_id.to_string(), status.to_string());
        }
    }

    pub fn snapshot(&self) -> Vec<TunnelerStatusEntry> {
        self.inner
            .read()
            .map(|map| {
                map.iter()
                    .map(|(id, st)| TunnelerStatusEntry {
                        tunneler_id: id.clone(),
                        status: st.clone(),
                    })
                    .collect()
            })
            .unwrap_or_default()
    }
}

#[derive(serde::Serialize)]
pub struct TunnelerStatusEntry {
    pub tunneler_id: String,
    pub status: String,
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
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "info".into()),
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
                trust_domain: cfg.trust_domain.clone(),
                token: cfg.enrollment_token.clone(),
                private_ip: cfg.private_ip.clone(),
                version: buildinfo::version().to_string(),
                ca_pem: cfg.ca_pem.clone(),
            };
            let enrolled = enroll::enroll(&enroll_cfg).await?;
            info!("connector enrolled as {}", enrolled.spiffe_id);
            if let Err(e) = persistence::save_enrollment(&enrolled) {
                warn!("failed to persist enrollment state: {}", e);
            }
            enrolled
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
        result.key_der.to_vec(),
        not_after,
        total_ttl,
    );

    // Use the connector ID from the issued SPIFFE cert, not the config value.
    // The controller derives the policy signing key using this ID as the TLS
    // exporter context, so both sides must agree on the same value.
    let enrolled_connector_id = tls::spiffe::connector_id_from_spiffe(&result.spiffe_id)
        .unwrap_or_else(|| cfg.connector_id.clone());

    let allowlist = Arc::new(TunnelerAllowlist::new());
    let acl = Arc::new(PolicyCache::new(cfg.policy_key.clone(), cfg.stale_grace));
    let (send_ch, recv_ch) = mpsc::channel::<ControlMessage>(16);
    let tunneler_registry = Arc::new(TunnelerRegistry::new());

    // Start tunneler-facing gRPC server
    tokio::spawn(server::server_loop(
        cfg.listen_addr.clone(),
        cfg.trust_domain.clone(),
        store.clone(),
        result.ca_pem.clone(),
        allowlist.clone(),
        acl.clone(),
        send_ch.clone(),
        enrolled_connector_id.clone(),
        tunneler_registry.clone(),
    ));

    // Start certificate renewal loop
    tokio::spawn(renewal::renewal_loop(
        cfg.controller_addr.clone(),
        enrolled_connector_id.clone(),
        cfg.trust_domain.clone(),
        store.clone(),
        result.ca_pem.clone(),
    ));

    // Run control plane loop (blocks until context cancelled)
    control_plane_loop(
        cfg.controller_addr.clone(),
        cfg.trust_domain.clone(),
        enrolled_connector_id.clone(),
        cfg.private_ip.clone(),
        store.clone(),
        result.ca_pem.clone(),
        allowlist.clone(),
        acl.clone(),
        send_ch,
        recv_ch,
        tunneler_registry,
    )
    .await;

    Ok(())
}

/// Outer reconnect loop around the control plane stream.
#[allow(clippy::too_many_arguments)]
async fn control_plane_loop(
    controller_addr: String,
    trust_domain: String,
    connector_id: String,
    private_ip: String,
    store: CertStore,
    ca_pem: Vec<u8>,
    allowlist: Arc<TunnelerAllowlist>,
    acl: Arc<PolicyCache>,
    send_ch: mpsc::Sender<ControlMessage>,
    mut recv_ch: mpsc::Receiver<ControlMessage>,
    tunneler_registry: Arc<TunnelerRegistry>,
) {
    let mut backoff = Duration::from_secs(2);
    loop {
        match connect_control_plane(
            &controller_addr,
            &trust_domain,
            &connector_id,
            &private_ip,
            &store,
            &ca_pem,
            &allowlist,
            &acl,
            &send_ch,
            &mut recv_ch,
            &tunneler_registry,
        )
        .await
        {
            Ok(()) => {}
            Err(e) => warn!("control-plane connection ended: {}", e),
        }

        tokio::time::sleep(backoff).await;
        if backoff < Duration::from_secs(30) {
            backoff *= 2;
        }
    }
}

#[allow(clippy::too_many_arguments)]
async fn connect_control_plane(
    controller_addr: &str,
    trust_domain: &str,
    connector_id: &str,
    private_ip: &str,
    store: &CertStore,
    ca_pem: &[u8],
    allowlist: &Arc<TunnelerAllowlist>,
    acl: &Arc<PolicyCache>,
    _send_ch: &mpsc::Sender<ControlMessage>,
    recv_ch: &mut mpsc::Receiver<ControlMessage>,
    tunneler_registry: &Arc<TunnelerRegistry>,
) -> Result<()> {
    let policy_cb = {
        let acl = acl.clone();
        Arc::new(move |key: Vec<u8>| {
            acl.set_signing_key(key);
            tracing::info!("derived policy signing key from mTLS");
        })
    };
    let channel = tls::client_cfg::build_tonic_channel_with_policy_key(
        controller_addr,
        trust_domain,
        store,
        ca_pem,
        connector_id,
        Some(policy_cb),
    )
    .await?;

    let mut client =
        enroll::pb::control_plane_client::ControlPlaneClient::new(channel);

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
                            handle_control_message(&m, allowlist, acl);
                        }
                    }
                    Ok(None) => return Ok(()),
                    Err(e) => return Err(anyhow::anyhow!("stream recv: {}", e)),
                }
            }
            Some(out_msg) = recv_ch.recv() => {
                stream_tx.send(out_msg).await?;
            }
            _ = heartbeat.tick() => {
                let tunnelers = tunneler_registry.snapshot();
                let payload = serde_json::to_vec(&tunnelers).unwrap_or_default();
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

fn handle_control_message(
    msg: &ControlMessage,
    allowlist: &Arc<TunnelerAllowlist>,
    acl: &Arc<PolicyCache>,
) {
    match msg.r#type.as_str() {
        "tunneler_allowlist" => {
            if let Ok(items) = serde_json::from_slice::<Vec<TunnelerInfo>>(&msg.payload) {
                allowlist.replace(items);
            }
        }
        "tunneler_allow" => {
            if let Ok(item) = serde_json::from_slice::<TunnelerInfo>(&msg.payload) {
                allowlist.add(&item.spiffe_id);
            }
        }
        "policy_snapshot" => {
            if let Ok(snap) = serde_json::from_slice::<PolicySnapshot>(&msg.payload) {
                acl.replace_snapshot(snap);
            }
        }
        _ => {}
    }
}
