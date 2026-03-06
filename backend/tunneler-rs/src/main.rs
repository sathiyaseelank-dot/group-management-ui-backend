mod config;
mod enroll;
mod renewal;
mod run;
mod tls;

use anyhow::Result;
use clap::{Parser, Subcommand};
use tracing::error;

#[derive(Parser)]
#[command(name = "grpctunneler2", about = "Arise tunneler (Rust)")]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Enroll this tunneler with the controller (one-time)
    Enroll,
    /// Run the tunneler service
    Run,
}

#[tokio::main]
async fn main() {
    rustls::crypto::ring::default_provider()
        .install_default()
        .expect("Failed to install rustls crypto provider");

    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "info".into()),
        )
        .init();

    let cli = Cli::parse();
    if let Err(e) = run_cli(cli).await {
        error!("{:#}", e);
        std::process::exit(1);
    }
}

async fn run_cli(cli: Cli) -> Result<()> {
    match cli.command {
        Commands::Enroll => {
            let cfg = config::enroll_config_from_env()?;
            let result = enroll::enroll(&cfg).await?;
            println!("Enrolled tunneler with SPIFFE ID: {}", result.spiffe_id);
            tracing::info!("enrollment completed successfully");
            Ok(())
        }
        Commands::Run => run::run().await,
    }
}
