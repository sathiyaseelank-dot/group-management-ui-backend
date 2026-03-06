use hmac::{Hmac, Mac};
use ipnet::IpNet;
use sha2::Sha256;
use std::collections::{HashMap, HashSet};
use std::net::IpAddr;
use std::sync::RwLock;
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use tracing::warn;

use super::types::{PolicyResource, PolicySnapshot};

type HmacSha256 = Hmac<Sha256>;

#[derive(Default)]
struct Inner {
    by_id: HashMap<String, PolicyResource>,
    by_dns: HashMap<String, Vec<String>>,
    by_ip: HashMap<IpAddr, Vec<String>>,
    cidr_list: Vec<(IpNet, Vec<String>)>,
    internet_ids: Vec<String>,
    /// ACL table: "identity::resource_id" -> ()
    acl_table: HashSet<String>,
    valid_until: Option<SystemTime>,
    has_snapshot: bool,
}

pub struct PolicyCache {
    inner: RwLock<Inner>,
    signing_key: Vec<u8>,
    stale_grace: Duration,
}

impl PolicyCache {
    pub fn new(signing_key: Vec<u8>, stale_grace: Duration) -> Self {
        Self {
            inner: RwLock::new(Inner::default()),
            signing_key,
            stale_grace,
        }
    }

    pub fn replace_snapshot(&self, snap: PolicySnapshot) -> bool {
        if !self.verify_snapshot(&snap) {
            self.clear();
            warn!("policy snapshot rejected: invalid signature");
            return false;
        }

        let valid_until = match parse_rfc3339(&snap.snapshot_meta.valid_until) {
            Some(t) => t,
            None => {
                self.clear();
                warn!("policy snapshot rejected: invalid valid_until");
                return false;
            }
        };

        let now = SystemTime::now();
        if now > valid_until + self.stale_grace {
            self.clear();
            warn!("policy snapshot rejected: expired beyond grace");
            return false;
        }

        let mut w = self.inner.write().unwrap();
        *w = Inner::default();
        w.valid_until = Some(valid_until);
        w.has_snapshot = true;

        for res in &snap.resources {
            w.by_id.insert(res.resource_id.clone(), res.clone());
            let addr = res.address.trim().to_lowercase();
            match res.resource_type.trim().to_lowercase().as_str() {
                "internet" => {
                    w.internet_ids.push(res.resource_id.clone());
                }
                "cidr" => {
                    if let Ok(net) = addr.parse::<IpNet>() {
                        // Append all matching CIDR resources (match Go behavior)
                        w.cidr_list.push((net, vec![res.resource_id.clone()]));
                    }
                }
                _ => {
                    if !addr.is_empty() {
                        w.by_dns
                            .entry(addr.clone())
                            .or_default()
                            .push(res.resource_id.clone());
                        if let Ok(ip) = addr.parse::<IpAddr>() {
                            w.by_ip.entry(ip).or_default().push(res.resource_id.clone());
                        }
                    }
                }
            }
            for identity in &res.allowed_identities {
                if !identity.is_empty() {
                    w.acl_table
                        .insert(format!("{}::{}", identity, res.resource_id));
                }
            }
        }

        true
    }

