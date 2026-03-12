mod auth;
mod config;
mod server;
mod token_store;

use clap::Parser;
use config::Config;
use server::{router, AppState};
use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use tracing::info;

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt::init();

    let config = Config::parse();
    let port = config.port;

    let state = AppState {
        config,
        pending: Arc::new(Mutex::new(HashMap::new())),
    };

    let app = router(state);
    let addr = format!("127.0.0.1:{}", port);
    info!("ztna-client listening on http://{}", addr);

    let listener = tokio::net::TcpListener::bind(&addr)
        .await
        .expect("failed to bind");
    axum::serve(listener, app).await.expect("server failed");
}
