//! DNS query interception at the TUN level.
//!
//! Intercepts UDP packets to port 53 and resolves queries for known resource
//! domains locally, preventing DNS leaks to the system resolver.  Queries for
//! non-resource domains are passed through unmodified.

use std::collections::HashSet;
use std::net::IpAddr;

use tracing::{debug, warn};

use crate::token_store::StoredResource;

// DNS constants
const TYPE_A: u16 = 1;
const TYPE_AAAA: u16 = 28;
const CLASS_IN: u16 = 1;
const RCODE_OK: u16 = 0;
const RCODE_NXDOMAIN: u16 = 3;
const TTL: u32 = 60;

/// Collect the set of domain-based resource addresses (lowercased).
/// IP-based addresses are excluded since they don't need DNS interception.
pub fn resource_domains(resources: &[StoredResource]) -> HashSet<String> {
    let mut domains = HashSet::new();
    for r in resources {
        let addr = r.address.trim();
        if addr.is_empty() {
            continue;
        }
        // Skip if it parses as an IP
        if addr.parse::<IpAddr>().is_ok() {
            continue;
        }
        domains.insert(addr.to_ascii_lowercase());
    }
    domains
}

/// Try to handle a DNS query for a known resource domain.
///
/// Returns `Some(response_bytes)` if the query was for a resource domain,
/// `None` if the query should be passed through to the normal resolver.
pub async fn handle_dns_query(
    query: &[u8],
    domains: &HashSet<String>,
) -> Option<Vec<u8>> {
    let parsed = parse_query(query)?;

    let qname_lower = parsed.qname.to_ascii_lowercase();
    // Also try without trailing dot
    let qname_trimmed = qname_lower.trim_end_matches('.');

    if !domains.contains(qname_trimmed) && !domains.contains(&qname_lower) {
        return None; // Not a resource domain — pass through
    }

    debug!("[dns-intercept] intercepting query for {}", parsed.qname);

    // Resolve via system resolver
    let lookup = format!("{}:0", qname_trimmed);
    let result = tokio::net::lookup_host(&lookup).await;
    match result {
        Ok(addrs) => {
            let ips: Vec<IpAddr> = addrs.map(|a| a.ip()).collect();
            if ips.is_empty() {
                Some(build_response(query, &parsed, RCODE_NXDOMAIN, &[]))
            } else {
                Some(build_response(query, &parsed, RCODE_OK, &ips))
            }
        }
        Err(e) => {
            warn!("[dns-intercept] resolve failed for {}: {}", qname_trimmed, e);
            Some(build_response(query, &parsed, RCODE_NXDOMAIN, &[]))
        }
    }
}

// ---------------------------------------------------------------------------
// Minimal DNS parser (handles simple single-question queries)
// ---------------------------------------------------------------------------

struct ParsedQuery {
    id: u16,
    qname: String,
    qtype: u16,
    #[allow(dead_code)]
    qclass: u16,
    /// Byte offset where the question section ends (for building responses).
    question_end: usize,
}

fn parse_query(data: &[u8]) -> Option<ParsedQuery> {
    if data.len() < 12 {
        return None;
    }

    let id = u16::from_be_bytes([data[0], data[1]]);
    let flags = u16::from_be_bytes([data[2], data[3]]);

    // Must be a standard query (QR=0, Opcode=0)
    if flags & 0xF800 != 0 {
        return None;
    }

    let qdcount = u16::from_be_bytes([data[4], data[5]]);
    if qdcount != 1 {
        return None; // Only handle single-question queries
    }

    // Parse QNAME (sequence of labels)
    let mut pos = 12;
    let mut qname = String::new();
    loop {
        if pos >= data.len() {
            return None;
        }
        let label_len = data[pos] as usize;
        pos += 1;
        if label_len == 0 {
            break; // Root label — end of name
        }
        if label_len > 63 || pos + label_len > data.len() {
            return None; // Invalid label
        }
        if !qname.is_empty() {
            qname.push('.');
        }
        qname.push_str(std::str::from_utf8(&data[pos..pos + label_len]).ok()?);
        pos += label_len;
    }

    if pos + 4 > data.len() {
        return None;
    }
    let qtype = u16::from_be_bytes([data[pos], data[pos + 1]]);
    let qclass = u16::from_be_bytes([data[pos + 2], data[pos + 3]]);
    pos += 4;

    Some(ParsedQuery {
        id,
        qname,
        qtype,
        qclass,
        question_end: pos,
    })
}

// ---------------------------------------------------------------------------
// DNS response builder
// ---------------------------------------------------------------------------

