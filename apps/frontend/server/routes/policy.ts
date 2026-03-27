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
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// GET /api/policy/compile/:connectorId
router.get('/compile/:connectorId', async (req: Request, res: Response) => {
  try {
    const { connectorId } = req.params
    const policy = await proxyToBackend(`/api/policy/compile/${connectorId}`, {}, getJWTFromRequest(req))
    res.json(policy)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

export default router
