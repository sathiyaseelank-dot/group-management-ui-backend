# Team Quick Start Guide

## 🎯 For New Team Members

Welcome to the ZTNA project! This guide will get you up and running quickly.

## 📋 Prerequisites

Install these tools before starting:

- **Go** 1.21+ → [Download](https://go.dev/dl/)
- **Rust** 1.70+ → [Install](https://rustup.rs/)
- **Node.js** 18+ → [Download](https://nodejs.org/)
- **Protobuf** → `sudo apt install protobuf-compiler` (Linux)

## 🚀 Quick Setup (5 minutes)

### 1. Clone Repository
```bash
git clone https://github.com/vairabarath/zero-trust.git
cd zero-trust
```

### 2. Choose Your Component

#### Working on Controller?
```bash
cd services/controller
go mod download
./setup.sh
make dev-controller
```

#### Working on Connector?
```bash
cd services/connector
cargo build
make dev-connector
```

#### Working on Agent?
```bash
cd services/agent
cargo build
make dev-agent
```

#### Working on Frontend?
```bash
cd apps/frontend
npm install
make dev-frontend
```

## 📂 Repository Structure

```
services/
  ├── controller/    ← Go backend (CA + API)
  ├── connector/     ← Rust gateway
  └── agent/         ← Rust client (nftables firewall enforcer)

apps/
  └── frontend/      ← React UI

shared/
  ├── proto/         ← Protobuf definitions
  └── configs/       ← Config examples

docs/                ← Documentation
scripts/             ← Deployment scripts
```

## 🔧 Common Commands

### Build
```bash
make build-all              # Build everything
make build-controller       # Build your component
```

### Run (Development)
```bash
make dev-controller         # Run with hot reload
make dev-connector
make dev-agent
make dev-frontend
```

### Test
```bash
make test-all               # Test everything
make test-controller        # Test your component
```

### Help
```bash
make help                   # Show all commands
```

## 🌿 Git Workflow

### 1. Start New Feature
```bash
git checkout develop
git pull origin develop
git checkout -b feature/component-description
```

### 2. Make Changes
```bash
# Edit files in your component directory
# Test locally: make dev-<component>
```

### 3. Commit
```bash
git add .
git commit -m "feat(component): description"
```

### 4. Push & Create PR
```bash
git push origin feature/component-description
# Create PR on GitHub to 'develop' branch
```

## 👥 Component Ownership

| Component | Directory | Tech | Owner |
|-----------|-----------|------|-------|
| Controller | `services/controller/` | Go | Member 1 |
| Connector | `services/connector/` | Rust | Member 2 |
| Agent | `services/agent/` | Rust | Member 3 |
| Frontend | `apps/frontend/` | React | Member 4 |

## 🔗 Integration Points

### When to Coordinate

1. **Protobuf Changes** → Affects all services
   - Location: `shared/proto/`
   - Notify team before changing

2. **API Changes** → Affects frontend
   - Document in `docs/api-reference.md`
   - Update frontend accordingly

3. **Environment Variables** → Affects deployment
   - Update `shared/configs/.env.example`
   - Document in component README

## 📝 Commit Message Format

```bash
feat(controller): add user enrollment endpoint
fix(connector): resolve connection timeout
docs(api): update authentication flow
test(agent): add integration tests
refactor(frontend): improve component structure
```

## 🐛 Troubleshooting

### Build Fails?
```bash
make clean
make build-<component>
```

### Dependencies Issue?
```bash
# Go
cd services/controller && go mod tidy

# Rust
cd services/connector && cargo update

# Node
cd apps/frontend && npm install
```

### Can't Find Command?
```bash
make help    # Shows all available commands
```

## 📚 Documentation

- **README.md** - Project overview
- **docs/development.md** - Detailed development guide
- **docs/architecture.md** - System architecture
- **Component READMEs** - Component-specific docs

## 💬 Communication

### Daily Standup (15 min)
- What did you work on yesterday?
- What are you working on today?
- Any blockers?

### Code Reviews
- Review within 24 hours
- Be constructive
- Ask questions

### Questions?
- Check documentation first
- Ask in team chat
- Create GitHub issue

## ✅ Checklist for First Day

- [ ] Clone repository
- [ ] Install prerequisites
- [ ] Build your component
- [ ] Run your component locally
- [ ] Read `docs/development.md`
- [ ] Create test branch
- [ ] Make small change
- [ ] Create test PR

## 🎓 Learning Resources

### Go (Controller)
- [Go Tour](https://go.dev/tour/)
- [Effective Go](https://go.dev/doc/effective_go)

### Rust (Connector/Agent)
- [Rust Book](https://doc.rust-lang.org/book/)
- [Rust by Example](https://doc.rust-lang.org/rust-by-example/)

### React (Frontend)
- [React Docs](https://react.dev/)
- [TypeScript Handbook](https://www.typescriptlang.org/docs/)

### gRPC
- [gRPC Docs](https://grpc.io/docs/)
- [Protobuf Guide](https://protobuf.dev/)

## 🚀 Ready to Start?

1. Pick your component
2. Run `make dev-<component>`
3. Make a small change
4. Create your first PR
5. Get it reviewed
6. Celebrate! 🎉

---

**Need help? Ask your team! We're here to support each other. 💪**
