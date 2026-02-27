import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function GET() {
  try {
    const networks = await proxyToBackend('/api/admin/remote-networks');
    return NextResponse.json(networks);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function POST(req: Request) {
  try {
    const body = await req.json();
    const network = await proxyToBackend('/api/admin/remote-networks', {
      method: 'POST',
      body: JSON.stringify(body),
    });
    return NextResponse.json(network);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
