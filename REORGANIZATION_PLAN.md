# ZTNA Project Reorganization Plan

## Objectives
1. Remove deprecated Go connector/tunneler
2. Organize codebase for 4-member parallel development
3. Clear separation of concerns
4. Efficient CI/CD workflows

## Phase 1: Cleanup (Do First)

### Remove Go Implementations
```bash
# Backup first (optional)
mkdir -p ../backup
cp -r backend/connector ../backup/
cp -r backend/tunneler ../backup/

# Remove Go connector and tunneler
rm -rf backend/connector
rm -rf backend/tunneler
rm -f backend/go.mod backend/go.sum

# Remove old binaries
rm -f dist/grpcconnector2-linux-*
rm -f dist/grpctunneler-linux-*

# Update scripts to remove Go references
# (Review and update scripts/setup.sh, scripts/tunneler-setup.sh)
```

## Phase 2: Restructure

### New Directory Structure
```
ztna-project/
├── .github/
│   └── workflows/
│       ├── controller.yml
│       ├── connector.yml
│       ├── tunneler.yml
│       └── frontend.yml
├── docs/
│   ├── README.md
│   ├── architecture.md
│   ├── api-reference.md
│   └── deployment.md
├── services/
│   ├── controller/         # From backend/controller
│   │   ├── api/
│   │   ├── admin/
│   │   ├── gen/
│   │   ├── main.go
│   │   ├── go.mod
│   │   └── README.md
│   ├── connector/          # From backend/connector-rs
│   │   ├── src/
│   │   ├── Cargo.toml
│   │   └── README.md
│   └── tunneler/           # From backend/tunneler-rs
│       ├── src/
│       ├── Cargo.toml
│       └── README.md
├── apps/
│   └── frontend/           # From frontend/
│       ├── src/
│       ├── components/
│       ├── package.json
│       └── README.md
├── shared/
│   ├── proto/              # From backend/proto
│   │   └── controller.proto
│   └── configs/
│       └── .env.example
├── scripts/
│   ├── build-all.sh
│   ├── dev-setup.sh
│   └── deploy.sh
├── systemd/                # Service files
├── .gitignore
├── README.md
└── Makefile                # Unified build commands
```

## Phase 3: Team Workflow Setup

### Branch Strategy
```
main                    # Production-ready code
├── develop            # Integration branch
├── feature/controller-*
├── feature/connector-*
├── feature/tunneler-*
└── feature/frontend-*
```

### Component Ownership (Suggested)
- **Member 1**: Controller backend + API
- **Member 2**: Connector (Rust)
- **Member 3**: Tunneler (Rust)
- **Member 4**: Frontend + Integration

### Development Workflow
1. Create feature branch from `develop`
2. Work on your component independently
3. Run component-specific tests
4. Create PR to `develop`
5. Code review by at least 1 team member
6. Merge after CI passes

## Phase 4: Tooling & Automation

### Makefile Commands
```makefile
# Development
make dev-controller
make dev-connector
make dev-tunneler
make dev-frontend

# Build
make build-all
make build-controller
make build-connector
make build-tunneler
make build-frontend

# Test
make test-all
make test-controller
make test-connector
make test-tunneler
make test-frontend

# Clean
make clean
```

### CI/CD Pipeline
- Separate workflows for each component
- Only build/test changed components
- Parallel builds for faster feedback
- Automated releases with tags

## Phase 5: Documentation

### Required Docs
1. **README.md** - Project overview, quick start
2. **docs/architecture.md** - System design, components
3. **docs/development.md** - Setup, workflows, standards
4. **docs/api-reference.md** - API documentation
5. **docs/deployment.md** - Deployment guide
6. **Component READMEs** - Each service/app specific docs

## Migration Steps

### Step 1: Create New Structure
```bash
mkdir -p services/{controller,connector,tunneler}
mkdir -p apps/frontend
mkdir -p shared/{proto,configs}
mkdir -p docs
```

### Step 2: Move Components
```bash
# Controller
mv backend/controller/* services/controller/

# Connector (Rust)
mv backend/connector-rs/* services/connector/

# Tunneler (Rust)
mv backend/tunneler-rs/* services/tunneler/

# Frontend
mv frontend/* apps/frontend/

# Shared
mv backend/proto/* shared/proto/
```

### Step 3: Update Paths
- Update import paths in code
- Update CI/CD workflow paths
- Update script references
- Update systemd service paths

### Step 4: Clean Up
```bash
rm -rf backend/
rm -rf frontend/
```

## Benefits

### For Team
- **Clear ownership**: Each member owns specific components
- **Parallel work**: No stepping on each other's toes
- **Faster CI**: Only build what changed
- **Better reviews**: Smaller, focused PRs

### For Project
- **Maintainability**: Clear structure, easy to navigate
- **Scalability**: Easy to add new services
- **Documentation**: Centralized and component-specific
- **Deployment**: Independent service deployment

## Communication & Coordination

### Daily Standup Topics
- What component are you working on?
- Any blockers or dependencies?
- Any API/interface changes needed?

### Integration Points
- **Proto files**: Coordinate changes in shared/proto
- **API contracts**: Document in docs/api-reference.md
- **Environment variables**: Update shared/configs/.env.example
- **Database schema**: Coordinate controller changes

### Code Review Guidelines
- Review your area of expertise
- Check for breaking changes
- Verify documentation updates
- Test integration points

## Timeline

- **Week 1**: Cleanup Go code, create new structure
- **Week 2**: Migrate components, update paths
- **Week 3**: Setup CI/CD, documentation
- **Week 4**: Team training, workflow adoption

## Next Steps

1. Team meeting to discuss and approve plan
2. Assign component ownership
3. Create migration branch
4. Execute Phase 1 (Cleanup)
5. Execute Phase 2 (Restructure)
6. Test everything works
7. Merge to main
8. Update team workflows
