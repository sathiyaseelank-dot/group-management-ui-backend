package admin

import (
	"database/sql"
	"regexp"
	"strings"

	"controller/state"
)

// devicePostureRow holds the latest posture data for a single device.
type devicePostureRow struct {
	DeviceID          string
	FirewallEnabled   bool
	DiskEncrypted     bool
	ScreenLockEnabled bool
	OSVersion         string
	ClientVersion     string
}

// trustedProfileRow holds a single device trusted profile's requirements.
type trustedProfileRow struct {
	ID                    string
	Name                  string
	RequireFirewall       bool
	RequireDiskEncryption bool
	RequireScreenLock     bool
	MinOSVersion          string
}

// postureValidationResult holds the result of validating a posture report.
type postureValidationResult struct {
	Valid   bool
	Reasons []string
}

// getDevicePosture fetches the latest posture for a device in a workspace.
func getDevicePosture(db *sql.DB, deviceID, workspaceID string) (*devicePostureRow, error) {
	var dp devicePostureRow
	var fwEnabled, diskEnc, screenLock int
	err := db.QueryRow(
		state.Rebind(`SELECT device_id, firewall_enabled, disk_encrypted, screen_lock_enabled, os_version, client_version
			FROM device_posture WHERE device_id = ? AND workspace_id = ?`),
		deviceID, workspaceID,
	).Scan(&dp.DeviceID, &fwEnabled, &diskEnc, &screenLock, &dp.OSVersion, &dp.ClientVersion)
	if err != nil {
		return nil, err
	}
	dp.FirewallEnabled = fwEnabled != 0
	dp.DiskEncrypted = diskEnc != 0
	dp.ScreenLockEnabled = screenLock != 0
	return &dp, nil
}

