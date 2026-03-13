import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Shield, ArrowRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { getWorkspaceClaims, decodeJWTPayload } from '@/lib/jwt'

export default function WelcomePage() {
  const navigate = useNavigate()
  const [error, setError] = useState('')

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const token = params.get('token')
    if (token) {
      localStorage.setItem('authToken', token)
      const url = new URL(window.location.href)
      url.searchParams.delete('token')
      window.history.replaceState({}, '', url.toString())
    }
  }, [])

  const token = localStorage.getItem('authToken')
  const claims = getWorkspaceClaims(token)
  const payload = token ? decodeJWTPayload(token) : null
  const email = payload?.sub as string | undefined

  if (!token || !claims) {
    return (
      <div className="flex min-h-[calc(100vh-3.5rem)] items-center justify-center bg-background p-4">
        <div className="w-full max-w-md space-y-4 rounded-xl border bg-card p-8 shadow-sm text-center">
          <p className="text-sm text-muted-foreground">Invalid or expired invite session.</p>
          <Button variant="outline" onClick={() => navigate('/login', { replace: true })}>
            Go to Login
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="flex min-h-[calc(100vh-3.5rem)] items-center justify-center bg-background p-4">
      <div className="w-full max-w-lg space-y-6 rounded-xl border bg-card p-8 shadow-sm">
        <div className="flex flex-col items-center space-y-3">
          <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Shield className="h-6 w-6 text-primary" />
          </div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Welcome to {claims.wslug || 'your workspace'}!
          </h1>
          {email && (
            <p className="text-sm text-muted-foreground">
              Signed in as <span className="font-medium text-foreground">{email}</span>
            </p>
          )}
        </div>

        <div className="space-y-3 text-center">
          <p className="text-sm text-muted-foreground">
            Your account has been created. Set up the ZTNA client to securely access your network resources.
          </p>
        </div>

        <Button className="w-full gap-2" onClick={() => navigate('/app/install')}>
          Continue to Client Setup
          <ArrowRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}
