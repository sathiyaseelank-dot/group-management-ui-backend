import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

// GET /api/devices
router.get('/', async (_req: Request, res: Response) => {
  try {
    const result = await proxyToBackend<any[]>('/api/devices')
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
