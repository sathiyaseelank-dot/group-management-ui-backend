import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'

const router = Router()

// GET /api/subjects
router.get('/', async (req: Request, res: Response) => {
  try {
    const typeParam = req.query.type as string | undefined
    const subjects: any[] = []

    if (!typeParam || typeParam === 'USER') {
      const users = await proxyToBackend<any[]>('/api/admin/users')
      users.forEach((u: any) => {
        const id = u.id ?? u.ID
        const name = u.name ?? u.Name ?? ''
        subjects.push({
          id,
          name,
          type: 'USER',
          displayLabel: `User: ${name || id || 'Unknown'}`,
        })
      })
    }

    if (!typeParam || typeParam === 'GROUP') {
      const groups = await proxyToBackend<any[]>('/api/admin/user-groups')
      groups.forEach((g: any) => {
        const id = g.id ?? g.ID
        const name = g.name ?? g.Name ?? ''
        subjects.push({
          id,
          name,
          type: 'GROUP',
          displayLabel: `Group: ${name || id || 'Unknown'}`,
        })
      })
    }

    if (!typeParam || typeParam === 'SERVICE') {
      try {
        const services = await proxyToBackend<any[]>('/api/admin/service-accounts')
        services.forEach((s: any) => {
          const id = s.id ?? s.ID
          const name = s.name ?? s.Name ?? ''
          subjects.push({
            id,
            name,
            type: 'SERVICE',
            displayLabel: `Service: ${name || id || 'Unknown'}`,
          })
        })
      } catch {
        // Service accounts endpoint doesn't exist, skip
      }
    }

    res.json(subjects)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

export default router
