//! Length-prefixed datagram framing over byte streams.
//!
//! UDP datagrams need boundary preservation when sent over a TLS byte stream.
//! Each datagram is sent as `[u32 big-endian length][payload]`.

use anyhow::Result;
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt};

const MAX_DATAGRAM: usize = 65535;

/// Write a single length-prefixed frame.
pub async fn write_frame<W: AsyncWrite + Unpin>(w: &mut W, data: &[u8]) -> Result<()> {
    let len = (data.len() as u32).to_be_bytes();
    w.write_all(&len).await?;
    w.write_all(data).await?;
    Ok(())
}

/// Read a single length-prefixed frame.  Returns `None` on clean EOF.
pub async fn read_frame<R: AsyncRead + Unpin>(r: &mut R) -> Result<Option<Vec<u8>>> {
    let mut len_buf = [0u8; 4];
    match r.read_exact(&mut len_buf).await {
        Ok(_) => {}
        Err(e) if e.kind() == std::io::ErrorKind::UnexpectedEof => return Ok(None),
        Err(e) => return Err(e.into()),
    }
    let len = u32::from_be_bytes(len_buf) as usize;
    if len > MAX_DATAGRAM {
        anyhow::bail!("frame too large: {} bytes (max {})", len, MAX_DATAGRAM);
    }
    let mut buf = vec![0u8; len];
    r.read_exact(&mut buf).await?;
    Ok(Some(buf))
}
