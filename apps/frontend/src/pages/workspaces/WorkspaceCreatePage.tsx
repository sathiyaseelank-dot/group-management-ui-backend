import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Shield, ArrowLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

function nameToSlug(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 63)
}

export default function WorkspaceCreatePage() {
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [slugEdited, setSlugEdited] = useState(false)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const handleNameChange = (val: string) => {
    setName(val)
    if (!slugEdited) {
      setSlug(nameToSlug(val))
    }
  }

  const handleSlugChange = (val: string) => {
    setSlugEdited(true)
    setSlug(val.toLowerCase().replace(/[^a-z0-9-]/g, ''))
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setSubmitting(true)

    const token = localStorage.getItem('authToken')
    if (!token) {
      navigate('/login', { replace: true })
      return
    }

    try {
      const res = await fetch(`${API_BASE}/workspaces`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ name: name.trim(), slug }),
      })

      if (!res.ok) {
        const text = await res.text()
        setError(text || 'Failed to create workspace')
        setSubmitting(false)
        return
      }

      const data = await res.json()
      if (data.token) {
        localStorage.setItem('authToken', data.token)
      }
      navigate('/dashboard', { replace: true })
    } catch (err) {
      setError((err as Error).message)
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-md space-y-6 rounded-xl border bg-card p-8 shadow-sm">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="icon" onClick={() => navigate('/workspaces')}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary">
            <Shield className="h-6 w-6 text-primary-foreground" />
          </div>
          <div>
            <h1 className="text-xl font-bold">Create Workspace</h1>
            <p className="text-sm text-muted-foreground">Set up a new workspace</p>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="name">Workspace Name</Label>
            <Input
              id="name"
              value={name}
              onChange={(e) => handleNameChange(e.target.value)}
              placeholder="My Company"
              required
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="slug">Slug</Label>
            <Input
              id="slug"
              value={slug}
              onChange={(e) => handleSlugChange(e.target.value)}
              placeholder="my-company"
              required
            />
            <p className="text-xs text-muted-foreground">
              Trust domain: <span className="font-mono">{slug || '...'}.zerotrust.com</span>
            </p>
          </div>

          {error && (
            <p className="text-sm text-destructive">{error}</p>
          )}

          <Button type="submit" className="w-full" disabled={submitting || !name.trim() || !slug}>
            {submitting ? 'Creating...' : 'Create Workspace'}
          </Button>
        </form>
      </div>
    </div>
  )
}
