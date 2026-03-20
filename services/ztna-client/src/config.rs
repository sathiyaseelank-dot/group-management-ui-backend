use clap::{Parser, Subcommand};
use serde::Deserialize;
use std::net::{IpAddr, UdpSocket};
use std::path::{Path, PathBuf};
use tracing::info;

// ---------------------------------------------------------------------------
// TOML config file
// ---------------------------------------------------------------------------

/// Schema for the optional TOML configuration file.
///
/// All fields are optional.  Missing keys fall through to environment
/// variables (via clap), then to hard-coded defaults.
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default)]
struct FileConfig {
    controller_url: Option<String>,
    controller_grpc_addr: Option<String>,
    tenant: Option<String>,
    ca_cert_path: Option<String>,
    mode: Option<String>,
    connector_tunnel_addr: Option<String>,
    socks5_addr: Option<String>,
    port: Option<u16>,
    callback_bind_addr: Option<String>,
    callback_host: Option<String>,
    tun_name: Option<String>,
    tun_addr: Option<String>,
    tun_mtu: Option<u16>,
    internal_ca_cert: Option<String>,
    state_dir: Option<String>,
}

/// Load and parse a TOML config file.  Returns `(config, loaded)`.
/// Missing files silently return defaults; parse errors are warned.
fn load_config_file(path: &str) -> (FileConfig, bool) {
    let p = Path::new(path);
    if !p.exists() {
        return (FileConfig::default(), false);
    }
    match std::fs::read_to_string(p) {
        Ok(contents) => match toml::from_str::<FileConfig>(&contents) {
            Ok(cfg) => (cfg, true),
            Err(e) => {
                eprintln!("warning: failed to parse {}: {}", path, e);
                (FileConfig::default(), false)
            }
        },
        Err(e) => {
            eprintln!("warning: cannot read {}: {}", path, e);
            (FileConfig::default(), false)
        }
    }
}

// ---------------------------------------------------------------------------
// CLI argument parsing (raw — config fields are Option for precedence)
// ---------------------------------------------------------------------------

#[derive(Parser, Debug)]
#[command(name = "ztna-client", about = "ZTNA native client")]
struct CliArgs {
    /// Path to TOML configuration file
    #[arg(long, default_value = "/etc/ztna-client/client.conf")]
    config_file: String,

    /// Controller HTTP URL
    #[arg(long, env = "CONTROLLER_URL")]
    controller_url: Option<String>,

    /// Controller gRPC address (host:port)
    #[arg(long, env = "CONTROLLER_GRPC_ADDR")]
    controller_grpc_addr: Option<String>,

    /// Local port for browser OAuth callbacks
    #[arg(long, env = "ZTNA_CLIENT_PORT")]
    port: Option<u16>,

    /// Bind address for OAuth callback server
    #[arg(long, env = "ZTNA_CLIENT_CALLBACK_BIND_ADDR")]
    callback_bind_addr: Option<String>,

    /// Host advertised in OAuth redirect URI
    #[arg(long, env = "ZTNA_CLIENT_CALLBACK_HOST")]
    callback_host: Option<String>,

    /// SOCKS5 proxy listen address
    #[arg(long, env = "SOCKS5_ADDR")]
    socks5_addr: Option<String>,

    /// Default workspace slug
    #[arg(long, env = "ZTNA_TENANT")]
    tenant: Option<String>,

    /// Connector device tunnel address (host:port)
    #[arg(long, env = "CONNECTOR_TUNNEL_ADDR")]
    connector_tunnel_addr: Option<String>,

    /// Inline PEM for the connector CA
    #[arg(long, env = "INTERNAL_CA_CERT")]
    internal_ca_cert: Option<String>,

    /// Path to connector CA PEM file
    #[arg(long, env = "CA_CERT_PATH")]
    ca_cert_path: Option<String>,

    /// Transport mode: "tun" or "socks5"
    #[arg(long, env = "ZTNA_MODE")]
    mode: Option<String>,

    /// TUN device name
    #[arg(long, env = "TUN_NAME")]
    tun_name: Option<String>,

