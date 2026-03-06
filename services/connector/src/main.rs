mod allowlist;
mod buildinfo;
mod config;
mod control_plane;
mod enroll;
mod net_util;
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
use std::sync::Arc;
use std::time::{Duration, SystemTime};
use tls::cert_store::CertStore;
use tokio::sync::mpsc;
use tracing::{error, info, warn};

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

    // Enroll to get certs
    let enroll_cfg = config::EnrollConfig {
        controller_addr: cfg.controller_addr.clone(),
        connector_id: cfg.connector_id.clone(),
        trust_domain: cfg.trust_domain.clone(),
        token: cfg.enrollment_token.clone(),
        private_ip: cfg.private_ip.clone(),
        version: buildinfo::version().to_string(),
        ca_pem: cfg.ca_pem.clone(),
    };

    let result = enroll::enroll(&enroll_cfg).await?;
    info!("connector enrolled as {}", result.spiffe_id);

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

    let allowlist = Arc::new(TunnelerAllowlist::new());
    let acl = Arc::new(PolicyCache::new(cfg.policy_key.clone(), cfg.stale_grace));
    let (send_ch, recv_ch) = mpsc::channel::<ControlMessage>(16);

    // Start tunneler-facing gRPC server
    tokio::spawn(server::server_loop(
        cfg.listen_addr.clone(),
        cfg.trust_domain.clone(),
        store.clone(),
        result.ca_pem.clone(),
        allowlist.clone(),
        acl.clone(),
        send_ch.clone(),
        cfg.connector_id.clone(),
    ));

    // Start certificate renewal loop
    tokio::spawn(renewal::renewal_loop(
        cfg.controller_addr.clone(),
        cfg.connector_id.clone(),
        cfg.trust_domain.clone(),
        store.clone(),
        result.ca_pem.clone(),
    ));

    // Run control plane loop (blocks until context cancelled)
    control_plane_loop(
        cfg.controller_addr.clone(),
        cfg.trust_domain.clone(),
        cfg.connector_id.clone(),
        cfg.private_ip.clone(),
        store.clone(),
        result.ca_pem.clone(),
        allowlist.clone(),
        acl.clone(),
        send_ch,
        recv_ch,
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
) -> Result<()> {
    let channel =
        tls::client_cfg::build_tonic_channel(controller_addr, trust_domain, store, ca_pem)
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
                    Ok(Some(m)) => handle_control_message(&m, allowlist, acl),
                    Ok(None) => return Ok(()),
                    Err(e) => return Err(anyhow::anyhow!("stream recv: {}", e)),
                }
            }
            Some(out_msg) = recv_ch.recv() => {
                stream_tx.send(out_msg).await?;
            }
            _ = heartbeat.tick() => {
                stream_tx.send(ControlMessage {
                    r#type: "heartbeat".to_string(),
                    connector_id: connector_id.to_string(),
                    private_ip: private_ip.to_string(),
                    status: "ONLINE".to_string(),
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
