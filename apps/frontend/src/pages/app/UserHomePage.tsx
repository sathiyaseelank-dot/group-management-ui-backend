import { useNavigate } from 'react-router-dom'
import { Shield, Settings, Download, ArrowRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { getWorkspaceClaims } from '@/lib/jwt'

export default function UserHomePage() {
  const navigate = useNavigate()
  const token = localStorage.getItem('authToken')
  const claims = getWorkspaceClaims(token)
  const isAdmin = claims?.wrole === 'admin' || claims?.wrole === 'owner'

  return (
    <div className="flex min-h-[calc(100vh-3.5rem)] items-center justify-center bg-background p-4">
      <div className="w-full max-w-lg space-y-6">
        <div className="flex flex-col items-center space-y-3 text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-full bg-primary/10">
            <Shield className="h-7 w-7 text-primary" />
          </div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Welcome to {claims?.wslug || 'your workspace'}
          </h1>
          <p className="text-sm text-muted-foreground">
            You have access to this workspace as a {claims?.wrole}
          </p>
        </div>

        <div className="rounded-lg border bg-card p-4 space-y-3">
          <div className="flex items-center gap-3">
            <Shield className="h-5 w-5 text-primary shrink-0" />
            <div className="space-y-1">
              <p className="text-sm font-medium">Workspace</p>
              <p className="text-sm text-muted-foreground">{claims?.wslug}</p>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <Settings className="h-5 w-5 text-muted-foreground shrink-0" />
            <div className="space-y-1">
              <p className="text-sm font-medium">Trust Domain</p>
              <p className="text-sm text-muted-foreground font-mono">{claims?.wslug}.zerotrust.com</p>
            </div>
          </div>
        </div>

        <div className="space-y-3">
          <Button className="w-full gap-2" onClick={() => navigate('/app/install')}>
            <Download className="h-4 w-4" />
            Set up Client
            <ArrowRight className="h-4 w-4" />
          </Button>

          {isAdmin && (
            <Button variant="outline" className="w-full gap-2" onClick={() => navigate('/dashboard/groups')}>
              <Settings className="h-4 w-4" />
              Go to Admin Dashboard
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