    /// TUN device address (CIDR)
    #[arg(long, env = "TUN_ADDR")]
    tun_addr: Option<String>,

    /// TUN device MTU
    #[arg(long, env = "TUN_MTU")]
    tun_mtu: Option<u16>,

    /// Runtime state directory override
    #[arg(long, env = "ZTNA_STATE_DIR")]
    state_dir: Option<String>,

    #[command(subcommand)]
    command: Option<Command>,
}

// ---------------------------------------------------------------------------
// Subcommands
// ---------------------------------------------------------------------------

#[derive(Subcommand, Debug, Clone)]
pub enum Command {
    /// Run the interactive terminal client
    Ui {
        /// Workspace slug to prefill for login
        #[arg(long)]
        tenant: Option<String>,
    },
    /// Start browser login for a workspace
    Login {
        /// Workspace slug (defaults to tenant in config)
        #[arg(long)]
        tenant: Option<String>,
        /// Timeout in seconds waiting for browser callback
        #[arg(long, default_value_t = 180)]
        timeout_secs: u64,
    },
    /// Show saved workspace sessions
    Status {
        #[arg(long)]
        tenant: Option<String>,
    },
    /// Sync resources for a workspace
    Sync {
        /// Workspace slug (defaults to tenant in config)
        #[arg(long)]
        tenant: Option<String>,
    },
    /// List authorized resources
    Resources {
        #[arg(long)]
        tenant: Option<String>,
    },
    /// Revoke and clear a workspace session
    Disconnect {
        /// Workspace slug (defaults to tenant in config)
        #[arg(long)]
        tenant: Option<String>,
    },
    /// Revoke and clear a workspace session (alias for disconnect)
    Logout {
        /// Workspace slug (defaults to tenant in config)
        #[arg(long)]
        tenant: Option<String>,
    },
    /// Run the background service (for systemd)
    Serve,
}

// ---------------------------------------------------------------------------
// Resolved configuration
// ---------------------------------------------------------------------------

/// Fully resolved configuration.
///
/// Precedence for each field: CLI flag > environment variable > config file > default.
#[derive(Debug, Clone)]
pub struct Config {
    /// Controller HTTP URL (OAuth callbacks, ACL checks)
    pub controller_url: String,
    /// Controller gRPC address (host:port)
    pub controller_grpc_addr: String,
    /// Local port for browser OAuth callbacks
    pub port: u16,
    /// Bind address for OAuth callback server
    pub callback_bind_addr: String,
    /// Host advertised in OAuth redirect URI (empty = auto-detect)
    pub callback_host: String,
    /// SOCKS5 proxy listen address
    pub socks5_addr: String,
    /// Default workspace slug
    pub tenant: String,
    /// Connector device tunnel address (empty = split-tunnel disabled)
    pub connector_tunnel_addr: String,
    /// Inline PEM for the connector CA
    pub internal_ca_cert: String,
    /// Path to connector CA PEM file
    pub ca_cert_path: String,
    /// Transport mode: "tun" or "socks5"
    pub mode: String,
    /// TUN device name
    pub tun_name: String,
    /// TUN device address (CIDR)
    pub tun_addr: String,
    /// TUN device MTU
    pub tun_mtu: u16,
    /// Resolved runtime state directory
    pub state_dir: PathBuf,
    /// Whether a TOML config file was successfully loaded at startup.
    /// When true, indicates a product install (as opposed to dev mode).
    pub config_file_loaded: bool,
    /// Active subcommand
    pub command: Option<Command>,
}

