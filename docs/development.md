# Development Guide

## Team Structure

This project is designed for 4-member parallel development:

- **Member 1**: Controller (Go backend)
- **Member 2**: Connector (Rust gateway)
- **Member 3**: Agent (Rust client)
- **Member 4**: Frontend (React UI)

## Getting Started

### 1. Clone and Setup

```bash
git clone https://github.com/vairabarath/zero-trust.git
cd zero-trust
```

### 2. Install Dependencies

**Controller (Go):**
```bash
cd services/controller
go mod download
```

**Connector (Rust):**
```bash
cd services/connector
cargo build
```

**Agent (Rust):**
```bash
cd services/agent
cargo build
```

**Frontend (React):**
```bash
cd apps/frontend
npm install
```

## Development Workflow

### Branch Strategy

```
main                    # Production code
├── develop            # Integration branch
├── feature/controller-*
├── feature/connector-*
├── feature/agent-*
└── feature/frontend-*
```

### Creating a Feature Branch

```bash
git checkout develop
git pull origin develop
git checkout -b feature/component-name-description
```

### Working on Your Component

1. Make changes in your component directory
2. Test locally: `make dev-<component>`
3. Run tests: `make test-<component>`
4. Commit with clear messages

### Creating a Pull Request

1. Push your branch: `git push origin feature/your-branch`
2. Create PR to `develop` branch
3. Request review from team member
4. Address review comments
5. Merge after approval and CI passes

## Component-Specific Guidelines

### Controller (Go)

**Directory:** `services/controller/`

**Running:**
```bash
make dev-controller
# or
cd services/controller && go run .
```

**Testing:**
```bash
make test-controller
```

**Key Files:**
- `main.go` - Entry point
- `api/` - API handlers
- `admin/` - Admin endpoints

### Connector (Rust)

**Directory:** `services/connector/`

**Running:**
```bash
make dev-connector
# or
cd services/connector && cargo run
```

**Testing:**
```bash
make test-connector
```

**Key Files:**
- `src/main.rs` - Entry point
- `src/grpc/` - gRPC handlers
- `Cargo.toml` - Dependencies

### Agent (Rust)

**Directory:** `services/agent/`

**Running:**
```bash
make dev-agent
# or
cd services/agent && cargo run
```

**Testing:**
```bash
make test-agent
```

### Frontend (React)

**Directory:** `apps/frontend/`

**Running:**
```bash
make dev-frontend
# or
cd apps/frontend && npm run dev
```

**Testing:**
```bash
make test-frontend
```

**Key Directories:**
- `src/pages/` - Page components
- `src/components/` - Reusable components
- `lib/` - Utilities and helpers

## Integration Points

### Protobuf Definitions

**Location:** `shared/proto/`

When updating proto files:
1. Modify `shared/proto/controller.proto`
2. Regenerate code for all services
3. Update affected components
4. Coordinate with team

### API Contracts

Document API changes in `docs/api-reference.md`

### Environment Variables

Update `shared/configs/.env.example` when adding new env vars

## Code Review Guidelines

### As a Reviewer

- Check for breaking changes
- Verify tests are included
- Ensure documentation is updated
- Test integration points
- Review security implications

### As an Author

- Write clear PR descriptions
- Include test coverage
- Update relevant documentation
- Keep PRs focused and small
- Respond to feedback promptly

## Testing Strategy

### Unit Tests

Test individual functions and modules

### Integration Tests

Test component interactions

### End-to-End Tests

Test complete workflows

## Common Tasks

### Adding a New API Endpoint

1. Update proto file (if gRPC)
2. Implement in controller
3. Update frontend to consume
4. Add tests
5. Document in API reference

### Updating Dependencies

**Go:**
```bash
cd services/controller
go get -u ./...
go mod tidy
```

**Rust:**
```bash
cd services/connector  # or agent
cargo update
```

**Node:**
```bash
cd apps/frontend
npm update
```

## Troubleshooting

### Build Failures

- Check dependency versions
- Clear build cache: `make clean`
- Verify environment variables

### Test Failures

- Run tests locally before pushing
- Check for race conditions
- Verify test data setup

### Integration Issues

- Check proto file compatibility
- Verify API contracts
- Test with latest `develop` branch

## Communication

### Daily Standup

- What did you work on?
- What are you working on today?
- Any blockers?

### Code Reviews

- Review within 24 hours
- Be constructive and respectful
- Ask questions if unclear

### Documentation

- Update docs with code changes
- Keep README files current
- Document breaking changes

## Best Practices

### Git Commits

```bash
# Good commit messages
feat(controller): add user enrollment endpoint
fix(connector): resolve connection timeout issue
docs(api): update authentication flow

# Bad commit messages
update
fix bug
changes
```

### Code Style

- Follow language-specific conventions
- Use linters and formatters
- Keep functions small and focused
- Write self-documenting code

### Security

- Never commit secrets
- Use environment variables
- Validate all inputs
- Follow principle of least privilege

## Resources

- [Go Documentation](https://go.dev/doc/)
- [Rust Book](https://doc.rust-lang.org/book/)
- [React Documentation](https://react.dev/)
- [gRPC Documentation](https://grpc.io/docs/)
