import { Router, Request, Response } from 'express'
import { proxyToBackend, getJWTFromRequest } from '../../lib/proxy'

const router = Router()

// GET /api/users
router.get('/', async (req: Request, res: Response) => {
  try {
    const users = await proxyToBackend('/api/users', {}, getJWTFromRequest(req))
    res.json(Array.isArray(users) ? users : [])
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// POST /api/users
router.post('/', async (req: Request, res: Response) => {
  try {
    const user = await proxyToBackend('/api/users', {
      method: 'POST',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(user)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// GET /api/users/:userId
router.get('/:userId', async (req: Request, res: Response) => {
  try {
    const { userId } = req.params
    const user = await proxyToBackend(`/api/admin/users/${userId}`, {}, getJWTFromRequest(req))
    res.json(user)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// PUT /api/users/:userId
router.put('/:userId', async (req: Request, res: Response) => {
  try {
    const { userId } = req.params
    const user = await proxyToBackend(`/api/admin/users/${userId}`, {
      method: 'PUT',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(user)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// PATCH /api/users/:userId
router.patch('/:userId', async (req: Request, res: Response) => {
  try {
    const { userId } = req.params
    const user = await proxyToBackend(`/api/admin/users/${userId}`, {
      method: 'PATCH',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(user)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// DELETE /api/users/:userId
router.delete('/:userId', async (req: Request, res: Response) => {
  try {
    const { userId } = req.params
    const result = await proxyToBackend(`/api/admin/users/${userId}`, {
      method: 'DELETE',
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

// POST /api/users/invite — forwards to controller invite endpoint
router.post('/invite', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend('/api/admin/users/invite', {
      method: 'POST',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
  }
})

export default router
