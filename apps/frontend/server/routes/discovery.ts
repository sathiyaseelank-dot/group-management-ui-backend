import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

// POST /api/discovery/scan — start a network discovery scan
router.post('/scan', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend('/api/admin/discovery/scan', {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/discovery/scan/:requestId — get scan status
router.get('/scan/:requestId', async (req: Request, res: Response) => {
  try {
    const { requestId } = req.params
    const result = await proxyToBackend(`/api/admin/discovery/scan/${requestId}`)
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/discovery/results — get all discovered resources
router.get('/results', async (req: Request, res: Response) => {
  try {
    const connectorId = req.query.connector_id
    const query = connectorId ? `?connector_id=${connectorId}` : ''
    const result = await proxyToBackend(`/api/admin/discovery/results${query}`)
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
