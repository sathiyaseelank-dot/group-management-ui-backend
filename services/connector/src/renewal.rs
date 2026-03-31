use crate::tls::cert_store::CertStore;
use std::sync::Arc;
use std::time::{Duration, SystemTime};
use tokio::sync::Notify;
use tracing::{error, info, warn};

fn is_transport_error_message(msg: &str) -> bool {
    let msg = msg.to_ascii_lowercase();
    msg.contains("transport error")
        || msg.contains("connection refused")
        || msg.contains("broken pipe")
        || msg.contains("unexpected eof")
        || msg.contains("http2")
        || msg.contains("h2 protocol error")
}

/// Background task: renews the workload certificate before it expires.
pub async fn renewal_loop(
    controller_addr: String,
    connector_id: String,
    trust_domain: String,
    store: CertStore,
    controller_ca_pem: Vec<u8>,
    workload_ca_pem: Vec<u8>,
    reload: Arc<Notify>,
    controller_reset: Arc<Notify>,
    shutdown: Arc<Notify>,
) {
    let mut transport_error_count: u32 = 0;
    loop {
        let sleep_dur = next_renewal_delay(store.not_after(), store.total_ttl());
        tokio::time::sleep(sleep_dur).await;

        match crate::enroll::renew(
            &controller_addr,
            &connector_id,
            &trust_domain,
            &store,
            &controller_ca_pem,
            &workload_ca_pem,
        )
        .await
        {
            Ok(result) => {
                transport_error_count = 0;
                let (not_before, not_after) = crate::enroll::cert_validity(&result.cert_der)
                    .unwrap_or((
                        SystemTime::now(),
                        SystemTime::now() + Duration::from_secs(3600),
                    ));
                let total_ttl = not_after
                    .duration_since(not_before)
                    .unwrap_or(Duration::from_secs(3600));
                store.update(
                    result.cert_der,
                    result.cert_chain_der,
                    result.key_der.to_vec(),
                    not_after,
                    total_ttl,
                );
                info!("certificate renewed successfully (memory-only state updated)");
                reload.notify_one();
            }
            Err(e) => {
                let msg = format!("{}", e);
                if msg.contains("PermissionDenied") {
                    error!(
                        "certificate renewal permanently rejected: {} ; shutting down",
                        e
                    );
                    shutdown.notify_one();
                    return;
                }
                if is_transport_error_message(&msg) {
                    transport_error_count += 1;
                    warn!(
                        "certificate renewal failed (consecutive transport errors={}): {}",
                        transport_error_count,
                        e
                    );
                    if transport_error_count >= 3 {
                        warn!("connector renewal forcing immediate reconnect/reset after repeated controller transport errors");
                        controller_reset.notify_one();
                    }
                    continue;
                }
                transport_error_count = 0;
                warn!("certificate renewal failed: {}", e);
            }
        }
    }
}

fn next_renewal_delay(not_after: SystemTime, total_ttl: Duration) -> Duration {
    let now = SystemTime::now();
    let remaining = not_after.duration_since(now).unwrap_or(Duration::ZERO);

    if remaining.is_zero() {
        return Duration::from_secs(10);
    }

    let ttl = if total_ttl.is_zero() {
        remaining
    } else {
        total_ttl
    };

    // Renew at 70% of TTL (i.e. 30% before expiry)
    let renew_offset = ttl * 30 / 100;
    let renew_at = not_after.checked_sub(renew_offset).unwrap_or(not_after);

    let delay = renew_at.duration_since(now).unwrap_or(Duration::ZERO);
    if delay < Duration::from_secs(10) {
        Duration::from_secs(10)
    } else {
        delay
    }
}
