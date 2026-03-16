use chrono::Utc;
use serde::Serialize;
use std::process::Command;

#[derive(Debug, Serialize)]
pub struct DevicePostureReport {
    pub device_id: String,
    pub spiffe_id: String,
    pub os_type: String,
    pub os_version: String,
    pub hostname: String,
    pub firewall_enabled: bool,
    pub disk_encrypted: bool,
    pub screen_lock_enabled: bool,
    pub client_version: String,
    pub collected_at: String,
}

pub fn collect(device_id: &str, spiffe_id: &str) -> DevicePostureReport {
    let os_type = std::env::consts::OS.to_string();
    let hostname = hostname::get()
        .map(|h| h.to_string_lossy().to_string())
        .unwrap_or_default();
    DevicePostureReport {
        device_id: device_id.to_string(),
        spiffe_id: spiffe_id.to_string(),
        os_type: os_type.clone(),
        os_version: collect_os_version(&os_type),
        hostname,
        firewall_enabled: check_firewall(&os_type),
        disk_encrypted: check_disk_encryption(&os_type),
        screen_lock_enabled: check_screen_lock(&os_type),
        client_version: env!("CARGO_PKG_VERSION").to_string(),
        collected_at: Utc::now().to_rfc3339(),
    }
}

fn run_cmd(cmd: &str, args: &[&str]) -> String {
    Command::new(cmd)
        .args(args)
        .output()
        .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
        .unwrap_or_default()
}

/// Returns true if the given systemd service is active (exit code 0).
/// Does not require root.
fn systemctl_is_active(service: &str) -> bool {
    Command::new("systemctl")
        .args(["is-active", "--quiet", service])
        .status()
        .map(|s| s.success())
        .unwrap_or(false)
}

fn collect_os_version(os_type: &str) -> String {
    match os_type {
        "linux" => run_cmd("uname", &["-r"]),
        "macos" => run_cmd("sw_vers", &["-productVersion"]),
        "windows" => run_cmd("cmd", &["/C", "ver"]),
        _ => String::new(),
    }
}

fn check_firewall(os_type: &str) -> bool {
    match os_type {
        "linux" => {
            // Check via systemctl (no root required) — covers ufw, firewalld, nftables
            if systemctl_is_active("ufw")
                || systemctl_is_active("firewalld")
                || systemctl_is_active("nftables")
            {
                return true;
            }
            // Fallback: check /proc/net/ip_tables_names (iptables rules loaded)
            std::fs::read_to_string("/proc/net/ip_tables_names")
                .map(|s| !s.trim().is_empty())
                .unwrap_or(false)
        }
        "macos" => {
            let out = run_cmd(
                "defaults",
                &["read", "/Library/Preferences/com.apple.alf", "globalstate"],
            );
            out.trim() == "1" || out.trim() == "2"
        }
        "windows" => run_cmd("netsh", &["advfirewall", "show", "allprofiles", "state"])
            .to_lowercase()
            .contains("on"),
        _ => false,
    }
}

fn check_disk_encryption(os_type: &str) -> bool {
    match os_type {
        "linux" => run_cmd("lsblk", &["-o", "TYPE"])
            .lines()
            .any(|l| l.trim() == "crypt"),
        "macos" => run_cmd("fdesetup", &["status"])
            .to_lowercase()
            .contains("filevault is on"),
        "windows" => run_cmd("manage-bde", &["-status"])
            .to_lowercase()
            .contains("protection on"),
        _ => false,
    }
}

fn check_screen_lock(os_type: &str) -> bool {
    match os_type {
        "linux" => {
            // Check if any known screen locker process is running
            let lockers = [
                "gnome-screensaver",
                "xscreensaver",
                "swaylock",
                "i3lock",
                "kscreenlocker_greet",
                "xfce4-screensaver",
                "light-locker",
            ];
            for locker in &lockers {
                if !run_cmd("pgrep", &["-x", locker]).is_empty() {
                    return true;
                }
            }
            // GNOME: check lock-enabled setting (works even when not currently locked)
            if run_cmd(
                "gsettings",
                &["get", "org.gnome.desktop.screensaver", "lock-enabled"],
            )
            .trim()
                == "true"
            {
                return true;
            }
            // KDE: check kscreenlocker config for Autolock=true
            let kde_cfg_path = std::env::var("HOME").unwrap_or_default()
                + "/.config/kscreenlockerrc";
            if std::fs::read_to_string(&kde_cfg_path)
                .unwrap_or_default()
                .lines()
                .any(|l| l.trim().eq_ignore_ascii_case("autolock=true"))
            {
                return true;
            }
            false
        }
        "macos" => run_cmd(
            "defaults",
            &["read", "com.apple.screensaver", "askForPassword"],
        )
        .trim()
            == "1",
        "windows" => run_cmd(
            "reg",
            &[
                "query",
                "HKCU\\Control Panel\\Desktop",
                "/v",
                "ScreenSaverIsSecure",
            ],
        )
        .contains("0x1"),
        _ => false,
    }
}
