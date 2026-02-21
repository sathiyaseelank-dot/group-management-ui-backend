import { NextResponse } from 'next/server';
import { addGroupResources } from '@/lib/data';

export const runtime = 'nodejs';

export async function POST(
  req: Request,
  { params }: { params: Promise<{ groupId: string }> }
) {
  const { groupId } = await params;
  const body = await req.json();
  if (!Array.isArray(body?.resourceIds)) {
    return NextResponse.json({ error: 'resourceIds must be an array' }, { status: 400 });
  }
  addGroupResources(groupId, body.resourceIds);
  return NextResponse.json({ ok: true });
}
