# BFF Authentication Migration: Static Token → Per-User JWT

## Overview

The frontend BFF (Express server) proxies all admin API calls to the Go controller.
Previously every request used a single shared static token regardless of which user
was logged in. After the migration, each request carries the user's own JWT so the
controller can enforce per-user authorization.

---

## Before — Static `ADMIN_AUTH_TOKEN`

### `apps/frontend/lib/proxy.ts` (old)

```ts
// Every request to the controller used the same static bearer token.
export async function proxyToBackend<T = any>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const response = await fetch(`${getBackendUrl()}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${getAdminAuthToken()}`,  // static token, always
      ...options.headers,
    },
  });
  ...
}
```

### Route handler (old)

```ts
// connectors.ts — no JWT extracted, static token used implicitly
router.get('/', async (_req, res) => {
  const connectors = await proxyToBackend('/api/connectors')
  res.json(connectors)
})
```

### Controller `adminAuth` middleware (unchanged — the bypass still exists)

```go
func (s *Server) adminAuth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        auth := r.Header.Get("Authorization")
        if auth == "Bearer "+s.AdminAuthToken {
            next.ServeHTTP(w, r)   // ← email / role check skipped entirely
            return
        }
        // JWT path — never reached from the BFF
        ...
    })
}
```

### Security problem

| Issue | Detail |
|---|---|
| Any logged-in user = admin | BFF sends `ADMIN_AUTH_TOKEN`; controller skips all email/role checks |
| `ADMIN_LOGIN_EMAILS` useless | Only blocks direct API calls on port 8081, not BFF traffic |
| One token for everything | Token compromise grants full access to every admin endpoint |
| No per-request identity | Controller has no idea which user triggered an action |

---

## After — Per-User JWT

### `apps/frontend/lib/proxy.ts` (new)

```ts
// JWT extracted from the incoming request's Authorization header.
export function getJWTFromRequest(req): string | undefined {
  const auth = req.headers.authorization
  if (auth?.startsWith('Bearer ')) return auth.slice(7)
  return undefined
}

// Falls back to static token only if no JWT is present (e.g. internal/server calls).
export async function proxyToBackend<T = any>(
  path: string,
  options: RequestInit = {},
  userJWT?: string          // ← new optional parameter
): Promise<T> {
  const authHeader = userJWT
    ? `Bearer ${userJWT}`
    : `Bearer ${getAdminAuthToken()}`   // fallback for non-browser callers

  const response = await fetch(`${getBackendUrl()}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      'Authorization': authHeader,
      ...options.headers,
    },
  });
  ...
}
```

### Route handlers (new)

```ts
// connectors.ts — JWT forwarded from the browser session
router.get('/', async (req, res) => {
  const connectors = await proxyToBackend('/api/connectors', {}, getJWTFromRequest(req))
  res.json(connectors)
})

router.delete('/:id', async (req, res) => {
  await proxyToBackend(`/api/admin/connectors/${req.params.id}`, {
    method: 'DELETE',
  }, getJWTFromRequest(req))
  res.json({ ok: true })
})
```

All 19 route files (`connectors`, `agents`, `users`, `resources`, `groups`,
`access-rules`, `audit-logs`, `discovery`, `remote-networks`, `tokens`, …) now
pass `getJWTFromRequest(req)` to `proxyToBackend`.

### Controller `adminAuth` middleware — JWT path now reached

```go
// Same middleware, JWT branch is now exercised for every BFF request.
func (s *Server) adminAuth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        auth := r.Header.Get("Authorization")
        if auth == "Bearer "+s.AdminAuthToken {
            next.ServeHTTP(w, r)   // still works for internal/service calls
            return
        }

        // BFF now reaches here with the user's JWT ↓
        claims, err := parseAllClaims(getTokenFromRequest(r), s.JWTSecret)
        if err != nil { /* 401 */ }

        if claims.aud == "device" { /* 401 — device tokens rejected */ }

        if s.Sessions != nil {
            if valid, _ := s.Sessions.IsValid(claims.jti); !valid { /* 401 */ }
        }

        if !s.isAdminEmail(claims.email) { /* 403 */ }

        // Workspace + email injected into request context
        ctx := withSessionEmail(r.Context(), claims.email)
        ctx  = withWorkspace(ctx, claims.userID, claims.wsID, claims.wsSlug, claims.wsRole)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

---

## What Changes End-to-End

```
Before
  Browser → Express BFF → Controller
                ↓
        ADMIN_AUTH_TOKEN (static, same for every user)
                ↓
        adminAuth bypass → handler (no identity known)

After
  Browser → Express BFF → Controller
                ↓
        Bearer <user-jwt> (unique per session, 24 h expiry)
                ↓
        adminAuth validates JWT audience + session + email/role
                ↓
        handler receives email + workspace in context
```

---

## Affected Files

| File | Change |
|---|---|
| `apps/frontend/lib/proxy.ts` | Added `userJWT` param to `proxyToBackend`; added `getJWTFromRequest` helper |
| `apps/frontend/server/routes/*.ts` | All 19 route files pass `getJWTFromRequest(req)` |
| `services/controller/admin/server.go` | No code change — JWT branch was already implemented, now actually reached |

---

## Remaining Gaps (not yet addressed)

| Gap | Risk |
|---|---|
| Static `ADMIN_AUTH_TOKEN` bypass still exists in `adminAuth` | Internal/service callers still work; could be abused if token leaks |
| `ADMIN_LOGIN_EMAILS` only checked on JWT path | Still irrelevant for static-token callers |
| Signup is open (no domain allowlist) | Anyone can create a workspace |
| `withWorkspaceContext` does not require a JWT | Handlers using it must manually enforce auth |
