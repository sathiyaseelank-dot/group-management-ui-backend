import { Router, Request, Response } from 'express'
import { proxyWithJWT } from '../../lib/proxy'

const router = Router()

function getJWT(req: Request): string {
  const auth = req.headers.authorization || ''
  if (auth.startsWith('Bearer ')) {
    return auth.slice(7)
  }
  return ''
}

interface BackendWorkspace {
  id?: string
  name?: string
  slug?: string
  trust_domain?: string
  ca_cert_pem?: string
  status?: string
  role?: string
  created_at?: string
  updated_at?: string
}

function mapWorkspace(ws: BackendWorkspace) {
  return {
    id: ws.id ?? '',
    name: ws.name ?? '',
    slug: ws.slug ?? '',
    trustDomain: ws.trust_domain ?? '',
    caCertPem: ws.ca_cert_pem ?? '',
    status: ws.status ?? 'active',
    role: ws.role ?? '',
    createdAt: ws.created_at ?? '',
    updatedAt: ws.updated_at ?? '',
  }
}

// GET /api/workspaces
router.get('/', async (req: Request, res: Response) => {
  try {
    const jwt = getJWT(req)
    const workspaces = await proxyWithJWT<BackendWorkspace[]>('/api/workspaces', jwt)
    res.json(workspaces.map(mapWorkspace))
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/workspaces
router.post('/', async (req: Request, res: Response) => {
  try {
    const jwt = getJWT(req)
    const result = await proxyWithJWT('/api/workspaces', jwt, {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    if (result.workspace) {
      result.workspace = mapWorkspace(result.workspace)
    }
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/workspaces/:id
router.get('/:id', async (req: Request, res: Response) => {
  try {
    const jwt = getJWT(req)
    const ws = await proxyWithJWT<BackendWorkspace>(`/api/workspaces/${req.params.id}`, jwt)
    res.json(mapWorkspace(ws))
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PUT /api/workspaces/:id
router.put('/:id', async (req: Request, res: Response) => {
  try {
    const jwt = getJWT(req)
    const ws = await proxyWithJWT(`/api/workspaces/${req.params.id}`, jwt, {
      method: 'PUT',
      body: JSON.stringify(req.body),
    })
    res.json(ws)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// DELETE /api/workspaces/:id
router.delete('/:id', async (req: Request, res: Response) => {
  try {
    const jwt = getJWT(req)
    const result = await proxyWithJWT(`/api/workspaces/${req.params.id}`, jwt, {
      method: 'DELETE',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/workspaces/:id/select
router.post('/:id/select', async (req: Request, res: Response) => {
  try {
    const jwt = getJWT(req)
    const result = await proxyWithJWT(`/api/workspaces/${req.params.id}/select`, jwt, {
      method: 'POST',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/workspaces/:id/members
router.get('/:id/members', async (req: Request, res: Response) => {
  try {
    const jwt = getJWT(req)
    const members = await proxyWithJWT(`/api/workspaces/${req.params.id}/members`, jwt)
    res.json(members)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/workspaces/:id/members
router.post('/:id/members', async (req: Request, res: Response) => {
  try {
    const jwt = getJWT(req)
    const result = await proxyWithJWT(`/api/workspaces/${req.params.id}/members`, jwt, {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PUT /api/workspaces/:id/members/:uid
router.put('/:id/members/:uid', async (req: Request, res: Response) => {
  try {
    const jwt = getJWT(req)
    const result = await proxyWithJWT(`/api/workspaces/${req.params.id}/members/${req.params.uid}`, jwt, {
      method: 'PUT',
      body: JSON.stringify(req.body),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// DELETE /api/workspaces/:id/members/:uid
router.delete('/:id/members/:uid', async (req: Request, res: Response) => {
  try {
    const jwt = getJWT(req)
    const result = await proxyWithJWT(`/api/workspaces/${req.params.id}/members/${req.params.uid}`, jwt, {
      method: 'DELETE',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
