import { Router, Request, Response } from 'express'
import { proxyToBackend, getJWTFromRequest } from '../../lib/proxy'

const router = Router()

interface BackendGroup {
  id?: string
  ID?: string
  name?: string
  Name?: string
  description?: string
  Description?: string
  memberCount?: number
  MemberCount?: number
  members?: number
  Members?: number
  resourceCount?: number
  ResourceCount?: number
  resource_count?: number
  createdAt?: string
  created_at?: string
  CreatedAt?: string
  updatedAt?: string
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
    memberCount: group.memberCount ?? group.MemberCount ?? group.members ?? group.Members ?? 0,
    resourceCount: group.resourceCount ?? group.ResourceCount ?? group.resource_count ?? 0,
    createdAt: group.createdAt ?? group.created_at ?? group.CreatedAt ?? '',
    updatedAt: group.updatedAt ?? group.updated_at ?? group.UpdatedAt ?? '',
  }
}

// GET /api/groups
router.get('/', async (req: Request, res: Response) => {
  try {
    const groups = await proxyToBackend<BackendGroup[]>('/api/groups', {}, getJWTFromRequest(req))
    res.json(groups.map(mapBackendGroup))
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// POST /api/groups
router.post('/', async (req: Request, res: Response) => {
  try {
    const group = await proxyToBackend<BackendGroup>('/api/groups', {
      method: 'POST',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(mapBackendGroup(group))
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// GET /api/groups/:groupId
router.get('/:groupId', async (req: Request, res: Response) => {
  try {
    const { groupId } = req.params
    const payload = await proxyToBackend(`/api/groups/${groupId}`, {}, getJWTFromRequest(req))
    res.json(payload)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// PUT /api/groups/:groupId
router.put('/:groupId', async (req: Request, res: Response) => {
  try {
    const { groupId } = req.params
    const group = await proxyToBackend(`/api/admin/user-groups/${groupId}`, {
      method: 'PUT',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(group)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// DELETE /api/groups/:groupId
router.delete('/:groupId', async (req: Request, res: Response) => {
  try {
    const { groupId } = req.params
    const result = await proxyToBackend(`/api/admin/user-groups/${groupId}`, {
      method: 'DELETE',
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// GET /api/groups/:groupId/members
router.get('/:groupId/members', async (req: Request, res: Response) => {
  try {
    const { groupId } = req.params
    const members = await proxyToBackend(`/api/admin/user-groups/${groupId}/members`, {}, getJWTFromRequest(req))
    res.json(members)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// POST /api/groups/:groupId/members
router.post('/:groupId/members', async (req: Request, res: Response) => {
  try {
    const { groupId } = req.params
    const result = await proxyToBackend(`/api/groups/${groupId}/members`, {
      method: 'POST',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// DELETE /api/groups/:groupId/members/:userId
router.delete('/:groupId/members/:userId', async (req: Request, res: Response) => {
  try {
    const { groupId, userId } = req.params
    const result = await proxyToBackend(`/api/groups/${groupId}/members/${userId}`, {
      method: 'DELETE',
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// POST /api/groups/:groupId/resources
router.post('/:groupId/resources', async (req: Request, res: Response) => {
  try {
    const { groupId } = req.params
    const result = await proxyToBackend(`/api/groups/${groupId}/resources`, {
      method: 'POST',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

export default router
