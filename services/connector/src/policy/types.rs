use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct PolicySnapshot {
    pub snapshot_meta: SnapshotMeta,
    pub resources: Vec<PolicyResource>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct SnapshotMeta {
    pub connector_id: String,
    pub policy_version: i64,
    pub compiled_at: String,
    pub valid_until: String,
    pub signature: String,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct PolicyResource {
    pub resource_id: String,
    #[serde(rename = "type")]
    pub resource_type: String,
    pub address: String,
    pub port: u16,
    pub protocol: String,
    pub port_from: Option<u16>,
    pub port_to: Option<u16>,
    pub allowed_identities: Vec<String>,
}
