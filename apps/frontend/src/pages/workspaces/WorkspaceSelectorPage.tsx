import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Shield, Plus, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { Workspace } from '@/lib/types'
import { getWorkspaceClaims } from '@/lib/jwt'

const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

export default function WorkspaceSelectorPage() {
  const navigate = useNavigate()
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const token = localStorage.getItem('authToken')
    if (!token) {
      navigate('/login', { replace: true })
      return
    }

    fetch(`${API_BASE}/workspaces`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((r) => r.json())
      .then((data) => {
        setWorkspaces(Array.isArray(data) ? data : [])
        setLoading(false)
      })
      .catch(() => setLoading(false))
  }, [navigate])

  const selectWorkspace = async (ws: Workspace) => {
    const token = localStorage.getItem('authToken')
    if (!token) return

    try {
      const res = await fetch(`${API_BASE}/workspaces/${ws.id}/select`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
      })
      const data = await res.json()
      if (data.token) {
        localStorage.setItem('authToken', data.token)
        const wsClaims = getWorkspaceClaims(data.token)
        if (wsClaims?.wrole === 'member') {
          navigate('/app', { replace: true })
        } else {
          navigate('/dashboard', { replace: true })
        }
      }
    } catch (err) {
      console.error('Failed to select workspace:', err)
    }
  }

  const roleBadge = (role?: string) => {
    const colors: Record<string, string> = {
      owner: 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200',
      admin: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
      member: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
    }
    return (
      <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${colors[role || 'member'] || colors.member}`}>
        {role || 'member'}
      </span>
    )
  }

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <p className="text-muted-foreground">Loading workspaces...</p>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-md space-y-6 rounded-xl border bg-card p-8 shadow-sm">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary">
            <Shield className="h-6 w-6 text-primary-foreground" />
          </div>
          <div>
            <h1 className="text-xl font-bold">Select Workspace</h1>
            <p className="text-sm text-muted-foreground">Choose a workspace to continue</p>
          </div>
        </div>

        {workspaces.length > 0 ? (
          <div className="space-y-2">
            {workspaces.map((ws) => (
              <button
                key={ws.id}
                onClick={() => selectWorkspace(ws)}
                className="flex w-full items-center justify-between rounded-lg border p-4 text-left transition-colors hover:bg-muted"
              >
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium">{ws.name}</span>
                    {roleBadge(ws.role)}
                  </div>
                  <p className="text-xs text-muted-foreground">{ws.slug}.zerotrust.com</p>
                </div>
                <ChevronRight className="h-4 w-4 text-muted-foreground" />
              </button>
            ))}
          </div>
        ) : (
          <p className="text-center text-sm text-muted-foreground">
            You don't have any workspaces yet. Create one to get started.
          </p>
        )}

        <Button className="w-full gap-2" onClick={() => navigate('/workspaces/create')}>
          <Plus className="h-4 w-4" />
          Create New Workspace
        </Button>
      </div>
    </div>
  )
}
