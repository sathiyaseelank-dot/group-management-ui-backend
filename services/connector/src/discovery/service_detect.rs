use std::net::{IpAddr, SocketAddr};
use std::time::Duration;
use tokio::io::AsyncReadExt;
use tokio::net::TcpStream;

/// Static lookup of well-known port numbers to service names.
pub fn service_from_port(port: u16) -> &'static str {
    match port {
        21 => "FTP",
        22 => "SSH",
        25 => "SMTP",
        53 => "DNS",
        80 => "HTTP",
        110 => "POP3",
        143 => "IMAP",
        443 => "HTTPS",
        445 => "SMB",
        993 => "IMAPS",
        1433 => "MSSQL",
        3306 => "MySQL",
        3389 => "RDP",
        5432 => "PostgreSQL",
        5900 => "VNC",
        6379 => "Redis",
        8080 => "HTTP",
        8443 => "HTTPS",
        9443 => "HTTPS",
        27017 => "MongoDB",
        _ => "Unknown",
    }
}

/// Identify the service from a TCP banner (first bytes received after connect).
/// Falls back to static port mapping if the banner is unrecognized.
pub fn identify_from_banner(banner: &[u8], port: u16) -> &'static str {
    if banner.len() >= 4 && &banner[..4] == b"SSH-" {
        return "SSH";
    }
    if banner.len() >= 4 && (&banner[..4] == b"220 " || &banner[..4] == b"220-") {
        return if port == 21 { "FTP" } else { "SMTP" };
    }
    if banner.len() >= 3 && &banner[..3] == b"+OK" {
        return "POP3";
    }
    if banner.len() >= 4 && &banner[..4] == b"* OK" {
        return "IMAP";
    }
    if banner.len() >= 5 && &banner[..5] == b"HTTP/" {
        return "HTTP";
    }
    if banner.len() >= 4 && &banner[..4] == b"RFB " {
        return "VNC";
    }
    // MySQL greeting: first byte after length header is protocol version 0x0a
    if banner.len() >= 5 && banner[4] == 0x0a {
        return "MySQL";
    }
    if banner.len() >= 4 && (&banner[..4] == b"-ERR" || banner.starts_with(b"-DENIED")) {
        return "Redis";
    }

    service_from_port(port)
}

/// Connect to ip:port with the given timeout, attempt to read a banner,
/// and return (is_open, service_name).
pub async fn detect_service(ip: IpAddr, port: u16, timeout: Duration) -> (bool, String) {
    let addr = SocketAddr::new(ip, port);
    let mut stream = match tokio::time::timeout(timeout, TcpStream::connect(addr)).await {
        Ok(Ok(s)) => s,
        _ => return (false, String::new()),
    };

    // Try to read a banner with a short timeout
    let mut buf = [0u8; 256];
    let banner_timeout = Duration::from_millis(300);
    let service = match tokio::time::timeout(banner_timeout, stream.read(&mut buf)).await {
        Ok(Ok(n)) if n > 0 => identify_from_banner(&buf[..n], port),
        _ => service_from_port(port),
    };

    (true, service.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_service_from_port() {
        assert_eq!(service_from_port(22), "SSH");
        assert_eq!(service_from_port(443), "HTTPS");
        assert_eq!(service_from_port(5432), "PostgreSQL");
        assert_eq!(service_from_port(12345), "Unknown");
    }

    #[test]
    fn test_identify_ssh_banner() {
        assert_eq!(identify_from_banner(b"SSH-2.0-OpenSSH_8.9", 22), "SSH");
        assert_eq!(identify_from_banner(b"SSH-2.0-OpenSSH_8.9", 2222), "SSH");
    }

    #[test]
    fn test_identify_smtp_banner() {
        assert_eq!(identify_from_banner(b"220 mail.example.com ESMTP", 25), "SMTP");
    }

    #[test]
    fn test_identify_ftp_banner() {
        assert_eq!(identify_from_banner(b"220 FTP server ready", 21), "FTP");
    }

    #[test]
    fn test_identify_http_banner() {
        assert_eq!(identify_from_banner(b"HTTP/1.1 200 OK", 8080), "HTTP");
    }

    #[test]
    fn test_fallback_to_port() {
        assert_eq!(identify_from_banner(b"random garbage", 443), "HTTPS");
        assert_eq!(identify_from_banner(b"random garbage", 3389), "RDP");
    }
}
