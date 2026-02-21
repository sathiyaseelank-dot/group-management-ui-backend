import { NextResponse } from 'next/server';
import { updateGroupMembers } from '@/lib/data';

export const runtime = 'nodejs';

export async function POST(
  req: Request,
  { params }: { params: Promise<{ groupId: string }> }
) {
  const { groupId } = await params;
  const body = await req.json();
  if (!Array.isArray(body?.memberIds)) {
    return NextResponse.json({ error: 'memberIds must be an array' }, { status: 400 });
  }
  updateGroupMembers(groupId, body.memberIds);
  return NextResponse.json({ ok: true });
}
