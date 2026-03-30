import { Router, Request, Response } from 'express'
import { proxyToBackend, getJWTFromRequest } from '../../lib/proxy'

const router = Router()

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
      agentIds: r.agentIds ?? r.agent_ids ?? [],
      protocol: r.protocol ?? r.Protocol ?? 'TCP',
      portFrom: r.portFrom ?? r.port_from ?? r.PortFrom,
      portTo: r.portTo ?? r.port_to ?? r.PortTo,
      firewallStatus: r.firewallStatus ?? r.firewall_status ?? r.FirewallStatus ?? 'unprotected',
    }))

    res.json(formatted)
  } catch (error) {
    console.error('request failed:', error)
    res.status(500).json({ error: 'Internal server error' })
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
    res.status(500).json({ error: 'Internal server error' })
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
    res.status(500).json({ error: 'Internal server error' })
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
    res.status(500).json({ error: 'Internal server error' })
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
    res.status(500).json({ error: 'Internal server error' })
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
    res.status(500).json({ error: 'Internal server error' })
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
    res.status(500).json({ error: 'Internal server error' })
  }
})

export default router
