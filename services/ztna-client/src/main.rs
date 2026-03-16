mod auth;
mod config;
mod server;
mod token_store;

use std::collections::HashMap;
use std::io::{self, Write};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use anyhow::Result;
use clap::Parser;
use config::{Command, Config};
use server::{
    begin_login, disconnect_workspace, ensure_workspace_state, router, wait_for_login, AppState,
};
use token_store::list_workspace_states;
use tracing::info;

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt::init();

    let config = Config::parse();
    if let Err(err) = run(config).await {
        eprintln!("error: {err:#}");
        std::process::exit(1);
    }
}

async fn run(config: Config) -> Result<()> {
    match config
        .command
        .clone()
        .unwrap_or(Command::Ui { tenant: None })
    {
        Command::Serve => {
            let app_state = AppState {
                config: config.clone(),
                pending: Arc::new(Mutex::new(HashMap::new())),
            };
            run_callback_server(app_state).await
        }
        Command::Login {
            tenant,
            timeout_secs,
        } => {
            let app_state = AppState {
                config: config.clone(),
                pending: Arc::new(Mutex::new(HashMap::new())),
            };
            let _server = tokio::spawn(run_callback_server(app_state.clone()));
            let auth_url = begin_login(&app_state, &tenant).await?;
            println!("Open this URL if your browser did not launch:\n{auth_url}");
            let state = wait_for_login(&tenant, Duration::from_secs(timeout_secs)).await?;
            println!(
                "Connected to workspace {} as {}",
                state.workspace.slug, state.user.email
            );
            Ok(())
        }
        Command::Status { tenant } => {
            if let Some(tenant) = tenant {
                let state = ensure_workspace_state(&config, &tenant, false).await?;
                print_workspace_status(&state);
            } else {
                for state in list_workspace_states()? {
                    print_workspace_status(&state);
                }
            }
            Ok(())
        }
        Command::Sync { tenant } => {
            let state = ensure_workspace_state(&config, &tenant, true).await?;
            print_workspace_status(&state);
            println!("Resources synced: {}", state.resources.len());
            Ok(())
        }
        Command::Resources { tenant } => {
            if let Some(tenant) = tenant {
                let state = ensure_workspace_state(&config, &tenant, false).await?;
                print_resources(&state);
            } else {
                for state in list_workspace_states()? {
                    print_resources(&state);
                }
            }
            Ok(())
        }
        Command::Disconnect { tenant } => {
            disconnect_workspace(&config, &tenant).await?;
            println!("Disconnected from workspace {tenant}");
            Ok(())
        }
        Command::Ui { tenant } => run_ui(config, tenant).await,
    }
}

async fn run_callback_server(state: AppState) -> Result<()> {
    let app = router(state.clone());
    let addr = format!("127.0.0.1:{}", state.config.port);
    info!("ztna-client listening on http://{}", addr);
    let listener = tokio::net::TcpListener::bind(&addr)
        .await
        .expect("failed to bind");
    axum::serve(listener, app).await.expect("server failed");
    Ok(())
}

async fn run_ui(config: Config, initial_tenant: Option<String>) -> Result<()> {
    let app_state = AppState {
        config: config.clone(),
        pending: Arc::new(Mutex::new(HashMap::new())),
    };
    let _server = tokio::spawn(run_callback_server(app_state.clone()));

    println!("ZTNA Client");
    println!("Commands: login <tenant>, status [tenant], resources [tenant], sync <tenant>, disconnect <tenant>, quit");

    if let Some(tenant) = initial_tenant {
        let auth_url = begin_login(&app_state, &tenant).await?;
        println!("Starting login for {tenant}");
        println!("Open this URL if needed:\n{auth_url}");
    }

    loop {
        print!("ztna-client> ");
        io::stdout().flush()?;

        let mut input = String::new();
        if io::stdin().read_line(&mut input)? == 0 {
            break;
        }
        let input = input.trim();
        if input.is_empty() {
            continue;
        }
        let parts: Vec<&str> = input.split_whitespace().collect();
        match parts.as_slice() {
            ["quit"] | ["exit"] => break,
            ["login", tenant] => {
                let auth_url = begin_login(&app_state, tenant).await?;
                println!("Login started for {tenant}");
                println!("Open this URL if needed:\n{auth_url}");
            }
            ["status"] => {
                for state in list_workspace_states()? {
                    print_workspace_status(&state);
                }
            }
            ["status", tenant] => {
                let state = ensure_workspace_state(&config, tenant, false).await?;
                print_workspace_status(&state);
            }
            ["resources"] => {
                for state in list_workspace_states()? {
                    print_resources(&state);
                }
            }
            ["resources", tenant] => {
                let state = ensure_workspace_state(&config, tenant, false).await?;
                print_resources(&state);
            }
            ["sync", tenant] => {
                let state = ensure_workspace_state(&config, tenant, true).await?;
                println!("Synced {}", state.workspace.slug);
                print_resources(&state);
            }
            ["disconnect", tenant] => {
                disconnect_workspace(&config, tenant).await?;
                println!("Disconnected from {tenant}");
            }
            ["help"] => {
                println!("Commands: login <tenant>, status [tenant], resources [tenant], sync <tenant>, disconnect <tenant>, quit");
            }
            _ => {
                println!("Unknown command. Type `help`.");
            }
        }
    }

    Ok(())
}

fn print_workspace_status(state: &token_store::StoredWorkspaceState) {
    println!(
        "[{}] user={} role={} device={} resources={} token_expiry={}",
        state.workspace.slug,
        state.user.email,
        state.user.role,
        state.device.id,
        state.resources.len(),
        state.session.expires_at
    );
}

fn print_resources(state: &token_store::StoredWorkspaceState) {
    println!("Workspace: {}", state.workspace.slug);
    if state.resources.is_empty() {
        println!("  no authorized resources");
        return;
    }
    for resource in &state.resources {
        let ports = match (resource.port_from, resource.port_to) {
            (Some(from), Some(to)) if from == to => from.to_string(),
            (Some(from), Some(to)) => format!("{from}-{to}"),
            (Some(from), None) => from.to_string(),
            _ => "-".to_string(),
        };
        println!(
            "  {} [{}] {}:{} via {} ({})",
            resource.name,
            resource.firewall_status,
            resource.address,
            ports,
            resource.remote_network_name,
            resource.protocol
        );
    }
}
