import { Router, Request, Response } from 'express'
import { proxyToBackend, getJWTFromRequest } from '../../lib/proxy'

const router = Router()

// GET /api/devices
router.get('/', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend<any[]>('/api/devices', {}, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

export default router
