use std::collections::{HashMap, HashSet, VecDeque};
use std::net::{IpAddr, Ipv4Addr};
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use smoltcp::iface::{Config as IfaceConfig, Interface, SocketHandle, SocketSet};
use smoltcp::phy::{Device, DeviceCapabilities, Medium, RxToken, TxToken};
use smoltcp::socket::tcp::{Socket as TcpSocket, SocketBuffer, State as TcpState};
use smoltcp::time::Instant as SmolInstant;
use smoltcp::wire::{IpAddress, IpCidr, Ipv4Address};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::sync::{mpsc, watch};
use tracing::{debug, info, warn};

use crate::acl;
use crate::config::Config;
use crate::framing;
use crate::server::ensure_workspace_state;
use crate::token_store::{
    connector_tunnel_addr_for_resource, StoredResource, StoredWorkspaceState,
};
use crate::tun_dns_intercept;
use crate::tun_routing::{RouteManager, BYPASS_FWMARK};
use crate::tunnel;

const SOCKET_BUF_SIZE: usize = 65536;
const QUIC_FALLBACK_TIMEOUT: Duration = Duration::from_millis(200);

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
    /// UDP datagram received from tunnel — build raw packet and inject into TUN.
    UdpData { key: ConnKey, data: Vec<u8> },
    /// UDP tunnel closed.
    UdpClosed { key: ConnKey },
}

// ---------------------------------------------------------------------------
// UDP connection tracking (bypasses smoltcp entirely)
// ---------------------------------------------------------------------------

const UDP_IDLE_TIMEOUT: Duration = Duration::from_secs(30);

struct UdpConnEntry {
    tx: mpsc::Sender<Vec<u8>>,
    last_activity: Instant,
}

/// Parse a raw IP packet and return (dst_ip, dst_port, src_port, payload) if
/// it is a UDP datagram.
fn parse_udp_packet(data: &[u8]) -> Option<(Ipv4Addr, u16, u16, Vec<u8>)> {
    use etherparse::PacketHeaders;

    let headers = PacketHeaders::from_ip_slice(data).ok()?;
    let ipv4 = match headers.net? {
        etherparse::NetHeaders::Ipv4(h, _) => h,
        _ => return None,
    };
    let udp = match headers.transport? {
        etherparse::TransportHeader::Udp(h) => h,
        _ => return None,
    };
    let payload = headers.payload.slice().to_vec();
    Some((
        Ipv4Addr::from(ipv4.destination),
        udp.destination_port,
        udp.source_port,
        payload,
    ))
}

