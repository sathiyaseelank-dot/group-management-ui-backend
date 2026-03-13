# Connector-RS Run & Test Guide

This guide covers running and testing the Rust connector (`grpcconnector2`) in two scenarios:

1. **Localhost** -- everything on one machine
2. **LAN** -- controller on one machine, connector on another machine in the same network

---

## Prerequisites

- Go 1.21+ (for the controller)
- Rust toolchain (for building connector-rs)
- `curl` or `wget`
- `protobuf-compiler` (`protoc`) -- only needed if regenerating proto code

### Build the Controller (Go)

```bash
cd backend/controller
go build -o controller ./...
```

### Build the Connector (Rust)

```bash
cd backend/connector-rs
cargo build --release
# Binary at: target/release/grpcconnector2
```

---

## 1. Localhost Setup

Everything runs on `127.0.0.1`. No firewall or network config needed.

### Step 1: Start the Controller

```bash
cd backend/controller

sudo \
  TRUST_DOMAIN="mycorp.internal" \
  INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
  INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)" \
  ADMIN_AUTH_TOKEN="7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4" \
  INTERNAL_API_TOKEN="e4b2f8d1c3a9e6f7b0d2a4c9e8f1a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3" \
  CONTROLLER_ADDR="127.0.0.1:8443" \
  ADMIN_HTTP_ADDR="0.0.0.0:8081" \
  ./controller
```

The controller now listens on:
- gRPC: `127.0.0.1:8443` (mTLS enrollment + control plane)
- HTTP: `0.0.0.0:8081` (admin API + CA cert endpoint)

### Step 2: Create an Enrollment Token

```bash
curl -s -X POST http://127.0.0.1:8081/api/admin/tokens \
  -H "Authorization: Bearer 7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4" | jq .
```

Response:
```json
{
  "token": "56ac57dea40aacfa98cb8205fd0f23f2",
  "expires_at": "2026-03-06T12:00:00Z"
}
```

Save the `token` value -- you'll need it below.

### Step 3: Download the CA Certificate

```bash
curl -fsSL http://127.0.0.1:8081/ca.crt -o /tmp/controller-ca.crt
```

### Step 4: Run the Rust Connector

```bash
cd backend/connector-rs

sudo \
  CONTROLLER_ADDR="127.0.0.1:8443" \
  CONNECTOR_ID="connector-local-01" \
  ENROLLMENT_TOKEN="<token-from-step-2>" \
  TRUST_DOMAIN="mycorp.internal" \
  POLICY_SIGNING_KEY="e4b2f8d1c3a9e6f7b0d2a4c9e8f1a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3" \
  CONTROLLER_CA="$(cat /tmp/controller-ca.crt)" \
  ./target/release/grpcconnector2 run
```

You should see logs like:
```
INFO grpcconnector2: connector enrolled as spiffe://mycorp.internal/connector/connector-local-01
```

### Step 5: Verify

**Check the controller sees the connector:**

```bash
curl -s http://127.0.0.1:8081/api/admin/connectors \
  -H "Authorization: Bearer 7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4" | jq .
```

The connector should appear with `status: "ONLINE"`.

**Check heartbeat in controller logs:**

The controller should print periodic heartbeat messages from the connector.

---

## 2. LAN Setup

Controller runs on **Machine A**, connector runs on **Machine B** (another computer on the same network).

### Identify the Controller's LAN IP

On Machine A:
```bash
ip addr show | grep "inet " | grep -v 127.0.0.1
# Example: 192.168.1.213
```

### Step 1: Start the Controller (Machine A)

```bash
cd backend/controller

sudo \
  TRUST_DOMAIN="mycorp.internal" \
  INTERNAL_CA_CERT="$(cat ca/ca.crt)" \
  INTERNAL_CA_KEY="$(cat ca/ca.pkcs8.key)" \
  ADMIN_AUTH_TOKEN="7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4" \
  INTERNAL_API_TOKEN="e4b2f8d1c3a9e6f7b0d2a4c9e8f1a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3" \
  CONTROLLER_ADDR="192.168.1.213:8443" \
  ADMIN_HTTP_ADDR="0.0.0.0:8081" \
  ./controller
```

