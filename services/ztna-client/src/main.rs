mod acl;
mod auth;
mod config;
mod posture;
mod product;
mod server;
mod service_auth;
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
use config::{Command, Config};
use product::{CliResourceInfo, CliWorkspaceStatus};
use server::{
    begin_login, callback_router, disconnect_workspace, ensure_workspace_state, management_router,
    wait_for_login, AppState,
};
use sha2::{Digest, Sha256};
use token_store::{connector_tunnel_addr_for_resource, list_workspace_states};
use tracing::{info, warn};
use tracing_subscriber::EnvFilter;

/// Initialize logging with a command-appropriate default level.
///
/// Service mode (`serve`) keeps INFO-level output — operators need to see
/// connection events and route changes in journal logs.
///
/// All other CLI commands default to WARN so that routine operations like
/// `ztna-client status` or `ztna-client doctor` don't spray internal config
/// and routing logs at the user.  Set `RUST_LOG=info` to override.
fn init_logging(command: &Option<Command>) {
    let default_level = if matches!(command, Some(Command::Serve)) {
        "info"
    } else {
        "warn"
    };
    let filter = EnvFilter::builder()
        .with_default_directive(default_level.parse().expect("invalid log level"))
        .from_env_lossy();
    tracing_subscriber::fmt().with_env_filter(filter).init();
}

#[tokio::main]
async fn main() {
    rustls::crypto::ring::default_provider()
        .install_default()
        .expect("failed to install rustls crypto provider");

    // Parse config first so we know the active command before initializing logging.
    let config = Config::load();
    init_logging(&config.command);
    token_store::init_state_dir(config.state_dir.clone());
    if let Err(err) = run(config).await {
        eprintln!("error: {err:#}");
        std::process::exit(1);
    }
}