/// Build a raw IPv4 + UDP response packet for injection into the TUN device.
fn build_udp_response_packet(
    src_ip: Ipv4Addr,
    src_port: u16,
    dst_ip: Ipv4Addr,
    dst_port: u16,
    payload: &[u8],
) -> Vec<u8> {
    use etherparse::{Ipv4Header, UdpHeader};

    let udp_hdr_len = 8; // UDP header is always 8 bytes
    let udp_len = udp_hdr_len + payload.len();

    let mut ip = Ipv4Header::new(
        udp_len as u16,
        64,
        etherparse::IpNumber::UDP,
        src_ip.octets(),
        dst_ip.octets(),
    )
    .expect("valid ipv4 header");
    ip.header_checksum = ip.calc_header_checksum();

    let udp = UdpHeader::with_ipv4_checksum(src_port, dst_port, &ip, payload)
        .expect("valid udp header");

    let total_len = ip.header_len() + udp_hdr_len + payload.len();
    let mut buf = Vec::with_capacity(total_len);
    buf.extend_from_slice(&ip.to_bytes());
    buf.extend_from_slice(&udp.to_bytes());
    buf.extend_from_slice(payload);
    buf
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

pub async fn run_tun_listener(
    config: &Config,
    mut ws_rx: watch::Receiver<Option<StoredWorkspaceState>>,
) -> Result<()> {
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

    // ---- 3. Initial workspace state + routes + DNS domains ----
    // force_sync=true validates the session with the controller before
    // installing any routes, preventing stale cached state from granting
    // access after a revocation or unclean shutdown.
    let mut ws_state: Option<StoredWorkspaceState> = None;
    let mut dns_domains: HashSet<String> = HashSet::new();
    if !config.tenant.is_empty() {
        match ensure_workspace_state(config, &config.tenant, true).await {
            Ok(state) => {
                if let Err(e) = route_manager.sync_routes(&state.resources).await {
                    warn!("[tun] initial route sync failed: {}", e);
                }
                dns_domains = tun_dns_intercept::resource_domains(&state.resources);
                info!(
                    "[tun] installed routes for {} resources, tracking {} DNS domains",
                    state.resources.len(),
                    dns_domains.len()
                );
                ws_state = Some(state);
            }
            Err(e) => {
                warn!(
                    "[tun] no valid session on startup: {}. Routes will install after login.",
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
    let mut udp_connections: HashMap<ConnKey, UdpConnEntry> = HashMap::new();
    let (event_tx, mut event_rx) = mpsc::channel::<TunEvent>(4096);

    let ca_pem: Arc<Vec<u8>> = Arc::new(if !config.internal_ca_cert.is_empty() {
        config.internal_ca_cert.as_bytes().to_vec()
    } else if !config.ca_cert_path.is_empty() {
        std::fs::read(&config.ca_cert_path).unwrap_or_default()
    } else {
        Vec::new()
    });

    // ---- 5b. QUIC connection pool (Option C discovery) ----
    let quic_addr_cache = crate::quic_tunnel::QuicAddrCache::new();
    let quic_pool = if !ca_pem.is_empty() {
        match crate::quic_tunnel::QuicPool::new(&ca_pem) {
            Ok(pool) => Some(Arc::new(pool)),
            Err(e) => {
                warn!("[tun] failed to init QUIC pool: {} (QUIC disabled)", e);
                None
            }
        }
    } else {
        None
    };

    // ---- 5c. ACL decision cache (avoids repeated controller round-trips) ----
    let acl_cache = Arc::new(acl::AclCache::new(Duration::from_secs(60)));

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

                // --- UDP packet handling (bypasses smoltcp) ---
                if let Some((dst_ip, dst_port, src_port, payload)) = parse_udp_packet(&pkt) {
                    // DNS interception: intercept queries to port 53 for resource domains
                    if dst_port == 53 && !dns_domains.is_empty() {
                        if let Some(response) = tun_dns_intercept::handle_dns_query(&payload, &dns_domains).await {
                            let resp_pkt = build_udp_response_packet(
                                dst_ip, 53, tun_ip, src_port, &response,
                            );
                            debug!("[tun] dns-intercept response for src_port {}", src_port);
                            if let Err(e) = tun_writer.write_all(&resp_pkt).await {
                                warn!("[tun] write DNS response failed: {}", e);
                            }
                            continue; // Handled — don't forward to tunnel
                        }
                        // Not a resource domain — fall through to normal UDP tunnel
                    }

                    let key: ConnKey = (IpAddr::V4(dst_ip), dst_port, src_port);
                    if let Some(conn) = udp_connections.get_mut(&key) {
                        conn.last_activity = Instant::now();
                        let _ = conn.tx.try_send(payload);
                    } else {
                        info!(
                            "[tun] new UDP flow {}:{} ← src_port {}",
                            dst_ip, dst_port, src_port
                        );
                        let (tx, rx) = mpsc::channel::<Vec<u8>>(256);
                        let _ = tx.try_send(payload);
                        udp_connections.insert(key, UdpConnEntry {
                            tx,
                            last_activity: Instant::now(),
                        });

                        let event_tx = event_tx.clone();
                        let controller_url = config.controller_url.clone();
                        let connector_tunnel_addr = config.connector_tunnel_addr.clone();
                        let workspace_resources = ws_state
                            .as_ref()
                            .map(|s| s.resources.clone())
                            .unwrap_or_default();
                        let ca_pem = Arc::clone(&ca_pem);
                        let access_token = match ws_state
                            .as_ref()
                            .map(|s| s.session.access_token.as_str())
                            .filter(|t| !t.is_empty())
                        {
                            Some(t) => t.to_owned(),
                            None => {
                                warn!(
                                    "[tun] no active session — rejecting UDP {}:{}",
                                    dst_ip, dst_port
                                );
                                udp_connections.remove(&key);
                                continue;
                            }
                        };
                        let destination = route_manager
                            .lookup_domain(IpAddr::V4(dst_ip))
                            .unwrap_or_else(|| dst_ip.to_string());

                        let udp_acl_cache = acl_cache.clone();
                        tokio::spawn(udp_tunnel_relay_task(
                            key,
                            rx,
                            event_tx,
                            controller_url,
                            access_token,
                            connector_tunnel_addr,
                            workspace_resources,
                            ca_pem,
                            destination,
                            dst_port,
                            udp_acl_cache,
                        ));
                    }
                    // Don't feed UDP packets to smoltcp.
                    continue;
                }

                // --- TCP SYN handling ---
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
                    Some(TunEvent::UdpData { key, data }) => {
                        // Build a raw UDP/IP response packet and inject into TUN.
                        let (dst_ip, dst_port, src_port) = key;
                        if let (IpAddr::V4(dst), IpAddr::V4(src)) = (dst_ip, IpAddr::V4(tun_ip)) {
                            let pkt = build_udp_response_packet(dst, dst_port, src, src_port, &data);
                            debug!("[tun] udp response {}:{} → {} {} bytes", dst, dst_port, src_port, data.len());
                            if let Err(e) = tun_writer.write_all(&pkt).await {
                                warn!("[tun] write UDP response to TUN failed: {}", e);
                            }
                        }
                    }
                    Some(TunEvent::UdpClosed { key }) => {
                        info!("[tun] UDP tunnel closed {:?}", key);
                        udp_connections.remove(&key);
                    }
                    None => break,
                }
            }

            // -- Immediate session change notification (login / disconnect) --
            Ok(()) = ws_rx.changed() => {
                let update = ws_rx.borrow_and_update().clone();
                match update {
                    None => {
                        // Disconnect: clear routes and session state immediately.
                        info!("[tun] session cleared — removing all routes");
                        if let Err(e) = route_manager.sync_routes(&[]).await {
                            warn!("[tun] failed to clear routes after disconnect: {}", e);
                        }
                        ws_state = None;
                        dns_domains = HashSet::new();
                    }
                    Some(state) => {
                        // Login: install routes for the new session.
                        info!(
                            "[tun] new session — syncing routes for {} resources",
                            state.resources.len()
                        );
                        if let Err(e) = route_manager.sync_routes(&state.resources).await {
                            warn!("[tun] route sync after login failed: {}", e);
                        }
                        dns_domains = tun_dns_intercept::resource_domains(&state.resources);
                        ws_state = Some(state);
                    }
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
                            dns_domains = tun_dns_intercept::resource_domains(&state.resources);
                            ws_state = Some(state);
                        }
                        Err(e) => {
                            warn!("[tun] workspace refresh failed: {}", e);
                            // If the state file is gone (e.g. post-disconnect race),
                            // remove stale routes rather than leaving them installed.
                            if crate::token_store::load_workspace_state(&config.tenant).is_none() {
                                info!("[tun] no workspace state — clearing stale routes");
                                if let Err(ce) = route_manager.sync_routes(&[]).await {
                                    warn!("[tun] failed to clear stale routes: {}", ce);
                                }
                                ws_state = None;
                                dns_domains = HashSet::new();
                            }
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
            let workspace_ca_pem = ws_state
                .as_ref()
                .map(|s| s.device.ca_cert_pem.as_bytes().to_vec())
                .unwrap_or_default();
            let ca_pem = Arc::clone(&ca_pem);
            let dst_port = conn.dst_port;
            let destination = route_manager
                .lookup_domain(conn.dst_ip)
                .unwrap_or_else(|| conn.dst_ip.to_string());

            // Require an active session before opening a tunnel. An empty
            // or missing token means we are logged out; reject immediately
            // rather than making a doomed ACL call to the controller.
            let access_token = match ws_state
                .as_ref()
                .map(|s| s.session.access_token.as_str())
                .filter(|t| !t.is_empty())
            {
                Some(t) => t.to_owned(),
                None => {
                    warn!(
                        "[tun] no active session — rejecting {}:{}",
                        conn.dst_ip, dst_port
                    );
                    conn.stage = ConnStage::Closing;
                    let socket = sockets.get_mut::<TcpSocket>(conn.handle);
                    socket.close();
                    continue;
                }
            };

            let task_quic_pool = quic_pool.clone();
            let task_quic_cache = quic_addr_cache.clone();
            let task_acl_cache = acl_cache.clone();
            tokio::spawn(tunnel_relay_task(
                key,
                rx,
                event_tx,
                controller_url,
                access_token,
                        tenant,
                        connector_tunnel_addr,
                        workspace_resources,
                        workspace_ca_pem,
                        ca_pem,
                        destination,
                        dst_port,
                        "tcp".to_string(),
                task_quic_pool,
                task_quic_cache,
                task_acl_cache,
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

        // ---- Reap idle UDP connections ----
        udp_connections.retain(|_key, entry| entry.last_activity.elapsed() < UDP_IDLE_TIMEOUT);

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
    workspace_ca_pem: Vec<u8>,
    ca_pem: Arc<Vec<u8>>,
    destination: String,
    dst_port: u16,
    protocol: String,
    quic_pool: Option<Arc<crate::quic_tunnel::QuicPool>>,
    quic_cache: crate::quic_tunnel::QuicAddrCache,
    acl_cache: Arc<acl::AclCache>,
) {
    info!(
        "[tun-relay] starting ACL check for {}:{}",
        destination, dst_port
    );

    // ACL check (cached — avoids controller round-trip on repeated connections)
    let acl_resp =
        match acl_cache.check_access(&controller_url, &access_token, &destination, dst_port, &protocol).await {
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

    let effective_ca_pem = if !ca_pem.is_empty() {
        ca_pem.as_ref().clone()
    } else if !workspace_ca_pem.is_empty() {
        workspace_ca_pem
    } else {
        Vec::new()
    };

    if effective_ca_pem.is_empty() {
        warn!(
            "[tun-relay] connector CA not available for tunnel; CA_CERT_PATH='{}' INTERNAL_CA_CERT={} cached_workspace_ca={}",
            std::env::var("CA_CERT_PATH").unwrap_or_default(),
            !std::env::var("INTERNAL_CA_CERT").unwrap_or_default().trim().is_empty(),
            true
        );
        let _ = to_smoltcp.send(TunEvent::Closed { key }).await;
        return;
    }

    info!(
        "[tun-relay] using connector CA: {}",
        crate::describe_ca_pem(&effective_ca_pem)
    );

    // Try QUIC first if a cached QUIC address is available, fall back to TLS.
    let cached_quic = quic_cache.get(&connector_tunnel_addr).await;
    let mut use_quic_stream: Option<crate::quic_tunnel::QuicBiStream> = None;

    if let (Some(quic_addr), Some(pool)) = (&cached_quic, &quic_pool) {
        info!("[tun-relay] trying QUIC at {} for {}:{}", quic_addr, destination, dst_port);
        match tokio::time::timeout(
            QUIC_FALLBACK_TIMEOUT,
            pool.open_stream(quic_addr, &access_token, &destination, dst_port, &protocol),
        )
        .await
        {
            Ok(Ok(stream)) => {
                info!("[tun-relay] QUIC tunnel connected {}:{}", destination, dst_port);
                use_quic_stream = Some(stream);
            }
            Ok(Err(e)) => {
                info!("[tun-relay] QUIC failed, falling back to TLS: {}", e);
                quic_cache.remove(&connector_tunnel_addr).await;
            }
            Err(_) => {
                info!(
                    "[tun-relay] QUIC timed out after {:?}, falling back to TLS",
                    QUIC_FALLBACK_TIMEOUT
                );
                quic_cache.remove(&connector_tunnel_addr).await;
            }
        }
    }

    // Fall back to TLS if QUIC didn't work
    if use_quic_stream.is_none() {
        let tunnel_result = match tunnel::open(
            &connector_tunnel_addr,
            &effective_ca_pem,
            &access_token,
            &destination,
            dst_port,
            &protocol,
        )
        .await
        {
            Ok(r) => r,
            Err(e) => {
                warn!(
                    "[tun-relay] tunnel open failed for {}:{}: {}",
                    destination, dst_port, e
                );
                let _ = to_smoltcp.send(TunEvent::Closed { key }).await;
                return;
            }
        };

        // Cache QUIC address for future connections (Option C discovery)
        if let Some(quic) = &tunnel_result.quic_addr {
            info!("[tun-relay] discovered QUIC at {} — caching for future connections", quic);
            quic_cache.set(&connector_tunnel_addr, quic.clone()).await;
        }

        // Use TLS stream for relay
        let mut tls_stream = tunnel_result.stream;
        info!(
            "[tun-relay] TLS tunnel connected {}:{} — relaying",
            destination, dst_port
        );

        relay_tcp_bidirectional(
            &mut from_smoltcp,
            &to_smoltcp,
            &mut tls_stream,
            key,
            &destination,
            dst_port,
        )
        .await;
        // Send a clean FIN to the connector so its send_task gets EOF and
        // can exit cleanly rather than waiting forever for client data.
        let _ = tls_stream.shutdown().await;

        let _ = to_smoltcp.send(TunEvent::Closed { key }).await;
        info!("[tun-relay] closed {}:{}", destination, dst_port);
        return;
    }

    // QUIC stream relay
    let mut quic_stream = use_quic_stream.unwrap();
    relay_tcp_bidirectional(
        &mut from_smoltcp,
        &to_smoltcp,
        &mut quic_stream,
        key,
        &destination,
        dst_port,
    )
    .await;
    // Finish the QUIC send stream cleanly (sends FIN_STREAM instead of
    // RESET_STREAM). Without this the connector gets a stream reset error,
    // skips sending connector_tunnel_close to the agent, and leaks the session.
    let _ = quic_stream.shutdown().await;

    let _ = to_smoltcp.send(TunEvent::Closed { key }).await;
    info!("[tun-relay] closed {}:{}", destination, dst_port);
}

/// Bidirectional relay between a smoltcp channel and an async stream.
async fn relay_tcp_bidirectional<S: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin>(
    from_smoltcp: &mut mpsc::Receiver<Vec<u8>>,
    to_smoltcp: &mpsc::Sender<TunEvent>,
    tunnel_stream: &mut S,
    key: ConnKey,
    destination: &str,
    dst_port: u16,
) {
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
}

// ---------------------------------------------------------------------------
// UDP tunnel relay task — runs per-flow in its own tokio task
// ---------------------------------------------------------------------------

async fn udp_tunnel_relay_task(
    key: ConnKey,
    mut from_tun: mpsc::Receiver<Vec<u8>>,
    to_tun: mpsc::Sender<TunEvent>,
    controller_url: String,
    access_token: String,
    fallback_connector_tunnel_addr: String,
    workspace_resources: Vec<StoredResource>,
    ca_pem: Arc<Vec<u8>>,
    destination: String,
    dst_port: u16,
    acl_cache: Arc<acl::AclCache>,
) {
    info!(
        "[udp-relay] starting ACL check for {}:{}",
        destination, dst_port
    );

    // ACL check (cached)
    let acl_resp =
        match acl_cache.check_access(&controller_url, &access_token, &destination, dst_port, "udp")
            .await
        {
            Ok(r) => r,
            Err(e) => {
                warn!(
                    "[udp-relay] ACL check failed for {}:{}: {}",
                    destination, dst_port, e
                );
                let _ = to_tun.send(TunEvent::UdpClosed { key }).await;
                return;
            }
        };

    if !acl_resp.allowed {
        info!(
            "[udp-relay] ACL denied {}:{} — dropping UDP",
            destination, dst_port
        );
        let _ = to_tun.send(TunEvent::UdpClosed { key }).await;
        return;
    }

    let connector_tunnel_addr = connector_tunnel_addr_for_resource(
        &workspace_resources,
        &acl_resp.resource_id,
        &fallback_connector_tunnel_addr,
    );

    if connector_tunnel_addr.trim().is_empty() || ca_pem.is_empty() {
        warn!(
            "[udp-relay] no connector address or CA for {}:{}",
            destination, dst_port
        );
        let _ = to_tun.send(TunEvent::UdpClosed { key }).await;
        return;
    }

    // Open tunnel with protocol "udp"
    let mut tunnel_stream = match tunnel::open(
        &connector_tunnel_addr,
        &ca_pem,
        &access_token,
        &destination,
        dst_port,
        "udp",
    )
    .await
    {
        Ok(r) => r.stream,
        Err(e) => {
            warn!(
                "[udp-relay] tunnel open failed for {}:{}: {}",
                destination, dst_port, e
            );
            let _ = to_tun.send(TunEvent::UdpClosed { key }).await;
            return;
        }
    };

    info!(
        "[udp-relay] tunnel connected {}:{} — relaying datagrams",
        destination, dst_port
    );

    // Bidirectional relay with length-prefixed framing
    let (mut tunnel_reader, mut tunnel_writer) = tokio::io::split(&mut tunnel_stream);
    loop {
        tokio::select! {
            // Datagram from TUN → tunnel (length-prefixed)
            data = from_tun.recv() => {
                match data {
                    Some(data) if !data.is_empty() => {
                        if framing::write_frame(&mut tunnel_writer, &data).await.is_err() {
                            break;
                        }
                    }
                    _ => break,
                }
            }
            // Datagram from tunnel → TUN
            frame = framing::read_frame(&mut tunnel_reader) => {
                match frame {
                    Ok(Some(data)) => {
                        if to_tun
                            .send(TunEvent::UdpData { key, data })
                            .await
                            .is_err()
                        {
                            break;
                        }
                    }
                    _ => break,
                }
            }
        }
    }

    let _ = to_tun.send(TunEvent::UdpClosed { key }).await;
    info!("[udp-relay] closed {}:{}", destination, dst_port);
}
