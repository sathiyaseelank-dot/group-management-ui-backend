import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function DELETE(
  _req: Request,
  { params }: { params: Promise<{ groupId: string; userId: string }> }
) {
  try {
    const { groupId, userId } = await params;
    const result = await proxyToBackend(`/api/admin/user-groups/${groupId}/members`, {
      method: 'DELETE',
      body: JSON.stringify({ user_id: userId }),
    });
    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
