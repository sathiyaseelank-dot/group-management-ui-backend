use std::collections::{HashMap, VecDeque};
use std::net::{IpAddr, Ipv4Addr};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use smoltcp::iface::{Config as IfaceConfig, Interface, SocketHandle, SocketSet};
use smoltcp::phy::{Device, DeviceCapabilities, Medium, RxToken, TxToken};
use smoltcp::socket::tcp::{Socket as TcpSocket, SocketBuffer, State as TcpState};
use smoltcp::time::Instant as SmolInstant;
use smoltcp::wire::{IpAddress, IpCidr, Ipv4Address};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::sync::mpsc;
use tracing::{debug, info, warn};

use crate::acl;
use crate::config::Config;
use crate::server::ensure_workspace_state;
use crate::token_store::{
    connector_tunnel_addr_for_resource, StoredResource, StoredWorkspaceState,
};
use crate::tun_routing::{RouteManager, BYPASS_FWMARK};
use crate::tunnel;

const SOCKET_BUF_SIZE: usize = 65536;

// ---------------------------------------------------------------------------
// smoltcp virtual device backed by packet queues
// ---------------------------------------------------------------------------

struct VirtualDevice {
    rx_queue: VecDeque<Vec<u8>>,
    tx_queue: VecDeque<Vec<u8>>,
    mtu: usize,
}

impl VirtualDevice {
    fn new(mtu: usize) -> Self {
        Self {
            rx_queue: VecDeque::new(),
            tx_queue: VecDeque::new(),
            mtu,
        }
    }
}

struct VirtRxToken(Vec<u8>);
struct VirtTxToken<'a>(&'a mut VecDeque<Vec<u8>>);

impl Device for VirtualDevice {
    type RxToken<'a>
        = VirtRxToken
    where
        Self: 'a;
    type TxToken<'a>
        = VirtTxToken<'a>
    where
        Self: 'a;

    fn receive(
        &mut self,
        _timestamp: SmolInstant,
    ) -> Option<(Self::RxToken<'_>, Self::TxToken<'_>)> {
        let pkt = self.rx_queue.pop_front()?;
        Some((VirtRxToken(pkt), VirtTxToken(&mut self.tx_queue)))
    }

    fn transmit(&mut self, _timestamp: SmolInstant) -> Option<Self::TxToken<'_>> {
        Some(VirtTxToken(&mut self.tx_queue))
    }

    fn capabilities(&self) -> DeviceCapabilities {
        let mut caps = DeviceCapabilities::default();
        caps.medium = Medium::Ip;
        caps.max_transmission_unit = self.mtu;
        // Only compute checksums on TX.  Skip validation on RX because the
        // kernel may deliver packets with partial/offloaded checksums through
        // the TUN device.
        caps.checksum.ipv4 = smoltcp::phy::Checksum::Tx;
        caps.checksum.tcp = smoltcp::phy::Checksum::Tx;
        caps.checksum.udp = smoltcp::phy::Checksum::Tx;
        caps.checksum.icmpv4 = smoltcp::phy::Checksum::Tx;
        caps
    }
}

impl RxToken for VirtRxToken {
    fn consume<R, F>(mut self, f: F) -> R
    where
        F: FnOnce(&mut [u8]) -> R,
    {
        f(&mut self.0)
    }
}

impl<'a> TxToken for VirtTxToken<'a> {
    fn consume<R, F>(self, len: usize, f: F) -> R
    where
        F: FnOnce(&mut [u8]) -> R,
    {
        let mut buf = vec![0u8; len];
        let result = f(&mut buf);
        self.0.push_back(buf);
        result
    }
}

// ---------------------------------------------------------------------------
// Connection tracking
// ---------------------------------------------------------------------------

/// (dst_ip, dst_port, src_port) uniquely identifies each TCP flow.
type ConnKey = (IpAddr, u16, u16);

enum ConnStage {
    /// TCP handshake in progress inside smoltcp.
    Handshaking,
    /// Handshake complete — relay task is running.
    Relaying { tx: mpsc::Sender<Vec<u8>> },
    /// Connection is closing; waiting for smoltcp to reach Closed state.
    Closing,
}

struct ConnEntry {
    handle: SocketHandle,
    dst_ip: IpAddr,
    dst_port: u16,
    #[allow(dead_code)]
    src_port: u16,
    stage: ConnStage,
}

