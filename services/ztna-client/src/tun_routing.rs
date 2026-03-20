use std::collections::{HashMap, HashSet};
use std::net::IpAddr;
use std::process::Command;

use anyhow::Result;
use tracing::{info, warn};

use crate::token_store::StoredResource;
use crate::tun_dns::{self, ResolvedResource};

/// Dedicated routing table ID for ZTNA resource routes.
/// Chosen to avoid collision with common table IDs.
const ZTNA_TABLE: &str = "51820";

/// ip rule priority — must be lower (higher priority) than existing policy
/// rules (which start at 100 on this host) so our per-IP rules are evaluated
/// first.
const RULE_PRIORITY: &str = "50";

/// Fwmark value used to bypass the ZTNA routing table.  Sockets marked with
/// this value use the `main` table instead, so ACL-denied traffic (e.g. SSH
/// to a resource IP) goes through the normal interface instead of being killed.
pub const BYPASS_FWMARK: u32 = 0x5a;
const BYPASS_MARK_STR: &str = "0x5a";
const BYPASS_PRIORITY: &str = "49";

/// Manages kernel routes and policy rules that direct traffic for authorized
/// resource IPs through the TUN device.
///
/// For each resource IP we install:
///   ip rule  add to <ip>/32 lookup 51820 priority 50
///   ip route add <ip>/32 dev ztna0 table 51820
///
/// This ensures our routes win even when the host has existing policy routing
/// tables that would otherwise match first.
pub struct RouteManager {
    tun_name: String,
    /// Currently installed routes: IP → resolved resource info
    active_routes: HashMap<IpAddr, ResolvedResource>,
}

impl RouteManager {
    pub fn new(tun_name: &str) -> Self {
        // Install a bypass rule: packets with our fwmark use the main table,
        // so direct-connect (ACL-denied) traffic exits via the real NIC.
        let _ = Command::new("ip")
            .args([
                "rule",
                "add",
                "fwmark",
                BYPASS_MARK_STR,
                "lookup",
                "main",
                "priority",
                BYPASS_PRIORITY,
            ])
            .output();
        info!(
            "[routing] installed bypass fwmark rule (mark={}, prio={})",
            BYPASS_MARK_STR, BYPASS_PRIORITY
        );

        Self {
            tun_name: tun_name.to_string(),
            active_routes: HashMap::new(),
        }
    }

    /// Resolve resources and diff against current routes.
    /// Adds routes + rules for new IPs and removes them for IPs no longer authorized.
    pub async fn sync_routes(&mut self, resources: &[StoredResource]) -> Result<()> {
        let resolved = tun_dns::resolve_resources(resources).await;
        let desired: HashMap<IpAddr, ResolvedResource> =
            resolved.into_iter().map(|r| (r.ip, r)).collect();

        let current_ips: HashSet<IpAddr> = self.active_routes.keys().copied().collect();
        let desired_ips: HashSet<IpAddr> = desired.keys().copied().collect();

        // Remove stale routes + rules
        for ip in current_ips.difference(&desired_ips) {
            if let Err(e) = self.del_route(*ip) {
                warn!("[routing] failed to remove route for {}: {}", ip, e);
            }
            if let Err(e) = self.del_rule(*ip) {
                warn!("[routing] failed to remove rule for {}: {}", ip, e);
            }
            info!("[routing] removed route+rule for {}", ip);
            self.active_routes.remove(ip);
        }

        // Add new routes + rules
        for ip in desired_ips.difference(&current_ips) {
            if let Err(e) = self.add_route(*ip) {
                warn!("[routing] failed to add route for {}: {}", ip, e);
            } else if let Err(e) = self.add_rule(*ip) {
                warn!("[routing] failed to add rule for {}: {}", ip, e);
                // Roll back the route if the rule failed
                let _ = self.del_route(*ip);
            } else {
                info!("[routing] added route+rule for {}", ip);
            }
            if let Some(r) = desired.get(ip) {
                self.active_routes.insert(*ip, r.clone());
            }
        }

        // Update metadata for existing routes (in case resource info changed)
        for ip in current_ips.intersection(&desired_ips) {
            if let Some(r) = desired.get(ip) {
                self.active_routes.insert(*ip, r.clone());
            }
        }

        Ok(())
    }