    /// Returns (allowed, resource_id, reason)
    pub fn allowed(
        &self,
        identity_id: &str,
        dest: &str,
        protocol: &str,
        port: u16,
    ) -> (bool, String, &'static str) {
        let r = self.inner.read().unwrap();
        if !r.has_snapshot {
            return (false, String::new(), "no_snapshot");
        }
        if let Some(valid_until) = r.valid_until {
            if SystemTime::now() > valid_until + self.stale_grace {
                warn!("policy snapshot expired beyond grace; denying all");
                return (false, String::new(), "snapshot_expired");
            }
        }

        let key = dest.trim().to_lowercase();
        let mut resource_ids: Vec<String> = Vec::new();

        if let Ok(ip) = key.parse::<IpAddr>() {
            if let Some(ids) = r.by_ip.get(&ip) {
                resource_ids.extend_from_slice(ids);
            }
            for (net, ids) in &r.cidr_list {
                if net.contains(&ip) {
                    resource_ids.extend_from_slice(ids);
                }
            }
        }

        if resource_ids.is_empty() && !key.is_empty() {
            if let Some(ids) = r.by_dns.get(&key) {
                resource_ids.extend_from_slice(ids);
            }
        }

        if resource_ids.is_empty() && !r.internet_ids.is_empty() {
            resource_ids.extend_from_slice(&r.internet_ids);
        }

        if resource_ids.is_empty() {
            return (false, String::new(), "resource_not_found");
        }

        let mut seen = HashSet::new();
        for resource_id in &resource_ids {
            if !seen.insert(resource_id.clone()) {
                continue;
            }
            let res = match r.by_id.get(resource_id) {
                Some(r) => r,
                None => continue,
            };
            if !res.protocol.is_empty()
                && !protocol.is_empty()
                && !res.protocol.eq_ignore_ascii_case(protocol)
            {
                continue;
            }
            if !port_matches(res, port) {
                continue;
            }
            let acl_key = format!("{}::{}", identity_id, resource_id);
            if r.acl_table.contains(&acl_key) {
                return (true, resource_id.clone(), "allowed");
            }
        }

        (false, String::new(), "not_allowed")
    }

    fn verify_snapshot(&self, snap: &PolicySnapshot) -> bool {
        if self.signing_key.is_empty() {
            return false;
        }
        // Strip the signature, serialize, compute HMAC
        let mut snap_copy = snap.clone();
        snap_copy.snapshot_meta.signature = String::new();
        let data = match serde_json::to_vec(&snap_copy) {
            Ok(d) => d,
            Err(_) => return false,
        };
        let mut mac = HmacSha256::new_from_slice(&self.signing_key).unwrap();
        mac.update(&data);
        let computed = hex::encode(mac.finalize().into_bytes());

        // Strip optional "sha256:" prefix
        let provided_sig = snap.snapshot_meta.signature.trim();
        let provided_sig = provided_sig.strip_prefix("sha256:").unwrap_or(provided_sig);
        let provided = match hex::decode(provided_sig) {
            Ok(b) => b,
            Err(_) => return false,
        };
        let want = match hex::decode(&computed) {
            Ok(b) => b,
            Err(_) => return false,
        };
        // Constant-time compare
        if provided.len() != want.len() {
            return false;
        }
        let mut diff = 0u8;
        for (a, b) in provided.iter().zip(want.iter()) {
            diff |= a ^ b;
        }
        diff == 0
    }

    fn clear(&self) {
        let mut w = self.inner.write().unwrap();
        *w = Inner::default();
    }
}

fn port_matches(res: &PolicyResource, port: u16) -> bool {
    match (res.port_from, res.port_to) {
        (None, None) => res.port == 0 || port == res.port,
        (from, to) => {
            let start = from.unwrap_or(0);
            let end = to.unwrap_or(start);
            if start == 0 && end == 0 {
                return true;
            }
            let end = if end == 0 { start } else { end };
            port >= start && port <= end
        }
    }
}

fn parse_rfc3339(s: &str) -> Option<SystemTime> {
    // Parse RFC3339 manually via chrono-like approach using only std
    // Format: 2025-01-01T00:00:00Z or with offset
    // We'll use a simple parse
    // Try simple epoch-based parse
    // RFC3339: YYYY-MM-DDTHH:MM:SSZ
    let s = s.trim();
    // Strip trailing Z or +00:00
    let s = s.strip_suffix('Z').unwrap_or(s);
    let s = s.strip_suffix("+00:00").unwrap_or(s);

    // Split on T
    let (date_part, time_part) = s.split_once('T')?;
    let date_parts: Vec<&str> = date_part.split('-').collect();
    let time_parts: Vec<&str> = time_part.split(':').collect();
    if date_parts.len() < 3 || time_parts.len() < 3 {
        return None;
    }

    let year: i64 = date_parts[0].parse().ok()?;
    let month: i64 = date_parts[1].parse().ok()?;
    let day: i64 = date_parts[2].parse().ok()?;
    let hour: i64 = time_parts[0].parse().ok()?;
    let min: i64 = time_parts[1].parse().ok()?;
    let sec_str = time_parts[2].split('.').next().unwrap_or(time_parts[2]);
    let sec: i64 = sec_str.parse().ok()?;

    // Compute Unix timestamp (Gregorian calendar, no leap seconds)
    let days = days_from_civil(year, month, day)?;
    let unix_secs = days * 86400 + hour * 3600 + min * 60 + sec;
    if unix_secs < 0 {
        return None;
    }
    Some(UNIX_EPOCH + Duration::from_secs(unix_secs as u64))
}

