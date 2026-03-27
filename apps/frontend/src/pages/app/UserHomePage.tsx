import { useNavigate } from 'react-router-dom'
import { Shield, Settings, Download, ArrowRight, Lock, Fingerprint, Globe } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { getWorkspaceClaims } from '@/lib/jwt'

export default function UserHomePage() {
  const navigate = useNavigate()
  const token = localStorage.getItem('authToken')
  const claims = getWorkspaceClaims(token)
  const isAdmin = claims?.wrole === 'admin' || claims?.wrole === 'owner'

  return (
    <div className="relative flex min-h-[calc(100vh-3.5rem)] items-center justify-center bg-background p-4 overflow-hidden">
      {/* Background */}
      <div className="absolute inset-0 bg-grid-subtle opacity-30" />
      <div className="absolute inset-0" style={{
        background: 'radial-gradient(ellipse 50% 40% at 50% 40%, oklch(0.68 0.19 250 / 0.04), transparent 70%)'
      }} />

      <div className="relative z-10 w-full max-w-lg space-y-6">
        <div className="flex flex-col items-center space-y-4 text-center">
          <div className="relative flex h-16 w-16 items-center justify-center rounded-2xl bg-primary/10 ring-1 ring-primary/20">
            <Shield className="h-8 w-8 text-primary" />
            <div className="absolute -top-1 -right-1 flex h-4 w-4 items-center justify-center rounded-full bg-secure">
              <Lock className="h-2.5 w-2.5 text-secure-foreground" />
            </div>
          </div>
          <div>
            <h1 className="font-display text-2xl font-bold uppercase tracking-wide">
              {claims?.wslug || 'your workspace'}
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Authenticated as {claims?.wrole}
            </p>
          </div>
        </div>

        <div className="rounded-xl border border-border/40 bg-card/80 backdrop-blur-sm p-4 space-y-3">
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary/10">
              <Fingerprint className="h-4 w-4 text-primary" />
            </div>
            <div>
              <p className="font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/60">Workspace</p>
              <p className="text-sm font-medium">{claims?.wslug}</p>
            </div>
          </div>
          <div className="h-px bg-border/30" />
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary/10">
              <Globe className="h-4 w-4 text-primary" />
            </div>
            <div>
              <p className="font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/60">Trust Domain</p>
              <p className="text-sm font-mono">{claims?.wslug}.zerotrust.com</p>
            </div>
          </div>
        </div>

        <div className="space-y-3">
          <Button className="w-full gap-2 font-display font-semibold uppercase tracking-wider" onClick={() => navigate('/app/install')}>
            <Download className="h-4 w-4" />
            Set up Client
            <ArrowRight className="h-4 w-4 ml-auto" />
          </Button>

          {isAdmin && (
            <Button variant="outline" className="w-full gap-2" onClick={() => navigate('/dashboard/groups')}>
              <Settings className="h-4 w-4" />
              Admin Dashboard
            </Button>
          )}
        </div>

        <div className="flex items-center justify-center gap-1.5">
          <Lock className="h-3 w-3 text-primary/40" />
          <span className="text-[10px] text-muted-foreground/40 font-mono uppercase tracking-[0.2em]">
            mTLS secured
          </span>
        </div>
      </div>
    </div>
  )
}