> Replace `192.168.1.213` with Machine A's actual LAN IP.

### Step 2: Create an Enrollment Token (Machine A or B)

```bash
curl -s -X POST http://192.168.1.213:8081/api/admin/tokens \
  -H "Authorization: Bearer 7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4" | jq .
```

### Step 3: Install the Connector on Machine B

**Option A: One-line script install (from GitHub releases)**

```bash
curl -fsSL https://raw.githubusercontent.com/vairabarath/zero-trust/main/scripts/setup.sh | sudo \
  CONTROLLER_ADDR="192.168.1.213:8443" \
  CONTROLLER_HTTP_ADDR="192.168.1.213:8081" \
  CONNECTOR_ID="connector-lan-01" \
  ENROLLMENT_TOKEN="<token-from-step-2>" \
  POLICY_SIGNING_KEY="e4b2f8d1c3a9e6f7b0d2a4c9e8f1a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3" \
  bash
```

This downloads the binary, fetches the CA cert, writes the config to `/etc/grpcconnector2/connector.conf`, installs the systemd unit, and starts the service.

**Option B: Manual run (for development/testing)**

On Machine B, copy the built binary and run directly:

```bash
# Fetch the CA cert from the controller
curl -fsSL http://192.168.1.213:8081/ca.crt -o /tmp/controller-ca.crt

# Run the connector
sudo \
  CONTROLLER_ADDR="192.168.1.213:8443" \
  CONNECTOR_ID="connector-lan-01" \
  ENROLLMENT_TOKEN="<token-from-step-2>" \
  TRUST_DOMAIN="mycorp.internal" \
  POLICY_SIGNING_KEY="e4b2f8d1c3a9e6f7b0d2a4c9e8f1a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3" \
  CONTROLLER_CA="$(cat /tmp/controller-ca.crt)" \
  ./grpcconnector2 run
```

### Step 4: Verify (from Machine A or B)

```bash
curl -s http://192.168.1.213:8081/api/admin/connectors \
  -H "Authorization: Bearer 7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4" | jq .
```

### Step 5: Check systemd Service (if script-installed)

On Machine B:
```bash
sudo systemctl status grpcconnector2.service
sudo journalctl -u grpcconnector2.service -f
```

---

## Using the Frontend (Optional)

The Next.js admin UI provides a dashboard to manage connectors, agents, resources, and policies.

```bash
cd frontend
npm install
npm run dev
```

Open `http://localhost:3001` (or `http://<machine-a-ip>:3001` from another machine).

Set `BACKEND_URL` if the controller is on a different host:
```bash
BACKEND_URL="http://192.168.1.213:8081" npm run dev
```

From the UI you can create connectors, remote networks, and view connector status without using `curl`.

---

## Uninstalling

If the connector was installed via the setup script:

```bash
# Connector
sudo bash scripts/uninstall-connector.sh

# Agent (if installed)
sudo bash scripts/uninstall-agent.sh
```

These scripts stop the service, remove the binary, config, systemd unit, and runtime directory.

---

## Environment Variable Reference

### Controller

| Variable | Required | Default | Description |
|---|---|---|---|
| `INTERNAL_CA_CERT` | Yes | -- | PEM CA certificate |
| `INTERNAL_CA_KEY` | Yes | -- | PEM PKCS#8 CA private key |
| `ADMIN_AUTH_TOKEN` | Yes | -- | Bearer token for admin API |
| `INTERNAL_API_TOKEN` | Yes | -- | Token for internal endpoints |
| `TRUST_DOMAIN` | No | `mycorp.internal` | SPIFFE trust domain |
| `CONTROLLER_ADDR` | No | `:8443` | gRPC listen address |
| `ADMIN_HTTP_ADDR` | No | `:8081` | HTTP admin listen address |
| `DB_PATH` | No | in-memory | SQLite database path |
| `POLICY_SIGNING_KEY` | No | falls back to `INTERNAL_API_TOKEN` | HMAC key for policy signing |