async fn run(config: Config) -> Result<()> {
    // Detect a broken product install early: config file exists but is invalid.
    // `doctor` always runs (it's the diagnostic tool).  `serve` has its own
    // loop.  Everything else gets a clear actionable error.
    if config.is_product_install_broken() {
        let active_cmd = config
            .command
            .clone()
            .unwrap_or(Command::Ui { tenant: None });
        if !matches!(
            active_cmd,
            Command::Doctor | Command::Report | Command::Serve
        ) {
            eprintln!(
                "Config file exists but failed to parse: /etc/ztna-client/client.conf\n\
                 \n\
                 Fix the syntax and restart the service:\n\
                 \n\
                 \x20 sudo nano /etc/ztna-client/client.conf\n\
                 \x20 sudo systemctl restart ztna-client\n\
                 \n\
                 Run 'ztna-client doctor' for a full diagnostic."
            );
            std::process::exit(1);
        }
    }

    match config
        .command
        .clone()
        .unwrap_or(Command::Ui { tenant: None })
    {
        Command::Doctor => run_doctor(&config).await,
        Command::Report => run_report(&config).await,
        Command::Serve => {
            if !config.is_configured() {
                info!("client not configured; create /etc/ztna-client/client.conf and restart the service");
                loop {
                    tokio::time::sleep(Duration::from_secs(60)).await;
                    info!("waiting for configuration...");
                }
            }
            info!("starting ztna-client service in {} mode", config.mode);
            let service_token =
                service_auth::init_service_token(&config.state_dir).unwrap_or_else(|e| {
                    warn!(
                        "failed to write service token: {}; management API will be unprotected",
                        e
                    );
                    String::new()
                });
            let app_state = AppState {
                config: config.clone(),
                pending: Arc::new(Mutex::new(HashMap::new())),
                service_token,
            };
            run_service_listeners(app_state).await
        }
        Command::Login {
            tenant,
            timeout_secs,
        } => {
            let tenant = config.require_tenant(tenant.as_deref())?;
            if config.should_proxy_to_service() {
                run_proxied_login(&config, &tenant, timeout_secs).await
            } else {
                run_direct_login(config, &tenant, timeout_secs).await
            }
        }
        Command::Status { tenant } => {
            if config.should_proxy_to_service() {
                run_proxied_status(&config, tenant.as_deref()).await
            } else {
                run_direct_status(&config, tenant.as_deref()).await
            }
        }
        Command::Sync { tenant } => {
            let tenant = config.require_tenant(tenant.as_deref())?;
            if config.should_proxy_to_service() {
                run_proxied_sync(&config, &tenant).await
            } else {
                run_direct_sync(&config, &tenant).await
            }
        }
        Command::Resources { tenant } => {
            if config.should_proxy_to_service() {
                run_proxied_resources(&config, tenant.as_deref()).await
            } else {
                run_direct_resources(&config, tenant.as_deref()).await
            }
        }
        Command::Disconnect { tenant } => {
            let tenant = config.require_tenant(tenant.as_deref())?;
            if config.should_proxy_to_service() {
                run_proxied_disconnect(&config, &tenant, "Disconnected").await
            } else {
                disconnect_workspace(&config, &tenant).await?;
                println!("Disconnected from workspace {tenant}");
                Ok(())
            }
        }
        Command::Logout { tenant } => {
            let tenant = config.require_tenant(tenant.as_deref())?;
            if config.should_proxy_to_service() {
                run_proxied_disconnect(&config, &tenant, "Logged out").await
            } else {
                disconnect_workspace(&config, &tenant).await?;
                println!("Logged out from workspace {tenant}");
                Ok(())
            }
        }
        Command::Ui { tenant } => {
            if config.should_proxy_to_service() && tenant.is_none() {
                run_product_noargs(&config).await
            } else {
                run_ui(config, tenant).await
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Service proxy helpers
// ---------------------------------------------------------------------------

async fn ensure_service_reachable(config: &Config) -> Result<()> {
    if product::is_service_running(config.management_port()).await {
        return Ok(());
    }
    anyhow::bail!(
        "ztna-client service is not reachable on 127.0.0.1:{}.\n\
         \n\
         If the service is not running:\n\
         \n\
         \x20 sudo systemctl start ztna-client\n\
         \n\
         If the service is running but not configured:\n\
         \n\
         \x20 sudo nano /etc/ztna-client/client.conf\n\
         \x20 sudo systemctl restart ztna-client",
        config.management_port()
    );
}

/// Read the service token from the product state directory.
///
/// Returns `None` in dev mode (state dir is XDG, no token file) or when the
/// file is missing.  Proxy functions treat `None` as "no auth header" — the
/// server will reject if enforcing.
fn load_service_token(config: &Config) -> Option<String> {
    service_auth::read_service_token(&config.state_dir)
}

// ---------------------------------------------------------------------------
// Proxied command handlers — CLI → service HTTP API
// ---------------------------------------------------------------------------

async fn run_proxied_login(config: &Config, tenant: &str, timeout_secs: u64) -> Result<()> {
    ensure_service_reachable(config).await?;
    let base_url = config.service_url();
    let token = load_service_token(config);

    let auth_url = product::proxy_login(&base_url, tenant, token.as_deref()).await?;
    if let Err(err) = open::that(&auth_url) {
        tracing::error!("failed to open browser: {}", err);
    }
    println!("Open this URL if your browser did not launch:\n{auth_url}");

    let ws = product::poll_login_complete(
        &base_url,
        tenant,
        Duration::from_secs(timeout_secs),
        token.as_deref(),
    )
    .await?;
    println!(
        "Connected to workspace {} as {}",
        ws.workspace_slug, ws.user_email
    );
    Ok(())
}

async fn run_proxied_status(config: &Config, tenant: Option<&str>) -> Result<()> {
    ensure_service_reachable(config).await?;
    let base_url = config.service_url();
    let token = load_service_token(config);
    let resp = product::proxy_status(&base_url, token.as_deref()).await?;

    if !resp.configured {
        eprintln!(
            "Service is running but not configured.\n\
             Edit /etc/ztna-client/client.conf and restart:\n\
             \n\
             \x20 sudo systemctl restart ztna-client"
        );
        return Ok(());
    }

    if let Some(tenant) = tenant {
        match resp.workspaces.iter().find(|w| w.workspace_slug == tenant) {
            Some(ws) => print_cli_status(ws, &resp.mode),
            None => println!("No session for workspace {tenant}. Run: ztna-client login"),
        }
    } else if resp.workspaces.is_empty() {
        println!(
            "Service running ({} mode), no active sessions.\nRun: ztna-client login",
            resp.mode
        );
    } else {
        for ws in &resp.workspaces {
            print_cli_status(ws, &resp.mode);
        }
    }
    Ok(())
}

async fn run_proxied_resources(config: &Config, tenant: Option<&str>) -> Result<()> {
    ensure_service_reachable(config).await?;
    let base_url = config.service_url();
    let token = load_service_token(config);

    if let Some(tenant) = tenant {
        let resources = product::proxy_resources(&base_url, tenant, token.as_deref()).await?;
        print_cli_resources(tenant, &resources);
    } else {
        let resp = product::proxy_status(&base_url, token.as_deref()).await?;
        if resp.workspaces.is_empty() {
            println!("No active sessions. Run: ztna-client login");
        } else {
            for ws in &resp.workspaces {
                print_cli_resources(&ws.workspace_slug, &ws.resources);
            }
        }
    }
    Ok(())
}

async fn run_proxied_sync(config: &Config, tenant: &str) -> Result<()> {
    ensure_service_reachable(config).await?;
    let base_url = config.service_url();
    let token = load_service_token(config);
    let ws = product::proxy_sync(&base_url, tenant, token.as_deref()).await?;
    print_cli_status(&ws, "");
    println!("Resources synced: {}", ws.resources.len());
    Ok(())
}

async fn run_proxied_disconnect(config: &Config, tenant: &str, verb: &str) -> Result<()> {
    ensure_service_reachable(config).await?;
    let base_url = config.service_url();
    let token = load_service_token(config);
    product::proxy_disconnect(&base_url, tenant, token.as_deref()).await?;
    println!("{verb} from workspace {tenant}");
    Ok(())
}

// ---------------------------------------------------------------------------
// Direct command handlers — state-file access (root or dev mode)
// ---------------------------------------------------------------------------

async fn run_direct_login(config: Config, tenant: &str, timeout_secs: u64) -> Result<()> {
    let app_state = AppState {
        config: config.clone(),
        pending: Arc::new(Mutex::new(HashMap::new())),
        service_token: String::new(),
    };
    let _server = tokio::spawn(run_direct_callback_server(app_state.clone()));
    let auth_url = begin_login(&app_state, tenant).await?;
    if let Err(err) = open::that(&auth_url) {
        tracing::error!("failed to open browser: {}", err);
    }
    println!("Open this URL if your browser did not launch:\n{auth_url}");
    let state = wait_for_login(tenant, Duration::from_secs(timeout_secs)).await?;
    println!(
        "Connected to workspace {} as {}",
        state.workspace.slug, state.user.email
    );
    Ok(())
}

async fn run_direct_status(config: &Config, tenant: Option<&str>) -> Result<()> {
    if let Some(tenant) = tenant {
        let state = ensure_workspace_state(config, tenant, false).await?;
        print_stored_status(&state);
    } else {
        let states = list_workspace_states()?;
        if states.is_empty() {
            println!("No active sessions.");
        } else {
            for state in states {
                print_stored_status(&state);
            }
        }
    }
    Ok(())
}

async fn run_direct_resources(config: &Config, tenant: Option<&str>) -> Result<()> {
    if let Some(tenant) = tenant {
        let state = ensure_workspace_state(config, tenant, false).await?;
        print_stored_resources(&state);
    } else {
        let states = list_workspace_states()?;
        if states.is_empty() {
            println!("No active sessions. Run: ztna-client login");
        } else {
            for state in states {
                print_stored_resources(&state);
            }
        }
    }
    Ok(())
}

async fn run_direct_sync(config: &Config, tenant: &str) -> Result<()> {
    let state = ensure_workspace_state(config, tenant, true).await?;
    print_stored_status(&state);
    println!("Resources synced: {}", state.resources.len());
    Ok(())
}

// ---------------------------------------------------------------------------
// Service listeners (systemd mode) and direct callback (dev/root mode)
// ---------------------------------------------------------------------------

/// Start both the management listener and the OAuth callback listener.
///
/// Used by `Command::Serve` only.  The management API binds exclusively to
/// 127.0.0.1 so control-plane endpoints are never reachable from the LAN.
/// The callback listener uses `callback_bind_addr` and may be LAN-exposed
/// for testing — only the `/callback` route is served there.
async fn run_service_listeners(state: AppState) -> Result<()> {
    match state.config.mode.as_str() {
        "socks5" => start_socks_listener(&state.config),
        _ => start_tun_listener(&state.config),
    }
    tokio::spawn(server::run_posture_reporter(state.config.clone()));

    // Management API — always localhost-only.
    let mgmt_addr = format!("127.0.0.1:{}", state.config.management_port());
    info!(
        "management API listening on http://{} (localhost only)",
        mgmt_addr
    );
    let mgmt_listener = tokio::net::TcpListener::bind(&mgmt_addr)
        .await
        .expect("failed to bind management listener");

    // OAuth callback listener — may bind to LAN for testing.
    let cb_addr = format!("{}:{}", state.config.callback_bind_addr, state.config.port);
    let cb_host = state.config.effective_callback_host();
    if !is_loopback_bind(&state.config.callback_bind_addr) {
        warn!(
            "callback listener bound to {} — non-loopback address intended for testing only",
            state.config.callback_bind_addr
        );
    }
    info!(
        "OAuth callback listening on http://{} (redirect host: {})",
        cb_addr, cb_host
    );
    let cb_listener = tokio::net::TcpListener::bind(&cb_addr)
        .await
        .expect("failed to bind callback listener");

    let mgmt_app = management_router(state.clone());
    let cb_app = callback_router(state.clone());

    // Run both listeners until a shutdown signal is received.
    //
    // Catching SIGTERM/SIGINT here triggers a graceful tokio runtime shutdown:
    // when this function returns, the runtime drops all spawned tasks (TUN
    // listener, posture reporter, etc.) and their Drop impls run — which
    // includes RouteManager::cleanup() that removes kernel routing rules.
    tokio::select! {
        _ = shutdown_signal() => {
            info!("shutdown signal received; stopping service");
        }
        result = axum::serve(mgmt_listener, mgmt_app) => {
            if let Err(e) = result { warn!("management server exited: {}", e); }
        }
        result = axum::serve(cb_listener, cb_app) => {
            if let Err(e) = result { warn!("callback server exited: {}", e); }
        }
    }
    Ok(())
}

/// Returns a future that resolves on SIGTERM (Unix) or Ctrl-C.
async fn shutdown_signal() {
    #[cfg(unix)]
    {
        use tokio::signal::unix::{signal, SignalKind};
        let mut sigterm =
            signal(SignalKind::terminate()).expect("failed to install SIGTERM handler");
        tokio::select! {
            _ = sigterm.recv() => { info!("received SIGTERM"); }
            _ = tokio::signal::ctrl_c() => { info!("received SIGINT"); }
        }
    }
    #[cfg(not(unix))]
    {
        tokio::signal::ctrl_c().await.ok();
    }
}

/// Start only the OAuth callback listener (no management API).
///
/// Used by direct-access paths (`run_direct_login`, `run_ui`) where the CLI
/// talks to state files directly and does not need the management API.
async fn run_direct_callback_server(state: AppState) -> Result<()> {
    let addr = format!("{}:{}", state.config.callback_bind_addr, state.config.port);
    let callback_host = state.config.effective_callback_host();
    if !is_loopback_bind(&state.config.callback_bind_addr) {
        warn!(
            "callback listener bound to {} — non-loopback address intended for testing only",
            state.config.callback_bind_addr
        );
    }
    info!(
        "OAuth callback listening on http://{} (redirect host: {})",
        addr, callback_host
    );
    let listener = tokio::net::TcpListener::bind(&addr)
        .await
        .expect("failed to bind callback listener");
    let app = callback_router(state);
    axum::serve(listener, app)
        .await
        .expect("callback server failed");
    Ok(())
}

fn is_loopback_bind(addr: &str) -> bool {
    addr == "127.0.0.1" || addr == "::1" || addr == "localhost"
}

// ---------------------------------------------------------------------------
// doctor — product diagnostics
// ---------------------------------------------------------------------------

async fn run_doctor(config: &Config) -> Result<()> {
    println!("ztna-client diagnostics");
    println!("{}", "-".repeat(40));

    // ── Install mode ──────────────────────────────────────────────────────
    let config_path = "/etc/ztna-client/client.conf";
    if config.config_file_loaded {
        println!("[OK]   install:  product ({config_path})");
    } else if config.config_file_exists {
        println!("[ERR]  install:  product config exists but FAILED TO PARSE");
        println!("       file:     {config_path}");
        println!("       action:   sudo nano {config_path}");
        println!("                 sudo systemctl restart ztna-client");
    } else if config.running_as_root() {
        println!("[OK]   install:  running as root (dev/direct mode)");
    } else {
        println!("[INFO] install:  dev mode (no product config file)");
    }

    // ── Config values ────────────────────────────────────────────────────
    if config.config_file_loaded {
        let configured = config.is_configured();
        let controller = &config.controller_url;
        let tenant = if config.tenant.is_empty() {
            "(none)"
        } else {
            &config.tenant
        };
        if configured {
            println!(
                "[OK]   config:   controller={controller}  tenant={tenant}  mode={}",
                config.mode
            );
        } else {
            println!("[WARN] config:   loaded but incomplete — set controller_url and tenant");
            println!("       controller: {controller}");
            println!("       tenant:     {tenant}");
        }
        let cb_bind = &config.callback_bind_addr;
        let cb_port = config.port;
        if is_loopback_bind(cb_bind) {
            println!("[OK]   callback:  {cb_bind}:{cb_port} (localhost)");
        } else {
            println!("[WARN] callback:  {cb_bind}:{cb_port} — LAN-exposed (for testing only)");
        }
    } else if config.config_file_exists {
        println!("[ERR]  config:   cannot check — config file is invalid");
    } else {
        println!("[INFO] config:   no product config (dev mode defaults)");
        println!(
            "       controller: {}  mode: {}",
            config.controller_url, config.mode
        );
    }

    // ── Service reachability ──────────────────────────────────────────────
    if config.config_file_loaded || config.config_file_exists {
        let mgmt_port = config.management_port();
        if product::is_service_running(mgmt_port).await {
            // Fetch live status from the service
            let base_url = config.service_url();
            let token = load_service_token(config);
            match product::proxy_status(&base_url, token.as_deref()).await {
                Ok(status) => {
                    if status.configured {
                        println!("[OK]   service:  running (mode={})", status.mode);
                    } else {
                        println!("[WARN] service:  running but not configured");
                        println!("       action:   edit {config_path} and restart");
                    }
                    // Sessions
                    if status.workspaces.is_empty() {
                        println!("[WARN] sessions: none — run: ztna-client login");
                    } else {
                        println!("[OK]   sessions: {} active", status.workspaces.len());
                        for ws in &status.workspaces {
                            let expiry = format_session_expiry(ws.session_expires_at);
                            println!(
                                "         [{}]  user={}  resources={}  session={}",
                                ws.workspace_slug,
                                ws.user_email,
                                ws.resources.len(),
                                expiry
                            );
                        }
                    }
                }
                Err(e) => {
                    println!("[WARN] service:  reachable but status failed: {e}");
                }
            }
        } else if config.config_file_loaded {
            println!("[ERR]  service:  not running on 127.0.0.1:{mgmt_port}");
            println!("       action:   sudo systemctl start ztna-client");
            println!("       logs:     sudo journalctl -u ztna-client -f");
        } else {
            // Broken config: service may or may not be running — we don't know the port
            println!("[WARN] service:  status unknown (config invalid, using default port)");
        }
    } else {
        // Dev mode — check direct session state
        match token_store::list_workspace_states() {
            Ok(states) if states.is_empty() => {
                println!("[INFO] sessions: none — run: ztna-client login");
            }
            Ok(states) => {
                println!("[OK]   sessions: {} active (dev mode)", states.len());
                for ws in &states {
                    let expiry = format_session_expiry(ws.session.expires_at);
                    println!(
                        "         [{}]  user={}  resources={}  session={}",
                        ws.workspace.slug,
                        ws.user.email,
                        ws.resources.len(),
                        expiry
                    );
                }
            }
            Err(e) => {
                println!("[WARN] sessions: could not read state: {e}");
            }
        }
    }

    println!("{}", "-".repeat(40));
    Ok(())
}

/// Machine-readable JSON support report — no secrets included.
///
/// Emits a JSON object to stdout with install mode, config status, service
/// reachability, and sanitized session info.  Intended for diagnostics,
/// support, and automated health checks.
async fn run_report(config: &Config) -> Result<()> {
    let mut report = serde_json::json!({
        "version": env!("CARGO_PKG_VERSION"),
        "install": {
            "product": config.config_file_loaded,
            "config_file_exists": config.config_file_exists,
            "config_file_parsed": config.config_file_loaded,
            "running_as_root": config.running_as_root(),
        },
        "config": {
            "configured": config.is_configured(),
            "controller_url": config.controller_url,
            "mode": config.mode,
            "tenant": config.tenant,
        },
    });

    // Service reachability and sessions
    if config.config_file_loaded || config.config_file_exists {
        let port = config.management_port();
        let running = product::is_service_running(port).await;
        report["service"] = serde_json::json!({ "running": running });
        if running {
            let token = load_service_token(config);
            match product::proxy_status(&config.service_url(), token.as_deref()).await {
                Ok(status) => {
                    report["service"]["configured"] = status.configured.into();
                    report["service"]["mode"] = status.mode.clone().into();
                    let sessions: Vec<_> = status
                        .workspaces
                        .iter()
                        .map(|w| {
                            serde_json::json!({
                                "workspace_slug": w.workspace_slug,
                                "user_email": w.user_email,
                                "resource_count": w.resources.len(),
                                "session_expires_at": w.session_expires_at,
                            })
                        })
                        .collect();
                    report["sessions"] = sessions.into();
                }
                Err(e) => {
                    report["service"]["error"] = e.to_string().into();
                }
            }
        }
    } else {
        // Dev mode — read state files directly
        if let Ok(states) = token_store::list_workspace_states() {
            let sessions: Vec<_> = states
                .iter()
                .map(|w| {
                    serde_json::json!({
                        "workspace_slug": w.workspace.slug,
                        "user_email": w.user.email,
                        "resource_count": w.resources.len(),
                        "session_expires_at": w.session.expires_at,
                    })
                })
                .collect();
            report["sessions"] = sessions.into();
        }
    }

    println!("{}", serde_json::to_string_pretty(&report)?);
    Ok(())
}

/// Default no-args behavior in product mode (config file loaded, non-root).
///
/// Shows current service status if reachable, then a concise usage hint.
/// Does not launch the interactive terminal UI.
async fn run_product_noargs(config: &Config) -> Result<()> {
    if product::is_service_running(config.management_port()).await {
        run_proxied_status(config, None).await?;
    } else {
        eprintln!("ztna-client service is not running.");
        eprintln!("  sudo systemctl start ztna-client");
    }
    println!();
    println!("Usage: ztna-client <command>");
    println!("  login        Start browser login for a workspace");
    println!("  status       Show active sessions");
    println!("  resources    List authorized resources");
    println!("  sync         Refresh resources from the controller");
    println!("  disconnect   Revoke session and clear local state");
    println!();
    println!("Run 'ztna-client <command> --help' for details.");
    Ok(())
}

async fn run_ui(config: Config, initial_tenant: Option<String>) -> Result<()> {
    let app_state = AppState {
        config: config.clone(),
        pending: Arc::new(Mutex::new(HashMap::new())),
        service_token: String::new(),
    };
    let _server = tokio::spawn(run_direct_callback_server(app_state.clone()));

    println!("ZTNA Client");
    println!("Commands: login <tenant>, status [tenant], resources [tenant], sync <tenant>, disconnect <tenant>, quit");

    if let Some(tenant) = initial_tenant.or_else(|| {
        if config.tenant.is_empty() {
            None
        } else {
            Some(config.tenant.clone())
        }
    }) {
        let auth_url = begin_login(&app_state, &tenant).await?;
        if let Err(err) = open::that(&auth_url) {
            tracing::error!("failed to open browser: {}", err);
        }
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
                if let Err(err) = open::that(&auth_url) {
                    tracing::error!("failed to open browser: {}", err);
                }
                println!("Login started for {tenant}");
                println!("Open this URL if needed:\n{auth_url}");
            }
            ["status"] => {
                for state in list_workspace_states()? {
                    print_stored_status(&state);
                }
            }
            ["status", tenant] => {
                let state = ensure_workspace_state(&config, tenant, false).await?;
                print_stored_status(&state);
            }
            ["resources"] => {
                for state in list_workspace_states()? {
                    print_stored_resources(&state);
                }
            }
            ["resources", tenant] => {
                let state = ensure_workspace_state(&config, tenant, false).await?;
                print_stored_resources(&state);
            }
            ["sync", tenant] => {
                let state = ensure_workspace_state(&config, tenant, true).await?;
                println!("Synced {}", state.workspace.slug);
                print_stored_resources(&state);
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

// ---------------------------------------------------------------------------
// Display helpers
// ---------------------------------------------------------------------------

fn now_unix() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs() as i64
}

fn format_session_expiry(expires_at: i64) -> String {
    let remaining = expires_at - now_unix();
    if remaining <= 0 {
        return "expired".to_string();
    }
    let mins = remaining / 60;
    if mins >= 60 {
        format!("expires in {}h {}m", mins / 60, mins % 60)
    } else {
        format!("expires in {}m", mins)
    }
}

/// Print status from a sanitized CliWorkspaceStatus (proxy path).
fn print_cli_status(ws: &CliWorkspaceStatus, mode: &str) {
    let session = format_session_expiry(ws.session_expires_at);
    println!("\n{}", "─".repeat(50));
    println!("Workspace: {} ({})", ws.workspace_name, ws.workspace_slug);
    println!("──────────────────────────────────────────────────");
    println!("  User:            {} ({})", ws.user_email, ws.user_role);
    println!("  Device ID:       {}", ws.device_id);
    println!("  Resources:       {}", ws.resources.len());
    println!("  Session:         {}", session);
    if !mode.is_empty() {
        println!("  Mode:            {}", mode);
    }
}

/// Print status from a StoredWorkspaceState (direct path).
fn print_stored_status(state: &token_store::StoredWorkspaceState) {
    let session = format_session_expiry(state.session.expires_at);
    println!(
        "[{}] user={} role={} device={} resources={} session={}",
        state.workspace.slug,
        state.user.email,
        state.user.role,
        state.device.id,
        state.resources.len(),
        session,
    );
}

/// Print resources from sanitized CliResourceInfo (proxy path).
fn print_cli_resources(slug: &str, resources: &[CliResourceInfo]) {
    println!("\n{}", "─".repeat(60));
    println!("Workspace: {}", slug);
    println!("────────────────────────────────────────────────────────────");

    if resources.is_empty() {
        println!("  No authorized resources");
        println!();
        println!("  Resources will appear here after your administrator grants");
        println!("  access to networks and applications.");
        return;
    }

    println!("  {:<25} {:<12} {:<20}", "Name", "Status", "Address");
    println!(
        "  {:<25} {:<12} {:<20}",
        "─".repeat(25),
        "─".repeat(12),
        "─".repeat(20)
    );

    for resource in resources {
        let ports = format_ports(resource.port_from, resource.port_to);
        let address = format!("{}:{}", resource.address, ports);
        let status_indicator = match resource.firewall_status.as_str() {
            "allowed" => "✓ allowed",
            "denied" => "✗ denied",
            other => other,
        };
        println!(
            "  {:<25} {:<12} {:<20}",
            resource.name, status_indicator, address
        );
    }
    println!();
}

/// Print resources from StoredWorkspaceState (direct path).
fn print_stored_resources(state: &token_store::StoredWorkspaceState) {
    println!("\n{}", "─".repeat(60));
    println!("Workspace: {}", state.workspace.slug);
    println!("────────────────────────────────────────────────────────────");

    if state.resources.is_empty() {
        println!("  No authorized resources");
        println!();
        println!("  Resources will appear here after your administrator grants");
        println!("  access to networks and applications.");
        return;
    }

    println!("  {:<25} {:<12} {:<20}", "Name", "Status", "Address");
    println!(
        "  {:<25} {:<12} {:<20}",
        "─".repeat(25),
        "─".repeat(12),
        "─".repeat(20)
    );

    for resource in &state.resources {
        let ports = format_ports(resource.port_from, resource.port_to);
        let address = format!("{}:{}", resource.address, ports);
        let status_indicator = match resource.firewall_status.as_str() {
            "allowed" => "✓ allowed",
            "denied" => "✗ denied",
            other => other,
        };
        println!(
            "  {:<25} {:<12} {:<20}",
            resource.name, status_indicator, address
        );
    }
    println!();
}

fn format_ports(port_from: Option<i32>, port_to: Option<i32>) -> String {
    match (port_from, port_to) {
        (Some(from), Some(to)) if from == to => from.to_string(),
        (Some(from), Some(to)) => format!("{from}-{to}"),
        (Some(from), None) => from.to_string(),
        _ => "-".to_string(),
    }
}

// ---------------------------------------------------------------------------
// Network listeners
// ---------------------------------------------------------------------------

fn start_socks_listener(config: &Config) {
    if config.socks5_addr.trim().is_empty() {
        return;
    }

    let config_for_ca = config.clone();
    let socks5_addr = config.socks5_addr.clone();
    let socks5_addr_for_listener = socks5_addr.clone();
    let controller_url = config.controller_url.clone();
    let controller_grpc_addr = config.controller_grpc_addr.clone();
    let callback_bind_addr = config.callback_bind_addr.clone();
    let callback_host = config.callback_host.clone();
    let tenant = config.tenant.clone();
    let connector_tunnel_addr = config.connector_tunnel_addr.clone();

    tokio::spawn(async move {
        let fallback_ca_pem: Arc<Vec<u8>> = Arc::new(load_ca_pem(&config_for_ca).await);
        let handler = move |req: socks5::ConnectRequest, mut stream: tokio::net::TcpStream| {
            let config = Config {
                controller_url: controller_url.clone(),
                controller_grpc_addr: controller_grpc_addr.clone(),
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
                state_dir: std::path::PathBuf::new(),
                config_file_loaded: false,
                config_file_exists: false,
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

                let connector_tunnel_addr = connector_tunnel_addr_for_resource(
                    &state.resources,
                    &acl_resp.resource_id,
                    &connector_tunnel_addr,
                );

                if connector_tunnel_addr.trim().is_empty() {
                    tracing::warn!(
                        "no connector tunnel address for resource_id={}; cannot forward {}:{} through the split tunnel",
                        acl_resp.resource_id,
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

// ---------------------------------------------------------------------------
// CA helpers
// ---------------------------------------------------------------------------

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
