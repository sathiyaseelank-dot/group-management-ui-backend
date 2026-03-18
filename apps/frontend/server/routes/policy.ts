import { Router, Request, Response } from 'express'
import { proxyToBackend, getJWTFromRequest } from '../../lib/proxy'

const router = Router()

// GET /api/policy/acl/:connectorId
router.get('/acl/:connectorId', async (req: Request, res: Response) => {
  try {
    const { connectorId } = req.params
    const policy = await proxyToBackend(`/api/policy/acl/${connectorId}`, {}, getJWTFromRequest(req))
    res.json(policy)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/policy/compile/:connectorId
router.get('/compile/:connectorId', async (req: Request, res: Response) => {
  try {
    const { connectorId } = req.params
    const policy = await proxyToBackend(`/api/policy/compile/${connectorId}`, {}, getJWTFromRequest(req))
    res.json(policy)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
