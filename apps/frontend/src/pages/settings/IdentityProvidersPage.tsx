import { useState, useEffect } from 'react'
import { Plus, Trash2, Edit2, Check, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

interface IdentityProvider {
  id: string
  workspace_id: string
  provider_type: string
  client_id: string
  redirect_uri: string
  issuer_url: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export default function IdentityProvidersPage() {
  const [providers, setProviders] = useState<IdentityProvider[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState({
    provider_type: 'google',
    client_id: '',
    client_secret: '',
    redirect_uri: '',
    issuer_url: '',
    enabled: true,
  })

  const token = localStorage.getItem('authToken')
  const headers = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${token}`,
  }

  const load = async () => {
    setLoading(true)
    try {
      const res = await fetch(`${API_BASE}/admin/identity-providers`, { headers })
      if (!res.ok) throw new Error(await res.text())
      setProviders(await res.json())
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      const method = editingId ? 'PUT' : 'POST'
      const url = editingId
        ? `${API_BASE}/admin/identity-providers/${editingId}`
        : `${API_BASE}/admin/identity-providers`
      const res = await fetch(url, { method, headers, body: JSON.stringify(form) })
      if (!res.ok) throw new Error(await res.text())
      setShowForm(false)
      setEditingId(null)
      setForm({ provider_type: 'google', client_id: '', client_secret: '', redirect_uri: '', issuer_url: '', enabled: true })
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to save')
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this identity provider?')) return
    try {
      const res = await fetch(`${API_BASE}/admin/identity-providers/${id}`, { method: 'DELETE', headers })
      if (!res.ok) throw new Error(await res.text())
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to delete')
    }
  }

  const startEdit = (idp: IdentityProvider) => {
    setEditingId(idp.id)
    setForm({
      provider_type: idp.provider_type,
      client_id: idp.client_id,
      client_secret: '',
      redirect_uri: idp.redirect_uri,
      issuer_url: idp.issuer_url,
      enabled: idp.enabled,
    })
    setShowForm(true)
  }

  return (
    <div className="p-6 max-w-3xl">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-semibold">Identity Providers</h1>
          <p className="text-sm text-muted-foreground mt-1">Configure OAuth providers for this workspace</p>
        </div>
        <Button onClick={() => { setShowForm(true); setEditingId(null) }} className="gap-2">
          <Plus className="h-4 w-4" /> Add Provider
        </Button>
      </div>

      {error && <p className="text-sm text-destructive mb-4">{error}</p>}

      {showForm && (
        <form onSubmit={handleSubmit} className="rounded-lg border bg-card p-4 space-y-4 mb-6">
          <h2 className="font-medium">{editingId ? 'Edit' : 'Add'} Identity Provider</h2>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>Provider Type</Label>
              <select
                value={form.provider_type}
                onChange={e => setForm(f => ({ ...f, provider_type: e.target.value }))}
                className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm"
              >
                <option value="google">Google</option>
                <option value="github">GitHub</option>
                <option value="oidc">OIDC</option>
              </select>
            </div>
            <div className="space-y-2">
              <Label>Client ID</Label>
              <Input value={form.client_id} onChange={e => setForm(f => ({ ...f, client_id: e.target.value }))} required />
            </div>
            <div className="space-y-2">
              <Label>Client Secret {editingId && <span className="text-muted-foreground">(leave blank to keep current)</span>}</Label>
              <Input type="password" value={form.client_secret} onChange={e => setForm(f => ({ ...f, client_secret: e.target.value }))} required={!editingId} />
            </div>
            <div className="space-y-2">
              <Label>Redirect URI</Label>
              <Input value={form.redirect_uri} onChange={e => setForm(f => ({ ...f, redirect_uri: e.target.value }))} placeholder="https://..." />
            </div>
            {form.provider_type === 'oidc' && (
              <div className="space-y-2 col-span-2">
                <Label>Issuer URL</Label>
                <Input value={form.issuer_url} onChange={e => setForm(f => ({ ...f, issuer_url: e.target.value }))} placeholder="https://accounts.example.com" />
              </div>
            )}
          </div>
          <div className="flex items-center gap-2">
            <input type="checkbox" id="enabled" checked={form.enabled} onChange={e => setForm(f => ({ ...f, enabled: e.target.checked }))} />
            <Label htmlFor="enabled">Enabled</Label>
          </div>
          <div className="flex gap-2">
            <Button type="submit" size="sm" className="gap-1"><Check className="h-3 w-3" /> Save</Button>
            <Button type="button" variant="outline" size="sm" className="gap-1" onClick={() => { setShowForm(false); setEditingId(null) }}><X className="h-3 w-3" /> Cancel</Button>
          </div>
        </form>
      )}

      {loading ? (
        <p className="text-sm text-muted-foreground">Loading...</p>
      ) : providers.length === 0 ? (
        <div className="rounded-lg border border-dashed p-8 text-center">
          <p className="text-sm text-muted-foreground">No identity providers configured</p>
        </div>
      ) : (
        <div className="space-y-3">
          {providers.map(idp => (
            <div key={idp.id} className="rounded-lg border bg-card p-4 flex items-center justify-between">
              <div>
                <div className="flex items-center gap-2">
                  <span className="font-medium capitalize">{idp.provider_type}</span>
                  <span className={`text-xs px-1.5 py-0.5 rounded-full ${idp.enabled ? 'bg-green-100 text-green-700' : 'bg-muted text-muted-foreground'}`}>
                    {idp.enabled ? 'enabled' : 'disabled'}
                  </span>
                </div>
                <p className="text-sm text-muted-foreground font-mono">{idp.client_id}</p>
              </div>
              <div className="flex gap-2">
                <Button variant="ghost" size="sm" onClick={() => startEdit(idp)}><Edit2 className="h-4 w-4" /></Button>
                <Button variant="ghost" size="sm" onClick={() => handleDelete(idp.id)}><Trash2 className="h-4 w-4 text-destructive" /></Button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
