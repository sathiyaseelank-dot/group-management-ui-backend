import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

// GET /api/device-trusted-profiles
router.get('/', async (_req: Request, res: Response) => {
  try {
    const result = await proxyToBackend<any[]>('/api/device-trusted-profiles')
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/device-trusted-profiles
router.post('/', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend('/api/device-trusted-profiles', {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PATCH /api/device-trusted-profiles/:id
router.patch('/:id', async (req: Request, res: Response) => {
  try {
    const { id } = req.params
    const result = await proxyToBackend(`/api/device-trusted-profiles/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(req.body),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// DELETE /api/device-trusted-profiles/:id
router.delete('/:id', async (req: Request, res: Response) => {
  try {
    const { id } = req.params
    await proxyToBackend(`/api/device-trusted-profiles/${id}`, {
      method: 'DELETE',
    })
    res.json({ ok: true })
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
