use anyhow::{anyhow, Result};
use rand::RngCore;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::{Arc, RwLock};
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::sync::mpsc;
use tonic::Status;

use crate::control_plane::ControlMessage;

type AgentStreamTx = mpsc::Sender<Result<ControlMessage, Status>>;

#[derive(Clone, Debug)]
pub struct AgentTunnelHub {
    inner: Arc<RwLock<Inner>>,
}

#[derive(Debug)]
struct Inner {
    agents: HashMap<String, AgentStreamTx>,
    sessions: HashMap<String, mpsc::UnboundedSender<TunnelEvent>>,
}

#[derive(Debug)]
pub enum TunnelEvent {
    Opened(Result<(), String>),
    Data(Vec<u8>),
    Closed(Option<String>),
}

#[derive(Debug, Serialize, Deserialize)]
pub struct TunnelOpen {
    pub connection_id: String,
    pub destination: String,
    pub port: u16,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct TunnelOpened {
    pub connection_id: String,
    pub ok: bool,
    pub error: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct TunnelData {
    pub connection_id: String,
    pub data: Vec<u8>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct TunnelClose {
    pub connection_id: String,
    pub error: Option<String>,
}

impl AgentTunnelHub {
    pub fn new() -> Self {
        Self {
            inner: Arc::new(RwLock::new(Inner {
                agents: HashMap::new(),
                sessions: HashMap::new(),
            })),
        }
    }

    pub fn register_agent(&self, agent_id: &str, tx: AgentStreamTx) {
        if let Ok(mut inner) = self.inner.write() {
            inner.agents.insert(agent_id.to_string(), tx);
        }
    }

    pub fn unregister_agent(&self, agent_id: &str) {
        if let Ok(mut inner) = self.inner.write() {
            inner.agents.remove(agent_id);
        }
    }

    pub fn first_agent_id(&self) -> Option<String> {
        self.inner
            .read()
            .ok()
            .and_then(|inner| inner.agents.keys().next().cloned())
    }

    pub fn register_session(&self, connection_id: &str) -> mpsc::UnboundedReceiver<TunnelEvent> {
        let (tx, rx) = mpsc::unbounded_channel();
        if let Ok(mut inner) = self.inner.write() {
            inner.sessions.insert(connection_id.to_string(), tx);
        }
        rx
    }

    pub fn unregister_session(&self, connection_id: &str) {
        if let Ok(mut inner) = self.inner.write() {
            inner.sessions.remove(connection_id);
        }
    }

    pub async fn send_message<T: Serialize>(
        &self,
        agent_id: &str,
        message_type: &str,
        payload: &T,
    ) -> Result<()> {
        let tx = self
            .inner
            .read()
            .ok()
            .and_then(|inner| inner.agents.get(agent_id).cloned())
            .ok_or_else(|| anyhow!("agent {} is not connected", agent_id))?;
        let data = serde_json::to_vec(payload)?;
        tx.send(Ok(ControlMessage {
            r#type: message_type.to_string(),
            payload: data,
            ..Default::default()
        }))
        .await
        .map_err(|_| anyhow!("failed to send {} to {}", message_type, agent_id))?;
        Ok(())
    }

    pub fn handle_incoming(&self, msg: &ControlMessage) -> bool {
        match msg.r#type.as_str() {
            "connector_tunnel_opened" => {
                if let Ok(payload) = serde_json::from_slice::<TunnelOpened>(&msg.payload) {
                    self.dispatch(
                        &payload.connection_id,
                        TunnelEvent::Opened(if payload.ok {
                            Ok(())
                        } else {
                            Err(payload.error.unwrap_or_else(|| "open failed".to_string()))
                        }),
                    );
                    return true;
                }
            }
            "connector_tunnel_data" => {
                if let Ok(payload) = serde_json::from_slice::<TunnelData>(&msg.payload) {
                    self.dispatch(&payload.connection_id, TunnelEvent::Data(payload.data));
                    return true;
                }
            }
            "connector_tunnel_close" => {
                if let Ok(payload) = serde_json::from_slice::<TunnelClose>(&msg.payload) {
                    self.dispatch(&payload.connection_id, TunnelEvent::Closed(payload.error));
                    return true;
                }
            }
            _ => {}
        }
        false
    }

    fn dispatch(&self, connection_id: &str, event: TunnelEvent) {
        let tx = self
            .inner
            .read()
            .ok()
            .and_then(|inner| inner.sessions.get(connection_id).cloned());
        if let Some(tx) = tx {
            let _ = tx.send(event);
        }
    }
}

pub fn random_connection_id() -> String {
    let mut buf = [0u8; 16];
    rand::thread_rng().fill_bytes(&mut buf);
    hex::encode(buf)
}

pub async fn relay_stream<S>(
    hub: AgentTunnelHub,
    agent_id: &str,
    stream: S,
    destination: &str,
    port: u16,
) -> Result<()>
where
    S: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin + Send + 'static,
{
    let connection_id = random_connection_id();
    let mut session_rx = hub.register_session(&connection_id);
    hub.send_message(
        agent_id,
        "connector_tunnel_open",
        &TunnelOpen {
            connection_id: connection_id.clone(),
            destination: destination.to_string(),
            port,
        },
    )
    .await?;

    match tokio::time::timeout(Duration::from_secs(5), session_rx.recv()).await {
        Ok(Some(TunnelEvent::Opened(Ok(())))) => {}
        Ok(Some(TunnelEvent::Opened(Err(err)))) => {
            hub.unregister_session(&connection_id);
            return Err(anyhow!("agent open failed: {}", err));
        }
        Ok(Some(TunnelEvent::Closed(err))) => {
            hub.unregister_session(&connection_id);
            return Err(anyhow!(
                "agent closed before open: {}",
                err.unwrap_or_else(|| "closed".to_string())
            ));
        }
        Ok(Some(TunnelEvent::Data(_))) => {}
        Ok(None) => {
            hub.unregister_session(&connection_id);
            return Err(anyhow!("agent relay channel closed"));
        }
        Err(_) => {
            hub.unregister_session(&connection_id);
            return Err(anyhow!("timed out waiting for agent tunnel open"));
        }
    }

    let (mut reader, mut writer) = tokio::io::split(stream);
    let write_hub = hub.clone();
    let write_agent = agent_id.to_string();
    let write_conn = connection_id.clone();
    let send_task = tokio::spawn(async move {
        let mut buf = [0u8; 16 * 1024];
        loop {
            let n = reader.read(&mut buf).await?;
            if n == 0 {
                break;
            }
            write_hub
                .send_message(
                    &write_agent,
                    "connector_tunnel_data",
                    &TunnelData {
                        connection_id: write_conn.clone(),
                        data: buf[..n].to_vec(),
                    },
                )
                .await?;
        }
        let _ = write_hub
            .send_message(
                &write_agent,
                "connector_tunnel_close",
                &TunnelClose {
                    connection_id: write_conn,
                    error: None,
                },
            )
            .await;
        Ok::<(), anyhow::Error>(())
    });

    let recv_conn = connection_id.clone();
    let recv_task = tokio::spawn(async move {
        while let Some(event) = session_rx.recv().await {
            match event {
                TunnelEvent::Opened(_) => {}
                TunnelEvent::Data(data) => writer.write_all(&data).await?,
                TunnelEvent::Closed(err) => {
                    if let Some(err) = err {
                        return Err(anyhow!("agent tunnel closed: {}", err));
                    }
                    break;
                }
            }
        }
        Ok::<(), anyhow::Error>(())
    });

    let send_res = send_task.await.map_err(|e| anyhow!("send task join: {}", e))?;
    let recv_res = recv_task.await.map_err(|e| anyhow!("recv task join: {}", e))?;
    hub.unregister_session(&recv_conn);
    send_res?;
    recv_res?;
    Ok(())
}
