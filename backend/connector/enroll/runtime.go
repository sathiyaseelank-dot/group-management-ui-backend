package enroll

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"connector/internal/buildinfo"
)

const (
	privateIPEnv = "CONNECTOR_PRIVATE_IP"
	versionEnv   = "CONNECTOR_VERSION"
)

func ResolveVersion() string {
	if v := strings.TrimSpace(os.Getenv(versionEnv)); v != "" {
		return v
	}
	if v := strings.TrimSpace(buildinfo.Version); v != "" {
		return v
	}
	return "unknown"
}

func ResolvePrivateIP(controllerAddr string) (string, error) {
	if ip := strings.TrimSpace(os.Getenv(privateIPEnv)); ip != "" {
		return ip, nil
	}
	ip, err := discoverPrivateIP(controllerAddr)
	if err != nil {
		return "", err
	}
	return ip, nil
}

func discoverPrivateIP(controllerAddr string) (string, error) {
	host, err := controllerHost(controllerAddr)
	if err != nil {
		return "", err
	}
	conn, err := net.Dial("udp", net.JoinHostPort(host, "53"))
	if err != nil {
		return "", fmt.Errorf("failed to determine private IP: %w", err)
	}
	defer conn.Close()

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || localAddr.IP == nil {
		return "", fmt.Errorf("failed to determine private IP")
	}
	return localAddr.IP.String(), nil
}

func controllerHost(controllerAddr string) (string, error) {
	if strings.Contains(controllerAddr, "://") {
		return "", fmt.Errorf("CONTROLLER_ADDR must be host:port")
	}
	if host, _, err := net.SplitHostPort(controllerAddr); err == nil && host != "" {
		return host, nil
	}
	return "", fmt.Errorf("CONTROLLER_ADDR must be host:port")
}

func normalizeTrustDomain(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimSuffix(v, ".")
	return v
}

func loadExplicitCA() ([]byte, error) {
	if cred, err := ReadCredential("CONTROLLER_CA"); err != nil {
		return nil, err
	} else if cred != "" {
		return []byte(cred), nil
	}

	caPath := strings.TrimSpace(os.Getenv("CONTROLLER_CA_PATH"))
	if caPath == "" {
		return nil, fmt.Errorf(
			"CONTROLLER_CA_PATH is not set (explicit controller trust is required)",
		)
	}

	pemBytes, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read controller CA at %s: %w", caPath, err)
	}

	return pemBytes, nil
}

func ReadCredential(name string) (string, error) {
	dir := strings.TrimSpace(os.Getenv("CREDENTIALS_DIRECTORY"))
	if dir == "" {
		return "", nil
	}
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read credential %s: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}
