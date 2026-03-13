# ZTNA Architecture

## System Overview

The ZTNA (Zero Trust Network Access) system consists of four main components that work together to provide secure, policy-based access to resources.

## Components

### 1. Controller (Certificate Authority & Control Plane)

**Technology:** Go  
**Location:** `services/controller/`

**Responsibilities:**
- Internal Certificate Authority (CA)
- Identity enrollment and management
- Policy enforcement and validation
- Certificate issuance and rotation
- gRPC control plane

**Key Features:**
- SPIFFE ID management
- mTLS certificate generation
- SQLite database for state
- HTTP API for management
- Admin endpoints

**Trust Domain:** `spiffe://mycorp.internal`

### 2. Connector (Gateway Service)

**Technology:** Rust  
**Location:** `services/connector/`

**Responsibilities:**
- Gateway between agents and resources
- Accept inbound agent connections
- Policy-based routing
- High-performance proxy
- Connection management

**Key Features:**
- mTLS authentication
- Policy validation
- Connection pooling
- Metrics and monitoring
- Async I/O with Tokio

**SPIFFE ID:** `spiffe://mycorp.internal/connector/<id>`

### 3. Agent (Client Service)

**Technology:** Rust
**Location:** `services/agent/`

**Responsibilities:**
- Client-side service
- Connect to connector
- Local SOCKS5 proxy
- Certificate management
- User authentication
- nftables firewall enforcement

**Key Features:**
- mTLS client authentication
- Automatic certificate rotation
- Local proxy server
- Connection retry logic
- Async I/O with Tokio
- Per-port nftables firewall rules for protected resources

**SPIFFE ID:** `spiffe://mycorp.internal/tunneler/<id>` (path kept as `tunneler` for wire compatibility)

### 4. Frontend (Management UI)

**Technology:** React + TypeScript  
**Location:** `apps/frontend/`

**Responsibilities:**
- User management interface
- Policy configuration
- Real-time monitoring
- Device profile management
- Dashboard and analytics

**Key Features:**
- Modern React UI
- TailwindCSS styling
- Real-time updates
- Role-based access
- Responsive design

## Data Flow

### 1. Enrollment Flow

```
Agent/Connector
    │
    ├─► Enrollment Request (with token)
    │
Controller (CA)
    │
    ├─► Validate token
    ├─► Generate SPIFFE ID
    ├─► Issue mTLS certificate
    │
    └─► Return certificate + CA bundle
```

### 2. Connection Flow

```
Agent
    │
    ├─► mTLS handshake with Connector
    │
Connector
    │
    ├─► Validate certificate
    ├─► Check policy with Controller
    │
Controller
    │
    ├─► Validate SPIFFE ID
    ├─► Check access policy
    │
    └─► Return policy decision
    │
Connector
    │
    └─► Allow/Deny connection
```

### 3. Policy Enforcement

```
User Request
    │
    ├─► Agent (local proxy + firewall)
    │
    ├─► Connector (gateway)
    │       │
    │       ├─► Policy check
    │       │
    │   Controller
    │       │
    │       └─► Policy decision
    │
    └─► Target Resource (if allowed)
```

## Security Model

### Zero Trust Principles

1. **Never Trust, Always Verify**
   - Every connection requires authentication
   - Continuous verification of identity
   - No implicit trust based on network location

2. **Least Privilege Access**
   - Minimal permissions by default
   - Policy-based access control
   - Time-limited certificates

3. **Assume Breach**
   - Encrypted communication (mTLS)
   - Certificate rotation
   - Audit logging

### Authentication & Authorization

**Authentication:**
- mTLS for all connections
- SPIFFE IDs for identity
- Enrollment tokens for initial setup

**Authorization:**
- Policy-based access control
- Resource-level permissions
- User and device attributes

### Certificate Management

**Certificate Lifecycle:**
1. Enrollment with token
2. Certificate issuance
3. Automatic rotation
4. Revocation support

**Certificate Properties:**
- Short-lived (configurable TTL)
- SPIFFE ID in SAN
- Signed by internal CA
- Mutual TLS required

## Communication Protocols

### gRPC (Control Plane)

**Used for:**
- Enrollment requests
- Policy queries
- Certificate operations
- Health checks

**Benefits:**
- Type-safe with protobuf
- Bidirectional streaming
- Built-in mTLS support
- Efficient binary protocol

### HTTP/HTTPS (Management)

**Used for:**
- Frontend API
- Admin operations
- CA certificate distribution
- Health endpoints

## Database Schema

### Controller Database (SQLite)

**Tables:**
- `connectors` - Registered connectors
- `tunnelers` - Registered agents (table name kept for schema compatibility)
- `policies` - Access policies
- `certificates` - Issued certificates
- `audit_logs` - Security audit trail

## Deployment Architecture

### Single Region

```
┌─────────────────────────────────────┐
│           Load Balancer             │
└─────────────────────────────────────┘
                  │
    ┌─────────────┼─────────────┐
    │             │             │
┌───▼───┐    ┌───▼───┐    ┌───▼───┐
│Conn 1 │    │Conn 2 │    │Conn 3 │
└───┬───┘    └───┬───┘    └───┬───┘
    │             │             │
    └─────────────┼─────────────┘
                  │
          ┌───────▼────────┐
          │   Controller   │
          └────────────────┘
```

### Multi-Region

```
Region A                Region B
┌──────────┐           ┌──────────┐
│Connector │           │Connector │
└────┬─────┘           └────┬─────┘
     │                      │
     └──────────┬───────────┘
                │
        ┌───────▼────────┐
        │   Controller   │
        │   (Primary)    │
        └────────────────┘
```

## Scalability

### Horizontal Scaling

- **Connectors:** Stateless, scale independently
- **Controller:** Single instance (CA), read replicas possible
- **Frontend:** Stateless, scale behind load balancer

### Performance Considerations

- Connection pooling in connectors
- Certificate caching
- Policy caching with TTL
- Async I/O throughout

## Monitoring & Observability

### Metrics

- Connection counts
- Certificate issuance rate
- Policy evaluation latency
- Error rates

### Logging

- Structured logging (JSON)
- Audit trail for security events
- Connection logs
- Error tracking

### Health Checks

- Component health endpoints
- Certificate expiry monitoring
- Database connectivity
- gRPC service health

## Future Enhancements

- Multi-tenancy support
- Advanced policy engine
- Certificate revocation lists (CRL)
- OIDC integration
- Hardware security module (HSM) support
- Distributed tracing
