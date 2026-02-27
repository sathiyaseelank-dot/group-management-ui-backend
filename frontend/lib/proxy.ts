const BACKEND_URL = process.env.NEXT_PUBLIC_API_BASE_URL || 'http://localhost:8081';
const ADMIN_AUTH_TOKEN = process.env.ADMIN_AUTH_TOKEN || '7f8e91a2b3c4d5e6f7a8b9c0d1e2f3a4';

export async function proxyToBackend<T = any>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const url = `${BACKEND_URL}${path}`;
  
  const response = await fetch(url, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${ADMIN_AUTH_TOKEN}`,
      ...options.headers,
    },
  });

  if (!response.ok) {
    const error = await response.text();
    throw new Error(error || `Backend error: ${response.status}`);
  }

  return response.json();
}

export { BACKEND_URL, ADMIN_AUTH_TOKEN };
