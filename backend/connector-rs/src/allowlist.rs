use serde::Deserialize;
use std::collections::HashSet;
use std::sync::RwLock;

#[derive(Debug, Deserialize)]
#[allow(dead_code)]
pub struct TunnelerInfo {
    pub tunneler_id: String,
    pub spiffe_id: String,
}

/// In-memory SPIFFE-ID allowlist for tunnelers.
pub struct TunnelerAllowlist {
    inner: RwLock<HashSet<String>>,
}

impl TunnelerAllowlist {
    pub fn new() -> Self {
        Self {
            inner: RwLock::new(HashSet::new()),
        }
    }

    pub fn allowed(&self, spiffe_id: &str) -> bool {
        self.inner.read().unwrap().contains(spiffe_id)
    }

    pub fn replace(&self, items: Vec<TunnelerInfo>) {
        let mut w = self.inner.write().unwrap();
        *w = items
            .into_iter()
            .filter(|i| !i.spiffe_id.is_empty())
            .map(|i| i.spiffe_id)
            .collect();
    }

    pub fn add(&self, spiffe_id: &str) {
        if spiffe_id.is_empty() {
            return;
        }
        self.inner.write().unwrap().insert(spiffe_id.to_string());
    }
}

impl Default for TunnelerAllowlist {
    fn default() -> Self {
        Self::new()
    }
}
