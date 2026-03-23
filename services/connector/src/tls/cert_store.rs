use std::sync::{Arc, RwLock};
use std::time::{Duration, SystemTime};

/// Holds the current workload certificate + private key in memory for hot rotation.
#[derive(Clone)]
pub struct CertStore {
    inner: Arc<RwLock<Inner>>,
}

struct Inner {
    cert_der: Vec<u8>,
    cert_chain_der: Vec<Vec<u8>>,
    key_der: Vec<u8>,
    not_after: SystemTime,
    total_ttl: Duration,
}

impl CertStore {
    pub fn new(
        cert_der: Vec<u8>,
        cert_chain_der: Vec<Vec<u8>>,
        key_der: Vec<u8>,
        not_after: SystemTime,
        total_ttl: Duration,
    ) -> Self {
        Self {
            inner: Arc::new(RwLock::new(Inner {
                cert_der,
                cert_chain_der,
                key_der,
                not_after,
                total_ttl,
            })),
        }
    }

    pub fn update(
        &self,
        cert_der: Vec<u8>,
        cert_chain_der: Vec<Vec<u8>>,
        key_der: Vec<u8>,
        not_after: SystemTime,
        total_ttl: Duration,
    ) {
        let mut w = self.inner.write().unwrap();
        w.cert_der = cert_der;
        w.cert_chain_der = cert_chain_der;
        w.key_der = key_der;
        w.not_after = not_after;
        w.total_ttl = total_ttl;
    }

    pub fn not_after(&self) -> SystemTime {
        self.inner.read().unwrap().not_after
    }

    pub fn total_ttl(&self) -> Duration {
        self.inner.read().unwrap().total_ttl
    }

    /// Returns a snapshot of (leaf cert, full chain, private key) for TLS identity use.
    pub fn snapshot(&self) -> (Vec<u8>, Vec<Vec<u8>>, Vec<u8>) {
        let r = self.inner.read().unwrap();
        (
            r.cert_der.clone(),
            r.cert_chain_der.clone(),
            r.key_der.clone(),
        )
    }
}
