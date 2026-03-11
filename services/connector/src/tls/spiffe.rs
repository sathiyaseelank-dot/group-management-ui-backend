use anyhow::{bail, Result};
use x509_parser::prelude::*;

/// Extract SPIFFE URI from a DER-encoded X.509 certificate.
/// Returns the full URI string, e.g. "spiffe://mycorp.internal/tunneler/abc123".
pub fn extract_spiffe_id(cert_der: &[u8]) -> Result<String> {
    let (_, cert) = X509Certificate::from_der(cert_der)
        .map_err(|e| anyhow::anyhow!("failed to parse X.509 cert: {}", e))?;

    let san_ext = cert
        .get_extension_unique(&oid_registry::OID_X509_EXT_SUBJECT_ALT_NAME)
        .map_err(|_| anyhow::anyhow!("duplicate SAN extension"))?;

    let ext = match san_ext {
        Some(e) => e,
        None => bail!("certificate has no SAN extension"),
    };

    let san = match ext.parsed_extension() {
        ParsedExtension::SubjectAlternativeName(san) => san,
        _ => bail!("could not parse SAN extension"),
    };

    let mut spiffe_uris: Vec<String> = Vec::new();
    for name in &san.general_names {
        if let GeneralName::URI(uri) = name {
            if uri.starts_with("spiffe://") {
                spiffe_uris.push(uri.to_string());
            }
        }
    }

    if spiffe_uris.len() != 1 {
        bail!("certificate must contain exactly one SPIFFE URI SAN, found {}", spiffe_uris.len());
    }

    Ok(spiffe_uris.into_iter().next().unwrap())
}

/// Verify a SPIFFE URI matches trust domain and expected role.
/// Expected role: "controller", "tunneler", "connector".
pub fn verify_spiffe_uri(uri: &str, trust_domain: &str, expected_role: &str) -> Result<String> {
    // uri format: spiffe://<trust_domain>/<role>/<id>
    let rest = uri.strip_prefix("spiffe://")
        .ok_or_else(|| anyhow::anyhow!("SPIFFE ID must use spiffe:// scheme"))?;

    let slash = rest.find('/').ok_or_else(|| anyhow::anyhow!("invalid SPIFFE ID format"))?;
    let host = &rest[..slash];
    let path = &rest[slash + 1..];

    if host != trust_domain {
        bail!("SPIFFE trust domain mismatch: got '{}', want '{}'", host, trust_domain);
    }

    let parts: Vec<&str> = path.splitn(2, '/').collect();
    let role = parts[0];

    if !expected_role.is_empty() && role != expected_role {
        bail!("unexpected SPIFFE role: got '{}', want '{}'", role, expected_role);
    }

    Ok(uri.to_string())
}

/// Parse tunneler ID from a SPIFFE URI: spiffe://<domain>/tunneler/<id>
pub fn tunneler_id_from_spiffe(spiffe_id: &str) -> Option<String> {
    let rest = spiffe_id.strip_prefix("spiffe://")?;
    let slash = rest.find('/')?;
    let path = &rest[slash + 1..];
    let parts: Vec<&str> = path.splitn(3, '/').collect();
    if parts.len() < 2 || parts[0] != "tunneler" {
        return None;
    }
    Some(parts[1].to_string())
}

/// Parse connector ID from a SPIFFE URI: spiffe://<domain>/connector/<id>
pub fn connector_id_from_spiffe(spiffe_id: &str) -> Option<String> {
    let rest = spiffe_id.strip_prefix("spiffe://")?;
    let slash = rest.find('/')?;
    let path = &rest[slash + 1..];
    let parts: Vec<&str> = path.splitn(3, '/').collect();
    if parts.len() < 2 || parts[0] != "connector" {
        return None;
    }
    Some(parts[1].to_string())
}
