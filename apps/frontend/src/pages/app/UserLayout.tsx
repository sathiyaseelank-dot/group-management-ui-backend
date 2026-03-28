import { Outlet, useNavigate } from 'react-router-dom'
import { LogOut, Shield, Lock } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { getWorkspaceClaims } from '@/lib/jwt'

const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

export default function UserLayout() {
  const navigate = useNavigate()
  const token = localStorage.getItem('authToken')
  const claims = getWorkspaceClaims(token)

  const handleLogout = async () => {
    try {
      await fetch(`${API_BASE}/oauth/logout`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
      })
    } catch {
      // ignore
    }
    localStorage.removeItem('authToken')
    navigate('/login', { replace: true })
  }

  return (
    <div className="flex min-h-screen flex-col bg-background">
      <header className="flex h-14 items-center justify-between border-b border-border/40 bg-card/80 backdrop-blur-md px-5">
        <div className="flex items-center gap-3">
          <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary/10 ring-1 ring-primary/20">
            <Shield className="h-4 w-4 text-primary" />
          </div>
          <span className="font-display text-sm font-bold uppercase tracking-wide">{claims?.wslug || 'Workspace'}</span>
          <div className="flex items-center gap-1 rounded-md bg-muted/60 px-2 py-0.5 ring-1 ring-border/30">
            <Lock className="h-2.5 w-2.5 text-secure" />
            <span className="text-[10px] font-mono text-muted-foreground uppercase tracking-wider">{claims?.wrole}</span>
          </div>
        </div>
        <Button variant="ghost" size="sm" className="gap-2 text-muted-foreground text-xs" onClick={handleLogout}>
          <LogOut className="h-3.5 w-3.5" />
          Sign out
        </Button>
      </header>
      <main className="flex-1">
        <Outlet />
      </main>
    </div>
  )
}
