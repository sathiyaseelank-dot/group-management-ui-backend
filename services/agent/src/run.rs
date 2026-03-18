use anyhow::Result;
use std::collections::HashSet;
use std::sync::Arc;
use std::time::{Duration, SystemTime};
use tokio::sync::Notify;
use tracing::{info, warn};

use crate::config;
use crate::enroll;
use crate::enroll::pb::ControlMessage;
use crate::firewall::FirewallEnforcer;
use crate::tls::cert_store::CertStore;

pub async fn run() -> Result<()> {
    let cfg = config::run_config_from_env()?;

    // Try loading saved enrollment state; fall back to fresh enrollment.
    let result = match crate::persistence::load_saved_enrollment() {
        Ok(Some(saved)) => {
            info!("reusing saved certificate for {}", saved.spiffe_id);
            saved
        }
        _ => {
            let enroll_cfg = config::EnrollConfig {
                controller_addr: cfg.controller_addr.clone(),
                agent_id: cfg.agent_id.clone(),
                trust_domain: cfg.trust_domain.clone(),
                token: cfg.enrollment_token.clone(),
                ca_pem: cfg.ca_pem.clone(),
            };
            let enrolled = enroll::enroll(&enroll_cfg).await?;
            info!("agent enrolled as {}", enrolled.spiffe_id);
            if let Err(e) = crate::persistence::save_enrollment(&enrolled) {
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

    // Initialize firewall enforcer
    let enforcer = Arc::new(FirewallEnforcer::new(&cfg.tun_name));
    if let Err(e) = enforcer.initialize().await {
        warn!("firewall enforcer initialization failed (nftables may not be available): {}", e);
    } else {
        // Restore persisted firewall state
        match crate::persistence::load_firewall_state() {
            Ok(Some(state)) => {
                if let Err(e) = enforcer.restore_from_state(&state).await {
                    warn!("failed to restore firewall state: {}", e);
                }
            }
            Ok(None) => {}
            Err(e) => warn!("failed to load firewall state: {}", e),
        }
    }

    let reload = Arc::new(Notify::new());

    // Start control plane loop (connects to connector)
    tokio::spawn(control_plane_loop(
        cfg.connector_addr.clone(),
        cfg.trust_domain.clone(),
        store.clone(),
        result.ca_pem.clone(),
        result.spiffe_id.clone(),
        cfg.agent_id.clone(),
        reload.clone(),
        enforcer.clone(),
    ));

    // Start certificate renewal loop
    tokio::spawn(crate::renewal::renewal_loop(
        cfg.controller_addr.clone(),
        cfg.agent_id.clone(),
        cfg.trust_domain.clone(),
        store.clone(),
        result.ca_pem.clone(),
        reload.clone(),
    ));

    // Wait for shutdown signal
    tokio::signal::ctrl_c().await.ok();
    info!("shutting down, cleaning up firewall rules");
    enforcer.cleanup_all().await;

    Ok(())
}

#[allow(clippy::too_many_arguments)]
async fn control_plane_loop(
    connector_addr: String,
    trust_domain: String,
    store: CertStore,
    ca_pem: Vec<u8>,
    spiffe_id: String,
    agent_id: String,
    reload: Arc<Notify>,
    enforcer: Arc<FirewallEnforcer>,
) {
    let mut backoff = Duration::from_secs(2);
    loop {
        tokio::select! {
            result = connect_to_connector(
                &connector_addr,
                &trust_domain,
                &store,
                &ca_pem,
                &spiffe_id,
                &agent_id,
                &enforcer,
            ) => {
                if let Err(e) = result {
                    warn!("connector connection ended: {}", e);
                }
            }
            _ = reload.notified() => {
                info!("cert reload signal received, reconnecting");
            }
        }

        tokio::time::sleep(backoff).await;
        if backoff < Duration::from_secs(30) {
            backoff *= 2;
        }
    }
}

async fn connect_to_connector(
    connector_addr: &str,
    trust_domain: &str,
    store: &CertStore,
    ca_pem: &[u8],
    spiffe_id: &str,
    agent_id: &str,
    enforcer: &Arc<FirewallEnforcer>,
) -> Result<()> {
    let channel = crate::tls::client_cfg::build_tonic_channel_with_role(
        connector_addr,
        trust_domain,
        store,
        ca_pem,
        "connector",
    )
    .await?;

    let mut client =
        enroll::pb::control_plane_client::ControlPlaneClient::new(channel);

    let (stream_tx, stream_rx) = tokio::sync::mpsc::channel::<ControlMessage>(16);
    let in_stream = tokio_stream::wrappers::ReceiverStream::new(stream_rx);

    let mut stream = client
        .connect(tonic::Request::new(in_stream))
        .await?
        .into_inner();

    // Send initial hello
    stream_tx
        .send(ControlMessage {
            r#type: "agent_hello".to_string(),
            ..Default::default()
        })
        .await?;

    let mut heartbeat = tokio::time::interval(Duration::from_secs(10));
    heartbeat.tick().await; // skip immediate tick

    let mut diff_tick = tokio::time::interval(Duration::from_secs(30));
    diff_tick.tick().await; // skip immediate tick

    let mut sync_tick = tokio::time::interval(Duration::from_secs(300));
    // Do NOT skip immediate tick — first tick fires now = on-connect full sync

    let mut sent_services: HashSet<(u16, String)> = HashSet::new();
    let mut last_fingerprint: u64 = 0;
    let mut seq: u64 = 0;
    let mut dirty = false;
    let mut last_report_time = std::time::Instant::now();

    // Load persisted discovery state
    match crate::persistence::load_discovery_state() {
        Ok(Some(state)) => {
            for svc in &state.services {
                sent_services.insert((svc.port, svc.protocol.clone()));
            }
            last_fingerprint = state.fingerprint;
            info!("discovery: loaded {} persisted services", sent_services.len());
        }
        Ok(None) => {}
        Err(e) => warn!("discovery: failed to load persisted state: {}", e),
    }

    let mut posture_interval = tokio::time::interval(Duration::from_secs(300));
    posture_interval.tick().await; // skip immediate first tick

    loop {
        tokio::select! {
            msg = stream.message() => {
                match msg {
                    Ok(Some(m)) => {
                        if let Err(e) = handle_inbound_message(&m, enforcer, &stream_tx, agent_id, spiffe_id).await {
                            warn!("failed to handle message type={}: {}", m.r#type, e);
                        }
                    }
                    Ok(None) => return Ok(()),
                    Err(e) => return Err(anyhow::anyhow!("stream recv: {}", e)),
                }
            }
            _ = heartbeat.tick() => {
                let payload = serde_json::to_vec(&serde_json::json!({
                    "agent_id": agent_id,
                    "spiffe_id": spiffe_id,
                    "ip": crate::enroll::get_local_ip(),
                })).unwrap_or_default();

                stream_tx.send(ControlMessage {
                    r#type: "agent_heartbeat".to_string(),
                    payload,
                    status: "ONLINE".to_string(),
                    ..Default::default()
                }).await?;
            }
            _ = sync_tick.tick() => {
                match crate::discovery::run_discovery_full_sync(
                    agent_id, enforcer, &stream_tx, &mut sent_services, &mut last_fingerprint, &mut seq, &mut dirty,
                ).await {
                    Ok(_) => {
                        last_report_time = std::time::Instant::now();
                    }
                    Err(e) => {
                        warn!("discovery full sync failed: {}", e);
                    }
                }
                if dirty {
                    persist_discovery_state(&sent_services, last_fingerprint);
                    dirty = false;
                }
            }
            _ = diff_tick.tick() => {
                match crate::discovery::run_discovery_diff(
                    agent_id, enforcer, &stream_tx, &mut sent_services, &mut last_fingerprint, &mut seq, &mut dirty,
                ).await {
                    Ok(true) => {
                        last_report_time = std::time::Instant::now();
                    }
                    Ok(false) => {
                        // Nothing changed — send heartbeat if 5 minutes elapsed
                        if last_report_time.elapsed() >= Duration::from_secs(300) {
                            let payload = serde_json::to_vec(&serde_json::json!({
                                "agent_id": agent_id,
                                "fingerprint": last_fingerprint,
                            })).unwrap_or_default();
                            if let Err(e) = stream_tx.send(ControlMessage {
                                r#type: "agent_discovery_heartbeat".to_string(),
                                payload,
                                ..Default::default()
                            }).await {
                                warn!("discovery: failed to send heartbeat: {}", e);
                            }
                            last_report_time = std::time::Instant::now();
                            info!("discovery: sent heartbeat (no changes for 5m)");
                        }
                    }
                    Err(e) => {
                        warn!("discovery diff scan failed: {}", e);
                    }
                }
                if dirty {
                    persist_discovery_state(&sent_services, last_fingerprint);
                    dirty = false;
                }
            }
            _ = posture_interval.tick() => {
                let posture = crate::posture::collect(agent_id, spiffe_id);
                if let Ok(payload) = serde_json::to_vec(&posture) {
                    let _ = stream_tx.send(ControlMessage {
                        r#type: "agent_posture".to_string(),
                        payload,
                        ..Default::default()
                    }).await;
                }
            }
        }
    }
}

fn persist_discovery_state(sent_services: &HashSet<(u16, String)>, fingerprint: u64) {
    let state = crate::persistence::DiscoveryState {
        services: sent_services
            .iter()
            .map(|(port, proto)| crate::persistence::DiscoveryServiceEntry {
                port: *port,
                protocol: proto.clone(),
            })
            .collect(),
        fingerprint,
    };
    if let Err(e) = crate::persistence::save_discovery_state(&state) {
        warn!("discovery: failed to persist state: {}", e);
    }
}

async fn handle_inbound_message(
    msg: &ControlMessage,
    enforcer: &Arc<FirewallEnforcer>,
    stream_tx: &tokio::sync::mpsc::Sender<ControlMessage>,
    agent_id: &str,
    spiffe_id: &str,
) -> Result<()> {
    match msg.r#type.as_str() {
        "firewall_policy" => {
            let summary = crate::firewall::handle_firewall_policy(&msg.payload, enforcer).await?;
            let payload = serde_json::json!({
                "agent_id": agent_id,
                "message": format!(
                    "firewall policy applied: action={} reason=connector pushed protected resource firewall update protected_ports={} ports={}",
                    summary.action,
                    summary.protected_port_count,
                    summary.ports,
                ),
            });
            let _ = stream_tx
                .send(ControlMessage {
                    r#type: "agent_log".to_string(),
                    payload: serde_json::to_vec(&payload).unwrap_or_default(),
                    ..Default::default()
                })
                .await;
        }
        "posture_requirements" => {
            #[derive(serde::Deserialize)]
            struct PostureReq {
                require_firewall: bool,
                require_disk_encryption: bool,
                require_screen_lock: bool,
            }
            if let Ok(req) = serde_json::from_slice::<PostureReq>(&msg.payload) {
                let p = crate::posture::collect(agent_id, spiffe_id);
                let mut violations: Vec<&str> = vec![];
                if req.require_firewall && !p.firewall_enabled {
                    violations.push("firewall not enabled");
                }
                if req.require_disk_encryption && !p.disk_encrypted {
                    violations.push("disk not encrypted");
                }
                if req.require_screen_lock && !p.screen_lock_enabled {
                    violations.push("screen lock not enabled");
                }
                if !violations.is_empty() {
                    let payload = serde_json::json!({
                        "agent_id": agent_id,
                        "message": format!("posture_check failed: {}", violations.join(", ")),
                    });
                    let _ = stream_tx
                        .send(ControlMessage {
                            r#type: "agent_log".to_string(),
                            payload: serde_json::to_vec(&payload).unwrap_or_default(),
                            ..Default::default()
                        })
                        .await;
                }
            }
        }
        "pong" => { /* expected response to ping */ }
        other => {
            info!("received unhandled message type: {}", other);
        }
    }
    Ok(())
}
