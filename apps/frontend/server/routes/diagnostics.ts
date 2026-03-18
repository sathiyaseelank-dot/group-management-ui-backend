import { Router, Request, Response } from 'express'
import { proxyToBackend, getJWTFromRequest } from '../../lib/proxy'

const router = Router()

// GET /api/diagnostics
router.get('/', async (req: Request, res: Response) => {
  try {
    const diagnostics = await proxyToBackend('/api/diagnostics', {}, getJWTFromRequest(req))
    res.json(diagnostics)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/diagnostics/ping/:connectorId
router.post('/ping/:connectorId', async (req: Request, res: Response) => {
  try {
    const { connectorId } = req.params
    const result = await proxyToBackend(`/api/diagnostics/ping/${connectorId}`, {
      method: 'POST',
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/diagnostics/trace
router.post('/trace', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend('/api/diagnostics/trace', {
      method: 'POST',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
