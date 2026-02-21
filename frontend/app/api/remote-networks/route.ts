import { NextResponse } from 'next/server';
import { addRemoteNetwork, listRemoteNetworks } from '@/lib/data';

export const runtime = 'nodejs';

export async function GET() {
  const networks = listRemoteNetworks();
  return NextResponse.json(networks);
}

export async function POST(req: Request) {
  const body = await req.json();
  if (!body?.name || !body?.location) {
    return NextResponse.json({ error: 'name and location are required' }, { status: 400 });
  }
  addRemoteNetwork({ name: body.name, location: body.location });
  return NextResponse.json({ ok: true });
}
