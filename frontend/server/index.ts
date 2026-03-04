import express from 'express'
import cors from 'cors'
import compression from 'compression'
import path from 'path'

import groupsRouter from './routes/groups'
import usersRouter from './routes/users'
import resourcesRouter from './routes/resources'
import connectorsRouter from './routes/connectors'
import remoteNetworksRouter from './routes/remote-networks'
import accessRulesRouter from './routes/access-rules'
import subjectsRouter from './routes/subjects'
import tokensRouter from './routes/tokens'
import serviceAccountsRouter from './routes/service-accounts'
import tunnelersRouter from './routes/tunnelers'
import policyRouter from './routes/policy'

const app = express()
const BACKEND_INTERNAL_URL = process.env.BACKEND_URL || 'http://localhost:8081'
const BACKEND_PUBLIC_URL =
  process.env.BACKEND_PUBLIC_URL || process.env.CONTROLLER_PUBLIC_URL || 'http://localhost:8081'

async function backendFetch(req: express.Request, pathName: string, options: RequestInit = {}) {
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string> | undefined),
  }
  if (req.headers.cookie) {
    headers.cookie = req.headers.cookie
  }
  const response = await fetch(`${BACKEND_INTERNAL_URL}${pathName}`, {
    ...options,
    headers,
  })
  return response
}

async function isAuthenticated(req: express.Request): Promise<boolean> {
  try {
    const response = await backendFetch(req, '/api/admin/users')
    return response.ok
  } catch {
    return false
  }
}

app.use(cors())
app.use(compression())
app.use(express.json())

app.get('/api/auth/login', (req, res) => {
  const inviteToken = String(req.query.invite_token || '').trim()
  if (inviteToken) {
    return res.redirect(`${BACKEND_PUBLIC_URL}/invite?token=${encodeURIComponent(inviteToken)}`)
  }
  return res.redirect(`${BACKEND_PUBLIC_URL}/auth/google/login`)
})

app.get('/api/auth/callback', (req, res) => {
  return res.redirect('/login')
})

app.get('/api/auth/session', async (req, res) => {
  const ok = await isAuthenticated(req)
  if (!ok) {
    return res.status(401).json({ authenticated: false, error: 'unauthorized' })
  }
  return res.json({ authenticated: true })
})

app.post('/api/auth/logout', async (req, res) => {
  try {
    const out = await backendFetch(req, '/auth/logout', { method: 'POST' })
    const setCookie = out.headers.get('set-cookie')
    if (setCookie) {
      res.setHeader('Set-Cookie', setCookie)
    }
    if (!out.ok) {
      const text = await out.text()
      return res.status(out.status).json({ ok: false, error: text || 'logout failed' })
    }
    return res.json({ ok: true })
  } catch (error) {
    return res.status(500).json({ ok: false, error: (error as Error).message })
  }
})

app.use('/api', async (req, res, next) => {
  if (req.path.startsWith('/auth/')) {
    return next()
  }
  if (!(await isAuthenticated(req))) {
    return res.status(401).json({ error: 'unauthorized' })
  }
  return next()
})

app.use('/api/groups', groupsRouter)
app.use('/api/users', usersRouter)
app.use('/api/resources', resourcesRouter)
app.use('/api/connectors', connectorsRouter)
app.use('/api/remote-networks', remoteNetworksRouter)
app.use('/api/access-rules', accessRulesRouter)
app.use('/api/subjects', subjectsRouter)
app.use('/api/tokens', tokensRouter)
app.use('/api/service-accounts', serviceAccountsRouter)
app.use('/api/tunnelers', tunnelersRouter)
app.use('/api/policy', policyRouter)

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
