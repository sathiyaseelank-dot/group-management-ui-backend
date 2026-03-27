import { Router, Request, Response } from 'express'
import { getBackendUrl } from '../../lib/proxy'

const router = Router()

router.get('/', async (_req: Request, res: Response) => {
  try {
    const response = await fetch(`${getBackendUrl()}/api/controller/config`)
    const data = await response.json()
    res.json(data)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
