import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'

export default function LoginPage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const inviteToken = params.get('invite_token') || ''
  const oauthError = params.get('error') || ''
  const [checking, setChecking] = useState(true)

  useEffect(() => {
    let mounted = true
    ;(async () => {
      try {
        const res = await fetch('/api/auth/session', { credentials: 'include' })
        if (mounted && res.ok) {
          navigate('/dashboard/groups', { replace: true })
          return
        }
      } catch {
        // no-op
      } finally {
        if (mounted) setChecking(false)
      }
    })()

    return () => {
      mounted = false
    }
  }, [navigate])

  const onLogin = () => {
    const url = new URL('/api/auth/login', window.location.origin)
    if (inviteToken) {
      url.searchParams.set('invite_token', inviteToken)
    }
    window.location.href = url.toString()
  }

  if (checking) {
    return null
  }

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100">
      <div className="mx-auto flex min-h-screen w-full max-w-md flex-col justify-center px-6">
        <h1 className="text-3xl font-semibold">ZTNA Login</h1>
        <p className="mt-2 text-sm text-slate-400">Continue with Google to access the dashboard.</p>
        {oauthError ? (
          <p className="mt-4 rounded border border-rose-600/40 bg-rose-950/40 px-3 py-2 text-sm text-rose-200">
            Login failed. Please try again.
          </p>
        ) : null}

        <button
          type="button"
          onClick={onLogin}
          className="mt-8 w-full rounded-md bg-cyan-600 px-4 py-2 text-sm font-medium text-white hover:bg-cyan-500"
        >
          Login with Google
        </button>
      </div>
    </div>
  )
}
