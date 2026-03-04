import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

// GET /api/resources
router.get('/', async (req: Request, res: Response) => {
  try {
    const resources = await proxyToBackend<any[]>('/api/resources', req)

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
    }))

    res.json(formatted)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/resources
router.post('/', async (req: Request, res: Response) => {
  try {
    const resource = await proxyToBackend('/api/resources', req, {
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

    const resources = await proxyToBackend<any[]>('/api/resources', req)
    const resource = Array.isArray(resources)
      ? resources.find((r: any) => (r.id ?? r.ID) === resourceId)
      : undefined

    if (!resource) {
      return res.status(404).json({ error: 'Resource not found' })
    }

    const formattedResource = {
      id: resource.id ?? resource.ID,
      name: resource.name ?? resource.Name,
      type: resource.type ?? resource.Type,
      address: resource.address ?? resource.Address,
      ports: resource.ports ?? resource.Ports ?? '',
      alias: resource.alias ?? resource.Alias,
      description: resource.description ?? resource.Description ?? '',
      remoteNetworkId: resource.remoteNetworkId ?? resource.remote_network_id ?? resource.RemoteNetwork,
      protocol: resource.protocol ?? resource.Protocol ?? 'TCP',
      portFrom: resource.portFrom ?? resource.port_from ?? resource.PortFrom,
      portTo: resource.portTo ?? resource.port_to ?? resource.PortTo,
    }

    const accessRules: any[] = []
    if (resource.Authorizations) {
      for (const auth of resource.Authorizations) {
        accessRules.push({
          id: `rule_${formattedResource.id}_${auth.PrincipalSPIFFE}`,
          name: `${auth.PrincipalSPIFFE} access`,
          resourceId: formattedResource.id,
          allowedGroups: [auth.PrincipalSPIFFE],
          enabled: true,
          createdAt: resource.CreatedAt ?? resource.created_at ?? '',
          updatedAt: resource.UpdatedAt ?? resource.updated_at ?? resource.CreatedAt ?? '',
        })
      }
    }

    res.json({ resource: formattedResource, accessRules })
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// PUT /api/resources/:resourceId
router.put('/:resourceId', async (req: Request, res: Response) => {
  try {
    const { resourceId } = req.params
    const result = await proxyToBackend(`/api/resources/${resourceId}`, req, {
      method: 'PUT',
      body: JSON.stringify(req.body),
    })
    res.json(result)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
