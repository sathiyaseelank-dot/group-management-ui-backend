# Branch: feature/user-enrollment

## Status: In Progress (WIP — not fully working)

---

## What This Branch Is About

Zero-trust access control management UI fixes — specifically making the **resource detail page group/access-rule flow** work end-to-end with the Go controller's database.

---

## Problem Being Solved

When adding a group (access rule) to a resource on the Resource Detail page, the group appeared to save but **disappeared after a page refresh**.

### Root Cause

The frontend BFF (Express) layer and the Go controller used two completely different storage paths that were never in sync:

| Operation | Before (broken) | After (fixed in this branch) |
|---|---|---|
| **Create access rule** | Wrote to `authorizations` table via `/api/admin/resources/:id/assign_principal` | Writes to `access_rules` + `access_rule_groups` via `/api/access-rules` |
| **Load resource detail** | Read `resource.Authorizations` (never populated by the UI endpoint) | Proxies to `/api/resources/:id` which reads from `access_rules` + `access_rule_groups` |
| **Delete access rule** | Parsed a fake synthetic ID to call `assign_principal` DELETE | Proxies to `/api/access-rules/:id` DELETE |
| **`createAccessRule` return value** | Synthesized a fake client-side object (lost on refresh) | Returns the real persisted object from the controller |

---

## Files Changed

### `apps/frontend/server/routes/resources.ts`
- `GET /api/resources/:resourceId` — now proxies directly to the Go controller's `/api/resources/:id` UI endpoint instead of reading from the admin ACL snapshot.

### `apps/frontend/server/routes/access-rules.ts`
- **All routes** now proxy to the Go controller's `/api/access-rules` UI endpoint.
- Removed the old SQLite-based `identity-count` handler (now proxied to controller).
- Removed the `getDb` import (no longer needed).

### `apps/frontend/lib/mock-api.ts`
- `createAccessRule` — returns the real API response instead of a synthesized client-side object, so the rule ID is stable and persists across reloads.

### `services/controller/admin/ui_handlers.go`
- Added `"log"` import.
- `handleUIAccessRules` — restructured from a simple POST-only handler to a full `switch r.Method`:
  - `GET /api/access-rules` — new: lists all access rules with their group associations from DB.
  - `POST /api/access-rules` — unchanged logic, now properly inside the switch case. Added real error logging (logs actual DB errors, returns them in response instead of generic message).
- Fixed `state.Rebind` missing on multiple raw `?` placeholder queries that were silently failing on PostgreSQL:
  - `INSERT INTO user_group_members` (group member assignment)
  - `SELECT / INSERT` stmts in the group-add-resources handler (lines ~476–478)
  - `INSERT INTO connector_logs` (connector revoke + heartbeat handlers)
  - `identity-count` query in `handleUIAccessRulesSubroutes`

---

## Current State / Known Issues

1. **Controller must be restarted** to pick up the `ui_handlers.go` changes. The binary compiled before these changes is still running. Run:
   ```bash
   make dev-controller
   ```

2. **`identity-count` shows 500** — was returning "failed to compute identity count" because the query used raw `?` instead of `state.Rebind(...)` for PostgreSQL. **Fixed in this branch**, but needs a controller restart.

3. **`GET /api/access-rules` in browser console** — appears multiple times on page load from an unknown source (possibly a browser extension or Vite HMR). Not from any React component. It now returns `[]` properly (no longer 405).

4. **After controller restart**, the full flow should work:
   - Add group to resource → persists in DB
   - Refresh page → group still shown
   - Delete group → removed from DB

---

## Architecture Notes

- The Go controller uses **PostgreSQL only** (`DATABASE_URL` required). SQLite is disabled in `state/db.go`.
- `state.Rebind()` must wrap every SQL query with `?` placeholders — it converts them to `$1, $2, ...` for PostgreSQL.
- The UI routes (`/api/resources`, `/api/access-rules`, `/api/groups`, etc.) are registered in `admin/ui_routes.go` via `RegisterUIRoutes` called from `RegisterRoutes` in `admin/handlers.go`.
- The Express BFF (port 3001) proxies all `/api/*` requests to the Go controller (port 8081) via `proxyToBackend` in `lib/proxy.ts`.
- Vite dev server (port 3000) proxies `/api/*` to the Express BFF (port 3001).

---

## How to Resume

1. Restart the controller: `make dev-controller`
2. Test the resource detail page — add a group, refresh, verify it persists
3. Check the controller terminal for any logged errors during the POST
4. If still failing, the error message in the toast will now be the actual DB error (not a generic one)
