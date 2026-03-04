import { Router, Request, Response } from 'express'
import { proxyToBackend } from '../../lib/proxy'
import { getDb } from '../../lib/db'

const router = Router()

// GET /api/access-rules
router.get('/', async (req: Request, res: Response) => {
  try {
    const resources = await proxyToBackend<any[]>('/api/admin/resources', req)
    const accessRules: any[] = []

    for (const resource of resources) {
      if (resource.Authorizations) {
        for (const auth of resource.Authorizations) {
          accessRules.push({
            id: `rule_${resource.ID}_${auth.PrincipalSPIFFE}`,
            name: `${auth.PrincipalSPIFFE} access to ${resource.Name}`,
            resourceId: resource.ID,
            allowedGroups: [auth.PrincipalSPIFFE],
            enabled: true,
            createdAt: resource.CreatedAt,
            updatedAt: resource.UpdatedAt || resource.CreatedAt,
          })
        }
      }
    }

    res.json(accessRules)
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// POST /api/access-rules
router.post('/', async (req: Request, res: Response) => {
  try {
    const { resourceId, name, groupIds, enabled } = req.body

    if (!resourceId || !name || !Array.isArray(groupIds)) {
      return res.status(400).json({ error: 'resourceId, name, and groupIds are required' })
    }

    for (const groupId of groupIds) {
      try {
        await proxyToBackend(`/api/admin/resources/${resourceId}/assign_principal`, req, {
          method: 'POST',
          body: JSON.stringify({
            principal_spiffe: groupId,
            filters: [],
          }),
        })
      } catch {
        // Continue even if one fails
      }
    }

    res.json({ ok: true })
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// DELETE /api/access-rules/:ruleId
router.delete('/:ruleId', async (req: Request, res: Response) => {
  try {
    const { ruleId } = req.params

    const parts = ruleId.replace('rule_', '').split('_')
    if (parts.length < 2) {
      return res.status(400).json({ error: 'Invalid rule ID format' })
    }

    const resourceId = parts[0]
    const principalSPIFFE = parts.slice(1).join('_')

    await proxyToBackend(`/api/admin/resources/${resourceId}/assign_principal/${principalSPIFFE}`, req, {
      method: 'DELETE',
    })

    res.json({ ok: true })
  } catch (error) {
    res.status(500).json({ error: (error as Error).message })
  }
})

// GET /api/access-rules/:ruleId/identity-count
router.get('/:ruleId/identity-count', (req: Request, res: Response) => {
  const { ruleId } = req.params
  const db = getDb()

  const row = db.prepare(
    `SELECT COUNT(DISTINCT u.id) as count
     FROM access_rule_groups arg
     JOIN group_members gm ON gm.group_id = arg.group_id
     JOIN users u ON u.id = gm.user_id
     WHERE arg.rule_id = ? AND u.certificate_identity IS NOT NULL`
  ).get(ruleId) as { count?: number } | undefined

  res.json({ count: row?.count ?? 0 })
})

export default router
