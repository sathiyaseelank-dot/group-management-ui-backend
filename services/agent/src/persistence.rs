use anyhow::Result;
use tracing::info;

use crate::enroll::EnrollResult;

/// Agent operates entirely in-memory. No state is persisted to disk.
/// Enrollment happens fresh on every start; firewall rules are rebuilt
/// from the connector's policy push.

pub fn load_saved_enrollment() -> Result<Option<EnrollResult>> {
    info!("agent runs in memory-only mode, skipping saved enrollment");
    Ok(None)
}

pub fn save_enrollment(_result: &EnrollResult) -> Result<()> {
    // no-op: agent does not persist to disk
    Ok(())
}

pub fn save_firewall_state(_state: &crate::firewall::FirewallState) -> Result<()> {
    // no-op: agent does not persist to disk
    Ok(())
}

pub fn load_firewall_state() -> Result<Option<crate::firewall::FirewallState>> {
    Ok(None)
}
