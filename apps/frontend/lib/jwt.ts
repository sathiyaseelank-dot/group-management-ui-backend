/**
 * Decode JWT payload (base64, no crypto verification - that's server-side).
 */
export function decodeJWTPayload(token: string): Record<string, unknown> | null {
  try {
    const parts = token.split('.')
    if (parts.length !== 3) return null
    const payload = parts[1]
    const decoded = atob(payload.replace(/-/g, '+').replace(/_/g, '/'))
    return JSON.parse(decoded)
  } catch {
    return null
  }
}

export function getWorkspaceClaims(token: string | null) {
  if (!token) return null
  const payload = decodeJWTPayload(token)
  if (!payload) return null
  const wid = payload.wid as string | undefined
  const wslug = payload.wslug as string | undefined
  const wrole = payload.wrole as string | undefined
  if (!wid) return null
  return { wid, wslug: wslug ?? '', wrole: wrole ?? 'member' }
}
