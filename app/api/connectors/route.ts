import { NextResponse } from 'next/server';
import { addConnector, listConnectors } from '@/lib/data';

export const runtime = 'nodejs';

export async function GET() {
  const connectors = listConnectors();
  return NextResponse.json(connectors);
}

export async function POST(req: Request) {
  const body = await req.json();
  if (!body?.name || !body?.remoteNetworkId) {
    return NextResponse.json({ error: 'name and remoteNetworkId are required' }, { status: 400 });
  }
  addConnector({ name: body.name, remoteNetworkId: body.remoteNetworkId });
  return NextResponse.json({ ok: true });
}
