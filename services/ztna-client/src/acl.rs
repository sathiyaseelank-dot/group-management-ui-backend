use anyhow::Result;
use reqwest::Client;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::OnceLock;
use std::time::{Duration, Instant};
use tokio::sync::RwLock;

static HTTP_CLIENT: OnceLock<Client> = OnceLock::new();

fn shared_client() -> &'static Client {
    HTTP_CLIENT.get_or_init(|| {
        Client::builder()
            .timeout(Duration::from_secs(5))
            .pool_max_idle_per_host(4)
            .build()
            .expect("failed to build HTTP client")
    })
}

#[derive(Serialize)]
struct CheckAccessRequest<'a> {
    destination: &'a str,
    protocol: &'a str,
    port: u16,
}

#[derive(Debug, Clone, Deserialize)]
pub struct CheckAccessResponse {
    pub allowed: bool,
    pub resource_id: String,
    pub reason: String,
}

/// Time-based cache for ACL decisions. Avoids repeated controller round-trips
/// for the same (destination, protocol, port) tuple.
pub struct AclCache {
    entries: RwLock<HashMap<(String, String, u16), (CheckAccessResponse, Instant)>>,
    ttl: Duration,
}

impl AclCache {
    pub fn new(ttl: Duration) -> Self {
        Self {
            entries: RwLock::new(HashMap::new()),
            ttl,
        }
    }

    /// Pre-populate the cache for a list of (destination, port) pairs.
    /// Errors are logged and swallowed — pre-warming is best-effort.
    pub async fn prewarm(
        &self,
        controller_url: &str,
        access_token: &str,
        destinations: &[(String, u16)],
    ) {
        // Fire all checks concurrently via JoinSet
        let mut handles = tokio::task::JoinSet::new();
        for (dest, port) in destinations {
            let key = (dest.clone(), "tcp".to_string(), *port);
            // Skip if already cached and fresh
            {
                let cache = self.entries.read().await;
                if let Some((_, cached_at)) = cache.get(&key) {
                    if cached_at.elapsed() < self.ttl {
                        continue;
                    }
                }
            }
            let url = controller_url.to_string();
            let token = access_token.to_string();
            let dest = dest.clone();
            let port = *port;
            handles.spawn(async move {
                match check_access_remote(&url, &token, &dest, port, "tcp").await {
                    Ok(resp) => Some(((dest, "tcp".to_string(), port), resp)),
                    Err(e) => {
                        tracing::debug!("[acl-prewarm] check-access for {}:{} failed: {}", dest, port, e);
                        None
                    }
                }
            });
        }

        let mut count = 0usize;
        while let Some(result) = handles.join_next().await {
            if let Ok(Some((key, resp))) = result {
                let mut cache = self.entries.write().await;
                cache.insert(key, (resp, Instant::now()));
                count += 1;
            }
        }
        tracing::info!("[acl-prewarm] pre-cached {} ACL decisions", count);
    }

    pub async fn check_access(
        &self,
        controller_url: &str,
        access_token: &str,
        destination: &str,
        port: u16,
        protocol: &str,
    ) -> Result<CheckAccessResponse> {
        let key = (
            destination.to_string(),
            protocol.to_lowercase(),
            port,
        );

        // Fast path: return cached decision if fresh.
        {
            let cache = self.entries.read().await;
            if let Some((resp, cached_at)) = cache.get(&key) {
                if cached_at.elapsed() < self.ttl {
                    return Ok(resp.clone());
                }
            }
        }

        // Cache miss or expired — call controller.
        let resp = check_access_remote(controller_url, access_token, destination, port, protocol).await?;

        // Store in cache.
        {
            let mut cache = self.entries.write().await;
            cache.insert(key, (resp.clone(), Instant::now()));
        }

        Ok(resp)
    }
}

/// Direct check without caching (used for one-off calls like SOCKS5).
pub async fn check_access(
    controller_url: &str,
    access_token: &str,
    destination: &str,
    port: u16,
    protocol: &str,
) -> Result<CheckAccessResponse> {
    check_access_remote(controller_url, access_token, destination, port, protocol).await
}

async fn check_access_remote(
    controller_url: &str,
    access_token: &str,
    destination: &str,
    port: u16,
    protocol: &str,
) -> Result<CheckAccessResponse> {
    let resp = shared_client()
        .post(format!("{}/api/device/check-access", controller_url))
        .bearer_auth(access_token)
        .json(&CheckAccessRequest {
            destination,
            protocol,
            port,
        })
        .send()
        .await?;

    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        anyhow::bail!("check-access: {}", text);
    }

    Ok(resp.json::<CheckAccessResponse>().await?)
}
