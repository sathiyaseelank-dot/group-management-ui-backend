import { NextResponse } from 'next/server';
import { getResourceDetail, updateResource } from '@/lib/data';

export const runtime = 'nodejs';

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ resourceId: string }> }
) {
  const { resourceId } = await params;
  const data = getResourceDetail(resourceId);
  return NextResponse.json(data);
}

export async function PUT(
  req: Request,
  { params }: { params: Promise<{ resourceId: string }> }
) {
  const { resourceId } = await params;
  const body = await req.json();
  const required = ['network_id', 'name', 'type', 'address', 'ports'];
  const missing = required.filter((key) => !(key in (body ?? {})));
  if (missing.length > 0) {
    return NextResponse.json({ error: `Missing fields: ${missing.join(', ')}` }, { status: 400 });
  }
  updateResource(resourceId, body);
  return NextResponse.json({ ok: true });
}
