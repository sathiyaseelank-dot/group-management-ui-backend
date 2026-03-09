import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

interface BackendUser {
  id?: string
  ID?: string
  name?: string
  Name?: string
  email?: string
  Email?: string
  status?: string
  Status?: string
  certificate_identity?: string
  CertificateIdentity?: string
  created_at?: string
  CreatedAt?: string
  updated_at?: string
  UpdatedAt?: string
}

interface BackendGroup {
  id?: string
  ID?: string
}

interface BackendGroupMember {
  user_id?: string
  userId?: string
}

function mapBackendUser(user: BackendUser, groups: string[]) {
  const name = user.name ?? user.Name ?? ''
  const email = user.email ?? user.Email ?? ''
  const status = (user.status ?? user.Status ?? 'active').toLowerCase()
  const certificateIdentity = user.certificate_identity ?? user.CertificateIdentity ?? undefined
  const createdAt = user.created_at ?? user.CreatedAt ?? ''
  return {
    id: user.id ?? user.ID ?? '',
    name,
    email,
    status,
    certificateIdentity,
    groups,
    createdAt,
    type: 'USER',
    displayLabel: `User: ${name || email || 'Unknown'}`,
  }
}

// GET /api/users
router.get('/', async (_req: Request, res: Response) => {
  try {
    const [users, groups] = await Promise.all([
      proxyToBackend<BackendUser[]>('/api/admin/users'),
      proxyToBackend<BackendGroup[]>('/api/admin/user-groups'),
    ])

    const membershipMap = new Map<string, Set<string>>()
    await Promise.all(
      groups.map(async (group) => {
        const groupId = group.id ?? group.ID
        if (!groupId) return
        const members = await proxyToBackend<BackendGroupMember[]>(
          `/api/admin/user-groups/${encodeURIComponent(groupId)}/members`
        )
        members.forEach((member) => {
          const userId = member.user_id ?? member.userId
          if (!userId) return
          if (!membershipMap.has(userId)) {
            membershipMap.set(userId, new Set())
          }
          membershipMap.get(userId)?.add(groupId)
        })
      })
    )

    const formattedUsers = users.map((user) => {
      const userId = user.id ?? user.ID ?? ''
      const groupsForUser = Array.from(membershipMap.get(userId) ?? [])
      return mapBackendUser(user, groupsForUser)
    })
    res.json(formattedUsers)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/users
router.post('/', async (req: Request, res: Response) => {
  try {
    const user = await proxyToBackend('/api/admin/users', {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(mapBackendUser(user as BackendUser, []))
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/users/:userId
router.get('/:userId', async (req: Request, res: Response) => {
  try {
    const { userId } = req.params
    const user = await proxyToBackend(`/api/admin/users/${userId}`)
    res.json(user)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PUT /api/users/:userId
router.put('/:userId', async (req: Request, res: Response) => {
  try {
    const { userId } = req.params
    const user = await proxyToBackend(`/api/admin/users/${userId}`, {
      method: 'PUT',
      body: JSON.stringify(req.body),
    })
    res.json(user)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PATCH /api/users/:userId
router.patch('/:userId', async (req: Request, res: Response) => {
  try {
    const { userId } = req.params
    const user = await proxyToBackend(`/api/admin/users/${userId}`, {
      method: 'PATCH',
      body: JSON.stringify(req.body),
    })
    res.json(user)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// DELETE /api/users/:userId
router.delete('/:userId', async (req: Request, res: Response) => {
  try {
    const { userId } = req.params
    const result = await proxyToBackend(`/api/admin/users/${userId}`, {
      method: 'DELETE',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/users/invite — forwards to controller invite endpoint
router.post('/invite', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend('/api/admin/users/invite', {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
