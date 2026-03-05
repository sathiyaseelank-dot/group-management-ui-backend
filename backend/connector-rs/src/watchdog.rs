use std::time::Duration;
use tracing::warn;

/// Send systemd READY=1, then heartbeat WATCHDOG=1 every WATCHDOG_USEC/2 µs.
pub async fn watchdog_loop() {
    let interval = match watchdog_interval() {
        Some(d) => d,
        None => return,
    };

    if let Err(e) = sd_notify::notify(false, &[sd_notify::NotifyState::Ready]) {
        warn!("systemd notify READY failed: {}", e);
        return;
    }

    let mut ticker = tokio::time::interval(interval);
    ticker.tick().await; // skip the first immediate tick
    loop {
        ticker.tick().await;
        if let Err(e) = sd_notify::notify(false, &[sd_notify::NotifyState::Watchdog]) {
            warn!("systemd notify WATCHDOG failed: {}", e);
        }
    }
}

fn watchdog_interval() -> Option<Duration> {
    let usec_str = std::env::var("WATCHDOG_USEC").ok()?;
    let usec: u64 = usec_str.trim().parse().ok()?;
    if usec == 0 {
        return None;
    }
    Some(Duration::from_micros(usec / 2))
}
