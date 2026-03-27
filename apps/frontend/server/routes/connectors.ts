import { Router, Request, Response } from 'express'
import { proxyToBackend, getJWTFromRequest } from '../../lib/proxy'

const router = Router()

// GET /api/connectors
router.get('/', async (req: Request, res: Response) => {
  try {
    const connectors = await proxyToBackend('/api/connectors', {}, getJWTFromRequest(req))
    res.json(connectors)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// POST /api/connectors
router.post('/', async (req: Request, res: Response) => {
  try {
    const connector = await proxyToBackend('/api/connectors', {
      method: 'POST',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(connector)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// GET /api/connectors/:connectorId
router.get('/:connectorId', async (req: Request, res: Response) => {
  try {
    const { connectorId } = req.params
    const connector = await proxyToBackend(`/api/connectors/${connectorId}`, {}, getJWTFromRequest(req))
    res.json(connector)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// DELETE /api/connectors/:connectorId
router.delete('/:connectorId', async (req: Request, res: Response) => {
  try {
    const { connectorId } = req.params
    const result = await proxyToBackend(`/api/admin/connectors/${connectorId}`, {
      method: 'DELETE',
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// POST /api/connectors/:connectorId/grant
router.post('/:connectorId/grant', async (req: Request, res: Response) => {
  try {
    const { connectorId } = req.params
    const result = await proxyToBackend(`/api/connectors/${connectorId}/grant`, {
      method: 'POST',
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// POST /api/connectors/:connectorId/revoke
router.post('/:connectorId/revoke', async (req: Request, res: Response) => {
  try {
    const { connectorId } = req.params
    const result = await proxyToBackend(`/api/connectors/${connectorId}/revoke`, {
      method: 'POST',
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
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
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

export default router
