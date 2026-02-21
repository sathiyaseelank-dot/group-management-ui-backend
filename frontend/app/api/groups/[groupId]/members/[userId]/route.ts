import { NextResponse } from 'next/server';
import { removeGroupMember } from '@/lib/data';

export const runtime = 'nodejs';

export async function DELETE(
  _req: Request,
  { params }: { params: Promise<{ groupId: string; userId: string }> }
) {
  const { groupId, userId } = await params;
  removeGroupMember(groupId, userId);
  return NextResponse.json({ ok: true });
}