fn build_response(query: &[u8], parsed: &ParsedQuery, rcode: u16, ips: &[IpAddr]) -> Vec<u8> {
    // Filter IPs by query type
    let answers: Vec<&IpAddr> = ips
        .iter()
        .filter(|ip| match (parsed.qtype, ip) {
            (TYPE_A, IpAddr::V4(_)) => true,
            (TYPE_AAAA, IpAddr::V6(_)) => true,
            _ => false,
        })
        .collect();

    let ancount = answers.len() as u16;
    let flags: u16 = 0x8180 | rcode; // QR=1, RD=1, RA=1

    let mut resp = Vec::with_capacity(512);

    // Header (12 bytes)
    resp.extend_from_slice(&parsed.id.to_be_bytes());
    resp.extend_from_slice(&flags.to_be_bytes());
    resp.extend_from_slice(&1u16.to_be_bytes()); // QDCOUNT = 1
    resp.extend_from_slice(&ancount.to_be_bytes()); // ANCOUNT
    resp.extend_from_slice(&0u16.to_be_bytes()); // NSCOUNT
    resp.extend_from_slice(&0u16.to_be_bytes()); // ARCOUNT

    // Question section — copy from original query
    resp.extend_from_slice(&query[12..parsed.question_end]);

    // Answer section
    for ip in &answers {
        // Name pointer to offset 12 (start of QNAME in question)
        resp.extend_from_slice(&[0xC0, 0x0C]);

        match ip {
            IpAddr::V4(v4) => {
                resp.extend_from_slice(&TYPE_A.to_be_bytes());
                resp.extend_from_slice(&CLASS_IN.to_be_bytes());
                resp.extend_from_slice(&TTL.to_be_bytes());
                resp.extend_from_slice(&4u16.to_be_bytes()); // RDLENGTH
                resp.extend_from_slice(&v4.octets());
            }
            IpAddr::V6(v6) => {
                resp.extend_from_slice(&TYPE_AAAA.to_be_bytes());
                resp.extend_from_slice(&CLASS_IN.to_be_bytes());
                resp.extend_from_slice(&TTL.to_be_bytes());
                resp.extend_from_slice(&16u16.to_be_bytes()); // RDLENGTH
                resp.extend_from_slice(&v6.octets());
            }
        }
    }

    resp
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::Ipv4Addr;

    fn make_a_query(domain: &str) -> Vec<u8> {
        let mut buf = Vec::new();
        // Header
        buf.extend_from_slice(&[0x12, 0x34]); // ID
        buf.extend_from_slice(&[0x01, 0x00]); // Flags: RD=1
        buf.extend_from_slice(&[0x00, 0x01]); // QDCOUNT=1
        buf.extend_from_slice(&[0x00, 0x00]); // ANCOUNT=0
        buf.extend_from_slice(&[0x00, 0x00]); // NSCOUNT=0
        buf.extend_from_slice(&[0x00, 0x00]); // ARCOUNT=0
        // Question
        for label in domain.split('.') {
            buf.push(label.len() as u8);
            buf.extend_from_slice(label.as_bytes());
        }
        buf.push(0); // root label
        buf.extend_from_slice(&TYPE_A.to_be_bytes());
        buf.extend_from_slice(&CLASS_IN.to_be_bytes());
        buf
    }

    #[test]
    fn test_parse_query() {
        let query = make_a_query("db.internal");
        let parsed = parse_query(&query).unwrap();
        assert_eq!(parsed.id, 0x1234);
        assert_eq!(parsed.qname, "db.internal");
        assert_eq!(parsed.qtype, TYPE_A);
        assert_eq!(parsed.qclass, CLASS_IN);
    }

    #[test]
    fn test_build_response_a_record() {
        let query = make_a_query("db.internal");
        let parsed = parse_query(&query).unwrap();
        let ips = vec![IpAddr::V4(Ipv4Addr::new(10, 0, 0, 5))];
        let resp = build_response(&query, &parsed, RCODE_OK, &ips);

        // Verify header
        assert_eq!(resp[0..2], [0x12, 0x34]); // ID matches
        assert_eq!(u16::from_be_bytes([resp[2], resp[3]]) & 0x8000, 0x8000); // QR=1
        assert_eq!(u16::from_be_bytes([resp[6], resp[7]]), 1); // ANCOUNT=1

        // Verify answer contains 10.0.0.5
        let ans_start = parsed.question_end;
        // Name pointer, TYPE, CLASS, TTL, RDLENGTH = 2+2+2+4+2 = 12 bytes before RDATA
        let rdata_start = ans_start + 12;
        assert_eq!(&resp[rdata_start..rdata_start + 4], &[10, 0, 0, 5]);
    }

    #[test]
    fn test_build_response_nxdomain() {
        let query = make_a_query("unknown.test");
        let parsed = parse_query(&query).unwrap();
        let resp = build_response(&query, &parsed, RCODE_NXDOMAIN, &[]);
        let flags = u16::from_be_bytes([resp[2], resp[3]]);
        assert_eq!(flags & 0x000F, RCODE_NXDOMAIN);
        assert_eq!(u16::from_be_bytes([resp[6], resp[7]]), 0); // ANCOUNT=0
    }

    fn make_resource(address: &str) -> StoredResource {
        StoredResource {
            id: String::new(),
            name: String::new(),
            r#type: String::new(),
            address: address.to_string(),
            protocol: "tcp".to_string(),
            port_from: None,
            port_to: None,
            alias: None,
            description: String::new(),
            remote_network_id: String::new(),
            remote_network_name: String::new(),
            firewall_status: String::new(),
            connector_tunnel_addr: String::new(),
        }
    }

    #[test]
    fn test_resource_domains() {
        let resources = vec![
            make_resource("db.internal"),
            make_resource("10.0.0.1"),
        ];
        let domains = resource_domains(&resources);
        assert!(domains.contains("db.internal"));
        assert!(!domains.contains("10.0.0.1")); // IPs excluded
    }
}