/// Events sent from tunnel relay tasks back to the main loop.
enum TunEvent {
    /// Data received from the remote tunnel — write into smoltcp socket.
    Data { key: ConnKey, data: Vec<u8> },
    /// Tunnel closed — tear down connection.
    Closed { key: ConnKey },
}

// ---------------------------------------------------------------------------
// Packet parsing / description helpers
// ---------------------------------------------------------------------------

/// Produce a human-readable one-liner for a raw IP packet (for logging).
fn describe_packet(data: &[u8]) -> String {
    use etherparse::PacketHeaders;
    match PacketHeaders::from_ip_slice(data) {
        Ok(headers) => {
            let (src_ip, dst_ip) = match &headers.net {
                Some(etherparse::NetHeaders::Ipv4(h, _)) => (
                    format!("{}", Ipv4Addr::from(h.source)),
                    format!("{}", Ipv4Addr::from(h.destination)),
                ),
                _ => ("?".into(), "?".into()),
            };
            let transport = match &headers.transport {
                Some(etherparse::TransportHeader::Tcp(h)) => {
                    let mut flags = String::new();
                    if h.syn {
                        flags.push('S');
                    }
                    if h.ack {
                        flags.push('A');
                    }
                    if h.fin {
                        flags.push('F');
                    }
                    if h.rst {
                        flags.push('R');
                    }
                    if h.psh {
                        flags.push('P');
                    }
                    format!(
                        "TCP {}→{} [{}] seq={} ack={}",
                        h.source_port,
                        h.destination_port,
                        flags,
                        h.sequence_number,
                        h.acknowledgment_number,
                    )
                }
                Some(etherparse::TransportHeader::Udp(h)) => {
                    format!("UDP {}→{}", h.source_port, h.destination_port)
                }
                _ => "other-transport".into(),
            };
            format!(
                "{} → {} {} ({} bytes)",
                src_ip,
                dst_ip,
                transport,
                data.len()
            )
        }
        Err(_) => {
            format!(
                "unparseable ({} bytes, first byte: 0x{:02x})",
                data.len(),
                data.first().copied().unwrap_or(0)
            )
        }
    }
}

/// Parse a raw IP packet and return (dst_ip, dst_port, src_port) if it is a
/// TCP SYN (and not SYN-ACK).
fn parse_tcp_syn(data: &[u8]) -> Option<(Ipv4Addr, u16, u16)> {
    use etherparse::PacketHeaders;

    let headers = PacketHeaders::from_ip_slice(data).ok()?;
    let ipv4 = match headers.net? {
        etherparse::NetHeaders::Ipv4(h, _) => h,
        _ => return None,
    };
    let tcp = match headers.transport? {
        etherparse::TransportHeader::Tcp(h) => h,
        _ => return None,
    };
    if !tcp.syn || tcp.ack {
        return None;
    }
    Some((
        Ipv4Addr::from(ipv4.destination),
        tcp.destination_port,
        tcp.source_port,
    ))
}

/// Parse a CIDR string like "10.200.0.1/24" into (ip, prefix_len).
fn parse_tun_addr(cidr: &str) -> Result<(Ipv4Addr, u8)> {
    let (ip_str, prefix_str) = cidr.split_once('/').unwrap_or((cidr, "24"));
    let ip: Ipv4Addr = ip_str.parse().context("invalid TUN IP address")?;
    let prefix: u8 = prefix_str.parse().context("invalid prefix length")?;
    Ok((ip, prefix))
}

fn prefix_to_netmask(prefix: u8) -> Ipv4Addr {
    let mask: u32 = if prefix == 0 {
        0
    } else {
        !0u32 << (32 - prefix)
    };
    Ipv4Addr::from(mask.to_be_bytes())
}

/// Return an IP that is offset +1 from `base` within the same subnet.
/// Used so that smoltcp's interface IP differs from the TUN device IP,
/// preventing smoltcp's internal loopback from swallowing response packets.
fn smoltcp_ip(base: Ipv4Addr) -> Ipv4Addr {
    let mut octets = base.octets();
    octets[3] = octets[3].wrapping_add(1);
    Ipv4Addr::from(octets)
}

