import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

interface BackendRemoteNetwork {
  id?: string
  ID?: string
  name?: string
  Name?: string
  location?: string
  Location?: string
  connectorCount?: number
  ConnectorCount?: number
  onlineConnectorCount?: number
  OnlineConnectorCount?: number
  resourceCount?: number
  ResourceCount?: number
  createdAt?: string
  CreatedAt?: string
  updatedAt?: string
  UpdatedAt?: string
  created_at?: string
  updated_at?: string
}

function mapBackendNetwork(n: BackendRemoteNetwork) {
  const createdAt = n.createdAt ?? n.CreatedAt ?? n.created_at ?? ''
  const updatedAt = n.updatedAt ?? n.UpdatedAt ?? n.updated_at ?? ''
  return {
    id: n.id ?? n.ID ?? '',
    name: n.name ?? n.Name ?? '',
    location: (n.location ?? n.Location ?? 'OTHER') as
      | 'AWS'
      | 'GCP'
      | 'AZURE'
      | 'ON_PREM'
      | 'OTHER',
    connectorCount: n.connectorCount ?? n.ConnectorCount ?? 0,
    onlineConnectorCount: n.onlineConnectorCount ?? n.OnlineConnectorCount ?? 0,
    resourceCount: n.resourceCount ?? n.ResourceCount ?? 0,
    createdAt,
    updatedAt: updatedAt || createdAt,
  }
}

// GET /api/remote-networks
router.get('/', async (_req: Request, res: Response) => {
  try {
    const networks = await proxyToBackend<BackendRemoteNetwork[]>('/api/remote-networks')
    res.json(networks.map(mapBackendNetwork))
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/remote-networks
router.post('/', async (req: Request, res: Response) => {
  try {
    const network = await proxyToBackend<BackendRemoteNetwork>('/api/remote-networks', {
      method: 'POST',
      body: JSON.stringify(req.body),
    })
    res.json(mapBackendNetwork(network))
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/remote-networks/:networkId
router.get('/:networkId', async (req: Request, res: Response) => {
  try {
    const { networkId } = req.params
    const network = await proxyToBackend(`/api/remote-networks/${networkId}`)
    res.json(network)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
