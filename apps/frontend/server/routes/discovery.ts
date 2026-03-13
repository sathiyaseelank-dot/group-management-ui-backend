import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

// Convert a snake_case DiscoveredResource from the Go backend to camelCase
function mapResource(r: Record<string, unknown>) {
  return {
    id: r.id,
    ip: r.ip,
    port: r.port,
    protocol: r.protocol,
    serviceName: r.service_name || 'Unknown',
    reachableFrom: r.reachable_from,
    firstSeen: r.first_seen,
  }
}

// Convert a snake_case ScanJob from the Go backend to camelCase
function mapScanJob(job: Record<string, unknown>) {
  const results = Array.isArray(job.results)
    ? job.results.map((r: Record<string, unknown>) => mapResource(r))
    : undefined
  return {
    requestId: job.request_id,
    connectorId: job.connector_id,
    status: job.status,
    targets: job.targets,
    ports: job.ports,
    startedAt: job.started_at,
    completedAt: job.completed_at,
    results,
    error: job.error,
  }
}

// POST /api/discovery/scan — start a network discovery scan
router.post('/scan', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend('/api/admin/discovery/scan', {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/discovery/scan/:requestId — get scan status
router.get('/scan/:requestId', async (req: Request, res: Response) => {
  try {
    const { requestId } = req.params
    const raw = await proxyToBackend(`/api/admin/discovery/scan/${requestId}`) as Record<string, unknown>
    res.json(mapScanJob(raw))
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/discovery/results — get all discovered resources
router.get('/results', async (req: Request, res: Response) => {
  try {
    const connectorId = req.query.connector_id
    const query = connectorId ? `?connector_id=${connectorId}` : ''
    const raw = await proxyToBackend(`/api/admin/discovery/results${query}`) as Record<string, unknown>[]
    res.json(Array.isArray(raw) ? raw.map(mapResource) : [])
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