// ---------------------------------------------------------------------------
// Bypass connect — for ACL-denied traffic that should use the real NIC
// ---------------------------------------------------------------------------

/// Set SO_MARK on a socket so the kernel routes it via the `main` table,
/// bypassing the ZTNA routing table (51820).
fn set_fwmark(fd: &impl std::os::unix::io::AsRawFd, mark: u32) -> Result<()> {
    let mark_val = mark as libc::c_int;
    let ret = unsafe {
        libc::setsockopt(
            fd.as_raw_fd(),
            libc::SOL_SOCKET,
            libc::SO_MARK,
            &mark_val as *const _ as *const libc::c_void,
            std::mem::size_of::<libc::c_int>() as libc::socklen_t,
        )
    };
    if ret == 0 {
        Ok(())
    } else {
        Err(std::io::Error::last_os_error().into())
    }
}

/// Open a TCP connection that bypasses the ZTNA TUN routing.
/// Uses SO_MARK so the kernel selects the main routing table.
async fn connect_bypass(dst_ip: IpAddr, dst_port: u16) -> Result<tokio::net::TcpStream> {
    let dest = std::net::SocketAddr::new(dst_ip, dst_port);
    let socket = if dst_ip.is_ipv4() {
        tokio::net::TcpSocket::new_v4()?
    } else {
        tokio::net::TcpSocket::new_v6()?
    };
    set_fwmark(&socket, BYPASS_FWMARK)?;
    let stream = socket.connect(dest).await?;
    Ok(stream)
}

// ---------------------------------------------------------------------------
// Main TUN listener
// ---------------------------------------------------------------------------

