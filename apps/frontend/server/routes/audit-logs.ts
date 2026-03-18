import { Router, Request, Response } from 'express'
import { proxyToBackend, getJWTFromRequest } from '../../lib/proxy'

const router = Router()

// GET /api/audit-logs?limit=50&offset=0
router.get('/', async (req: Request, res: Response) => {
  try {
    const { limit = '50', offset = '0' } = req.query as Record<string, string>
    const data = await proxyToBackend(`/api/admin/audit-logs?limit=${limit}&offset=${offset}`, {}, getJWTFromRequest(req))
    res.json(data)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
