# Zero-Trust gRPC Connector System

This repository contains a minimal Twingate-style zero-trust connector system built with Go, gRPC, mTLS, and SPIFFE IDs.

## Components

- **Controller**: Internal CA + enrollment/control-plane gRPC server.
- **Connector**: Long-running service that connects outbound to the controller and accepts inbound tunneler connections.
- **Tunneler**: Client that connects to a connector with mTLS.

## Trust Domain

- `spiffe://mycorp.internal`

Connector SPIFFE ID:
- `spiffe://mycorp.internal/connector/<id>`

Tunneler SPIFFE ID:
- `spiffe://mycorp.internal/tunneler/<id>`

## Environment Variables

### Controller

Required:
- `INTERNAL_CA_CERT` (PEM)
- `INTERNAL_CA_KEY` (PEM, PKCS#8, in-memory only)

Optional:
- `TRUST_DOMAIN` (default: `mycorp.internal`)
- `CONTROLLER_ID` (default: `default`)
- `CONTROLLER_CERT` (PEM, if you want to supply a fixed server cert)
- `CONTROLLER_KEY` (PEM)

### Connector

Required:
- `CONTROLLER_ADDR` (host:port)
- `CONNECTOR_ID`
- `INTERNAL_CA_CERT` (PEM)
- `BOOTSTRAP_CERT` (PEM, bootstrap identity to call enroll)
- `BOOTSTRAP_KEY` (PEM)

Optional:
- `TRUST_DOMAIN` (default: `mycorp.internal`)
- `CONNECTOR_LISTEN_ADDR` (default: `:9443`)

### Tunneler

Required:
- `CONTROLLER_ADDR` (host:port)
- `CONNECTOR_ADDR` (host:port)
- `TUNNELER_ID`
- `INTERNAL_CA_CERT` (PEM)
- `BOOTSTRAP_CERT` (PEM, bootstrap identity to call enroll)
- `BOOTSTRAP_KEY` (PEM)

Optional:
- `TRUST_DOMAIN` (default: `mycorp.internal`)

## Example systemd units

These unit files are designed to be compatible with hardening. Store secrets in an environment file managed by your secret distribution system.

### controller.service

```
[Unit]
Description=Zero-Trust Controller
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/controller
EnvironmentFile=/etc/zt/controller.env
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
MemoryDenyWriteExecute=true
LockPersonality=true
RestrictSUIDSGID=true
RestrictRealtime=true
SystemCallFilter=@system-service
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
```

### connector.service

```
[Unit]
Description=Zero-Trust Connector
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/connector run
EnvironmentFile=/etc/zt/connector.env
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
MemoryDenyWriteExecute=true
LockPersonality=true
RestrictSUIDSGID=true
RestrictRealtime=true
SystemCallFilter=@system-service
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
```

### tunneler.service

```
[Unit]
Description=Zero-Trust Tunneler
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/tunneler run
EnvironmentFile=/etc/zt/tunneler.env
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
MemoryDenyWriteExecute=true
LockPersonality=true
RestrictSUIDSGID=true
RestrictRealtime=true
SystemCallFilter=@system-service
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
```

## Build

Each component is a separate Go module:

```
cd controller && go build ./...
cd ../connector && go build ./...
cd ../tunneler && go build ./...
```

The repo root does not contain Go packages, so `go build ./...` there will report no matches.
