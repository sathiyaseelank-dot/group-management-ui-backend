use anyhow::Result;
use std::time::{Duration, SystemTime};
use tokio::sync::Notify;
use tracing::{info, warn};

use crate::config;
use crate::enroll;
use crate::enroll::pb::ControlMessage;
use crate::tls::cert_store::CertStore;

pub async fn run() -> Result<()> {
    let cfg = config::run_config_from_env()?;

    let enroll_cfg = config::EnrollConfig {
        controller_addr: cfg.controller_addr.clone(),
        tunneler_id: cfg.tunneler_id.clone(),
        trust_domain: cfg.trust_domain.clone(),
        token: cfg.enrollment_token.clone(),
        ca_pem: cfg.ca_pem.clone(),
    };

    let result = enroll::enroll(&enroll_cfg).await?;
    info!("tunneler enrolled as {}", result.spiffe_id);

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

    let reload = std::sync::Arc::new(Notify::new());

    // Start control plane loop (connects to connector)
    tokio::spawn(control_plane_loop(
        cfg.connector_addr.clone(),
        cfg.trust_domain.clone(),
        store.clone(),
        result.ca_pem.clone(),
        result.spiffe_id.clone(),
        cfg.tunneler_id.clone(),
        reload.clone(),
    ));

    // Start certificate renewal loop
    tokio::spawn(crate::renewal::renewal_loop(
        cfg.controller_addr.clone(),
        cfg.tunneler_id.clone(),
        cfg.trust_domain.clone(),
        store.clone(),
        result.ca_pem.clone(),
        reload.clone(),
    ));

    // Block forever (until the process is killed)
    std::future::pending::<()>().await;
    Ok(())
}

#[allow(clippy::too_many_arguments)]
async fn control_plane_loop(
    connector_addr: String,
    trust_domain: String,
    store: CertStore,
    ca_pem: Vec<u8>,
    spiffe_id: String,
    tunneler_id: String,
    reload: std::sync::Arc<Notify>,
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
                &tunneler_id,
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
    tunneler_id: &str,
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
            r#type: "tunneler_hello".to_string(),
            ..Default::default()
        })
        .await?;

    let mut heartbeat = tokio::time::interval(Duration::from_secs(10));
    heartbeat.tick().await; // skip immediate tick

    loop {
        tokio::select! {
            msg = stream.message() => {
                match msg {
                    Ok(Some(_)) => { /* process inbound messages if needed */ }
                    Ok(None) => return Ok(()),
                    Err(e) => return Err(anyhow::anyhow!("stream recv: {}", e)),
                }
            }
            _ = heartbeat.tick() => {
                let payload = serde_json::to_vec(&serde_json::json!({
                    "tunneler_id": tunneler_id,
                    "spiffe_id": spiffe_id,
                })).unwrap_or_default();

                stream_tx.send(ControlMessage {
                    r#type: "tunneler_heartbeat".to_string(),
                    payload,
                    status: "ONLINE".to_string(),
                    ..Default::default()
                }).await?;
            }
        }
    }
}