fn days_from_civil(y: i64, m: i64, d: i64) -> Option<i64> {
    // Algorithm from http://howardhinnant.github.io/date_algorithms.html
    let y = if m <= 2 { y - 1 } else { y };
    let era = if y >= 0 { y } else { y - 399 } / 400;
    let yoe = y - era * 400;
    let doy = (153 * (if m > 2 { m - 3 } else { m + 9 }) + 2) / 5 + d - 1;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    Some(era * 146097 + doe - 719468)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::policy::types::{PolicyResource, SnapshotMeta};

    const TEST_KEY: &str = "test-signing-key";

    fn sign_snapshot(key: &[u8], snap: &PolicySnapshot) -> String {
        let mut snap_copy = snap.clone();
        snap_copy.snapshot_meta.signature = String::new();
        let data = serde_json::to_vec(&snap_copy).unwrap();
        let mut mac = HmacSha256::new_from_slice(key).unwrap();
        mac.update(&data);
        hex::encode(mac.finalize().into_bytes())
    }

    fn format_rfc3339(t: SystemTime) -> String {
        let duration = t.duration_since(UNIX_EPOCH).unwrap();
        let secs = duration.as_secs();
        // Format as 2025-01-01T00:00:00Z
        let days = secs / 86400;
        let rem_secs = secs % 86400;
        let hour = rem_secs / 3600;
        let min = (rem_secs % 3600) / 60;
        let sec = rem_secs % 60;

        // Simple conversion (good enough for tests within ~100 years)
        let (year, month, day) = civil_from_days(days as i64);
        format!(
            "{:04}-{:02}-{:02}T{:02}:{:02}:{:02}Z",
            year, month, day, hour, min, sec
        )
    }

    fn civil_from_days(days: i64) -> (i32, u32, u32) {
        // From http://howardhinnant.github.io/date_algorithms.html
        let days = days + 719468;
        let era = if days >= 0 { days } else { days - 146096 } / 146097;
        let doe = days - era * 146097;
        let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
        let y = yoe + era * 400;
        let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
        let mp = (5 * doy + 2) / 153;
        let d = doy - (153 * mp + 2) / 5 + 1;
        let m = if mp < 10 { mp + 3 } else { mp - 9 };
        let y = if mp < 10 { y } else { y + 1 };
        (y as i32, m as u32, d as u32)
    }

    fn new_signed_snapshot(resources: Vec<PolicyResource>) -> PolicySnapshot {
        let now = SystemTime::now();
        let valid_until = now + Duration::from_secs(600);
        let mut snap = PolicySnapshot {
            snapshot_meta: SnapshotMeta {
                connector_id: "con_test".to_string(),
                policy_version: 1,
                compiled_at: format_rfc3339(now),
                valid_until: format_rfc3339(valid_until),
                signature: String::new(),
            },
            resources,
        };
        let sig = sign_snapshot(TEST_KEY.as_bytes(), &snap);
        snap.snapshot_meta.signature = sig;
        snap
    }

    fn new_cache(resources: Vec<PolicyResource>) -> PolicyCache {
        let cache = PolicyCache::new(TEST_KEY.as_bytes().to_vec(), Duration::from_secs(300));
        assert!(cache.replace_snapshot(new_signed_snapshot(resources)));
        cache
    }

    #[test]
    fn test_policy_cache_dns_allow() {
        let cache = new_cache(vec![PolicyResource {
            resource_id: "res_dns_allow".to_string(),
            resource_type: "dns".to_string(),
            address: "db.internal".to_string(),
            port: 0,
            protocol: "TCP".to_string(),
            port_from: None,
            port_to: None,
            allowed_identities: vec!["identity-1".to_string()],
        }]);

        let (allowed, _, reason) = cache.allowed("identity-1", "db.internal", "TCP", 5432);
        assert!(allowed, "expected allow, got reason={}", reason);
        assert_eq!(reason, "allowed");
    }

    #[test]
    fn test_policy_cache_dns_deny() {
        let cache = new_cache(vec![PolicyResource {
            resource_id: "res_dns_deny".to_string(),
            resource_type: "dns".to_string(),
            address: "db.internal".to_string(),
            port: 0,
            protocol: "TCP".to_string(),
            port_from: None,
            port_to: None,
            allowed_identities: vec!["identity-1".to_string()],
        }]);

        let (allowed, _, reason) = cache.allowed("identity-2", "db.internal", "TCP", 5432);
        assert!(!allowed, "expected deny");
        assert_eq!(reason, "not_allowed");
    }

    #[test]
    fn test_policy_cache_cidr_allow() {
        let cache = new_cache(vec![PolicyResource {
            resource_id: "res_cidr_allow".to_string(),
            resource_type: "cidr".to_string(),
            address: "10.0.10.0/24".to_string(),
            port: 0,
            protocol: "TCP".to_string(),
            port_from: None,
            port_to: None,
            allowed_identities: vec!["identity-1".to_string()],
        }]);

        let (allowed, _, reason) = cache.allowed("identity-1", "10.0.10.50", "TCP", 443);
        assert!(allowed, "expected allow, got reason={}", reason);
        assert_eq!(reason, "allowed");
    }

    #[test]
    fn test_policy_cache_cidr_no_match_hostname() {
        let cache = new_cache(vec![PolicyResource {
            resource_id: "res_cidr_only".to_string(),
            resource_type: "cidr".to_string(),
            address: "10.0.10.0/24".to_string(),
            port: 0,
            protocol: "TCP".to_string(),
            port_from: None,
            port_to: None,
            allowed_identities: vec!["identity-1".to_string()],
        }]);

        let (allowed, _, reason) = cache.allowed("identity-1", "db.internal", "TCP", 443);
        assert!(!allowed, "expected deny");
        assert_eq!(reason, "resource_not_found");
    }

    #[test]
    fn test_policy_cache_internet_fallback() {
        let cache = new_cache(vec![PolicyResource {
            resource_id: "res_internet".to_string(),
            resource_type: "internet".to_string(),
            address: "*".to_string(),
            port: 0,
            protocol: "TCP".to_string(),
            port_from: None,
            port_to: None,
            allowed_identities: vec!["identity-1".to_string()],
        }]);

        let (allowed, _, reason) = cache.allowed("identity-1", "unknown.host", "TCP", 443);
        assert!(allowed, "expected allow, got reason={}", reason);
        assert_eq!(reason, "allowed");
    }

    #[test]
    fn test_policy_cache_multi_resource_no_early_deny() {
        let cache = new_cache(vec![
            PolicyResource {
                resource_id: "res_denied".to_string(),
                resource_type: "dns".to_string(),
                address: "db.internal".to_string(),
                port: 0,
                protocol: "TCP".to_string(),
                port_from: None,
                port_to: None,
                allowed_identities: vec!["identity-2".to_string()],
            },
            PolicyResource {
                resource_id: "res_allowed".to_string(),
                resource_type: "dns".to_string(),
                address: "db.internal".to_string(),
                port: 0,
                protocol: "TCP".to_string(),
                port_from: None,
                port_to: None,
                allowed_identities: vec!["identity-1".to_string()],
            },
        ]);

        let (allowed, resource_id, reason) =
            cache.allowed("identity-1", "db.internal", "TCP", 5432);
        assert!(allowed, "expected allow, got reason={}", reason);
        assert_eq!(reason, "allowed");
        assert_eq!(resource_id, "res_allowed");
    }
}
