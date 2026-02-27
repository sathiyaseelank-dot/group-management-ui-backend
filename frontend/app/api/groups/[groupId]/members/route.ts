import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ groupId: string }> }
) {
  try {
    const { groupId } = await params;
    const members = await proxyToBackend(`/api/admin/user-groups/${groupId}/members`);
    return NextResponse.json(members);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function POST(
  req: Request,
  { params }: { params: Promise<{ groupId: string }> }
) {
  try {
    const { groupId } = await params;
    const body = await req.json();
    if (Array.isArray(body?.memberIds)) {
      const results: unknown[] = [];
      for (const memberId of body.memberIds) {
        if (!memberId) continue;
        const result = await proxyToBackend(`/api/admin/user-groups/${groupId}/members`, {
          method: 'POST',
          body: JSON.stringify({ user_id: memberId }),
        });
        results.push(result);
      }
      return NextResponse.json({ status: 'ok', added: results.length });
    }

    if (body?.user_id) {
      const result = await proxyToBackend(`/api/admin/user-groups/${groupId}/members`, {
        method: 'POST',
        body: JSON.stringify(body),
      });
      return NextResponse.json(result);
    }

    return NextResponse.json({ error: 'user_id required' }, { status: 400 });
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function DELETE(
  req: Request,
  { params }: { params: Promise<{ groupId: string }> }
) {
  try {
    const { groupId } = await params;
    const body = await req.json();
    const result = await proxyToBackend(`/api/admin/user-groups/${groupId}/members`, {
      method: 'DELETE',
      body: JSON.stringify(body),
    });
    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
