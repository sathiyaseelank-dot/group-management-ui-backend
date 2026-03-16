import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

// GET /api/users
router.get('/', async (_req: Request, res: Response) => {
  try {
    const users = await proxyToBackend('/api/users')
    res.json(Array.isArray(users) ? users : [])
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/users
router.post('/', async (req: Request, res: Response) => {
  try {
    const user = await proxyToBackend('/api/users', {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(user)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/users/:userId
router.get('/:userId', async (req: Request, res: Response) => {
  try {
    const { userId } = req.params
    const user = await proxyToBackend(`/api/admin/users/${userId}`)
    res.json(user)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PUT /api/users/:userId
router.put('/:userId', async (req: Request, res: Response) => {
  try {
    const { userId } = req.params
    const user = await proxyToBackend(`/api/admin/users/${userId}`, {
      method: 'PUT',
      body: JSON.stringify(req.body),
    })
    res.json(user)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PATCH /api/users/:userId
router.patch('/:userId', async (req: Request, res: Response) => {
  try {
    const { userId } = req.params
    const user = await proxyToBackend(`/api/admin/users/${userId}`, {
      method: 'PATCH',
      body: JSON.stringify(req.body),
    })
    res.json(user)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// DELETE /api/users/:userId
router.delete('/:userId', async (req: Request, res: Response) => {
  try {
    const { userId } = req.params
    const result = await proxyToBackend(`/api/admin/users/${userId}`, {
      method: 'DELETE',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/users/invite — forwards to controller invite endpoint
router.post('/invite', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend('/api/admin/users/invite', {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
