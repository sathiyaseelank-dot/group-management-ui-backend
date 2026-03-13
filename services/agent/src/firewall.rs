use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use tokio::process::Command;
use tokio::sync::Mutex;
use tracing::{info, warn};

#[derive(Deserialize)]
pub struct FirewallPolicy {
    pub action: String,
    pub protected_ports: Vec<PortRule>,
}

#[derive(Deserialize, Serialize, Clone, PartialEq, Eq, Hash, Debug)]
pub struct PortRule {
    pub port: u16,
    pub protocol: String,
}

#[derive(Serialize, Deserialize, Default, Debug)]
pub struct FirewallState {
    pub protected_ports: Vec<PortRule>,
}

struct ProtectionEntry {
    port: u16,
    protocol: String,
    rule_handles: Vec<u64>,
}

pub struct FirewallEnforcer {
    tun_name: String,
    table_name: String,
    chain_name: String,
    protected: Mutex<HashMap<u16, ProtectionEntry>>,
}

impl FirewallEnforcer {
    pub fn new(tun_name: &str) -> Self {
        Self {
            tun_name: tun_name.to_string(),
            table_name: "ztna".to_string(),
            chain_name: "input_filter".to_string(),
            protected: Mutex::new(HashMap::new()),
        }
    }

    /// Create the nftables table and chain if they don't exist.
    pub async fn initialize(&self) -> Result<()> {
        // Create table
        run_nft_allow_exists(&format!(
            "add table inet {}",
            self.table_name
        ))
        .await
        .context("failed to create nftables table")?;

        // Create input chain with filter hook
        run_nft_allow_exists(&format!(
            "add chain inet {} {} {{ type filter hook input priority 0 ; policy accept ; }}",
            self.table_name, self.chain_name
        ))
        .await
        .context("failed to create nftables chain")?;

        info!(
            "nftables initialized: table={} chain={}",
            self.table_name, self.chain_name
        );
        Ok(())
    }

    /// Diff current protected ports vs desired and add/remove as needed.
    pub async fn sync_policy(&self, desired: &[PortRule]) -> Result<()> {
        let mut protected = self.protected.lock().await;

        let desired_ports: HashMap<u16, &PortRule> =
            desired.iter().map(|r| (r.port, r)).collect();

        // Remove ports no longer desired
        let stale: Vec<u16> = protected
            .keys()
            .filter(|p| !desired_ports.contains_key(p))
            .copied()
            .collect();

        for port in stale {
            if let Some(entry) = protected.remove(&port) {
                if let Err(e) = self.delete_rules(&entry.rule_handles).await {
                    warn!("failed to unprotect port {}: {}", port, e);
                }
                info!("unprotected port {}/{}", port, entry.protocol);
            }
        }

        // Add new ports
        for rule in desired {
            if protected.contains_key(&rule.port) {
                continue;
            }
            match self.protect_port(rule.port, &rule.protocol).await {
                Ok(handles) => {
                    protected.insert(
                        rule.port,
                        ProtectionEntry {
                            port: rule.port,
                            protocol: rule.protocol.clone(),
                            rule_handles: handles,
                        },
                    );
                    info!("protected port {}/{}", rule.port, rule.protocol);
                }
                Err(e) => {
                    warn!("failed to protect port {}/{}: {}", rule.port, rule.protocol, e);
                }
            }
        }

        Ok(())
    }

    /// Add three nftables rules for a port:
    /// 1. Accept from loopback
    /// 2. Accept from TUN interface
    /// 3. Drop everything else
    async fn protect_port(&self, port: u16, protocol: &str) -> Result<Vec<u64>> {
        let proto = protocol.to_lowercase();
        let mut handles = Vec::new();

        // Rule 1: accept from lo
        let h1 = self
            .add_rule(&format!(
                "iifname \"lo\" {} dport {} accept",
                proto, port
            ))
            .await
            .context("lo accept rule")?;
        handles.push(h1);

        // Rule 2: accept from TUN
        let h2 = match self
            .add_rule(&format!(
                "iifname \"{}\" {} dport {} accept",
                self.tun_name, proto, port
            ))
            .await
        {
            Ok(h) => h,
            Err(e) => {
                // Rollback rule 1
                let _ = self.delete_rules(&handles).await;
                return Err(e).context("tun accept rule");
            }
        };
        handles.push(h2);

        // Rule 3: drop all other traffic to this port
        let h3 = match self
            .add_rule(&format!("{} dport {} drop", proto, port))
            .await
        {
            Ok(h) => h,
            Err(e) => {
                let _ = self.delete_rules(&handles).await;
                return Err(e).context("drop rule");
            }
        };
        handles.push(h3);

        Ok(handles)
    }

