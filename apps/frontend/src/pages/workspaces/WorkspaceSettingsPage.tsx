import { useEffect, useState } from 'react'
import { Shield, Trash2, UserPlus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getWorkspaceClaims } from '@/lib/jwt'
import type { Workspace, WorkspaceMember } from '@/lib/types'

const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

function getAuth() {
  const token = localStorage.getItem('authToken') || ''
  return { Authorization: `Bearer ${token}` }
}

export default function WorkspaceSettingsPage() {
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [members, setMembers] = useState<WorkspaceMember[]>([])
  const [wsName, setWsName] = useState('')
  const [newEmail, setNewEmail] = useState('')
  const [newRole, setNewRole] = useState('member')
  const [saving, setSaving] = useState(false)

  const claims = getWorkspaceClaims(localStorage.getItem('authToken'))
  const wsID = claims?.wid || ''
  const myRole = claims?.wrole || 'member'
  const canEdit = myRole === 'owner' || myRole === 'admin'
  const isOwner = myRole === 'owner'

  useEffect(() => {
    if (!wsID) return

    fetch(`${API_BASE}/workspaces/${wsID}`, { headers: getAuth() })
      .then((r) => r.json())
      .then((data) => {
        setWorkspace(data)
        setWsName(data.name || '')
      })
      .catch(() => {})

    fetch(`${API_BASE}/workspaces/${wsID}/members`, { headers: getAuth() })
      .then((r) => r.json())
      .then((data) => setMembers(Array.isArray(data) ? data : []))
      .catch(() => {})
  }, [wsID])

  const handleSaveName = async () => {
    if (!wsID || !wsName.trim()) return
    setSaving(true)
    try {
      await fetch(`${API_BASE}/workspaces/${wsID}`, {
        method: 'PUT',
        headers: { ...getAuth(), 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: wsName.trim() }),
      })
    } catch {}
    setSaving(false)
  }

  const handleAddMember = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!wsID || !newEmail.trim()) return
    try {
      await fetch(`${API_BASE}/workspaces/${wsID}/members`, {
        method: 'POST',
        headers: { ...getAuth(), 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: newEmail.trim(), role: newRole }),
      })
      setNewEmail('')
      // Refresh members
      const res = await fetch(`${API_BASE}/workspaces/${wsID}/members`, { headers: getAuth() })
      const data = await res.json()
      setMembers(Array.isArray(data) ? data : [])
    } catch {}
  }

  const handleRemoveMember = async (uid: string) => {
    if (!wsID) return
    try {
      await fetch(`${API_BASE}/workspaces/${wsID}/members/${uid}`, {
        method: 'DELETE',
        headers: getAuth(),
      })
      setMembers((prev) => prev.filter((m) => m.userId !== uid))
    } catch {}
  }

  const handleChangeRole = async (uid: string, role: string) => {
    if (!wsID) return
    try {
      await fetch(`${API_BASE}/workspaces/${wsID}/members/${uid}`, {
        method: 'PUT',
        headers: { ...getAuth(), 'Content-Type': 'application/json' },
        body: JSON.stringify({ role }),
      })
      setMembers((prev) =>
        prev.map((m) =>
          m.userId === uid ? { ...m, role: role as WorkspaceMember['role'] } : m
        )
      )
    } catch {}
  }

  const roleBadge = (role: string) => {
    const colors: Record<string, string> = {
      owner: 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200',
      admin: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
      member: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
    }
    return (
      <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${colors[role] || colors.member}`}>
        {role}
      </span>
    )
  }

  if (!wsID) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">No workspace selected. Please select a workspace first.</p>
      </div>
    )
  }

  return (
    <div className="space-y-8 p-6">
      <div>
        <h2 className="text-lg font-semibold">Workspace Settings</h2>
        <p className="text-sm text-muted-foreground">Manage your workspace configuration and members</p>
      </div>

      {/* Workspace Name */}
      <div className="space-y-3 rounded-lg border p-4">
        <h3 className="font-medium">General</h3>
        <div className="flex items-end gap-3">
          <div className="flex-1 space-y-2">
            <Label>Workspace Name</Label>
            <Input value={wsName} onChange={(e) => setWsName(e.target.value)} disabled={!canEdit} />
          </div>
          {canEdit && (
            <Button onClick={handleSaveName} disabled={saving}>
              {saving ? 'Saving...' : 'Save'}
            </Button>
          )}
        </div>
        <div className="space-y-1">
          <Label className="text-muted-foreground">Trust Domain</Label>
          <p className="font-mono text-sm">{workspace?.trustDomain || '...'}</p>
        </div>
        {workspace?.caCertPem && (
          <div className="space-y-1">
            <Label className="text-muted-foreground">CA Certificate</Label>
            <pre className="max-h-32 overflow-auto rounded bg-muted p-2 text-xs">
              {workspace.caCertPem}
            </pre>
          </div>
        )}
      </div>

      {/* Members */}
      <div className="space-y-3 rounded-lg border p-4">
        <h3 className="font-medium">Members</h3>

        <div className="divide-y rounded-md border">
          {members.map((m) => (
            <div key={m.userId} className="flex items-center justify-between px-4 py-3">
              <div>
                <p className="text-sm font-medium">{m.name || m.email || m.userId}</p>
                {m.email && m.name && (
                  <p className="text-xs text-muted-foreground">{m.email}</p>
                )}
              </div>
              <div className="flex items-center gap-2">
                {isOwner && m.role !== 'owner' ? (
                  <select
                    value={m.role}
                    onChange={(e) => handleChangeRole(m.userId, e.target.value)}
                    className="rounded border bg-background px-2 py-1 text-xs"
                  >
                    <option value="member">member</option>
                    <option value="admin">admin</option>
                    <option value="owner">owner</option>
                  </select>
                ) : (
                  roleBadge(m.role)
                )}
                {canEdit && m.role !== 'owner' && (
                  <Button variant="ghost" size="icon" onClick={() => handleRemoveMember(m.userId)}>
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                )}
              </div>
            </div>
          ))}
        </div>

        {canEdit && (
          <form onSubmit={handleAddMember} className="flex items-end gap-2">
            <div className="flex-1 space-y-1">
              <Label>Add Member</Label>
              <Input
                placeholder="user@example.com"
                value={newEmail}
                onChange={(e) => setNewEmail(e.target.value)}
              />
            </div>
            <select
              value={newRole}
              onChange={(e) => setNewRole(e.target.value)}
              className="rounded border bg-background px-2 py-2 text-sm"
            >
              <option value="member">member</option>
              <option value="admin">admin</option>
            </select>
            <Button type="submit" disabled={!newEmail.trim()}>
              <UserPlus className="mr-1 h-4 w-4" />
              Add
            </Button>
          </form>
        )}
      </div>
    </div>
  )
}
