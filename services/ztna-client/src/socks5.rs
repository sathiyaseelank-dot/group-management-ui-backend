use anyhow::{bail, Result};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{TcpListener, TcpStream};
use tracing::warn;

const SOCKS5_VERSION: u8 = 0x05;
const METHOD_NO_AUTH: u8 = 0x00;
const CMD_CONNECT: u8 = 0x01;
const ATYP_IPV4: u8 = 0x01;
const ATYP_DOMAIN: u8 = 0x03;
const ATYP_IPV6: u8 = 0x04;

const REP_SUCCESS: u8 = 0x00;
const REP_GENERAL_FAILURE: u8 = 0x01;
const REP_CMD_NOT_SUPPORTED: u8 = 0x07;
const REP_ATYP_NOT_SUPPORTED: u8 = 0x08;

#[derive(Debug, Clone)]
pub struct ConnectRequest {
    pub destination: String,
    pub port: u16,
}

pub async fn listen<F, Fut>(addr: &str, handler: F) -> Result<()>
where
    F: Fn(ConnectRequest, TcpStream) -> Fut + Send + Sync + Clone + 'static,
    Fut: std::future::Future<Output = ()> + Send + 'static,
{
    let listener = TcpListener::bind(addr).await?;
    tracing::info!("SOCKS5 proxy listening on {}", addr);

    loop {
        match listener.accept().await {
            Ok((stream, peer)) => {
                let handler = handler.clone();
                tokio::spawn(async move {
                    match handshake(stream).await {
                        Ok((req, stream)) => handler(req, stream).await,
                        Err(e) => warn!("SOCKS5 handshake from {}: {}", peer, e),
                    }
                });
            }
            Err(e) => warn!("SOCKS5 accept error: {}", e),
        }
    }
}

async fn handshake(mut stream: TcpStream) -> Result<(ConnectRequest, TcpStream)> {
    let mut hdr = [0u8; 2];
    stream.read_exact(&mut hdr).await?;
    if hdr[0] != SOCKS5_VERSION {
        bail!("not SOCKS5 (version={})", hdr[0]);
    }
    let nmethods = hdr[1] as usize;
    let mut methods = vec![0u8; nmethods];
    stream.read_exact(&mut methods).await?;
    stream.write_all(&[SOCKS5_VERSION, METHOD_NO_AUTH]).await?;

    let mut req = [0u8; 4];
    stream.read_exact(&mut req).await?;
    if req[0] != SOCKS5_VERSION {
        bail!("bad request version");
    }
    if req[1] != CMD_CONNECT {
        reply(&mut stream, REP_CMD_NOT_SUPPORTED).await?;
        bail!("unsupported command: {}", req[1]);
    }

    let destination = match req[3] {
        ATYP_IPV4 => {
            let mut b = [0u8; 4];
            stream.read_exact(&mut b).await?;
            format!("{}.{}.{}.{}", b[0], b[1], b[2], b[3])
        }
        ATYP_DOMAIN => {
            let len = stream.read_u8().await? as usize;
            let mut buf = vec![0u8; len];
            stream.read_exact(&mut buf).await?;
            String::from_utf8(buf)?
        }
        ATYP_IPV6 => {
            let mut b = [0u8; 16];
            stream.read_exact(&mut b).await?;
            format!("{}", std::net::Ipv6Addr::from(b))
        }
        other => {
            reply(&mut stream, REP_ATYP_NOT_SUPPORTED).await?;
            bail!("unsupported atyp: {}", other);
        }
    };
    let port = stream.read_u16().await?;

    Ok((ConnectRequest { destination, port }, stream))
}

async fn reply(stream: &mut TcpStream, code: u8) -> Result<()> {
    stream
        .write_all(&[SOCKS5_VERSION, code, 0x00, ATYP_IPV4, 0, 0, 0, 0, 0, 0])
        .await?;
    Ok(())
}

pub async fn reply_success(stream: &mut TcpStream) -> anyhow::Result<()> {
    reply(stream, REP_SUCCESS).await
}

pub async fn reply_error(stream: &mut TcpStream) {
    let _ = reply(stream, REP_GENERAL_FAILURE).await;
    let _ = stream.shutdown().await;
}
