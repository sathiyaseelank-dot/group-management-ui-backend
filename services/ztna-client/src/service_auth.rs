//! Service-to-CLI local auth token.
//!
//! The management API is bound to 127.0.0.1 (loopback-only).  To further
//! restrict which local processes may call it, the service writes a random
//! token to `<state_dir>/.service.token` at startup.  The CLI reads that
//! token and supplies it in every `X-Service-Token` request header.
//!
//! Token file layout:
//!   - path:  `<state_dir>/.service.token`
//!   - mode:  0644  (world-readable by filename; non-root can read it)
//!   - state_dir mode adjusted to 0711 so users can traverse but not list

use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};

use anyhow::Result;

const TOKEN_FILENAME: &str = ".service.token";
const TOKEN_BYTES: usize = 32;

fn token_path(state_dir: &Path) -> PathBuf {
    state_dir.join(TOKEN_FILENAME)
}

/// Generate a new random token, persist it to `<state_dir>/.service.token`,
/// and return the token string.
///
/// Also ensures `state_dir` is mode 0711 so non-root users can traverse into
/// it (stat a known filename) without being able to list its contents.
pub fn init_service_token(state_dir: &Path) -> Result<String> {
    use rand::RngCore;

    std::fs::create_dir_all(state_dir)?;
    // 0711: owner rwx, others --x (traverse only, no listing)
    std::fs::set_permissions(state_dir, std::fs::Permissions::from_mode(0o711))?;

    let mut bytes = [0u8; TOKEN_BYTES];
    rand::rngs::OsRng.fill_bytes(&mut bytes);
    let token: String = bytes.iter().map(|b| format!("{b:02x}")).collect();

    let path = token_path(state_dir);
    std::fs::write(&path, token.as_bytes())?;
    std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o644))?;

    Ok(token)
}

/// Read the service token from `<state_dir>/.service.token`.
///
/// Returns `None` if the file does not exist or cannot be read — callers
/// should treat this as "no auth configured" and proceed without a token
/// (the server will reject if it is enforcing).
pub fn read_service_token(state_dir: &Path) -> Option<String> {
    std::fs::read_to_string(token_path(state_dir))
        .ok()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
}
