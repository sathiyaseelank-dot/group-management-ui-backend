use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::{SystemTime, UNIX_EPOCH};

use axum::{
    extract::{Query, State},
    http::StatusCode,
    response::{Html, IntoResponse},
    routing::{get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use tracing::{error, info};

use crate::auth::{
    compute_code_challenge, exchange_device_code, generate_code_verifier, revoke_device_token,
};
use crate::config::Config;
use crate::token_store::{clear_tokens, load_tokens, save_tokens, StoredTokens};

#[derive(Clone)]
pub struct AppState {
    pub config: Config,
    pub pending: Arc<Mutex<HashMap<String, PendingAuth>>>,
}

#[derive(Debug, Clone)]
pub struct PendingAuth {
    pub code_verifier: String,
    pub tenant_slug: String,
}

#[derive(Debug, Serialize)]
pub struct StatusResponse {
    pub authenticated: bool,
    pub workspace: Option<String>,
    pub expires_at: Option<i64>,
}

pub fn router(state: AppState) -> Router {
    Router::new()
        .route("/connect", get(handle_connect))
        .route("/callback", get(handle_callback))
        .route("/status", get(handle_status))
        .route("/disconnect", post(handle_disconnect))
        .with_state(state)
}

#[derive(Deserialize)]
struct ConnectQuery {
    tenant: String,
}

async fn handle_connect(
    State(state): State<AppState>,
    Query(q): Query<ConnectQuery>,
) -> impl IntoResponse {
    let code_verifier = generate_code_verifier();
    let code_challenge = compute_code_challenge(&code_verifier);

    let port = state.config.port;
    let redirect_uri = format!("http://localhost:{}/callback", port);

    let auth_result = crate::auth::start_device_auth(
        &state.config.controller_url,
        &q.tenant,
        &code_challenge,
        &redirect_uri,
    )
    .await;

    match auth_result {
        Ok(resp) => {
            // Store the code_verifier keyed by state
            {
                let mut pending = state.pending.lock().unwrap();
                pending.insert(
                    resp.state.clone(),
                    PendingAuth {
                        code_verifier,
                        tenant_slug: q.tenant.clone(),
                    },
                );
            }

            // Open browser
            if let Err(e) = open::that(&resp.auth_url) {
                error!("failed to open browser: {}", e);
            }

            Html("<html><body><p>Authenticating... Please check your browser.</p></body></html>".to_string()).into_response()
        }
        Err(e) => {
            error!("device auth failed: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Html(format!("<html><body><p>Error: {}</p></body></html>", e)),
            )
                .into_response()
        }
    }
}

#[derive(Deserialize)]
struct CallbackQuery {
    code: Option<String>,
    state: Option<String>,
    error: Option<String>,
}

async fn handle_callback(
    State(state): State<AppState>,
    Query(q): Query<CallbackQuery>,
) -> impl IntoResponse {
    if let Some(err) = q.error {
        return Html(format!(
            "<html><body><p>Authentication error: {}</p></body></html>",
            err
        ));
    }

    let code = match q.code {
        Some(c) => c,
        None => {
            return Html(
                "<html><body><p>Missing authorization code.</p></body></html>".to_string(),
            )
        }
    };
    let oauth_state = match q.state {
        Some(s) => s,
        None => {
            return Html("<html><body><p>Missing state parameter.</p></body></html>".to_string())
        }
    };

    let pending_auth = {
        let mut pending = state.pending.lock().unwrap();
        pending.remove(&oauth_state)
    };

    let pending = match pending_auth {
        Some(p) => p,
        None => {
            return Html(
                "<html><body><p>Unknown or expired state. Please try again.</p></body></html>"
                    .to_string(),
            )
        }
    };

    match exchange_device_code(
        &state.config.controller_url,
        &code,
        &pending.code_verifier,
        &oauth_state,
    )
    .await
    {
        Ok(tokens) => {
            let now = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap_or_default()
                .as_secs() as i64;

            let stored = StoredTokens {
                access_token: tokens.access_token,
                refresh_token: tokens.refresh_token,
                workspace_name: pending.tenant_slug.clone(),
                workspace_slug: pending.tenant_slug.clone(),
                expires_at: now + tokens.expires_in,
            };

            if let Err(e) = save_tokens(&pending.tenant_slug, &stored) {
                error!("failed to save tokens: {}", e);
            }

            info!("authenticated to workspace: {}", pending.tenant_slug);
            Html(format!(
                "<html><body><h2>Connected to {}!</h2><p>You can close this tab.</p></body></html>",
                pending.tenant_slug
            ))
        }
        Err(e) => {
            error!("token exchange failed: {}", e);
            Html(format!(
                "<html><body><p>Authentication failed: {}</p></body></html>",
                e
            ))
        }
    }
}

async fn handle_status(State(_state): State<AppState>) -> Json<StatusResponse> {
    // Look for any stored tokens - simplified: check for common slugs
    // In practice, the client would need to know which tenant to check
    Json(StatusResponse {
        authenticated: false,
        workspace: None,
        expires_at: None,
    })
}

#[derive(Deserialize)]
struct DisconnectRequest {
    tenant: Option<String>,
}

async fn handle_disconnect(
    State(state): State<AppState>,
    Json(body): Json<DisconnectRequest>,
) -> impl IntoResponse {
    let tenant = match body.tenant {
        Some(t) => t,
        None => {
            return Json(serde_json::json!({ "status": "error", "message": "tenant required" }))
        }
    };

    if let Some(tokens) = load_tokens(&tenant) {
        if let Err(e) =
            revoke_device_token(&state.config.controller_url, &tokens.refresh_token).await
        {
            error!("revoke failed: {}", e);
        }
        clear_tokens(&tenant);
    }

    Json(serde_json::json!({ "status": "disconnected" }))
}
