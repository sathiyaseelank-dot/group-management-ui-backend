import { Router, Request, Response } from 'express'
import { proxyToBackend, getJWTFromRequest } from '../../lib/proxy'

const router = Router()

// GET /api/agents
router.get('/', async (req: Request, res: Response) => {
  try {
    const agents = await proxyToBackend('/api/agents', {}, getJWTFromRequest(req))
    res.json(Array.isArray(agents) ? agents : [])
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/agents
router.post('/', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend('/api/agents', {
      method: 'POST',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/agents/:agentId
router.get('/:agentId', async (req: Request, res: Response) => {
  try {
    const { agentId } = req.params
    const result = await proxyToBackend(`/api/agents/${agentId}`, {}, getJWTFromRequest(req))
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
    }, getJWTFromRequest(req))
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
    }, getJWTFromRequest(req))
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
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
