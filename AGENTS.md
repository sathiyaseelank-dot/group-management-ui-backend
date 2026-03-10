# Repository Guidelines

## Project Structure & Module Organization
- `services/controller/`: Go control plane (gRPC + admin HTTP API).
- `services/connector/`: Rust gateway service.
- `services/tunneler/`: Rust client service.
- `apps/frontend/`: Vite + React + TypeScript UI plus Express server.
- `shared/proto/`: Protobuf definitions shared across services.
- `shared/configs/`: Config and `.env` examples.
- `docs/`: Architecture and development notes.
- `scripts/`: Setup and deployment helpers.

## Build, Test, and Development Commands
Run from repo root unless noted.
- `make build-all`: Build all components.
- `make dev-controller|dev-connector|dev-tunneler|dev-frontend`: Run a single component in dev mode.
- `make test-all`: Run all component test suites.
- `make test-controller|test-connector|test-tunneler|test-frontend`: Run component tests.
- `make clean`: Clear build artifacts.

Component-local equivalents:
- Go: `cd services/controller && go build ./... && go test ./...`
- Rust: `cd services/connector && cargo build --release && cargo test`
- Frontend: `cd apps/frontend && npm run dev|build|lint`

## Coding Style & Naming Conventions
- Use language-standard formatting: `gofmt` for Go, `rustfmt` for Rust.
- Frontend linting via `npm run lint` (ESLint).
- Tests follow language conventions: Go `_test.go`, Rust `#[test]` modules, frontend tests live near `src/` where applicable.
- Branch names: `feature/<component>-short-description` (see `docs/development.md`).

## Testing Guidelines
- Prefer `make test-<component>` before pushing.
- When updating `shared/proto/`, regenerate and verify all services.
- Add unit, integration, or end-to-end tests based on impact (see `docs/development.md`).

## Commit & Pull Request Guidelines
- Commit messages generally follow Conventional Commits, often with scopes:
  - Examples: `feat(controller): ...`, `fix: ...`, `chore: ...`, `test(tunneler): ...`.
- PRs target the `develop` branch and should include:
  - Clear description, linked issue (if any), and tests run.
  - Updates to docs or configs when behavior or env vars change.
  - Small, focused changes that are easy to review.

## Security & Configuration Tips
- Never commit secrets. Use `shared/configs/.env.example` for new variables.
- Controller uses PostgreSQL when `DATABASE_URL` is set; otherwise SQLite.

## Agent-Specific Notes
- If you are using an automated assistant, read `CLAUDE.md` for architecture details, command summaries, and environment variables.