impl Config {
    /// Load configuration with precedence: CLI > ENV > config file > default.
    ///
    /// Clap handles CLI and ENV resolution.  If neither provides a value,
    /// the TOML config file is consulted before falling back to defaults.
    pub fn load() -> Self {
        let cli = CliArgs::parse();
        let (file, file_loaded) = load_config_file(&cli.config_file);

        // Log config file status
        if file_loaded {
            info!("config: loaded {}", cli.config_file);
        } else if Path::new(&cli.config_file).exists() {
            info!("config: {} exists but failed to parse", cli.config_file);
        } else {
            info!("config: no config file at {}", cli.config_file);
        }

        // Track tenant source for logging
        let tenant_source = if cli.tenant.is_some() {
            if std::env::var("ZTNA_TENANT").is_ok() { "env" } else { "cli" }
        } else if file.tenant.is_some() {
            "config"
        } else {
            "default"
        };

        let config = Config {
            controller_url: cli.controller_url
                .or(file.controller_url)
                .unwrap_or_else(|| "http://localhost:8081".to_string()),
            controller_grpc_addr: cli.controller_grpc_addr
                .or(file.controller_grpc_addr)
                .unwrap_or_else(|| "localhost:8443".to_string()),
            port: cli.port
                .or(file.port)
                .unwrap_or(19515),
            callback_bind_addr: cli.callback_bind_addr
                .or(file.callback_bind_addr)
                .unwrap_or_else(|| "0.0.0.0".to_string()),
            callback_host: cli.callback_host
                .or(file.callback_host)
                .unwrap_or_default(),
            socks5_addr: cli.socks5_addr
                .or(file.socks5_addr)
                .unwrap_or_else(|| "127.0.0.1:1080".to_string()),
            tenant: cli.tenant
                .or(file.tenant)
                .unwrap_or_default(),
            connector_tunnel_addr: cli.connector_tunnel_addr
                .or(file.connector_tunnel_addr)
                .unwrap_or_default(),
            internal_ca_cert: cli.internal_ca_cert
                .or(file.internal_ca_cert)
                .unwrap_or_default(),
            ca_cert_path: cli.ca_cert_path
                .or(file.ca_cert_path)
                .unwrap_or_default(),
            mode: cli.mode
                .or(file.mode)
                .unwrap_or_else(|| "tun".to_string()),
            tun_name: cli.tun_name
                .or(file.tun_name)
                .unwrap_or_else(|| "ztna0".to_string()),
            tun_addr: cli.tun_addr
                .or(file.tun_addr)
                .unwrap_or_else(|| "10.200.0.1/24".to_string()),
            tun_mtu: cli.tun_mtu
                .or(file.tun_mtu)
                .unwrap_or(1500),
            state_dir: resolve_state_dir(
                cli.state_dir.as_deref(),
                file.state_dir.as_deref(),
                file_loaded,
            ),
            config_file_loaded: file_loaded,
            command: cli.command,
        };

        // Log env overrides
        let mut env_overrides = Vec::new();
        for key in &[
            "CONTROLLER_URL",
            "CONTROLLER_GRPC_ADDR",
            "ZTNA_TENANT",
            "ZTNA_MODE",
            "CONNECTOR_TUNNEL_ADDR",
            "CA_CERT_PATH",
            "INTERNAL_CA_CERT",
        ] {
            if std::env::var(key).is_ok() {
                env_overrides.push(*key);
            }
        }
        if !env_overrides.is_empty() {
            info!("config: env overrides: {}", env_overrides.join(", "));
        }

        // Log final resolved values
        info!("config: controller_url={}", config.controller_url);
        info!(
            "config: tenant={} (from {})",
            if config.tenant.is_empty() { "(none)" } else { &config.tenant },
            tenant_source
        );
        info!("config: state_dir={}", config.state_dir.display());

        config
    }

    /// Whether the client has a usable configuration.
    ///
    /// Returns `true` when a meaningful controller URL is set (non-empty,
    /// not the bare localhost default), OR when using the localhost default
    /// but a tenant is also configured (indicating intentional dev use).
    pub fn is_configured(&self) -> bool {
        if self.controller_url.is_empty() {
            return false;
        }
        if self.controller_url == "http://localhost:8081" {
            // Localhost default is only considered "configured" if a tenant
            // was also explicitly set (e.g. dev mode with ZTNA_TENANT).
            return !self.tenant.is_empty();
        }
        true
    }

    /// Resolve a per-command tenant, falling back to the config-level tenant.
    pub fn resolve_tenant(&self, cmd_tenant: Option<&str>) -> Option<String> {
        cmd_tenant.map(|s| s.to_string()).or_else(|| {
            if self.tenant.is_empty() {
                None
            } else {
                Some(self.tenant.clone())
            }
        })
    }

