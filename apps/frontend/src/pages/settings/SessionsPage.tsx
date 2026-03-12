import { useState, useEffect } from 'react'
import { Shield, Laptop, XCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'

const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

interface Session {
  id: string
  user_id: string
  workspace_id: string
  session_type: string
  device_id: string
  ip_address: string
  user_agent: string
  created_at: number
  expires_at: number
  revoked: boolean
}

export default function SessionsPage() {
  const [sessions, setSessions] = useState<Session[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const token = localStorage.getItem('authToken')
  const headers = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${token}`,
  }

  const load = async () => {
    setLoading(true)
    try {
      const res = await fetch(`${API_BASE}/admin/sessions`, { headers })
      if (!res.ok) throw new Error(await res.text())
      setSessions(await res.json())
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const handleRevoke = async (id: string) => {
    if (!confirm('Revoke this session?')) return
    try {
      const res = await fetch(`${API_BASE}/admin/sessions/${id}`, { method: 'DELETE', headers })
      if (!res.ok) throw new Error(await res.text())
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to revoke')
    }
  }

  const handleRevokeAll = async (userId: string) => {
    if (!confirm('Revoke all sessions for this user?')) return
    try {
      const res = await fetch(`${API_BASE}/admin/sessions/user/${userId}`, { method: 'DELETE', headers })
      if (!res.ok) throw new Error(await res.text())
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to revoke')
    }
  }

  const active = sessions.filter(s => !s.revoked && s.expires_at > Date.now() / 1000)
  const revoked = sessions.filter(s => s.revoked || s.expires_at <= Date.now() / 1000)

  return (
    <div className="p-6 max-w-3xl">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold">Sessions</h1>
        <p className="text-sm text-muted-foreground mt-1">Active sessions in this workspace</p>
      </div>

      {error && <p className="text-sm text-destructive mb-4">{error}</p>}

      {loading ? (
        <p className="text-sm text-muted-foreground">Loading...</p>
      ) : (
        <>
          <div className="mb-4">
            <h2 className="text-sm font-medium mb-3">Active ({active.length})</h2>
            {active.length === 0 ? (
              <p className="text-sm text-muted-foreground">No active sessions</p>
            ) : (
              <div className="space-y-2">
                {active.map(sess => (
                  <div key={sess.id} className="rounded-lg border bg-card p-3 flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      {sess.session_type === 'device' ? (
                        <Laptop className="h-4 w-4 text-primary" />
                      ) : (
                        <Shield className="h-4 w-4 text-primary" />
                      )}
                      <div>
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-medium capitalize">{sess.session_type} session</span>
                          <span className="text-xs text-muted-foreground">{sess.ip_address}</span>
                        </div>
                        <p className="text-xs text-muted-foreground truncate max-w-xs">{sess.user_agent}</p>
                      </div>
                    </div>
                    <div className="flex gap-2">
                      {sess.user_id && (
                        <Button variant="ghost" size="sm" className="text-xs gap-1" onClick={() => handleRevokeAll(sess.user_id)}>
                          Revoke All
                        </Button>
                      )}
                      <Button variant="ghost" size="sm" onClick={() => handleRevoke(sess.id)}>
                        <XCircle className="h-4 w-4 text-destructive" />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {revoked.length > 0 && (
            <div>
              <h2 className="text-sm font-medium mb-3 text-muted-foreground">Expired/Revoked ({revoked.length})</h2>
              <div className="space-y-2">
                {revoked.slice(0, 5).map(sess => (
                  <div key={sess.id} className="rounded-lg border bg-muted/30 p-3 flex items-center gap-3 opacity-60">
                    <Shield className="h-4 w-4 text-muted-foreground" />
                    <span className="text-sm capitalize">{sess.session_type} — {sess.revoked ? 'revoked' : 'expired'}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}
