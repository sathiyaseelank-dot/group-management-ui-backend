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

#[derive(Debug, Clone)]
pub struct FirewallPolicySummary {
    pub action: String,
    pub protected_port_count: usize,
    pub ports: String,
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

/// Validate that a protocol string is an allowed value.
/// Prevents injection of arbitrary nft arguments via a crafted protocol field.
fn validate_protocol(proto: &str) -> Result<&'static str> {
    match proto.to_lowercase().as_str() {
        "tcp" => Ok("tcp"),
        "udp" => Ok("udp"),
        _ => anyhow::bail!("invalid protocol: {}", proto),
    }
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
        run_nft_cmd_allow_exists(&["add", "table", "inet", &self.table_name])
            .await
            .context("failed to create nftables table")?;

        // Create input chain with filter hook
        run_nft_cmd_allow_exists(&[
            "add", "chain", "inet", &self.table_name, &self.chain_name,
            "{", "type", "filter", "hook", "input", "priority", "0", ";",
            "policy", "accept", ";", "}",
        ])
        .await
        .context("failed to create nftables chain")?;

        // Flush stale rules from a previous run so we never duplicate.
        run_nft(&format!(
            "flush chain inet {} {}",
            self.table_name, self.chain_name
        ))
        .await
        .context("failed to flush nftables chain")?;

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
        let proto = validate_protocol(protocol)?;
        let port_str = port.to_string();
        let mut handles = Vec::new();

        // Rule 1: accept from lo
        let h1 = self
            .add_rule(&[
                "iifname", "lo", proto, "dport", &port_str, "accept",
            ])
            .await
            .context("lo accept rule")?;
        handles.push(h1);

        // Rule 2: accept from TUN
        let h2 = match self
            .add_rule(&[
                "iifname", &self.tun_name, proto, "dport", &port_str, "accept",
            ])
            .await
        {
            Ok(h) => h,
            Err(e) => {
                let _ = self.delete_rules(&handles).await;
                return Err(e).context("tun accept rule");
            }
        };
        handles.push(h2);

        // Rule 3: drop all other traffic to this port
        let h3 = match self
            .add_rule(&[proto, "dport", &port_str, "drop"])
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
    async fn add_rule(&self, rule_args: &[&str]) -> Result<u64> {
        let mut args: Vec<&str> = vec![
            "-ae", "add", "rule", "inet", &self.table_name, &self.chain_name,
        ];
        args.extend_from_slice(rule_args);
        let output = run_nft_cmd_output(&args).await?;

        // nft -ae outputs "# handle <N>" — extract the handle number
        parse_handle(&output)
            .ok_or_else(|| anyhow::anyhow!("could not parse rule handle from nft output"))
    }

    /// Delete rules by their handles.
    async fn delete_rules(&self, handles: &[u64]) -> Result<()> {
        for handle in handles {
            let handle_str = handle.to_string();
            if let Err(e) = run_nft_cmd(&[
                "delete", "rule", "inet", &self.table_name, &self.chain_name,
                "handle", &handle_str,
            ])
            .await
            {
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
        match run_nft_cmd(&["delete", "table", "inet", &self.table_name]).await {
            Ok(_) => info!("cleaned up nftables table {}", self.table_name),
            Err(e) => warn!("failed to cleanup nftables table: {}", e),
        }
    }
}

/// Handle an inbound `firewall_policy` message from the connector.
pub async fn handle_firewall_policy(
    payload: &[u8],
    enforcer: &FirewallEnforcer,
) -> Result<FirewallPolicySummary> {
    let policy: FirewallPolicy = serde_json::from_slice(payload)
        .context("failed to parse firewall_policy message")?;
    let protected_ports = format_port_rules(&policy.protected_ports);
    let summary = FirewallPolicySummary {
        action: policy.action.clone(),
        protected_port_count: policy.protected_ports.len(),
        ports: protected_ports.clone(),
    };

    match policy.action.as_str() {
        "sync" => {
            info!(
                "firewall policy received: action=sync reason=\"connector pushed protected resource firewall update\" protected_ports={} ports={}",
                policy.protected_ports.len(),
                protected_ports,
            );
            enforcer.sync_policy(&policy.protected_ports).await?;
            // Persist state after sync
            let state = enforcer.get_state().await;
            if let Err(e) = crate::persistence::save_firewall_state(&state) {
                warn!("failed to persist firewall state: {}", e);
            }
            info!(
                "firewall policy applied: action=sync protected_ports={} ports={}",
                policy.protected_ports.len(),
                protected_ports,
            );
        }
        other => {
            warn!("unknown firewall policy action: {}", other);
        }
    }

    Ok(summary)
}

/// Run an nft command with discrete arguments (no shell, no split_whitespace).
async fn run_nft_cmd(args: &[&str]) -> Result<std::process::Output> {
    let output = Command::new("nft")
        .args(args)
        .output()
        .await
        .context("failed to execute nft")?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        anyhow::bail!("nft {:?} failed: {}", args, stderr.trim());
    }
    Ok(output)
}

/// Run an nft command and ignore "File exists" errors.
async fn run_nft_cmd_allow_exists(args: &[&str]) -> Result<()> {
    match run_nft_cmd(args).await {
        Ok(_) => Ok(()),
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
async fn run_nft_cmd_output(args: &[&str]) -> Result<String> {
    let output = run_nft_cmd(args).await?;
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

fn format_port_rules(rules: &[PortRule]) -> String {
    let ports: Vec<String> = rules
        .iter()
        .map(|rule| format!("{}/{}", rule.port, rule.protocol))
        .collect();
    if ports.is_empty() {
        "none".to_string()
    } else {
        ports.join(",")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_validate_protocol() {
        assert_eq!(validate_protocol("tcp").unwrap(), "tcp");
        assert_eq!(validate_protocol("TCP").unwrap(), "tcp");
        assert_eq!(validate_protocol("udp").unwrap(), "udp");
        assert_eq!(validate_protocol("Udp").unwrap(), "udp");
        assert!(validate_protocol("icmp").is_err());
        assert!(validate_protocol("").is_err());
        assert!(validate_protocol("tcp dport 22 accept").is_err());
        assert!(validate_protocol("tcp; drop").is_err());
    }

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
