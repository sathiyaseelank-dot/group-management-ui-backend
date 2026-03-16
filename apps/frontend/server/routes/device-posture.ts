import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

// GET /api/device-posture
router.get('/', async (_req: Request, res: Response) => {
  try {
    const result = await proxyToBackend<any[]>('/api/device-posture')
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
