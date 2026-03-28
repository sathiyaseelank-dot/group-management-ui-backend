import { Router, Request, Response } from 'express'
import { proxyToBackend, getJWTFromRequest } from '../../lib/proxy'

const router = Router()

// GET /api/access-rules
router.get('/', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend<any[]>('/api/access-rules', {}, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// POST /api/access-rules
router.post('/', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend('/api/access-rules', {
      method: 'POST',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// DELETE /api/access-rules/:ruleId
router.delete('/:ruleId', async (req: Request, res: Response) => {
  try {
    const { ruleId } = req.params
    await proxyToBackend(`/api/access-rules/${ruleId}`, {
      method: 'DELETE',
    }, getJWTFromRequest(req))
    res.json({ ok: true })
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// GET /api/access-rules/:ruleId/identity-count
router.get('/:ruleId/identity-count', async (req: Request, res: Response) => {
  try {
    const { ruleId } = req.params
    const result = await proxyToBackend<{ count: number }>(`/api/access-rules/${ruleId}/identity-count`, {}, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

export default router
