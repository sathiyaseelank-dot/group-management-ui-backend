use anyhow::{anyhow, Result};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::Arc;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;
use tokio::net::tcp::OwnedWriteHalf;
use tokio::sync::Mutex;

use crate::enroll::pb::ControlMessage;

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

#[derive(Clone, Default)]
pub struct AgentTunnelManager {
    writers: Arc<Mutex<HashMap<String, Arc<Mutex<OwnedWriteHalf>>>>>,
}

impl AgentTunnelManager {
    pub fn new() -> Self {
        Self::default()
    }

    pub async fn open(
        &self,
        req: TunnelOpen,
        stream_tx: tokio::sync::mpsc::Sender<ControlMessage>,
    ) -> Result<()> {
        let dest = format!("{}:{}", req.destination, req.port);
        let stream = match TcpStream::connect(&dest).await {
            Ok(stream) => stream,
            Err(err) => {
                send_message(
                    &stream_tx,
                    "connector_tunnel_opened",
                    &TunnelOpened {
                        connection_id: req.connection_id,
                        ok: false,
                        error: Some(err.to_string()),
                    },
                )
                .await?;
                return Ok(());
            }
        };
        let (mut reader, writer) = stream.into_split();
        self.writers
            .lock()
            .await
            .insert(req.connection_id.clone(), Arc::new(Mutex::new(writer)));

        send_message(
            &stream_tx,
            "connector_tunnel_opened",
            &TunnelOpened {
                connection_id: req.connection_id.clone(),
                ok: true,
                error: None,
            },
        )
        .await?;

        let writers = self.writers.clone();
        tokio::spawn(async move {
            let mut buf = [0u8; 16 * 1024];
            loop {
                match reader.read(&mut buf).await {
                    Ok(0) => break,
                    Ok(n) => {
                        if send_message(
                            &stream_tx,
                            "connector_tunnel_data",
                            &TunnelData {
                                connection_id: req.connection_id.clone(),
                                data: buf[..n].to_vec(),
                            },
                        )
                        .await
                        .is_err()
                        {
                            break;
                        }
                    }
                    Err(err) => {
                        let _ = send_message(
                            &stream_tx,
                            "connector_tunnel_close",
                            &TunnelClose {
                                connection_id: req.connection_id.clone(),
                                error: Some(err.to_string()),
                            },
                        )
                        .await;
                        let _ = writers.lock().await.remove(&req.connection_id);
                        return;
                    }
                }
            }
            let _ = send_message(
                &stream_tx,
                "connector_tunnel_close",
                &TunnelClose {
                    connection_id: req.connection_id.clone(),
                    error: None,
                },
            )
            .await;
            let _ = writers.lock().await.remove(&req.connection_id);
        });

        Ok(())
    }

    pub async fn write(&self, msg: TunnelData) -> Result<()> {
        let writer = self
            .writers
            .lock()
            .await
            .get(&msg.connection_id)
            .cloned()
            .ok_or_else(|| anyhow!("unknown tunnel {}", msg.connection_id))?;
        writer.lock().await.write_all(&msg.data).await?;
        Ok(())
    }

    pub async fn close(&self, msg: TunnelClose) -> Result<()> {
        let writer = self.writers.lock().await.remove(&msg.connection_id);
        if let Some(writer) = writer {
            let _ = writer.lock().await.shutdown().await;
        }
        Ok(())
    }
}

async fn send_message<T: Serialize>(
    stream_tx: &tokio::sync::mpsc::Sender<ControlMessage>,
    message_type: &str,
    payload: &T,
) -> Result<()> {
    stream_tx
        .send(ControlMessage {
            r#type: message_type.to_string(),
            payload: serde_json::to_vec(payload)?,
            ..Default::default()
        })
        .await
        .map_err(|_| anyhow!("failed to send {}", message_type))?;
    Ok(())
}
