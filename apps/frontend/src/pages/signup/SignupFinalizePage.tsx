import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { AlertTriangle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useSignup } from '@/src/contexts/SignupContext'

export default function SignupFinalizePage() {
  const navigate = useNavigate()
  const { state } = useSignup()

  useEffect(() => {
    if (!state.email || !state.networkName) navigate('/signup', { replace: true })
  }, [state.email, state.networkName, navigate])

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-bold">Confirm your Network</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Review your organization's network details before creating
        </p>
      </div>

      <div className="rounded-lg border bg-muted/30 p-4 space-y-3">
        <div>
          <p className="text-xs text-muted-foreground">Network Name</p>
          <p className="text-lg font-bold">{state.networkName}</p>
        </div>
        <div>
          <p className="text-xs text-muted-foreground">Network URL</p>
          <span className="inline-block rounded bg-muted px-2 py-1 font-mono text-sm">
            {state.networkSlug}.zerotrust.com
          </span>
        </div>
      </div>

      <div className="flex items-start gap-2 rounded-lg border border-yellow-200 bg-yellow-50 p-3 dark:border-yellow-900 dark:bg-yellow-950">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-yellow-600 dark:text-yellow-400" />
        <p className="text-xs text-yellow-800 dark:text-yellow-200">
          Your organization's network name and URL cannot be changed after creation.
        </p>
      </div>

      <div className="flex gap-3">
        <Button variant="outline" onClick={() => navigate(-1)} className="flex-1">
          Back
        </Button>
        <Button onClick={() => navigate('/signup/auth')} className="flex-1">
          Go to your Network
        </Button>
      </div>
    </div>
  )
}
