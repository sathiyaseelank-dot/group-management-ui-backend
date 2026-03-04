import type { Request } from 'express';

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8081';
const ADMIN_AUTH_TOKEN = process.env.ADMIN_AUTH_TOKEN || '';

export async function proxyToBackend<T = any>(
  path: string,
  req: Request,
  options: RequestInit = {}
): Promise<T> {
  const url = `${BACKEND_URL}${path}`;

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string> | undefined),
  };
  if (ADMIN_AUTH_TOKEN) {
    headers['Authorization'] = `Bearer ${ADMIN_AUTH_TOKEN}`;
  }
  if (req.headers.cookie) {
    headers.cookie = req.headers.cookie;
  }

  const response = await fetch(url, {
    ...options,
    headers,
  });

  if (!response.ok) {
    const error = await response.text();
    throw new Error(error || `Backend error: ${response.status}`);
  }

  const text = await response.text();
  if (!text) return null as T;
  return JSON.parse(text) as T;
}

export { BACKEND_URL };
