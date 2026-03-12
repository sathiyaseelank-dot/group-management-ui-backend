import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Shield, ArrowRight, Search, Globe, Mail } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const CONTROLLER_URL = import.meta.env.VITE_CONTROLLER_URL || `${window.location.protocol}//${window.location.hostname}:8081`
const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

export default function LoginPage() {
  const navigate = useNavigate()
  const [mode, setMode] = useState<'slug' | 'email'>('slug')
  const [slug, setSlug] = useState('')
  const [email, setEmail] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [emailResults, setEmailResults] = useState<{ name: string; slug: string }[] | null>(null)

  // If a ?token= param is present (post-OAuth redirect), store it and go to dashboard.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const token = params.get('token')
    if (token) {
      localStorage.setItem('authToken', token)
      navigate('/dashboard/groups', { replace: true })
    }
  }, [navigate])

  const handleSlugSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!slug.trim()) return
    setError('')
    setLoading(true)

    try {
      const res = await fetch(`${API_BASE}/workspaces/lookup?slug=${encodeURIComponent(slug.trim().toLowerCase())}`)
      const data = await res.json()
      if (data.exists) {
        // Redirect to the provider login for this workspace
        window.location.href = `${CONTROLLER_URL}/oauth/google/login?return_to=${encodeURIComponent(window.location.origin)}`
      } else {
        setError('Network not found. Check the URL and try again.')
      }
    } catch {
      setError('Failed to look up network. Please try again.')
    } finally {
      setLoading(false)
    }
  }

  const handleEmailSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!email.trim()) return
    setError('')
    setEmailResults(null)
    setLoading(true)

    try {
      const res = await fetch(`${API_BASE}/workspaces/lookup?email=${encodeURIComponent(email.trim().toLowerCase())}`)
      const data = await res.json()
      if (data.networks && data.networks.length > 0) {
        setEmailResults(data.networks)
      } else {
        setError('No networks found for this email address.')
      }
    } catch {
      setError('Failed to look up email. Please try again.')
    } finally {
      setLoading(false)
    }
  }

  const handleNetworkSelect = (networkSlug: string) => {
    window.location.href = `${CONTROLLER_URL}/oauth/google/login?return_to=${encodeURIComponent(window.location.origin)}`
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-md space-y-8 rounded-xl border bg-card p-8 shadow-sm">
        {/* Logo + Title */}
        <div className="flex flex-col items-center gap-4">
          <div className="flex h-14 w-14 items-center justify-center rounded-xl bg-primary">
            <Shield className="h-8 w-8 text-primary-foreground" />
          </div>
          <div className="text-center">
            <h1 className="text-2xl font-bold">Welcome to ZTNA</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Sign in to an existing Network
            </p>
          </div>
        </div>

        {/* Slug lookup form */}
        {mode === 'slug' && !emailResults && (
          <form onSubmit={handleSlugSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="slug">Network URL</Label>
              <div className="flex items-center gap-0">
                <Input
                  id="slug"
                  value={slug}
                  onChange={e => {
                    setSlug(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))
                    setError('')
                  }}
                  placeholder="your-network"
                  className="rounded-r-none border-r-0"
                  required
                />
                <div className="flex h-9 items-center rounded-r-md border border-l-0 bg-muted px-3 text-sm text-muted-foreground">
                  .zerotrust.com
                </div>
              </div>
            </div>

            {error && <p className="text-sm text-destructive">{error}</p>}

            <Button type="submit" className="w-full gap-2" disabled={loading || !slug.trim()}>
              {loading ? 'Looking up...' : (
                <>
                  Sign In
                  <ArrowRight className="h-4 w-4" />
                </>
              )}
            </Button>
          </form>
        )}

        {/* Email lookup form */}
        {mode === 'email' && !emailResults && (
          <form onSubmit={handleEmailSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="email">Email address</Label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={e => {
                  setEmail(e.target.value)
                  setError('')
                }}
                placeholder="you@company.com"
                required
              />
              <p className="text-xs text-muted-foreground">
                We'll find the networks associated with your email
              </p>
            </div>

            {error && <p className="text-sm text-destructive">{error}</p>}

            <Button type="submit" className="w-full gap-2" disabled={loading || !email.trim()}>
              {loading ? 'Looking up...' : (
                <>
                  <Search className="h-4 w-4" />
                  Look Up Network
                </>
              )}
            </Button>
          </form>
        )}

        {/* Email lookup results */}
        {emailResults && (
          <div className="space-y-4">
            <div>
              <p className="text-sm font-medium">Networks for {email}</p>
              <p className="text-xs text-muted-foreground">Select a network to sign in</p>
            </div>
            <div className="space-y-2">
              {emailResults.map(network => (
                <button
                  key={network.slug}
                  onClick={() => handleNetworkSelect(network.slug)}
                  className="flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-colors hover:bg-muted/50"
                >
                  <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/10">
                    <Globe className="h-4 w-4 text-primary" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium truncate">{network.name}</p>
                    <p className="text-xs text-muted-foreground font-mono">{network.slug}.zerotrust.com</p>
                  </div>
                  <ArrowRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                </button>
              ))}
            </div>
            <Button
              variant="ghost"
              className="w-full text-sm"
              onClick={() => {
                setEmailResults(null)
                setError('')
              }}
            >
              Search again
            </Button>
          </div>
        )}

        {/* Mode toggle */}
        {!emailResults && (
          <div className="relative">
            <div className="absolute inset-0 flex items-center">
              <div className="w-full border-t" />
            </div>
            <div className="relative flex justify-center text-xs uppercase">
              <span className="bg-card px-2 text-muted-foreground">or</span>
            </div>
          </div>
        )}

        {!emailResults && mode === 'slug' && (
          <Button
            variant="outline"
            className="w-full gap-2"
            onClick={() => { setMode('email'); setError('') }}
          >
            <Mail className="h-4 w-4" />
            Look up your Network by email
          </Button>
        )}

        {!emailResults && mode === 'email' && (
          <Button
            variant="outline"
            className="w-full gap-2"
            onClick={() => { setMode('slug'); setError('') }}
          >
            <Globe className="h-4 w-4" />
            Enter your Network URL
          </Button>
        )}

        {/* Create network link */}
        <p className="text-center text-sm text-muted-foreground">
          Don't have a network?{' '}
          <a href="/signup" className="text-primary hover:underline">Create one</a>
        </p>
      </div>
    </div>
  )
}
