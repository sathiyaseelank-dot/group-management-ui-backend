import { NextResponse } from 'next/server';
import { addResource, listResources } from '@/lib/data';

export const runtime = 'nodejs';

export async function GET() {
  const resources = listResources();
  return NextResponse.json(resources);
}

export async function POST(req: Request) {
  const body = await req.json();
  const required = ['network_id', 'name', 'type', 'address', 'ports'];
  const missing = required.filter((key) => !(key in (body ?? {})));
  if (missing.length > 0) {
    return NextResponse.json({ error: `Missing fields: ${missing.join(', ')}` }, { status: 400 });
  }
  addResource(body);
  return NextResponse.json({ ok: true });
}
