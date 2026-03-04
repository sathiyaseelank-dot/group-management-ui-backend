# HTR (How To Run)

This file explains how to run `group-management-ui-backend` with the migrated features:
- OAuth invite flow
- PostgreSQL support
- Docker setup
- Invite via SMTP mail
- Device trust + device enrollment
- mTLS gRPC control plane

## 1. Prerequisites

- Docker + Docker Compose (for container run)
- Go 1.24+ (for manual run)
- Node.js 20+ (if running frontend)

## 2. Quick Run (Docker)

From project root:

```bash
cd /home/inkyank-03/Toji/Inkyank/ZTNA/group-management-ui-backend
cp .env.example .env
docker compose up --build
```

Services:
- Controller admin HTTP: `http://localhost:8081`
- Controller gRPC/mTLS: `localhost:8443`
- PostgreSQL: `localhost:5432`
- Frontend UI: `http://localhost:3000`
- Frontend BFF: `http://localhost:3001`

After containers are up, open:
- Login page: `http://localhost:3000/login`
- Dashboard: `http://localhost:3000/dashboard/groups`

## 3. Environment Variables

Main file: `.env` (copied from `.env.example`)

Important values:
- `ADMIN_AUTH_TOKEN`
- `INTERNAL_API_TOKEN`
- `POLICY_SIGNING_KEY`
- `DATABASE_URL`
- `GOOGLE_CLIENT_ID`
- `GOOGLE_CLIENT_SECRET`
- `USER_OAUTH_REDIRECT_URL`
- `INVITE_BASE_URL`
- `DASHBOARD_URL`
- `ADMIN_LOGIN_EMAILS` (comma-separated emails allowed for direct admin Google login)
- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USER`
- `SMTP_PASS`
- `SMTP_FROM`
- `VITE_INVITE_BASE_URL` (frontend login page base URL)

Notes:
- OAuth works only if Google client ID/secret are valid.
- Invite creation still works without SMTP, but mail delivery will fail/skip.

## 4. Manual Run (Without Docker)

### 4.1 Start PostgreSQL

Use local Postgres and create DB credentials matching `DATABASE_URL`.

Example `DATABASE_URL`:

```bash
export DATABASE_URL='postgres://gmuser:gmpass@localhost:5432/gmdb?sslmode=disable'
```

### 4.2 Run controller

```bash
cd backend/controller
go run .
```

Controller reads settings from environment (or `backend/controller/.env` if your shell loads it).

## 5. Build/Verify

Run these to verify everything compiles:

```bash
cd backend/controller && go build ./... && go test ./...
cd ../connector && go build ./...
cd ../tunneler && go build ./...
```

## 6. API Smoke Checks

### 6.1 Create invite

```bash
curl -X POST http://localhost:8081/api/admin/invites \
  -H "Authorization: Bearer 7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4" \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com"}'
```

### 6.2 Device list (admin)

```bash
curl http://localhost:8081/api/admin/devices \
  -H "Authorization: Bearer 7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4"
```

### 6.3 Trust/Block a device

```bash
curl -X PATCH http://localhost:8081/api/admin/devices/<device_id>/trust \
  -H "Authorization: Bearer 7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4" \
  -H "Content-Type: application/json" \
  -d '{"trust_state":"trusted"}'
```

## 7. OAuth Invite Flow (High-level)

1. Admin logs in directly from `http://localhost:3000/login` using Google.
2. Admin creates invite: `POST /api/admin/invites`
   - Dashboard "Add User" also triggers this invite API.
3. User opens invite link from email: `http://localhost:3000/login?invite_token=...`
4. Login page button redirects to invite landing: `GET /invite?token=...`
5. Redirect to Google OAuth: `GET /oauth/google/login`
6. Callback processes user and issues verification token: `GET /oauth/google/callback`
   - User is redirected back to `DASHBOARD_URL/login`.
7. Client enrolls device with verification token: `POST /api/devices/enroll`

## 8. Troubleshooting

- `401 unauthorized` on admin routes:
  - Verify `Authorization: Bearer <ADMIN_AUTH_TOKEN>`.
- OAuth callback fails:
  - Check `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, and redirect URI in Google console.
- Invite email not sent:
  - Check `SMTP_*` values and provider app-password requirements.
- DB connection errors:
  - Verify `DATABASE_URL` and Postgres container/service status.
