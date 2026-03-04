import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

// GET /api/connectors
router.get('/', async (_req: Request, res: Response) => {
  try {
    const connectors = await proxyToBackend('/api/connectors')
    res.json(connectors)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/connectors
router.post('/', async (req: Request, res: Response) => {
  try {
    const connector = await proxyToBackend('/api/connectors', {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(connector)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/connectors/:connectorId
router.get('/:connectorId', async (req: Request, res: Response) => {
  try {
    const { connectorId } = req.params
    const connector = await proxyToBackend(`/api/connectors/${connectorId}`)
    res.json(connector)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// DELETE /api/connectors/:connectorId
router.delete('/:connectorId', async (req: Request, res: Response) => {
  try {
    const { connectorId } = req.params
    const result = await proxyToBackend(`/api/admin/connectors/${connectorId}`, {
      method: 'DELETE',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/connectors/:connectorId/heartbeat
router.post('/:connectorId/heartbeat', async (req: Request, res: Response) => {
  try {
    const { connectorId } = req.params
    const result = await proxyToBackend(`/api/connectors/${connectorId}/heartbeat`, {
      method: 'POST',
      body: Object.keys(req.body).length ? JSON.stringify(req.body) : undefined,
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PATCH /api/connectors/:connectorId/heartbeat
router.patch('/:connectorId/heartbeat', async (req: Request, res: Response) => {
  try {
    if (typeof req.body?.last_policy_version !== 'number') {
      return res.status(400).json({ error: 'last_policy_version is required' })
    }
    res.json({
      update_available: false,
      current_version: req.body.last_policy_version,
    })
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
