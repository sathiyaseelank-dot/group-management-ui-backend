import { Router, Request, Response } from 'express'
import { proxyToBackend, getJWTFromRequest } from '../../lib/proxy'

const router = Router()

function sendBackendError(res: Response, error: unknown) {
  const message = error instanceof Error ? error.message : 'Internal server error'
  const statusMatch = message.match(/^Backend error: (\d+)(?::\s*)?(.*)$/s)
  if (statusMatch) {
    const status = Number(statusMatch[1])
    const body = statusMatch[2]?.trim()
    res.status(status).json({ error: body || `Backend error: ${status}` })
    return
  }
  res.status(500).json({ error: message })
}

// GET /api/resources
router.get('/', async (req: Request, res: Response) => {
  try {
    const resources = await proxyToBackend<any[]>('/api/resources', {}, getJWTFromRequest(req))

    let resourceList: any[] = []
    if (Array.isArray(resources)) {
      resourceList = resources
    } else if ((resources as any)?.Resources) {
      resourceList = (resources as any).Resources
    }

    const formatted = resourceList.map((r: any) => ({
      id: r.id ?? r.ID,
      name: r.name ?? r.Name,
      type: r.type ?? r.Type,
      address: r.address ?? r.Address,
      ports: r.ports ?? r.Ports ?? '',
      alias: r.alias ?? r.Alias,
      description: r.description ?? r.Description ?? '',
      remoteNetworkId: r.remoteNetworkId ?? r.remote_network_id ?? r.RemoteNetwork,
      connectorId: r.connectorId ?? r.connector_id ?? r.ConnectorID,
      agentIds: r.agentIds ?? r.agent_ids ?? [],
      protocol: r.protocol ?? r.Protocol ?? 'TCP',
      portFrom: r.portFrom ?? r.port_from ?? r.PortFrom,
      portTo: r.portTo ?? r.port_to ?? r.PortTo,
      firewallStatus: r.firewallStatus ?? r.firewall_status ?? r.FirewallStatus ?? 'unprotected',
    }))

    res.json(formatted)
  } catch (error) {
    console.error('request failed:', error)
    sendBackendError(res, error)
  }
})

// POST /api/resources
router.post('/', async (req: Request, res: Response) => {
  try {
    const resource = await proxyToBackend('/api/resources', {
      method: 'POST',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(resource)
  } catch (error) {
    console.error('request failed:', error)
    sendBackendError(res, error)
  }
})

// POST /api/resources/batch
router.post('/batch', async (req: Request, res: Response) => {
  try {
    const result = await proxyToBackend('/api/resources/batch', {
      method: 'POST',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    sendBackendError(res, error)
  }
})

// GET /api/resources/:resourceId
router.get('/:resourceId', async (req: Request, res: Response) => {
  try {
    const { resourceId } = req.params
    const result = await proxyToBackend<any>(`/api/resources/${resourceId}`, {}, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    sendBackendError(res, error)
  }
})

// PUT /api/resources/:resourceId
router.put('/:resourceId', async (req: Request, res: Response) => {
  try {
    const { resourceId } = req.params
    const result = await proxyToBackend(`/api/resources/${resourceId}`, {
      method: 'PUT',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    sendBackendError(res, error)
  }
})

// PATCH /api/resources/:resourceId (firewall status toggle)
router.patch('/:resourceId', async (req: Request, res: Response) => {
  try {
    const { resourceId } = req.params
    const result = await proxyToBackend(`/api/resources/${resourceId}`, {
      method: 'PATCH',
      body: JSON.stringify(req.body),
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    sendBackendError(res, error)
  }
})

// DELETE /api/resources/:resourceId
router.delete('/:resourceId', async (req: Request, res: Response) => {
  try {
    const { resourceId } = req.params
    const result = await proxyToBackend(`/api/resources/${resourceId}`, {
      method: 'DELETE',
    }, getJWTFromRequest(req))
    res.json(result)
  } catch (error) {
    console.error('request failed:', error)
    sendBackendError(res, error)
  }
})

export default router
