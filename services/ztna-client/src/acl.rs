use anyhow::Result;
use reqwest::Client;
use serde::{Deserialize, Serialize};

#[derive(Serialize)]
struct CheckAccessRequest<'a> {
    destination: &'a str,
    protocol: &'a str,
    port: u16,
}

#[derive(Debug, Deserialize)]
pub struct CheckAccessResponse {
    pub allowed: bool,
    pub resource_id: String,
    pub reason: String,
}

pub async fn check_access(
    controller_url: &str,
    access_token: &str,
    destination: &str,
    port: u16,
) -> Result<CheckAccessResponse> {
    let resp = Client::new()
        .post(format!("{}/api/device/check-access", controller_url))
        .bearer_auth(access_token)
        .json(&CheckAccessRequest {
            destination,
            protocol: "tcp",
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
