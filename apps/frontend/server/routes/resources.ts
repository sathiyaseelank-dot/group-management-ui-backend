import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

// GET /api/resources
router.get('/', async (_req: Request, res: Response) => {
  try {
    const resources = await proxyToBackend<any[]>('/api/resources')

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
      protocol: r.protocol ?? r.Protocol ?? 'TCP',
      portFrom: r.portFrom ?? r.port_from ?? r.PortFrom,
      portTo: r.portTo ?? r.port_to ?? r.PortTo,
      firewallStatus: r.firewallStatus ?? r.firewall_status ?? r.FirewallStatus ?? 'unprotected',
    }))

    res.json(formatted)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/resources
router.post('/', async (req: Request, res: Response) => {
  try {
    const resource = await proxyToBackend('/api/resources', {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(resource)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/resources/:resourceId
router.get('/:resourceId', async (req: Request, res: Response) => {
  try {
    const { resourceId } = req.params
    const result = await proxyToBackend<any>(`/api/resources/${resourceId}`)
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PUT /api/resources/:resourceId
router.put('/:resourceId', async (req: Request, res: Response) => {
  try {
    const { resourceId } = req.params
    const result = await proxyToBackend(`/api/resources/${resourceId}`, {
      method: 'PUT',
      body: JSON.stringify(req.body),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PATCH /api/resources/:resourceId (firewall status toggle)
router.patch('/:resourceId', async (req: Request, res: Response) => {
  try {
    const { resourceId } = req.params
    const result = await proxyToBackend(`/api/resources/${resourceId}`, {
      method: 'PATCH',
      body: JSON.stringify(req.body),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// DELETE /api/resources/:resourceId
router.delete('/:resourceId', async (req: Request, res: Response) => {
  try {
    const { resourceId } = req.params
    const result = await proxyToBackend(`/api/resources/${resourceId}`, {
      method: 'DELETE',
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
