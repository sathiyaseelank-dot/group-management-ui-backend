import { useNavigate } from 'react-router-dom'
import { Shield, Monitor, CheckCircle, ArrowLeft, Settings } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { getWorkspaceClaims } from '@/lib/jwt'

export default function InstallPage() {
  const navigate = useNavigate()
  const token = localStorage.getItem('authToken')
  const claims = getWorkspaceClaims(token)
  const isAdmin = claims?.wrole === 'admin' || claims?.wrole === 'owner'

  if (!token || !claims) {
    return (
      <div className="flex min-h-[calc(100vh-3.5rem)] items-center justify-center bg-background p-4">
        <div className="w-full max-w-md space-y-4 rounded-xl border bg-card p-8 shadow-sm text-center">
          <p className="text-sm text-muted-foreground">Invalid or expired session.</p>
          <Button variant="outline" onClick={() => navigate('/login', { replace: true })}>
            Go to Login
          </Button>
        </div>
      </div>
    )
  }

  const platform = detectPlatform()

  return (
    <div className="flex min-h-[calc(100vh-3.5rem)] items-center justify-center bg-background p-4">
      <div className="w-full max-w-lg space-y-6 rounded-xl border bg-card p-8 shadow-sm">
        <div className="flex flex-col items-center space-y-3">
          <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Monitor className="h-6 w-6 text-primary" />
          </div>
          <h1 className="text-2xl font-semibold tracking-tight">Install ZTNA Client</h1>
        </div>

        <div className="rounded-lg border bg-muted/50 p-4 space-y-3">
          <div className="flex items-start gap-3">
            <Shield className="h-5 w-5 text-primary mt-0.5 shrink-0" />
            <div className="space-y-1">
              <p className="text-sm font-medium">Platform: {platform}</p>
              <p className="text-sm text-muted-foreground">
                The ZTNA client for {platform} is coming soon. You'll be able to securely access
                your workspace resources directly from your terminal.
              </p>
            </div>
          </div>
        </div>

        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
            Workspace Info
          </p>
          <div className="space-y-1">
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">Workspace</span>
              <span className="font-medium">{claims.wslug}</span>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">Trust Domain</span>
              <span className="font-mono text-xs">{claims.wslug}.zerotrust.com</span>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">Role</span>
              <span className="capitalize">{claims.wrole}</span>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-2 rounded-lg border border-green-200 bg-green-50 p-3 dark:border-green-900 dark:bg-green-950">
          <CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400 shrink-0" />
          <p className="text-sm text-green-700 dark:text-green-300">
            Your account is ready. You'll receive a notification when the client is available.
          </p>
        </div>

        <div className="flex gap-3">
          <Button variant="outline" className="flex-1 gap-2" onClick={() => navigate('/app')}>
            <ArrowLeft className="h-4 w-4" />
            Back to Home
          </Button>
          {isAdmin && (
            <Button className="flex-1 gap-2" onClick={() => navigate('/dashboard/groups', { replace: true })}>
              <Settings className="h-4 w-4" />
              Go to Dashboard
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}

function detectPlatform(): string {
  const ua = navigator.userAgent.toLowerCase()
  if (ua.includes('linux')) return 'Linux'
  if (ua.includes('mac')) return 'macOS'
  if (ua.includes('win')) return 'Windows'
  return 'Unknown'
}
