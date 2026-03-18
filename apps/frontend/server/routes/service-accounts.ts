import { Router, Request, Response } from 'express'
import { proxyToBackend, getJWTFromRequest } from '../../lib/proxy'

const router = Router()

// GET /api/service-accounts
router.get('/', async (req: Request, res: Response) => {
  try {
    const serviceAccounts = await proxyToBackend<any[]>('/api/admin/service-accounts', {}, getJWTFromRequest(req))
    const formatted = serviceAccounts.map((s: any) => ({
      id: s.ID,
      name: s.Name,
      type: 'SERVICE',
      displayLabel: `Service: ${s.Name}`,
      status: s.Status,
      associatedResourceCount: s.AssociatedResourceCount,
      createdAt: s.CreatedAt,
    }))
    res.json(formatted)
  } catch {
    res.json([])
  }
})

export default router
