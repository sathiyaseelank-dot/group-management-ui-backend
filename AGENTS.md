# Repository Guidelines

## Project Structure & Module Organization
This repository has two main areas:
- `backend/`: Go services for zero-trust networking.
- `frontend/`: React + TypeScript admin UI (Vite).

Backend is split into modules:
- `backend/controller/`: control-plane API/admin server (`main.go`, `controller/api`, `controller/admin`, `controller/state`).
- `backend/connector/`: connector agent (`main.go`, `run/`, `enroll/`).
- `backend/tunneler/`: tunneler client (`main.go`, `run/`, `enroll/`).
- `backend/proto/`: protobuf definitions.

Frontend code lives in:
- `frontend/src/`: pages and app shell.
- `frontend/components/`: reusable UI and dashboard components.
- `frontend/server/`: lightweight server/proxy routes.

## Build, Test, and Development Commands
- `docker compose up --build` (repo root): start full local stack.
- `cd frontend && npm run dev`: run Vite UI + TSX server in watch mode.
- `cd frontend && npm run build`: produce production bundle in `frontend/dist`.
- `cd frontend && npm run lint`: run ESLint.
- `cd backend/controller && go build ./...` (repeat for `connector`, `tunneler`): compile Go services.
- `cd backend/controller && ./run-air.sh`: run controller with live reload (Air).

## Coding Style & Naming Conventions
- Go: keep code `gofmt`-formatted; package names lowercase; exported identifiers use `PascalCase`; prefer table-driven tests.
- TypeScript/React: 2-space indentation, strict TypeScript (`frontend/tsconfig.json`), components in `PascalCase.tsx`, hooks/utilities in `camelCase.ts`.
- Keep route/domain naming consistent with existing folders (e.g., `remote-networks`, `service-accounts`).

## Testing Guidelines
- Backend tests use Go’s `testing` package. Run per module: `go test ./...`.
- Place Go tests as `*_test.go` next to implementation files.
- Frontend has linting but no established test runner yet; when adding tests, colocate with source and document the run command in `frontend/package.json`.

## Commit & Pull Request Guidelines
- Current history favors short, imperative subjects (e.g., `user tab ui changes`, `tunneler ui defect resolve`).
- Prefer: `<area>: <change>` (example: `frontend: improve group detail actions`).
- PRs should include:
  - clear summary and scope,
  - linked issue/ticket (if available),
  - screenshots or short video for UI changes,
  - notes on config/env changes (`.env.example`, ports, tokens).

## Security & Configuration Tips
- Never commit real secrets or tokens; use `.env` locally and keep `.env.example` updated.
- Treat CA keys/certs and enrollment tokens as sensitive; rotate any leaked credentials immediately.
