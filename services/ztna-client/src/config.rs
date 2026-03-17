use clap::{Parser, Subcommand};

#[derive(Parser, Debug, Clone)]
#[command(name = "ztna-client", about = "ZTNA native client")]
pub struct Config {
    /// Controller URL (e.g. http://localhost:8081)
    #[arg(long, env = "CONTROLLER_URL", default_value = "http://localhost:8081")]
    pub controller_url: String,

    /// Local port to listen on for browser callbacks
    #[arg(long, env = "ZTNA_CLIENT_PORT", default_value_t = 19515)]
    pub port: u16,

    /// Local SOCKS5 proxy address used for split-tunneled access
    #[arg(long, env = "SOCKS5_ADDR", default_value = "127.0.0.1:1080")]
    pub socks5_addr: String,

    /// Default workspace slug used by the local split-tunnel proxy
    #[arg(long, env = "ZTNA_TENANT", default_value = "")]
    pub tenant: String,

    /// Connector device tunnel address (host:port). Empty disables split-tunneled transport.
    #[arg(long, env = "CONNECTOR_TUNNEL_ADDR", default_value = "")]
    pub connector_tunnel_addr: String,

    /// Inline PEM for the connector CA. Used when workspace CA is not yet cached locally.
    #[arg(long, env = "INTERNAL_CA_CERT", default_value = "")]
    pub internal_ca_cert: String,

    /// Path to a PEM file for the connector CA.
    #[arg(long, env = "CA_CERT_PATH", default_value = "")]
    pub ca_cert_path: String,

    /// Transport mode: "tun" (transparent, requires root) or "socks5" (proxy)
    #[arg(long, env = "ZTNA_MODE", default_value = "tun")]
    pub mode: String,

    /// TUN device name
    #[arg(long, env = "TUN_NAME", default_value = "ztna0")]
    pub tun_name: String,

    /// TUN device address in CIDR notation
    #[arg(long, env = "TUN_ADDR", default_value = "10.200.0.1/24")]
    pub tun_addr: String,

    /// TUN device MTU
    #[arg(long, env = "TUN_MTU", default_value_t = 1500)]
    pub tun_mtu: u16,

    #[command(subcommand)]
    pub command: Option<Command>,
}

#[derive(Subcommand, Debug, Clone)]
pub enum Command {
    /// Run the interactive terminal client
    Ui {
        /// Workspace slug to prefill for login
        #[arg(long)]
        tenant: Option<String>,
    },
    /// Start browser login for a workspace and wait for completion
    Login {
        #[arg(long)]
        tenant: String,
        #[arg(long, default_value_t = 180)]
        timeout_secs: u64,
    },
    /// Show saved workspace sessions
    Status {
        #[arg(long)]
        tenant: Option<String>,
    },
    /// Sync the current device-user view for a workspace
    Sync {
        #[arg(long)]
        tenant: String,
    },
    /// List authorized resources for a workspace
    Resources {
        #[arg(long)]
        tenant: Option<String>,
    },
    /// Revoke and clear a workspace session
    Disconnect {
        #[arg(long)]
        tenant: String,
    },
    /// Run only the callback server
    Serve,
}
