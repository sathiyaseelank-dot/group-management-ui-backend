import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

interface BackendGroup {
  id?: string
  ID?: string
  name?: string
  Name?: string
  description?: string
  Description?: string
  members?: number
  Members?: number
  resource_count?: number
  ResourceCount?: number
  created_at?: string
  CreatedAt?: string
  updated_at?: string
  UpdatedAt?: string
}

function mapBackendGroup(group: BackendGroup) {
  const name = group.name ?? group.Name ?? ''
  const description = group.description ?? group.Description ?? ''
  return {
    id: group.id ?? group.ID ?? '',
    name,
    description,
    type: 'GROUP',
    displayLabel: `Group: ${name || 'Unknown'}`,
    memberCount: group.members ?? group.Members ?? 0,
    resourceCount: group.resource_count ?? group.ResourceCount ?? 0,
    createdAt: group.created_at ?? group.CreatedAt ?? '',
    updatedAt: group.updated_at ?? group.UpdatedAt ?? '',
  }
}

// GET /api/groups
router.get('/', async (req: Request, res: Response) => {
  try {
    const groups = await proxyToBackend<BackendGroup[]>('/api/admin/user-groups', req)
    res.json(groups.map(mapBackendGroup))
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/groups
router.post('/', async (req: Request, res: Response) => {
  try {
    const group = await proxyToBackend<BackendGroup>('/api/admin/user-groups', req, {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(mapBackendGroup(group))
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/groups/:groupId
router.get('/:groupId', async (req: Request, res: Response) => {
  try {
    const { groupId } = req.params
    const payload = await proxyToBackend(`/api/groups/${groupId}`, req)
    res.json(payload)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PUT /api/groups/:groupId
router.put('/:groupId', async (req: Request, res: Response) => {
  try {
    const { groupId } = req.params
    const group = await proxyToBackend(`/api/admin/user-groups/${groupId}`, req, {
      method: 'PUT',
      body: JSON.stringify(req.body),
    })
    res.json(group)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// DELETE /api/groups/:groupId
router.delete('/:groupId', async (req: Request, res: Response) => {
  try {
    const { groupId } = req.params
    const result = await proxyToBackend(`/api/admin/user-groups/${groupId}`, req, {
      method: 'DELETE',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/groups/:groupId/members
router.get('/:groupId/members', async (req: Request, res: Response) => {
  try {
    const { groupId } = req.params
    const members = await proxyToBackend(`/api/admin/user-groups/${groupId}/members`, req)
    res.json(members)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/groups/:groupId/members
router.post('/:groupId/members', async (req: Request, res: Response) => {
  try {
    const { groupId } = req.params
    const body = req.body
    if (Array.isArray(body?.memberIds)) {
      const results: unknown[] = []
      for (const memberId of body.memberIds) {
        if (!memberId) continue
        const result = await proxyToBackend(`/api/admin/user-groups/${groupId}/members`, req, {
          method: 'POST',
          body: JSON.stringify({ user_id: memberId }),
        })
        results.push(result)
      }
      return res.json({ status: 'ok', added: results.length })
    }
    if (body?.user_id) {
      const result = await proxyToBackend(`/api/admin/user-groups/${groupId}/members`, req, {
        method: 'POST',
        body: JSON.stringify(body),
      })
      return res.json(result)
    }
    res.status(400).json({ error: 'user_id required' })
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// DELETE /api/groups/:groupId/members/:userId
router.delete('/:groupId/members/:userId', async (req: Request, res: Response) => {
  try {
    const { groupId, userId } = req.params
    const result = await proxyToBackend(`/api/admin/user-groups/${groupId}/members`, req, {
      method: 'DELETE',
      body: JSON.stringify({ user_id: userId }),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/groups/:groupId/resources
router.post('/:groupId/resources', async (req: Request, res: Response) => {
  try {
    const { groupId } = req.params
    const result = await proxyToBackend(`/api/groups/${groupId}/resources`, req, {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
