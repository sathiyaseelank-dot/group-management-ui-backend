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
