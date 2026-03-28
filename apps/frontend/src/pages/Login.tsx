import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { Shield, ArrowRight, Search, Globe, Mail, Lock, Fingerprint, Terminal, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getWorkspaceClaims } from '@/lib/jwt'
import { Particles } from '@/components/ui/particles'
import { ShimmerButton } from '@/components/ui/shimmer-button'
import { BorderBeam } from '@/components/ui/border-beam'

const CONTROLLER_URL = import.meta.env.VITE_CONTROLLER_URL || `${window.location.protocol}//${window.location.hostname}:8081`
const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

function buildWorkspaceLoginUrl(workspaceSlug: string) {
  const params = new URLSearchParams({
    return_to: window.location.origin,
    workspace_slug: workspaceSlug,
  })
  return `${CONTROLLER_URL}/oauth/google/login?${params.toString()}`
}

/* ── Animated data rain columns ── */
function DataRain() {
  const chars = '01アイウエオカキクケコ10'
  const columns = Array.from({ length: 18 }, (_, i) => ({
    left: `${(i / 18) * 100 + Math.random() * 3}%`,
    delay: `${Math.random() * 8}s`,
    duration: `${6 + Math.random() * 10}s`,
    opacity: 0.04 + Math.random() * 0.06,
    char: Array.from({ length: 12 }, () => chars[Math.floor(Math.random() * chars.length)]).join('\n'),
  }))

  return (
    <div className="absolute inset-0 overflow-hidden pointer-events-none select-none" aria-hidden>
      {columns.map((col, i) => (
        <div
          key={i}
          className="absolute top-0 font-mono text-[10px] leading-[14px] text-primary whitespace-pre"
          style={{
            left: col.left,
            opacity: col.opacity,
            animation: `data-fall ${col.duration} ${col.delay} linear infinite`,
          }}
        >
          {col.char}
        </div>
      ))}
    </div>
  )
}

/* ── Hex grid decorative element ── */
function HexGrid() {
  return (
    <svg className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[700px] h-[700px] opacity-[0.025]" viewBox="0 0 400 400" aria-hidden>
      {Array.from({ length: 6 }).map((_, ring) =>
        Array.from({ length: Math.max(6, ring * 6) }).map((_, i) => {
          const angle = (i / Math.max(6, ring * 6)) * Math.PI * 2
          const r = 30 + ring * 50
          const cx = 200 + Math.cos(angle) * r
          const cy = 200 + Math.sin(angle) * r
          return (
            <polygon
              key={`${ring}-${i}`}
              points={hexPoints(cx, cy, 16)}
              fill="none"
              stroke="currentColor"
              strokeWidth="0.5"
              className="text-primary"
            />
          )
        })
      )}
    </svg>
  )
}

function hexPoints(cx: number, cy: number, r: number) {
  return Array.from({ length: 6 }, (_, i) => {
    const a = (Math.PI / 3) * i - Math.PI / 6
    return `${cx + r * Math.cos(a)},${cy + r * Math.sin(a)}`
  }).join(' ')
}

/* ── Status bar (top of card) ── */
function StatusBar() {
  const [time, setTime] = useState(new Date())
  useEffect(() => {
    const t = setInterval(() => setTime(new Date()), 1000)
    return () => clearInterval(t)
  }, [])

  return (
    <div className="flex items-center justify-between px-5 py-2.5 border-b border-primary/10 bg-primary/[0.03]">
      <div className="flex items-center gap-2">
        <div className="h-2 w-2 rounded-full bg-primary pulse-secure" />
        <span className="font-mono text-[10px] uppercase tracking-widest text-primary/60">
          System Secure
        </span>
      </div>
      <span className="font-mono text-[10px] tabular-nums text-muted-foreground">
        {time.toLocaleTimeString('en-US', { hour12: false })}
      </span>
    </div>
  )
}

