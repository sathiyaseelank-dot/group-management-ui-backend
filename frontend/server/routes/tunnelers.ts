import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

interface BackendAdminTunneler {
  id: string
  status: 'ONLINE' | 'OFFLINE' | string
  connector_id: string
  last_seen: string
}

// GET /api/tunnelers
router.get('/', async (_req: Request, res: Response) => {
  try {
    const tunnelers = await proxyToBackend<BackendAdminTunneler[]>('/api/admin/tunnelers')
    const formatted = (Array.isArray(tunnelers) ? tunnelers : []).map((t) => ({
      id: t.id,
      name: t.id,
      status: String(t.status || '').toLowerCase() === 'online' ? 'online' : 'offline',
      version: '—',
      hostname: '—',
      remoteNetworkId: '',
    }))
    res.json(formatted)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
