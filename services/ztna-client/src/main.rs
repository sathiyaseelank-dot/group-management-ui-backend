mod acl;
mod auth;
mod config;
mod posture;
mod server;
mod socks5;
mod token_store;
mod tun;
mod tun_dns;
mod tun_routing;
mod tunnel;

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
use sha2::{Digest, Sha256};
use token_store::list_workspace_states;
use tracing::info;

#[tokio::main]
async fn main() {
    rustls::crypto::ring::default_provider()
        .install_default()
        .expect("failed to install rustls crypto provider");

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
    match state.config.mode.as_str() {
        "socks5" => start_socks_listener(&state.config),
        _ => start_tun_listener(&state.config),
    }
    let app = router(state.clone());
    let addr = format!("{}:{}", state.config.callback_bind_addr, state.config.port);
    let callback_host = state.config.effective_callback_host();
    info!(
        "ztna-client listening on http://{} (redirect host: {})",
        addr, callback_host
    );
    let listener = tokio::net::TcpListener::bind(&addr)
        .await
        .expect("failed to bind");
    tokio::spawn(server::run_posture_reporter(state.config.clone()));
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

    if let Some(tenant) = initial_tenant.or_else(|| {
        if stateful_default_tenant(&config).is_empty() {
            None
        } else {
            Some(stateful_default_tenant(&config))
        }
    }) {
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

fn stateful_default_tenant(config: &Config) -> String {
    config.tenant.clone()
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

fn start_socks_listener(config: &Config) {
    if config.socks5_addr.trim().is_empty() {
        return;
    }

    let config_for_ca = config.clone();
    let socks5_addr = config.socks5_addr.clone();
    let socks5_addr_for_listener = socks5_addr.clone();
    let controller_url = config.controller_url.clone();
    let callback_bind_addr = config.callback_bind_addr.clone();
    let callback_host = config.callback_host.clone();
    let tenant = config.tenant.clone();
    let connector_tunnel_addr = config.connector_tunnel_addr.clone();

    tokio::spawn(async move {
        let fallback_ca_pem: Arc<Vec<u8>> = Arc::new(load_ca_pem(&config_for_ca).await);
        let handler = move |req: socks5::ConnectRequest, mut stream: tokio::net::TcpStream| {
            let config = Config {
                controller_url: controller_url.clone(),
                port: 0,
                callback_bind_addr: callback_bind_addr.clone(),
                callback_host: callback_host.clone(),
                socks5_addr: socks5_addr.clone(),
                tenant: tenant.clone(),
                connector_tunnel_addr: connector_tunnel_addr.clone(),
                internal_ca_cert: String::from_utf8_lossy(fallback_ca_pem.as_ref()).to_string(),
                ca_cert_path: String::new(),
                mode: String::from("socks5"),
                tun_name: String::new(),
                tun_addr: String::new(),
                tun_mtu: 1500,
                command: None,
            };
            let tenant = tenant.clone();
            let connector_tunnel_addr = connector_tunnel_addr.clone();
            let fallback_ca_pem = Arc::clone(&fallback_ca_pem);

            async move {
                if tenant.is_empty() {
                    tracing::warn!(
                        "ZTNA_TENANT not set; split-tunnel SOCKS5 requests cannot be evaluated"
                    );
                    socks5::reply_error(&mut stream).await;
                    return;
                }

                let state = match ensure_workspace_state(&config, &tenant, false).await {
                    Ok(state) => state,
                    Err(e) => {
                        tracing::warn!(
                            "[split tunnel] failed to refresh workspace state for {}:{}: {}",
                            req.destination,
                            req.port,
                            e
                        );
                        socks5::reply_error(&mut stream).await;
                        return;
                    }
                };

                let acl_resp = match acl::check_access(
                    &config.controller_url,
                    &state.session.access_token,
                    &req.destination,
                    req.port,
                )
                .await
                {
                    Ok(resp) => resp,
                    Err(e) => {
                        tracing::warn!(
                            "[split tunnel] check-access failed for {}:{}: {}",
                            req.destination,
                            req.port,
                            e
                        );
                        socks5::reply_error(&mut stream).await;
                        return;
                    }
                };

                if !acl_resp.allowed {
                    info!(
                        "[split tunnel] denied {}:{} reason={} resource_id={}",
                        req.destination, req.port, acl_resp.reason, acl_resp.resource_id
                    );
                    socks5::reply_error(&mut stream).await;
                    return;
                }

                if connector_tunnel_addr.trim().is_empty() {
                    tracing::warn!(
                        "CONNECTOR_TUNNEL_ADDR not set; cannot forward {}:{} through the split tunnel",
                        req.destination,
                        req.port
                    );
                    socks5::reply_error(&mut stream).await;
                    return;
                }

                let ca_pem = if !fallback_ca_pem.is_empty() {
                    fallback_ca_pem.as_ref().clone()
                } else if !state.device.ca_cert_pem.is_empty() {
                    state.device.ca_cert_pem.as_bytes().to_vec()
                } else {
                    Vec::new()
                };
                if ca_pem.is_empty() {
                    tracing::warn!("connector CA not available for split tunnel");
                    socks5::reply_error(&mut stream).await;
                    return;
                }

                let mut tunnel_stream = match tunnel::open(
                    &connector_tunnel_addr,
                    &ca_pem,
                    &state.session.access_token,
                    &req.destination,
                    req.port,
                )
                .await
                {
                    Ok(stream) => stream,
                    Err(e) => {
                        tracing::warn!(
                            "[split tunnel] tunnel open failed for {}:{}: {}",
                            req.destination,
                            req.port,
                            e
                        );
                        socks5::reply_error(&mut stream).await;
                        return;
                    }
                };

                if let Err(e) = socks5::reply_success(&mut stream).await {
                    tracing::warn!("SOCKS5 reply failed: {}", e);
                    return;
                }

                match tokio::io::copy_bidirectional(&mut stream, &mut tunnel_stream).await {
                    Ok((sent, recv)) => info!(
                        "[split tunnel] closed {}:{} sent={} recv={}",
                        req.destination, req.port, sent, recv
                    ),
                    Err(e) => tracing::warn!(
                        "[split tunnel] I/O error {}:{}: {}",
                        req.destination,
                        req.port,
                        e
                    ),
                }
            }
        };

        if let Err(e) = socks5::listen(&socks5_addr_for_listener, handler).await {
            tracing::warn!("SOCKS5 listener stopped: {}", e);
        }
    });
}

fn start_tun_listener(config: &Config) {
    let config = config.clone();
    tokio::spawn(async move {
        let mut config = config;
        let ca_pem = load_ca_pem(&config).await;
        if !ca_pem.is_empty() && config.internal_ca_cert.is_empty() {
            config.internal_ca_cert = String::from_utf8_lossy(&ca_pem).to_string();
            config.ca_cert_path.clear();
        }
        if let Err(e) = tun::run_tun_listener(&config).await {
            tracing::error!("[tun] listener failed: {}", e);
            tracing::info!("Hint: TUN mode requires root / CAP_NET_ADMIN. Try --mode socks5 for unprivileged use.");
        }
    });
}

pub(crate) async fn load_ca_pem(config: &Config) -> Vec<u8> {
    if !config.internal_ca_cert.is_empty() {
        let bytes = config.internal_ca_cert.as_bytes().to_vec();
        info!(
            "loaded connector CA from INTERNAL_CA_CERT: {}",
            describe_ca_pem(&bytes)
        );
        return bytes;
    }
    if !config.ca_cert_path.is_empty() {
        match std::fs::read(&config.ca_cert_path) {
            Ok(bytes) => {
                info!(
                    "loaded connector CA from {}: {}",
                    config.ca_cert_path,
                    describe_ca_pem(&bytes)
                );
                return bytes;
            }
            Err(e) => tracing::warn!("failed to read CA cert from {}: {}", config.ca_cert_path, e),
        }
    }
    match fetch_controller_ca(&config.controller_url).await {
        Ok(bytes) => {
            info!(
                "loaded connector CA from {}/ca.crt: {}",
                config.controller_url.trim_end_matches('/'),
                describe_ca_pem(&bytes)
            );
            return bytes;
        }
        Err(e) => tracing::warn!(
            "failed to fetch controller CA from {}/ca.crt: {}",
            config.controller_url.trim_end_matches('/'),
            e
        ),
    }
    tracing::warn!(
        "no connector CA configured; set CA_CERT_PATH or INTERNAL_CA_CERT, or login once to cache the workspace CA"
    );
    Vec::new()
}

async fn fetch_controller_ca(controller_url: &str) -> Result<Vec<u8>> {
    let ca_url = format!("{}/ca.crt", controller_url.trim_end_matches('/'));
    info!("Fetching controller CA from {}...", ca_url);
    let resp = reqwest::Client::new().get(&ca_url).send().await?;
    if !resp.status().is_success() {
        anyhow::bail!("HTTP {}", resp.status());
    }
    Ok(resp.bytes().await?.to_vec())
}

pub(crate) fn describe_ca_pem(ca_pem: &[u8]) -> String {
    if ca_pem.is_empty() {
        return "empty PEM".to_string();
    }

    let certs = std::str::from_utf8(ca_pem)
        .ok()
        .and_then(|pem| pem::parse_many(pem).ok())
        .map(|entries| {
            entries
                .into_iter()
                .filter(|entry| entry.tag() == "CERTIFICATE")
                .count()
        })
        .unwrap_or(0);

    let digest = Sha256::digest(ca_pem);
    let fingerprint = digest[..8]
        .iter()
        .map(|byte| format!("{:02x}", byte))
        .collect::<String>();

    format!(
        "{} bytes, {} cert block(s), sha256:{}",
        ca_pem.len(),
        certs,
        fingerprint
    )
}