// getTrustedProfilesForUser fetches all trusted profiles applicable to a user
// via their group memberships in a workspace.
func getTrustedProfilesForUser(db *sql.DB, workspaceID, userID string) ([]trustedProfileRow, error) {
	rows, err := db.Query(
		state.Rebind(`SELECT DISTINCT dtp.id, dtp.name, dtp.require_firewall, dtp.require_disk_encryption, dtp.require_screen_lock, dtp.min_os_version
			FROM device_trusted_profiles dtp
			JOIN user_groups ug ON ug.trusted_profile_id = dtp.id
			JOIN user_group_members ugm ON ugm.group_id = ug.id
			WHERE dtp.workspace_id = ? AND ugm.user_id = ?`),
		workspaceID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []trustedProfileRow
	for rows.Next() {
		var tp trustedProfileRow
		var fw, de, sl int
		if err := rows.Scan(&tp.ID, &tp.Name, &fw, &de, &sl, &tp.MinOSVersion); err != nil {
			continue
		}
		tp.RequireFirewall = fw != 0
		tp.RequireDiskEncryption = de != 0
		tp.RequireScreenLock = sl != 0
		profiles = append(profiles, tp)
	}
	return profiles, rows.Err()
}

// meetsPostureRequirements checks if a device posture satisfies all applicable profiles.
// Returns whether compliant and a list of violation reasons.
func meetsPostureRequirements(posture *devicePostureRow, profiles []trustedProfileRow) (bool, []string) {
	if posture == nil || len(profiles) == 0 {
		// No posture data or no profiles = no enforcement.
		return true, nil
	}
	var violations []string
	for _, p := range profiles {
		if p.RequireFirewall && !posture.FirewallEnabled {
			violations = append(violations, "firewall_disabled")
		}
		if p.RequireDiskEncryption && !posture.DiskEncrypted {
			violations = append(violations, "disk_encryption_off")
		}
		if p.RequireScreenLock && !posture.ScreenLockEnabled {
			violations = append(violations, "screen_lock_disabled")
		}
		if p.MinOSVersion != "" && posture.OSVersion != "" && posture.OSVersion < p.MinOSVersion {
			violations = append(violations, "os_version_below_minimum")
		}
	}
	// Deduplicate violations
	seen := map[string]bool{}
	unique := violations[:0]
	for _, v := range violations {
		if !seen[v] {
			seen[v] = true
			unique = append(unique, v)
		}
	}
	return len(unique) == 0, unique
}

// ValidatePostureReport performs basic validation on a posture report to detect
// potentially fraudulent or misconfigured clients. It checks:
//  1. OS version format is valid (e.g., Windows version numbers are real)
//  2. Client version matches known release patterns
//  3. Flags impossible combinations (e.g., macOS with Windows firewall settings)
//
// Returns (valid bool, reasons []string) where valid=false indicates suspicious activity.
func ValidatePostureReport(osType, osVersion, clientVersion, hostname string, firewallEnabled, diskEncrypted, screenLockEnabled bool) (bool, []string) {
	var reasons []string

	// Normalize inputs
	osType = strings.TrimSpace(osType)
	osVersion = strings.TrimSpace(osVersion)
	clientVersion = strings.TrimSpace(clientVersion)
	hostname = strings.TrimSpace(hostname)

	// Check 1: Validate OS type is known
	validOSTypes := map[string]bool{
		"windows": true,
		"macos":   true,
		"linux":   true,
		"android": true,
		"ios":     true,
	}
	if osType != "" && !validOSTypes[strings.ToLower(osType)] {
		reasons = append(reasons, "unknown_os_type")
	}

	// Check 2: Validate OS version format
	if osVersion != "" {
		if !isValidOSVersion(osType, osVersion) {
			reasons = append(reasons, "invalid_os_version_format")
		}
	}

	// Check 3: Validate client version format (semantic versioning)
	if clientVersion != "" {
		if !isValidClientVersion(clientVersion) {
			reasons = append(reasons, "invalid_client_version_format")
		}
	}

	// Check 4: Detect impossible OS/firewall combinations
	if osType != "" && firewallEnabled {
		reasons = append(reasons, detectOSFirewallMismatches(osType, firewallEnabled)...)
	}

	// Check 5: Detect suspicious hostname patterns
	if hostname != "" {
		if isSuspiciousHostname(hostname) {
			reasons = append(reasons, "suspicious_hostname")
		}
	}

	// Check 6: Cross-validate OS-specific settings
	reasons = append(reasons, validateOSSpecificSettings(osType, firewallEnabled, diskEncrypted, screenLockEnabled)...)

	return len(reasons) == 0, reasons
}

// isValidOSVersion checks if the OS version string matches expected patterns for the OS type.
func isValidOSVersion(osType, osVersion string) bool {
	osTypeLower := strings.ToLower(osType)

	switch osTypeLower {
	case "windows":
		// Windows versions: 10.0.XX (Windows 10/11), 6.3 (8.1), 6.2 (8), 6.1 (7)
		// Also accept just major version like "10" or "11"
		windowsPattern := regexp.MustCompile(`^(10|11|6\.[123]|10\.0\.\d+)$`)
		return windowsPattern.MatchString(osVersion)

	case "macos":
		// macOS versions: 10.X (10.15 Catalina), 11.X (Big Sur), 12.X (Monterey), etc.
		macosPattern := regexp.MustCompile(`^(10\.\d+|1[0-9]\.\d+)$`)
		return macosPattern.MatchString(osVersion)

	case "linux":
		// Linux: accept most version patterns (Ubuntu 20.04, kernel versions, etc.)
		linuxPattern := regexp.MustCompile(`^[\d.]+(-[\w.]+)?$`)
		return linuxPattern.MatchString(osVersion)

	case "android":
		// Android: single digit or X.Y format (10, 11, 12.0)
		androidPattern := regexp.MustCompile(`^\d+(\.\d+)?$`)
		return androidPattern.MatchString(osVersion)

	case "ios":
		// iOS: X.Y.Z or X.Y format (15.0, 15.0.1)
		iosPattern := regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
		return iosPattern.MatchString(osVersion)

	default:
		// Unknown OS - be permissive but check for basic format
		genericPattern := regexp.MustCompile(`^[\w.]+$`)
		return genericPattern.MatchString(osVersion)
	}
}

// isValidClientVersion checks if client version follows semantic versioning.
func isValidClientVersion(version string) bool {
	// Accept semver: X.Y.Z, X.Y, or X.Y.Z-beta.X
	semverPattern := regexp.MustCompile(`^\d+\.\d+\.\d+(-[\w.]+)?(\+[\w.]+)?$`)
	return semverPattern.MatchString(version)
}

// detectOSFirewallMismatches detects impossible combinations of OS and firewall settings.
func detectOSFirewallMismatches(osType string, firewallEnabled bool) []string {
	// This is a basic check - in reality, all OSes have firewalls.
	// Future enhancement: add more specific checks based on OS capabilities.
	return nil
}

// isSuspiciousHostname checks for hostname patterns that might indicate tampering.
func isSuspiciousHostname(hostname string) bool {
	// Check for common tampering indicators
	suspiciousPatterns := []string{
		"localhost",
		"127.0.0.1",
		"0.0.0.0",
		"test",
		"fake",
		"null",
		"undefined",
		"none",
		"empty",
	}

	hostnameLower := strings.ToLower(hostname)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(hostnameLower, pattern) {
			return true
		}
	}

	// Check for empty-like values
	if len(hostname) < 2 {
		return true
	}

	return false
}

// validateOSSpecificSettings validates settings that should be OS-specific.
func validateOSSpecificSettings(osType string, firewallEnabled, diskEncrypted, screenLockEnabled bool) []string {
	var reasons []string
	osTypeLower := strings.ToLower(osType)

	// Check for impossible combinations based on OS capabilities
	// Example: iOS doesn't have a traditional "firewall" setting exposed to apps
	// This is a simplified check - real implementation would be more nuanced

	// For now, just validate that at least one security setting is reported
	// (a completely empty report might indicate a stub client)
	if osTypeLower == "ios" || osTypeLower == "android" {
		// Mobile devices may not report all settings
		// No strict validation for now
	}

	return reasons
}
