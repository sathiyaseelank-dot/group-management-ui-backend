import { Outlet, useNavigate } from 'react-router-dom'
import { LogOut, Shield } from 'lucide-react'
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
      <header className="flex h-14 items-center justify-between border-b bg-card px-4">
        <div className="flex items-center gap-2">
          <Shield className="h-5 w-5 text-primary" />
          <span className="font-semibold">{claims?.wslug || 'Workspace'}</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">{claims?.wrole}</span>
          <Button variant="ghost" size="sm" onClick={handleLogout}>
            <LogOut className="h-4 w-4" />
          </Button>
        </div>
      </header>
      <main className="flex-1">
        <Outlet />
      </main>
    </div>
  )
}
