import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ChevronDown, Plus, Building2 } from 'lucide-react'
import { getWorkspaceClaims } from '@/lib/jwt'
import type { Workspace } from '@/lib/types'

const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

export function WorkspaceSwitcher() {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [currentName, setCurrentName] = useState('')

  const claims = getWorkspaceClaims(localStorage.getItem('authToken'))
  const currentWsId = claims?.wid || ''
  const currentSlug = claims?.wslug || ''

  useEffect(() => {
    if (!currentWsId) return

    const token = localStorage.getItem('authToken')
    if (!token) return

    fetch(`${API_BASE}/workspaces`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((r) => r.json())
      .then((data) => {
        const list = Array.isArray(data) ? data : []
        setWorkspaces(list)
        const current = list.find((ws: Workspace) => ws.id === currentWsId)
        if (current) setCurrentName(current.name)
      })
      .catch(() => {})
  }, [currentWsId])

  const selectWorkspace = async (ws: Workspace) => {
    if (ws.id === currentWsId) {
      setOpen(false)
      return
    }
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
        setOpen(false)
        window.location.reload()
      }
    } catch {}
  }

  if (!currentWsId) return null

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 rounded-md border bg-background px-3 py-1.5 text-sm transition-colors hover:bg-muted"
      >
        <Building2 className="h-4 w-4 text-muted-foreground" />
        <span className="max-w-[150px] truncate font-medium">
          {currentName || currentSlug}
        </span>
        <ChevronDown className="h-3 w-3 text-muted-foreground" />
      </button>

      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="absolute right-0 top-full z-50 mt-1 w-56 rounded-md border bg-popover p-1 shadow-md">
            {workspaces.map((ws) => (
              <button
                key={ws.id}
                onClick={() => selectWorkspace(ws)}
                className={`flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm transition-colors hover:bg-accent ${
                  ws.id === currentWsId ? 'bg-accent' : ''
                }`}
              >
                <span className="flex-1 truncate text-left">{ws.name}</span>
                <span className="text-xs text-muted-foreground">{ws.role}</span>
              </button>
            ))}
            <div className="my-1 border-t" />
            <button
              onClick={() => {
                setOpen(false)
                navigate('/workspaces/create')
              }}
              className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm transition-colors hover:bg-accent"
            >
              <Plus className="h-3 w-3" />
              Create New
            </button>
          </div>
        </>
      )}
    </div>
  )
}
