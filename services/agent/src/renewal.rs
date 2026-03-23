use crate::tls::cert_store::CertStore;
use std::sync::Arc;
use std::time::{Duration, SystemTime};
use tokio::sync::Notify;
use tracing::{info, warn};

pub async fn renewal_loop(
    controller_addr: String,
    agent_id: String,
    controller_trust_domain: String,
    store: CertStore,
    controller_ca_pem: Vec<u8>,
    workload_ca_pem: Vec<u8>,
    reload: Arc<Notify>,
) {
    loop {
        let sleep_dur = next_renewal_delay(store.not_after(), store.total_ttl());
        tokio::time::sleep(sleep_dur).await;

        match crate::enroll::renew(
            &controller_addr,
            &agent_id,
            &controller_trust_domain,
            &store,
            &controller_ca_pem,
            &workload_ca_pem,
        )
        .await
        {
            Ok(result) => {
                let (not_before, not_after) = crate::enroll::cert_validity(&result.cert_der)
                    .unwrap_or((
                        SystemTime::now(),
                        SystemTime::now() + Duration::from_secs(3600),
                    ));
                let total_ttl = not_after
                    .duration_since(not_before)
                    .unwrap_or(Duration::from_secs(3600));
                if let Err(e) = crate::persistence::save_enrollment(&result) {
                    warn!("failed to persist renewed certificate: {}", e);
                }
                store.update(
                    result.cert_der,
                    result.cert_chain_der,
                    result.key_der.to_vec(),
                    not_after,
                    total_ttl,
                );
                info!("certificate renewed successfully");
                reload.notify_one();
            }
            Err(e) => {
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
