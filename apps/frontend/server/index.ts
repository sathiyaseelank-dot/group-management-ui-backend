import express, { Request, Response } from 'express'
import cors from 'cors'
import compression from 'compression'
import path from 'path'
import { getBackendUrl } from '../lib/proxy'

import groupsRouter from './routes/groups'
import usersRouter from './routes/users'
import resourcesRouter from './routes/resources'
import connectorsRouter from './routes/connectors'
import remoteNetworksRouter from './routes/remote-networks'
import accessRulesRouter from './routes/access-rules'
import subjectsRouter from './routes/subjects'
import tokensRouter from './routes/tokens'
import serviceAccountsRouter from './routes/service-accounts'
import agentsRouter from './routes/agents'
import policyRouter from './routes/policy'
import auditLogsRouter from './routes/audit-logs'
import discoveryRouter from './routes/discovery'
import agentDiscoveryRouter from './routes/agent-discovery'
import workspacesRouter from './routes/workspaces'
import diagnosticsRouter from './routes/diagnostics'
import deviceTrustedProfilesRouter from './routes/device-trusted-profiles'
import devicePostureRouter from './routes/device-posture'
import devicesRouter from './routes/devices'

const app = express()

app.use(cors())
app.use(compression())
app.use(express.json())

app.use('/api/groups', groupsRouter)
app.use('/api/users', usersRouter)
app.use('/api/resources', resourcesRouter)
app.use('/api/connectors', connectorsRouter)
app.use('/api/remote-networks', remoteNetworksRouter)
app.use('/api/access-rules', accessRulesRouter)
app.use('/api/subjects', subjectsRouter)
app.use('/api/tokens', tokensRouter)
app.use('/api/service-accounts', serviceAccountsRouter)
app.use('/api/agents', agentsRouter)
app.use('/api/policy', policyRouter)
app.use('/api/audit-logs', auditLogsRouter)
app.use('/api/discovery', discoveryRouter)
app.use('/api/agent-discovery', agentDiscoveryRouter)
app.use('/api/workspaces', workspacesRouter)
app.use('/api/diagnostics', diagnosticsRouter)
app.use('/api/device-trusted-profiles', deviceTrustedProfilesRouter)
app.use('/api/device-posture', devicePostureRouter)
app.use('/api/devices', devicesRouter)

// POST /api/auth/logout — forwards to controller OAuth logout, then signals client to clear token
app.post('/api/auth/logout', async (_req: Request, res: Response) => {
  try {
    await fetch(`${getBackendUrl()}/oauth/logout`, { method: 'POST' })
  } catch {
    // Best-effort
  }
  res.json({ status: 'logged out' })
})

// Serve built Vite app in production
if (process.env.NODE_ENV === 'production') {
  const dist = path.resolve(__dirname, '../dist')
  app.use(express.static(dist))
  app.get('*', (_req, res) => res.sendFile(path.join(dist, 'index.html')))
}

const PORT = process.env.PORT || 3001
app.listen(PORT, () => {
  console.log(`Express BFF server running on :${PORT}`)
})
