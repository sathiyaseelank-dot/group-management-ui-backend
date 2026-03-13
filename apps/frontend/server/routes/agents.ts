import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

interface BackendAdminAgent {
  id: string
  status: 'ONLINE' | 'OFFLINE' | string
  version: string
  hostname: string
  connector_id: string
  remote_network_id: string
  last_seen: string
}

// GET /api/agents
router.get('/', async (_req: Request, res: Response) => {
  try {
    const agents = await proxyToBackend<BackendAdminAgent[]>('/api/admin/agents')
    const formatted = (Array.isArray(agents) ? agents : []).map((t) => ({
      id: t.id,
      name: t.id,
      status: String(t.status || '').toLowerCase() === 'online' ? 'online' : 'offline',
      version: t.version || '',
      hostname: t.hostname || '',
      remoteNetworkId: t.remote_network_id || '',
    }))
    res.json(formatted)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/agents/:agentId
router.get('/:agentId', async (req: Request, res: Response) => {
  try {
    const { agentId } = req.params
    const result = await proxyToBackend(`/api/agents/${agentId}`)
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// DELETE /api/agents/:agentId
router.delete('/:agentId', async (req: Request, res: Response) => {
  try {
    const { agentId } = req.params
    const result = await proxyToBackend(`/api/agents/${agentId}`, {
      method: 'DELETE',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/agents/:agentId/revoke
router.post('/:agentId/revoke', async (req: Request, res: Response) => {
  try {
    const { agentId } = req.params
    const result = await proxyToBackend(`/api/agents/${agentId}/revoke`, {
      method: 'POST',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/agents/:agentId/grant
router.post('/:agentId/grant', async (req: Request, res: Response) => {
  try {
    const { agentId } = req.params
    const result = await proxyToBackend(`/api/agents/${agentId}/grant`, {
      method: 'POST',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