pub async fn run_tun_listener(config: &Config) -> Result<()> {
    // ---- 1. Create TUN device ----
    let (tun_ip, prefix) = parse_tun_addr(&config.tun_addr)?;
    let netmask = prefix_to_netmask(prefix);

    let mut tun_cfg = tun2::Configuration::default();
    tun_cfg
        .address(tun_ip)
        .netmask(netmask)
        .mtu(config.tun_mtu)
        .tun_name(&config.tun_name)
        .up();

    let tun = tun2::create_as_async(&tun_cfg)
        .context("failed to create TUN device (requires root / CAP_NET_ADMIN)")?;

    info!(
        "[tun] created device {} addr={}/{}",
        config.tun_name, tun_ip, prefix
    );

    let (mut tun_reader, mut tun_writer) = tokio::io::split(tun);

    // ---- 2. Route manager ----
    let mut route_manager = RouteManager::new(&config.tun_name);

    // ---- 3. Initial workspace state + routes ----
    let mut ws_state: Option<StoredWorkspaceState> = None;
    if !config.tenant.is_empty() {
        match ensure_workspace_state(config, &config.tenant, false).await {
            Ok(state) => {
                if let Err(e) = route_manager.sync_routes(&state.resources).await {
                    warn!("[tun] initial route sync failed: {}", e);
                }
                info!(
                    "[tun] installed routes for {} resources",
                    state.resources.len()
                );
                ws_state = Some(state);
            }
            Err(e) => {
                warn!(
                    "[tun] no workspace state yet: {}. Routes will sync after login.",
                    e
                );
            }
        }
    }

    // ---- 4. smoltcp interface ----
    //
    // CRITICAL: the smoltcp interface IP must differ from the TUN device IP.
    // The kernel uses the TUN device IP (tun_ip, e.g. 10.200.0.1) as the
    // source address for packets routed through the TUN.  smoltcp generates
    // responses back to that source address.  If smoltcp's own interface IP
    // equals the destination, smoltcp's built-in loopback (since v0.10)
    // swallows the packet instead of emitting it through the TxToken.
    //
    // Fix: give smoltcp tun_ip+1 (e.g. 10.200.0.2).  The /24 prefix keeps
    // the kernel's source (10.200.0.1) inside the directly-connected network
    // so smoltcp can route responses without a gateway.
    let iface_ip = smoltcp_ip(tun_ip);
    let smol_ip = Ipv4Address::from(iface_ip);
    info!(
        "[tun] smoltcp interface IP={} (TUN device IP={})",
        iface_ip, tun_ip
    );

    let mut device = VirtualDevice::new(config.tun_mtu as usize);

    let mut iface_cfg = IfaceConfig::new(smoltcp::wire::HardwareAddress::Ip);
    iface_cfg.random_seed = rand::random();

    let mut iface = Interface::new(iface_cfg, &mut device, SmolInstant::now());
    iface.update_ip_addrs(|addrs| {
        addrs
            .push(IpCidr::new(IpAddress::Ipv4(smol_ip), prefix))
            .ok();
    });
    iface.set_any_ip(true);

    // Add a default gateway so smoltcp can route to ANY remote IP.
    //
    // CRITICAL: the gateway must be the smoltcp interface's own IP, NOT the
    // TUN device IP.  smoltcp's any_ip acceptance logic
    // (iface/interface/ipv4.rs:107-121) does:
    //
    //   routes.lookup(dst_ip) → gateway_ip
    //   if !has_ip_addr(gateway_ip) → DROP
    //
    // If the gateway is 10.200.0.1 but the interface IP is 10.200.0.2,
    // has_ip_addr() returns false and every packet to a non-local IP is
    // silently discarded.  Setting the gateway to the interface's own IP
    // (10.200.0.2) makes the check pass.
    iface.routes_mut().add_default_ipv4_route(smol_ip).ok();

    // ---- 5. Socket set + connection tracking ----
    let mut sockets = SocketSet::new(vec![]);
    let mut connections: HashMap<ConnKey, ConnEntry> = HashMap::new();
    let (event_tx, mut event_rx) = mpsc::channel::<TunEvent>(4096);

    let ca_pem: Arc<Vec<u8>> = Arc::new(if !config.internal_ca_cert.is_empty() {
        config.internal_ca_cert.as_bytes().to_vec()
    } else if !config.ca_cert_path.is_empty() {
        std::fs::read(&config.ca_cert_path).unwrap_or_default()
    } else {
        Vec::new()
    });

    let mut route_timer = tokio::time::interval(Duration::from_secs(60));
    let mut poll_timer = tokio::time::interval(Duration::from_millis(5));

    info!("[tun] listener running");

    // Signal handling: a one-shot future that fires on SIGTERM or Ctrl-C,
    // allowing the loop to break cleanly so route_manager.cleanup() runs.
    let mut shutdown = std::pin::pin!(tun_shutdown_signal());

    // ---- 6. Main event loop ----
    let mut buf = vec![0u8; 65536];

    loop {
        // `biased` ensures TUN reads and tunnel events are checked before
        // the timers, so real I/O is never starved by periodic ticks.
        tokio::select! {
            biased;

            // -- Graceful shutdown signal (SIGTERM or Ctrl-C) --
            _ = &mut shutdown => {
                info!("[tun] shutdown signal received, cleaning up");
                break;
            }

            // -- TUN read: raw IP packet from kernel --
            result = tun_reader.read(&mut buf) => {
                let n = match result {
                    Ok(0) => {
                        info!("[tun] TUN device closed");
                        break;
                    }
                    Ok(n) => n,
                    Err(e) => {
                        warn!("[tun] read error: {}", e);
                        continue;
                    }
                };
                let pkt = buf[..n].to_vec();

                debug!("[tun] rx  {}", describe_packet(&pkt));

                // If this is a TCP SYN, ensure we have a listener socket.
                if let Some((dst_ip, dst_port, src_port)) = parse_tcp_syn(&pkt) {
                    let key: ConnKey = (IpAddr::V4(dst_ip), dst_port, src_port);
                    if !connections.contains_key(&key) {
                        info!(
                            "[tun] new TCP SYN  {}:{} ← src_port {}",
                            dst_ip, dst_port, src_port
                        );
                        let rx_buf = SocketBuffer::new(vec![0; SOCKET_BUF_SIZE]);
                        let tx_buf = SocketBuffer::new(vec![0; SOCKET_BUF_SIZE]);
                        let mut socket = TcpSocket::new(rx_buf, tx_buf);
                        match socket.listen(dst_port) {
                            Ok(()) => {
                                let handle = sockets.add(socket);
                                connections.insert(
                                    key,
                                    ConnEntry {
                                        handle,
                                        dst_ip: IpAddr::V4(dst_ip),
                                        dst_port,
                                        src_port,
                                        stage: ConnStage::Handshaking,
                                    },
                                );
                                debug!(
                                    "[tun] created listening socket on port {} (handle {:?})",
                                    dst_port, handle
                                );
                            }
                            Err(e) => {
                                warn!("[tun] listen on port {} failed: {:?}", dst_port, e);
                            }
                        }
                    }
                }

                device.rx_queue.push_back(pkt);
            }

            // -- Events from tunnel relay tasks --
            event = event_rx.recv() => {
                match event {
                    Some(TunEvent::Data { key, data }) => {
                        if let Some(conn) = connections.get(&key) {
                            let socket = sockets.get_mut::<TcpSocket>(conn.handle);
                            if socket.can_send() {
                                let written = socket.send_slice(&data).unwrap_or(0);
                                debug!(
                                    "[tun] tunnel→smoltcp {}:{} {} bytes",
                                    conn.dst_ip, conn.dst_port, written
                                );
                            }
                        }
                    }
                    Some(TunEvent::Closed { key }) => {
                        if let Some(conn) = connections.get_mut(&key) {
                            info!(
                                "[tun] tunnel closed for {}:{}",
                                conn.dst_ip, conn.dst_port
                            );
                            conn.stage = ConnStage::Closing;
                            let socket = sockets.get_mut::<TcpSocket>(conn.handle);
                            socket.close();
                        }
                    }
                    None => break,
                }
            }

            // -- Periodic route refresh (re-resolve DNS, sync routes) --
            _ = route_timer.tick() => {
                if !config.tenant.is_empty() {
                    match ensure_workspace_state(config, &config.tenant, false).await {
                        Ok(state) => {
                            if let Err(e) = route_manager.sync_routes(&state.resources).await {
                                warn!("[tun] route sync failed: {}", e);
                            }
                            ws_state = Some(state);
                        }
                        Err(e) => {
                            warn!("[tun] workspace refresh failed: {}", e);
                        }
                    }
                }
            }

            // -- smoltcp poll interval --
            _ = poll_timer.tick() => {}
        }

        // ---- Poll smoltcp ----
        let now = SmolInstant::now();
        iface.poll(now, &mut device, &mut sockets);

        // ---- Promote Handshaking → Relaying for established sockets ----
        let mut to_start: Vec<ConnKey> = Vec::new();
        for (key, conn) in connections.iter() {
            if matches!(conn.stage, ConnStage::Handshaking) {
                let socket = sockets.get::<TcpSocket>(conn.handle);
                let st = socket.state();
                if st == TcpState::Established {
                    info!(
                        "[tun] TCP established {}:{} — starting tunnel",
                        conn.dst_ip, conn.dst_port
                    );
                    to_start.push(*key);
                } else if st != TcpState::Listen && st != TcpState::SynReceived {
                    debug!(
                        "[tun] socket {}:{} unexpected state during handshake: {:?}",
                        conn.dst_ip, conn.dst_port, st
                    );
                }
            }
        }

        for key in to_start {
            let conn = connections.get_mut(&key).unwrap();

            // Create channel for main-loop → tunnel data
            let (tx, rx) = mpsc::channel::<Vec<u8>>(256);
            conn.stage = ConnStage::Relaying { tx };

            // Gather parameters for the tunnel task
            let event_tx = event_tx.clone();
            let controller_url = config.controller_url.clone();
            let tenant = config.tenant.clone();
            let connector_tunnel_addr = config.connector_tunnel_addr.clone();
            let workspace_resources = ws_state
                .as_ref()
                .map(|s| s.resources.clone())
                .unwrap_or_default();
            let ca_pem = Arc::clone(&ca_pem);
            let dst_port = conn.dst_port;
            let destination = route_manager
                .lookup_domain(conn.dst_ip)
                .unwrap_or_else(|| conn.dst_ip.to_string());

            // Access token from latest workspace state
            let access_token = ws_state
                .as_ref()
                .map(|s| s.session.access_token.clone())
                .unwrap_or_default();

            tokio::spawn(tunnel_relay_task(
                key,
                rx,
                event_tx,
                controller_url,
                access_token,
                tenant,
                connector_tunnel_addr,
                workspace_resources,
                ca_pem,
                destination,
                dst_port,
            ));
        }

        // ---- Relay data: smoltcp socket → tunnel task ----
        for (_key, conn) in connections.iter() {
            if let ConnStage::Relaying { tx } = &conn.stage {
                let socket = sockets.get_mut::<TcpSocket>(conn.handle);
                if socket.can_recv() {
                    let mut data = vec![0u8; SOCKET_BUF_SIZE];
                    match socket.recv_slice(&mut data) {
                        Ok(n) if n > 0 => {
                            data.truncate(n);
                            debug!(
                                "[tun] smoltcp→tunnel {}:{} {} bytes",
                                conn.dst_ip, conn.dst_port, n
                            );
                            // Best-effort: drop if channel full
                            let _ = tx.try_send(data);
                        }
                        _ => {}
                    }
                }
            }
        }

        // ---- Clean up fully closed connections ----
        let mut to_remove = Vec::new();
        for (key, conn) in connections.iter() {
            let socket = sockets.get::<TcpSocket>(conn.handle);
            let closed = matches!(conn.stage, ConnStage::Closing)
                && matches!(socket.state(), TcpState::Closed | TcpState::TimeWait);
            if closed {
                to_remove.push(*key);
            }
        }
        for key in to_remove {
            if let Some(conn) = connections.remove(&key) {
                sockets.remove(conn.handle);
                debug!(
                    "[tun] cleaned up connection {}:{}",
                    conn.dst_ip, conn.dst_port
                );
            }
        }

        // ---- Flush smoltcp output packets → TUN ----
        while let Some(pkt) = device.tx_queue.pop_front() {
            debug!("[tun] tx  {}", describe_packet(&pkt));
            if let Err(e) = tun_writer.write_all(&pkt).await {
                warn!("[tun] write to TUN failed: {}", e);
            }
        }
    }

    info!("[tun] shutting down, cleaning up routes");
    route_manager.cleanup();
    Ok(())
}