/* ── Main Login ── */
export default function LoginPage() {
  const navigate = useNavigate()
  const [mode, setMode] = useState<'slug' | 'email'>('slug')
  const [slug, setSlug] = useState('')
  const [email, setEmail] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [emailResults, setEmailResults] = useState<{ name: string; slug: string }[] | null>(null)
  const [mounted, setMounted] = useState(false)

  useEffect(() => { setMounted(true) }, [])

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const token = params.get('token')
    if (token) {
      localStorage.setItem('authToken', token)
      const claims = getWorkspaceClaims(token)
      if (!claims) {
        navigate('/workspaces', { replace: true })
      } else if (claims.wrole === 'member') {
        navigate('/app', { replace: true })
      } else {
        navigate('/dashboard/groups', { replace: true })
      }
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
        window.location.href = buildWorkspaceLoginUrl(slug.trim().toLowerCase())
      } else {
        setError('ERR::NETWORK_NOT_FOUND — verify URL and retry')
      }
    } catch {
      setError('ERR::LOOKUP_FAILED — connection refused')
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
        setError('ERR::NO_NETWORKS — no associations found')
      }
    } catch {
      setError('ERR::LOOKUP_FAILED — connection refused')
    } finally {
      setLoading(false)
    }
  }

  const handleNetworkSelect = (networkSlug: string) => {
    window.location.href = buildWorkspaceLoginUrl(networkSlug)
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center bg-background overflow-hidden">
      {/* Layer 1: Particles */}
      <Particles quantity={60} color="#5b8def" connectDistance={100} />

      {/* Layer 2: Hex grid */}
      <HexGrid />

      {/* Layer 3: Grid lines */}
      <div className="absolute inset-0 bg-grid-subtle" />

      {/* Layer 4: Radial glow */}
      <div className="absolute inset-0" style={{
        background: 'radial-gradient(ellipse 500px 350px at 50% 45%, oklch(0.68 0.19 250 / 0.08), transparent 70%)'
      }} />

      {/* Layer 5: Vignette */}
      <div className="absolute inset-0" style={{
        background: 'radial-gradient(ellipse 80% 70% at 50% 50%, transparent 50%, oklch(0.37 0 0) 100%)'
      }} />

      {/* ── Card ── */}
      <div
        className={`relative z-10 w-full max-w-[420px] mx-4 overflow-hidden rounded-lg border border-primary/15 bg-card/80 shadow-2xl backdrop-blur-md transition-all duration-700 ${mounted ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-4'}`}
      >
        {/* Scanline overlay */}
        <div className="absolute inset-0 scanline-subtle pointer-events-none" />

        {/* Animated border beam */}
        <BorderBeam size={250} duration={10} />

        {/* Top glow line */}
        <div className="absolute top-0 left-0 right-0 h-px bg-gradient-to-r from-transparent via-primary/50 to-transparent" />

        {/* Status bar */}
        <StatusBar />

        <div className="p-7 space-y-6">
          {/* Logo + Title */}
          <div
            className="flex flex-col items-center gap-4 animate-fade-up"
            style={{ animationDelay: '0.1s' }}
          >
            {/* Shield icon — angular container */}
            <div className="relative">
              <div className="flex h-[68px] w-[68px] items-center justify-center bg-primary/8 border border-primary/20 rounded-xl rotate-45">
                <Shield className="h-7 w-7 text-primary -rotate-45" />
              </div>
              {/* Live indicator */}
              <div className="absolute -top-0.5 -right-0.5 h-3 w-3 rounded-full bg-primary glow-primary pulse-secure" />
              {/* Corner accents */}
              <div className="absolute -bottom-1 -left-1 w-3 h-3 border-b border-l border-primary/30 rounded-bl-sm" />
              <div className="absolute -top-1 -right-1 w-3 h-3 border-t border-r border-primary/30 rounded-tr-sm" />
            </div>

            <div className="text-center space-y-1">
              <h1 className="font-display text-2xl font-bold tracking-wide uppercase text-foreground">
                ZTNA Gateway
              </h1>
              <p className="text-[13px] text-muted-foreground">
                Zero Trust Network Access Control
              </p>
            </div>
          </div>

          {/* ── Slug form ── */}
          {mode === 'slug' && !emailResults && (
            <form
              onSubmit={handleSlugSubmit}
              className="space-y-4 animate-fade-up"
              style={{ animationDelay: '0.2s' }}
            >
              <div className="space-y-2">
                <Label htmlFor="slug" className="font-mono text-[10px] uppercase tracking-[0.2em] text-primary/60 flex items-center gap-1.5">
                  <ChevronRight className="h-3 w-3" />
                  Network Endpoint
                </Label>
                <div className="flex items-center">
                  <Input
                    id="slug"
                    value={slug}
                    onChange={e => {
                      setSlug(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))
                      setError('')
                    }}
                    placeholder="your-network"
                    className="rounded-r-none border-r-0 font-mono text-sm bg-background/50 placeholder:text-muted-foreground/30"
                    required
                  />
                  <div className="flex h-9 items-center rounded-r-md border border-l-0 bg-primary/5 px-3 text-[11px] text-primary/50 font-mono">
                    .ztna.io
                  </div>
                </div>
              </div>

              {error && (
                <div className="flex items-center gap-2 rounded-md border border-critical/20 bg-critical/5 px-3 py-2 font-mono text-[11px] text-critical">
                  <Terminal className="h-3.5 w-3.5 shrink-0" />
                  {error}
                </div>
              )}

              <ShimmerButton
                type="submit"
                className="w-full h-11 font-display text-sm font-semibold uppercase tracking-wider px-4"
                disabled={loading || !slug.trim()}
              >
                {loading ? (
                  <span className="font-mono text-xs tracking-widest caret-blink">AUTHENTICATING</span>
                ) : (
                  <>
                    <Fingerprint className="h-4 w-4" />
                    Authenticate
                  </>
                )}
              </ShimmerButton>
            </form>
          )}

          {/* ── Email form ── */}
          {mode === 'email' && !emailResults && (
            <form
              onSubmit={handleEmailSubmit}
              className="space-y-4 animate-fade-up"
              style={{ animationDelay: '0.2s' }}
            >
              <div className="space-y-2">
                <Label htmlFor="email" className="font-mono text-[10px] uppercase tracking-[0.2em] text-primary/60 flex items-center gap-1.5">
                  <ChevronRight className="h-3 w-3" />
                  Identity Lookup
                </Label>
                <Input
                  id="email"
                  type="email"
                  value={email}
                  onChange={e => {
                    setEmail(e.target.value)
                    setError('')
                  }}
                  placeholder="you@company.com"
                  className="font-mono text-sm bg-background/50 placeholder:text-muted-foreground/30"
                  required
                />
                <p className="font-mono text-[10px] text-muted-foreground/40">
                  {'>'} resolve networks linked to this identity
                </p>
              </div>

              {error && (
                <div className="flex items-center gap-2 rounded-md border border-critical/20 bg-critical/5 px-3 py-2 font-mono text-[11px] text-critical">
                  <Terminal className="h-3.5 w-3.5 shrink-0" />
                  {error}
                </div>
              )}

              <ShimmerButton
                type="submit"
                className="w-full h-11 font-display text-sm font-semibold uppercase tracking-wider px-4"
                disabled={loading || !email.trim()}
              >
                {loading ? (
                  <span className="font-mono text-xs tracking-widest caret-blink">RESOLVING</span>
                ) : (
                  <>
                    <Search className="h-4 w-4" />
                    Resolve Network
                  </>
                )}
              </ShimmerButton>
            </form>
          )}

          {/* ── Email results ── */}
          {emailResults && (
            <div className="space-y-4 animate-fade-up">
              <div>
                <p className="text-sm font-medium">Networks for <span className="font-mono text-primary">{email}</span></p>
                <p className="font-mono text-[10px] text-muted-foreground/40">{'>'} select target endpoint</p>
              </div>
              <div className="space-y-2">
                {emailResults.map((network, i) => (
                  <button
                    key={network.slug}
                    onClick={() => handleNetworkSelect(network.slug)}
                    className="flex w-full items-center gap-3 rounded-md border border-border/50 p-3 text-left transition-all duration-200 hover:bg-primary/5 hover:border-primary/30 cursor-pointer group animate-fade-up"
                    style={{ animationDelay: `${0.05 * i}s` }}
                  >
                    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/8 ring-1 ring-primary/15 transition-shadow group-hover:glow-primary">
                      <Globe className="h-4 w-4 text-primary" />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium truncate">{network.name}</p>
                      <p className="font-mono text-[10px] text-muted-foreground">{network.slug}.ztna.io</p>
                    </div>
                    <ArrowRight className="h-4 w-4 shrink-0 text-muted-foreground/40 group-hover:text-primary transition-colors" />
                  </button>
                ))}
              </div>
              <Button
                variant="ghost"
                className="w-full text-xs font-mono uppercase tracking-wider text-muted-foreground"
                onClick={() => { setEmailResults(null); setError('') }}
              >
                {'<'} back
              </Button>
            </div>
          )}

          {/* ── Divider ── */}
          {!emailResults && (
            <div
              className="relative animate-fade-up"
              style={{ animationDelay: '0.3s' }}
            >
              <div className="absolute inset-0 flex items-center">
                <div className="w-full border-t border-primary/8" />
              </div>
              <div className="relative flex justify-center">
                <span className="bg-card px-4 font-mono text-[9px] uppercase tracking-[0.3em] text-muted-foreground/30">
                  alt
                </span>
              </div>
            </div>
          )}

          {!emailResults && mode === 'slug' && (
            <Button
              variant="outline"
              className="w-full gap-2 text-[13px] border-border/40 hover:border-primary/30 hover:bg-primary/5 animate-fade-up"
              style={{ animationDelay: '0.35s' }}
              onClick={() => { setMode('email'); setError('') }}
            >
              <Mail className="h-4 w-4 text-muted-foreground" />
              Identify by email
            </Button>
          )}

          {!emailResults && mode === 'email' && (
            <Button
              variant="outline"
              className="w-full gap-2 text-[13px] border-border/40 hover:border-primary/30 hover:bg-primary/5 animate-fade-up"
              style={{ animationDelay: '0.35s' }}
              onClick={() => { setMode('slug'); setError('') }}
            >
              <Globe className="h-4 w-4 text-muted-foreground" />
              Enter endpoint URL
            </Button>
          )}

          {/* Create link */}
          <p
            className="text-center text-[12px] text-muted-foreground animate-fade-up"
            style={{ animationDelay: '0.4s' }}
          >
            No network?{' '}
            <a href="/signup" className="text-primary hover:underline font-medium">Deploy one</a>
          </p>
        </div>

        {/* Bottom bar */}
        <div className="flex items-center justify-center gap-2 border-t border-primary/8 bg-primary/[0.02] px-5 py-2.5">
          <Lock className="h-3 w-3 text-primary/40" />
          <span className="font-mono text-[9px] uppercase tracking-[0.25em] text-primary/30">
            mTLS Verified &middot; E2E Encrypted &middot; SPIFFE Identity
          </span>
        </div>
      </div>
    </div>
  )
}
