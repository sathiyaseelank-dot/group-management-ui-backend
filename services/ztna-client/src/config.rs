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
