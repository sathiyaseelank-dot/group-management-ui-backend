# ZTNA (Zero Trust Network Access) Project

A production-ready Zero Trust Network Access system with mTLS authentication, SPIFFE IDs, and policy-based access control.

## 🏗️ Architecture

```
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│  Tunneler   │────────▶│  Connector  │────────▶│ Controller  │
│  (Client)   │  mTLS   │  (Gateway)  │  mTLS   │    (CA)     │
└─────────────┘         └─────────────┘         └─────────────┘
                                                        │
                                                        ▼
                                                  ┌──────────┐
                                                  │ Frontend │
                                                  │   (UI)   │
                                                  └──────────┘
```

## 📁 Project Structure

```
.
├── services/
│   ├── controller/      # Go - Certificate Authority & Control Plane
│   ├── connector/       # Rust - Gateway Service
│   └── tunneler/        # Rust - Client Service
├── apps/
│   └── frontend/        # React - Management UI
├── shared/
│   ├── proto/          # Protobuf definitions
│   └── configs/        # Shared configurations
├── scripts/            # Deployment & setup scripts
├── systemd/            # Systemd service files
└── docs/               # Documentation

```

## 🚀 Quick Start

### Prerequisites

- **Go** 1.21+ (for controller)
- **Rust** 1.70+ (for connector/tunneler)
- **Node.js** 18+ (for frontend)
- **Protobuf compiler** (protoc)

### Development Setup

```bash
# Clone the repository
git clone https://github.com/sathiyaseelank-dot/group-management-ui-backend.git
cd group-management-ui-backend

# Build all components
make build-all

# Or build individually
make build-controller
make build-connector
make build-tunneler
make build-frontend
```

### Running Services

```bash
# Run controller
make dev-controller

# Run connector (in another terminal)
make dev-connector

# Run tunneler (in another terminal)
make dev-tunneler

# Run frontend (in another terminal)
make dev-frontend
```

## 📦 Components

### Controller (Go)
- Internal Certificate Authority
- Enrollment & identity management
- Policy enforcement
- gRPC control plane

**Location:** `services/controller/`  
**Tech:** Go, gRPC, SQLite

### Connector (Rust)
- Gateway service
- Accepts tunneler connections
- Policy-based routing
- High-performance proxy

**Location:** `services/connector/`  
**Tech:** Rust, Tokio, gRPC

### Tunneler (Rust)
- Client service
- Connects to connector
- Local proxy
- mTLS authentication

**Location:** `services/tunneler/`  
**Tech:** Rust, Tokio, gRPC

### Frontend (React)
- Management dashboard
- User & policy management
- Real-time monitoring
- Device profiles

**Location:** `apps/frontend/`  
**Tech:** React, TypeScript, Vite, TailwindCSS

## 🔧 Development

### Makefile Commands

```bash
make help              # Show all available commands
make build-all         # Build all components
make test-all          # Run all tests
make clean             # Clean build artifacts
```

### Component-Specific Development

Each component has its own README with detailed instructions:
- [Controller README](services/controller/RUN.md)
- [Connector README](services/connector/run.md)
- Frontend README (in apps/frontend/)

## 🚢 Deployment

### Production Installation

Use the automated setup scripts:

```bash
# Install connector
curl -fsSL https://raw.githubusercontent.com/sathiyaseelank-dot/group-management-ui-backend/main/scripts/setup.sh | sudo bash

# Install tunneler
curl -fsSL https://raw.githubusercontent.com/sathiyaseelank-dot/group-management-ui-backend/main/scripts/tunneler-setup.sh | sudo bash
```

### Required Environment Variables

See [deployment documentation](docs/deployment.md) for complete configuration guide.

## 👥 Team Workflow

### Branch Strategy
- `main` - Production-ready code
- `develop` - Integration branch
- `feature/*` - Feature branches

### Component Ownership
- **Controller**: Backend API & CA management
- **Connector**: Gateway service & routing
- **Tunneler**: Client service & proxy
- **Frontend**: UI & user experience

### Development Workflow
1. Create feature branch from `develop`
2. Work on your component
3. Run tests: `make test-<component>`
4. Create PR to `develop`
5. Code review (1+ approvals)
6. Merge after CI passes

## 🔐 Security

- mTLS authentication for all connections
- SPIFFE IDs for identity management
- Policy-based access control
- Certificate rotation
- Secure enrollment process

**Trust Domain:** `spiffe://mycorp.internal`

## 📚 Documentation

- [Architecture Overview](docs/architecture.md)
- [Development Guide](docs/development.md)
- [API Reference](docs/api-reference.md)
- [Deployment Guide](docs/deployment.md)

## 🧪 Testing

```bash
# Test all components
make test-all

# Test individual components
make test-controller
make test-connector
make test-tunneler
make test-frontend
```

## 📝 License

[Add your license here]

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## 📞 Support

For issues and questions:
- GitHub Issues: [Create an issue](https://github.com/sathiyaseelank-dot/group-management-ui-backend/issues)
- Documentation: [docs/](docs/)

---

**Built with ❤️ by the ZTNA Team**
