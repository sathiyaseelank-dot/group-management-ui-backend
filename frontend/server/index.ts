import express from 'express'
import cors from 'cors'
import compression from 'compression'
import path from 'path'
import { Request, Response } from 'express'
import { ADMIN_AUTH_TOKEN, BACKEND_URL } from '../lib/proxy'

const app = express()

app.use(cors())
app.use(compression())
app.use(express.json())

app.use('/api', async (req: Request, res: Response) => {
  try {
    const url = `${BACKEND_URL}${req.originalUrl}`;
    const method = req.method.toUpperCase();
    const hasBody = !['GET', 'HEAD'].includes(method);

    const response = await fetch(url, {
      method,
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${ADMIN_AUTH_TOKEN}`,
      },
      body: hasBody ? JSON.stringify(req.body ?? {}) : undefined,
    });

    const contentType = response.headers.get('content-type') || 'application/json';
    const payload = await response.text();
    res.status(response.status).set('Content-Type', contentType).send(payload);
  } catch (error) {
    res.status(502).json({ error: (error as Error).message || 'Backend proxy error' });
  }
});

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