// ---------------------------------------------------------------------------
// Shutdown signal helper
// ---------------------------------------------------------------------------

/// Returns a future that resolves on SIGTERM (Unix) or Ctrl-C.
///
/// Used in the TUN main loop so the loop can break cleanly, allowing
/// `route_manager.cleanup()` to remove kernel routes before the process exits.
async fn tun_shutdown_signal() {
    #[cfg(unix)]
    {
        use tokio::signal::unix::{signal, SignalKind};
        match signal(SignalKind::terminate()) {
            Ok(mut sigterm) => {
                tokio::select! {
                    _ = sigterm.recv() => {}
                    _ = tokio::signal::ctrl_c() => {}
                }
            }
            Err(e) => {
                warn!("[tun] cannot install SIGTERM handler: {e}; falling back to Ctrl-C only");
                tokio::signal::ctrl_c().await.ok();
            }
        }
    }
    #[cfg(not(unix))]
    {
        tokio::signal::ctrl_c().await.ok();
    }
}

// ---------------------------------------------------------------------------
// Tunnel relay task — runs per-connection in its own tokio task
// ---------------------------------------------------------------------------

async fn tunnel_relay_task(
    key: ConnKey,
    mut from_smoltcp: mpsc::Receiver<Vec<u8>>,
    to_smoltcp: mpsc::Sender<TunEvent>,
    controller_url: String,
    access_token: String,
    _tenant: String,
    fallback_connector_tunnel_addr: String,
    workspace_resources: Vec<StoredResource>,
    ca_pem: Arc<Vec<u8>>,
    destination: String,
    dst_port: u16,
) {
    info!(
        "[tun-relay] starting ACL check for {}:{}",
        destination, dst_port
    );

    // ACL check
    let acl_resp =
        match acl::check_access(&controller_url, &access_token, &destination, dst_port).await {
            Ok(r) => r,
            Err(e) => {
                warn!(
                    "[tun-relay] ACL check failed for {}:{}: {}",
                    destination, dst_port, e
                );
                let _ = to_smoltcp.send(TunEvent::Closed { key }).await;
                return;
            }
        };

    if !acl_resp.allowed {
        info!(
            "[tun-relay] ACL denied {}:{} — bypassing via normal interface",
            destination, dst_port
        );

        // Connect directly through the real NIC (bypass TUN routing).
        match connect_bypass(key.0, dst_port).await {
            Ok(mut direct) => {
                info!("[tun-relay] bypass connected {}:{}", destination, dst_port);
                let mut buf = vec![0u8; 65536];
                loop {
                    tokio::select! {
                        data = from_smoltcp.recv() => {
                            match data {
                                Some(data) if !data.is_empty() => {
                                    if direct.write_all(&data).await.is_err() { break; }
                                }
                                _ => break,
                            }
                        }
                        result = direct.read(&mut buf) => {
                            match result {
                                Ok(0) | Err(_) => break,
                                Ok(n) => {
                                    if to_smoltcp
                                        .send(TunEvent::Data { key, data: buf[..n].to_vec() })
                                        .await
                                        .is_err()
                                    {
                                        break;
                                    }
                                }
                            }
                        }
                    }
                }
                info!("[tun-relay] bypass closed {}:{}", destination, dst_port);
            }
            Err(e) => {
                warn!(
                    "[tun-relay] bypass connect failed {}:{}: {}",
                    destination, dst_port, e
                );
            }
        }

        let _ = to_smoltcp.send(TunEvent::Closed { key }).await;
        return;
    }

    let connector_tunnel_addr = connector_tunnel_addr_for_resource(
        &workspace_resources,
        &acl_resp.resource_id,
        &fallback_connector_tunnel_addr,
    );

    info!(
        "[tun-relay] ACL allowed {}:{} resource_id={} — opening tunnel to {}",
        destination, dst_port, acl_resp.resource_id, connector_tunnel_addr
    );

    if connector_tunnel_addr.trim().is_empty() {
        warn!(
            "[tun-relay] no connector tunnel address for resource_id={}; cannot forward {}:{}",
            acl_resp.resource_id, destination, dst_port
        );
        let _ = to_smoltcp.send(TunEvent::Closed { key }).await;
        return;
    }

    if ca_pem.is_empty() {
        warn!(
            "[tun-relay] connector CA not available for tunnel; CA_CERT_PATH='{}' INTERNAL_CA_CERT={} cached_workspace_ca={}",
            std::env::var("CA_CERT_PATH").unwrap_or_default(),
            !std::env::var("INTERNAL_CA_CERT").unwrap_or_default().trim().is_empty(),
            false
        );
        let _ = to_smoltcp.send(TunEvent::Closed { key }).await;
        return;
    }

    info!(
        "[tun-relay] using connector CA: {}",
        crate::describe_ca_pem(&ca_pem)
    );

    // Open tunnel to connector
    let mut tunnel_stream = match tunnel::open(
        &connector_tunnel_addr,
        &ca_pem,
        &access_token,
        &destination,
        dst_port,
    )
    .await
    {
        Ok(s) => s,
        Err(e) => {
            warn!(
                "[tun-relay] tunnel open failed for {}:{}: {}",
                destination, dst_port, e
            );
            let _ = to_smoltcp.send(TunEvent::Closed { key }).await;
            return;
        }
    };

    info!(
        "[tun-relay] tunnel connected {}:{} — relaying",
        destination, dst_port
    );

    // Bidirectional relay: smoltcp socket ↔ tunnel
    let mut tunnel_buf = vec![0u8; 65536];
    loop {
        tokio::select! {
            // Data from smoltcp (app → tunnel)
            data = from_smoltcp.recv() => {
                match data {
                    Some(data) if !data.is_empty() => {
                        debug!(
                            "[tun-relay] app→tunnel {}:{} {} bytes",
                            destination, dst_port, data.len()
                        );
                        if tunnel_stream.write_all(&data).await.is_err() {
                            break;
                        }
                    }
                    _ => break,
                }
            }
            // Data from tunnel (remote → app)
            result = tunnel_stream.read(&mut tunnel_buf) => {
                match result {
                    Ok(0) | Err(_) => break,
                    Ok(n) => {
                        debug!(
                            "[tun-relay] tunnel→app {}:{} {} bytes",
                            destination, dst_port, n
                        );
                        if to_smoltcp
                            .send(TunEvent::Data {
                                key,
                                data: tunnel_buf[..n].to_vec(),
                            })
                            .await
                            .is_err()
                        {
                            break;
                        }
                    }
                }
            }
        }
    }

    let _ = to_smoltcp.send(TunEvent::Closed { key }).await;
    info!("[tun-relay] closed {}:{}", destination, dst_port);
}