    /// Like [`resolve_tenant`](Self::resolve_tenant) but returns an error
    /// when no tenant is available from either the command or config.
    pub fn require_tenant(&self, cmd_tenant: Option<&str>) -> anyhow::Result<String> {
        self.resolve_tenant(cmd_tenant).ok_or_else(|| {
            anyhow::anyhow!(
                "no tenant specified; use --tenant or set tenant in config file"
            )
        })
    }

    /// Returns true when this is a product install (config file loaded)
    /// and the current process is NOT running as root.  In this scenario,
    /// CLI commands should proxy through the running service instead of
    /// directly accessing state files.
    pub fn should_proxy_to_service(&self) -> bool {
        self.config_file_loaded && !is_running_as_root()
    }

    /// Port for the management API (always localhost-only).
    ///
    /// Derived as `port + 1` so that the callback listener can stay on
    /// `port` (default 19515) without a bind conflict when
    /// `callback_bind_addr = "0.0.0.0"`.  No separate config needed.
    pub fn management_port(&self) -> u16 {
        self.port.saturating_add(1)
    }

    /// Base URL for the locally running management API.
    pub fn service_url(&self) -> String {
        format!("http://127.0.0.1:{}", self.management_port())
    }

    /// Effective callback host — returns the configured host or auto-detects
    /// a LAN address, falling back to `localhost`.
    pub fn effective_callback_host(&self) -> String {
        let host = self.callback_host.trim();
        if !host.is_empty() {
            return host.to_string();
        }
        detect_lan_ip().unwrap_or_else(|| "localhost".to_string())
    }
}

/// Product state directory.  When a product install is detected (config file
/// at `/etc/ztna-client/client.conf` exists, or running as root), both the
/// daemon and CLI commands share this path so state is never split.
const PRODUCT_STATE_DIR: &str = "/var/lib/ztna-client";

/// Resolve the runtime state directory.
///
/// Priority:
///   1. CLI `--state-dir` / env `ZTNA_STATE_DIR` (already merged by clap)
///   2. TOML config `state_dir`
///   3. Product install detected (config file loaded OR running as root):
///      `/var/lib/ztna-client`
///   4. Dev fallback: XDG data dir (`~/.local/share/ztna-client`)
fn resolve_state_dir(
    cli_or_env: Option<&str>,
    file_val: Option<&str>,
    config_file_loaded: bool,
) -> PathBuf {
    // Explicit override from CLI/env
    if let Some(v) = cli_or_env {
        if !v.is_empty() {
            return PathBuf::from(v);
        }
    }
    // Config file value
    if let Some(v) = file_val {
        if !v.is_empty() {
            return PathBuf::from(v);
        }
    }
    // Product install — config file loaded OR running as root.
    // This ensures `sudo ztna-client login` and the systemd service
    // (which also reads the config file) both resolve to the same path.
    if config_file_loaded || is_running_as_root() {
        return PathBuf::from(PRODUCT_STATE_DIR);
    }
    // Dev fallback — XDG data directory (not config, since this is runtime state)
    directories::ProjectDirs::from("com", "zerotrust", "ztna-client")
        .map(|dirs| dirs.data_local_dir().to_path_buf())
        .unwrap_or_else(|| PathBuf::from("/tmp/ztna-client"))
}

fn is_running_as_root() -> bool {
    #[cfg(unix)]
    {
        unsafe { libc::geteuid() == 0 }
    }
    #[cfg(not(unix))]
    {
        false
    }
}

fn detect_lan_ip() -> Option<String> {
    for target in ["1.1.1.1:80", "8.8.8.8:80", "192.168.1.1:80"] {
        let socket = UdpSocket::bind("0.0.0.0:0").ok()?;
        if socket.connect(target).is_err() {
            continue;
        }
        let ip = socket.local_addr().ok()?.ip();
        if is_usable_lan_ip(ip) {
            return Some(ip.to_string());
        }
    }
    None
}

fn is_usable_lan_ip(ip: IpAddr) -> bool {
    !ip.is_loopback() && !ip.is_unspecified()
}