    /// Add a single rule and return its handle.
    async fn add_rule(&self, rule_expr: &str) -> Result<u64> {
        let cmd = format!(
            "-ae add rule inet {} {} {}",
            self.table_name, self.chain_name, rule_expr
        );
        let output = run_nft_output(&cmd).await?;

        // nft -ae outputs "# handle <N>" — extract the handle number
        parse_handle(&output)
            .ok_or_else(|| anyhow::anyhow!("could not parse rule handle from nft output"))
    }

    /// Delete rules by their handles.
    async fn delete_rules(&self, handles: &[u64]) -> Result<()> {
        for handle in handles {
            let cmd = format!(
                "delete rule inet {} {} handle {}",
                self.table_name, self.chain_name, handle
            );
            if let Err(e) = run_nft(&cmd).await {
                warn!("failed to delete rule handle {}: {}", handle, e);
            }
        }
        Ok(())
    }

    /// Remove protection for a specific port (public API for on-demand removal).
    #[allow(dead_code)]
    pub async fn unprotect_port(&self, port: u16) -> Result<()> {
        let mut protected = self.protected.lock().await;
        if let Some(entry) = protected.remove(&port) {
            self.delete_rules(&entry.rule_handles).await?;
            info!("unprotected port {}/{}", port, entry.protocol);
        }
        Ok(())
    }

    /// Return current state for persistence.
    pub async fn get_state(&self) -> FirewallState {
        let protected = self.protected.lock().await;
        FirewallState {
            protected_ports: protected
                .values()
                .map(|e| PortRule {
                    port: e.port,
                    protocol: e.protocol.clone(),
                })
                .collect(),
        }
    }

    /// Restore firewall rules from persisted state (called on startup).
    pub async fn restore_from_state(&self, state: &FirewallState) -> Result<()> {
        if state.protected_ports.is_empty() {
            return Ok(());
        }
        info!(
            "restoring firewall state for {} ports",
            state.protected_ports.len()
        );
        self.sync_policy(&state.protected_ports).await
    }

    /// Delete the entire nftables table (cleanup on shutdown).
    pub async fn cleanup_all(&self) {
        let cmd = format!("delete table inet {}", self.table_name);
        match run_nft(&cmd).await {
            Ok(()) => info!("cleaned up nftables table {}", self.table_name),
            Err(e) => warn!("failed to cleanup nftables table: {}", e),
        }
    }
}

/// Handle an inbound `firewall_policy` message from the connector.
pub async fn handle_firewall_policy(
    payload: &[u8],
    enforcer: &FirewallEnforcer,
) -> Result<()> {
    let policy: FirewallPolicy = serde_json::from_slice(payload)
        .context("failed to parse firewall_policy message")?;

    match policy.action.as_str() {
        "sync" => {
            enforcer.sync_policy(&policy.protected_ports).await?;
            // Persist state after sync
            let state = enforcer.get_state().await;
            if let Err(e) = crate::persistence::save_firewall_state(&state) {
                warn!("failed to persist firewall state: {}", e);
            }
        }
        other => {
            warn!("unknown firewall policy action: {}", other);
        }
    }

    Ok(())
}

/// Run an nft command, returning an error if it fails.
async fn run_nft(args: &str) -> Result<()> {
    let output = Command::new("nft")
        .args(args.split_whitespace())
        .output()
        .await
        .context("failed to execute nft")?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        anyhow::bail!("nft {} failed: {}", args, stderr.trim());
    }
    Ok(())
}

/// Run an nft command and ignore "File exists" errors.
async fn run_nft_allow_exists(args: &str) -> Result<()> {
    match run_nft(args).await {
        Ok(()) => Ok(()),
        Err(e) => {
            let msg = e.to_string();
            if msg.contains("File exists") {
                Ok(())
            } else {
                Err(e)
            }
        }
    }
}

/// Run an nft command and return stdout.
async fn run_nft_output(args: &str) -> Result<String> {
    let output = Command::new("nft")
        .args(args.split_whitespace())
        .output()
        .await
        .context("failed to execute nft")?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        anyhow::bail!("nft {} failed: {}", args, stderr.trim());
    }
    Ok(String::from_utf8_lossy(&output.stdout).to_string())
}

/// Parse "# handle <N>" from nft -ae output.
fn parse_handle(output: &str) -> Option<u64> {
    for line in output.lines() {
        if let Some(idx) = line.find("# handle ") {
            let rest = &line[idx + "# handle ".len()..];
            if let Ok(h) = rest.trim().parse::<u64>() {
                return Some(h);
            }
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_handle() {
        assert_eq!(parse_handle("# handle 42"), Some(42));
        assert_eq!(parse_handle("  # handle 123  "), Some(123));
        assert_eq!(parse_handle("no handle here"), None);
        assert_eq!(
            parse_handle("add rule inet ztna input_filter ...\n# handle 7"),
            Some(7)
        );
        assert_eq!(
            parse_handle("add rule inet ztna input_filter iifname \"lo\" tcp dport 22 accept # handle 5"),
            Some(5)
        );
    }
}