### Connector (Rust)

| Variable | Required | Default | Description |
|---|---|---|---|
| `CONTROLLER_ADDR` | Yes | -- | Controller gRPC `host:port` |
| `CONNECTOR_ID` | Yes | -- | Unique connector identifier |
| `ENROLLMENT_TOKEN` | Yes | -- | One-time enrollment token |
| `POLICY_SIGNING_KEY` | No | derived from mTLS | HMAC key for policy verification (optional override if derivation fails) |
| `TRUST_DOMAIN` | No | `mycorp.internal` | SPIFFE trust domain |
| `CONTROLLER_CA` | No* | -- | PEM CA cert (inline) |
| `CONTROLLER_CA_PATH` | No* | -- | Path to CA cert file |
| `CONNECTOR_LISTEN_ADDR` | No | `<private_ip>:9443` | Address for agent connections |
| `CONNECTOR_VERSION` | No | build info | Version string |
| `POLICY_STALE_GRACE_SECONDS` | No | `600` | Seconds before stale policy expires |

> *At least one of `CONTROLLER_CA`, `CONTROLLER_CA_PATH`, or systemd `LoadCredential` must provide the CA cert.

### Setup Script (`setup.sh`)

| Variable | Required | Description |
|---|---|---|
| `CONTROLLER_ADDR` | Yes | Controller gRPC `host:port` |
| `CONTROLLER_HTTP_ADDR` | Yes | Controller HTTP `host:port` (for CA download) |
| `CONNECTOR_ID` | Yes | Unique connector identifier |
| `ENROLLMENT_TOKEN` | Yes | One-time enrollment token |
| `POLICY_SIGNING_KEY` | No | Optional override if policy key derivation fails |

---

## Ports Summary

| Port | Service | Protocol | Description |
|---|---|---|---|
| `8443` | Controller | gRPC + mTLS | Enrollment, renewal, control plane |
| `8081` | Controller | HTTP | Admin API, CA cert endpoint |
| `9443` | Connector | gRPC + mTLS | Inbound agent connections |
| `3001` | Frontend | HTTP | Admin dashboard (dev) |

---

## Troubleshooting

### "Connecting to HTTPS without TLS enabled"
The tonic gRPC client URL scheme conflicted with the custom TLS connector. This was fixed by using `http://` as the endpoint scheme (the custom connector handles TLS regardless).

### CryptoProvider panic (exit code 101)
```
Could not automatically determine the process-level CryptoProvider
```
Fixed by calling `rustls::crypto::ring::default_provider().install_default()` at the top of `main()`. This is already applied.

### "CONTROLLER_CA_PATH is not set"
The connector needs the controller's CA certificate. Provide it via one of:
- `CONTROLLER_CA` env var (PEM content directly)
- `CONTROLLER_CA_PATH` env var (file path)
- systemd `LoadCredential=CONTROLLER_CA:/etc/grpcconnector2/ca.crt` (handled by the systemd unit)

### "enrollment RPC failed: token invalid"
Enrollment tokens are single-use. Generate a new one via the admin API:
```bash
curl -s -X POST http://<controller>:8081/api/admin/tokens \
  -H "Authorization: Bearer <ADMIN_AUTH_TOKEN>"
```

### Connector keeps restarting
Check logs:
```bash
sudo journalctl -u grpcconnector2.service -n 50 --no-pager
```
Common causes: wrong `CONTROLLER_ADDR`, expired token, missing CA cert, firewall blocking port 8443.
about the derived policy key and POLICY_SIGNING_KEY now being optional.