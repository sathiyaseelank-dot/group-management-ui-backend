import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function GET() {
  try {
    const connectors = await proxyToBackend('/api/connectors');
    return NextResponse.json(connectors);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function POST(req: Request) {
  try {
    const body = await req.json();
    const connector = await proxyToBackend('/api/connectors', {
      method: 'POST',
      body: JSON.stringify(body),
    });
    return NextResponse.json(connector);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
