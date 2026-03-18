import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

function mapService(s: Record<string, unknown>) {
  return {
    id: s.id,
    agentId: s.agent_id,
    port: s.port,
    protocol: s.protocol,
    boundIp: s.bound_ip,
    firstSeen: s.first_seen,
    lastSeen: s.last_seen,
    workspaceId: s.workspace_id,
    dismissed: s.dismissed === 1 || s.dismissed === true,
    status: s.status || 'active',
  }
}

// GET /api/agent-discovery/results?agent_id=xxx&include_dismissed=true
router.get('/results', async (req: Request, res: Response) => {
  try {
    const params = new URLSearchParams()
    if (req.query.agent_id) params.set('agent_id', String(req.query.agent_id))
    if (req.query.include_dismissed === 'true') params.set('include_dismissed', 'true')
    const query = params.toString() ? `?${params.toString()}` : ''
    const raw = await proxyToBackend(`/api/admin/agent-discovery/results${query}`) as Record<string, unknown>[]
    res.json(Array.isArray(raw) ? raw.map(mapService) : [])
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PATCH /api/agent-discovery/results/:id/dismiss
router.patch('/results/:id/dismiss', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend(`/api/admin/agent-discovery/results/${req.params.id}/dismiss`, {
      method: 'PATCH',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PATCH /api/agent-discovery/results/:id/undismiss
router.patch('/results/:id/undismiss', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend(`/api/admin/agent-discovery/results/${req.params.id}/undismiss`, {
      method: 'PATCH',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// DELETE /api/agent-discovery/results?agent_id=xxx
router.delete('/results', async (req: Request, res: Response) => {
  try {
    const params = new URLSearchParams()
    if (req.query.agent_id) params.set('agent_id', String(req.query.agent_id))
    const query = params.toString() ? `?${params.toString()}` : ''
    const result = await proxyToBackend(`/api/admin/agent-discovery/results${query}`, {
      method: 'DELETE',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/agent-discovery/summary
router.get('/summary', async (_req: Request, res: Response) => {
  try {
    const result = await proxyToBackend('/api/admin/agent-discovery/summary')
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