    /// Look up the original domain/address for a destination IP.
    pub fn lookup_domain(&self, ip: IpAddr) -> Option<String> {
        self.active_routes
            .get(&ip)
            .map(|r| r.original_address.clone())
    }

    /// Look up resource info for a destination IP.
    #[allow(dead_code)]
    pub fn lookup(&self, ip: IpAddr) -> Option<&ResolvedResource> {
        self.active_routes.get(&ip)
    }

    /// Remove all installed routes and rules. Called on shutdown.
    pub fn cleanup(&mut self) {
        let ips: Vec<IpAddr> = self.active_routes.keys().copied().collect();
        for ip in ips {
            if let Err(e) = self.del_rule(ip) {
                warn!("[routing] cleanup: failed to remove rule for {}: {}", ip, e);
            }
            if let Err(e) = self.del_route(ip) {
                warn!(
                    "[routing] cleanup: failed to remove route for {}: {}",
                    ip, e
                );
            }
        }
        self.active_routes.clear();
        // Flush the entire ZTNA table as a safety net
        let _ = Command::new("ip")
            .args(["route", "flush", "table", ZTNA_TABLE])
            .output();
        // Remove bypass fwmark rule
        let _ = Command::new("ip")
            .args([
                "rule",
                "del",
                "fwmark",
                BYPASS_MARK_STR,
                "lookup",
                "main",
                "priority",
                BYPASS_PRIORITY,
            ])
            .output();
        info!("[routing] all routes and rules cleaned up");
    }

    // -- Route helpers (dedicated table) --

    fn add_route(&self, ip: IpAddr) -> Result<()> {
        let output = Command::new("ip")
            .args([
                "route",
                "add",
                &format!("{}/32", ip),
                "dev",
                &self.tun_name,
                "table",
                ZTNA_TABLE,
            ])
            .output()?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            if !stderr.contains("File exists") {
                anyhow::bail!(
                    "ip route add (table {}) failed: {}",
                    ZTNA_TABLE,
                    stderr.trim()
                );
            }
        }
        Ok(())
    }

    fn del_route(&self, ip: IpAddr) -> Result<()> {
        let output = Command::new("ip")
            .args([
                "route",
                "del",
                &format!("{}/32", ip),
                "dev",
                &self.tun_name,
                "table",
                ZTNA_TABLE,
            ])
            .output()?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            if !stderr.contains("No such process") {
                anyhow::bail!(
                    "ip route del (table {}) failed: {}",
                    ZTNA_TABLE,
                    stderr.trim()
                );
            }
        }
        Ok(())
    }

    // -- Rule helpers (per-destination lookup into ZTNA table) --

    fn add_rule(&self, ip: IpAddr) -> Result<()> {
        let output = Command::new("ip")
            .args([
                "rule",
                "add",
                "to",
                &format!("{}/32", ip),
                "lookup",
                ZTNA_TABLE,
                "priority",
                RULE_PRIORITY,
            ])
            .output()?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            if !stderr.contains("File exists") {
                anyhow::bail!("ip rule add failed: {}", stderr.trim());
            }
        }
        Ok(())
    }

    fn del_rule(&self, ip: IpAddr) -> Result<()> {
        let output = Command::new("ip")
            .args([
                "rule",
                "del",
                "to",
                &format!("{}/32", ip),
                "lookup",
                ZTNA_TABLE,
                "priority",
                RULE_PRIORITY,
            ])
            .output()?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            if !stderr.contains("No such file or directory") && !stderr.contains("No such process")
            {
                anyhow::bail!("ip rule del failed: {}", stderr.trim());
            }
        }
        Ok(())
    }
}

impl Drop for RouteManager {
    fn drop(&mut self) {
        self.cleanup();
    }
}
