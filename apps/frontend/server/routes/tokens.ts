import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

// POST /api/tokens
router.post('/', async (_req: Request, res: Response) => {
  try {
    const token = await proxyToBackend('/api/admin/tokens', {
      method: 'POST',
    })
    res.json(token)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
