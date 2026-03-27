import fs from 'fs'
import path from 'path'

// Load apps/frontend/.env for the BFF server (Node) before reading process.env.
const envPath = path.resolve(__dirname, '../.env')
if (fs.existsSync(envPath)) {
  const contents = fs.readFileSync(envPath, 'utf8')
  contents.split(/\r?\n/).forEach((line) => {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) return
    const idx = trimmed.indexOf('=')
    if (idx === -1) return
    const key = trimmed.slice(0, idx).trim()
    const value = trimmed.slice(idx + 1).trim()
    if (key && process.env[key] === undefined) {
      process.env[key] = value
    }
  })
}

function getBackendUrl() {
  return process.env.BACKEND_URL || 'http://localhost:8081'
}

// Extract JWT from an Express request's Authorization header.
export function getJWTFromRequest(req: { headers: { authorization?: string } }): string | undefined {
  const auth = req.headers.authorization
  if (auth?.startsWith('Bearer ')) return auth.slice(7)
  return undefined
}

export async function proxyToBackend<T = any>(
  path: string,
  options: RequestInit = {},
  userJWT?: string
): Promise<T> {
  const url = `${getBackendUrl()}${path}`;

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  };
  if (userJWT) {
    headers['Authorization'] = `Bearer ${userJWT}`;
  }

  const response = await fetch(url, {
    ...options,
    headers,
  });

  if (!response.ok) {
    const error = await response.text();
    throw new Error(error || `Backend error: ${response.status}`);
  }

  return response.json();
}

/**
 * Proxy to backend using the user's JWT token instead of the static admin token.
 * Used for workspace endpoints where auth is per-user JWT.
 */
export async function proxyWithJWT<T = any>(
  path: string,
  jwtToken: string,
  options: RequestInit = {}
): Promise<T> {
  const url = `${getBackendUrl()}${path}`;

  const response = await fetch(url, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${jwtToken}`,
      ...options.headers,
    },
  });

  if (!response.ok) {
    const error = await response.text();
    throw new Error(error || `Backend error: ${response.status}`);
  }

  return response.json();
}

export { getBackendUrl };
