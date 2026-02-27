import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

interface BackendGroup {
  id?: string;
  ID?: string;
  name?: string;
  Name?: string;
  description?: string;
  Description?: string;
  created_at?: string;
  CreatedAt?: string;
  updated_at?: string;
  UpdatedAt?: string;
}

interface GroupMember {
  user_id?: string;
  UserID?: string;
  name?: string;
  UserName?: string;
  email?: string;
  Email?: string;
}

function mapBackendGroup(group: BackendGroup) {
  const name = group.name ?? group.Name ?? '';
  const description = group.description ?? group.Description ?? '';
  return {
    id: group.id ?? group.ID ?? '',
    name,
    description,
    type: 'GROUP',
    displayLabel: `Group: ${name || 'Unknown'}`,
    memberCount: 0,
    resourceCount: 0,
    createdAt: group.created_at ?? group.CreatedAt ?? '',
    updatedAt: group.updated_at ?? group.UpdatedAt ?? '',
  };
}

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ groupId: string }> }
) {
  try {
    const { groupId } = await params;
    const payload = await proxyToBackend(`/api/groups/${groupId}`);
    return NextResponse.json(payload);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function PUT(
  req: Request,
  { params }: { params: Promise<{ groupId: string }> }
) {
  try {
    const { groupId } = await params;
    const body = await req.json();
    const group = await proxyToBackend(`/api/admin/user-groups/${groupId}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    });
    return NextResponse.json(group);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function DELETE(
  _req: Request,
  { params }: { params: Promise<{ groupId: string }> }
) {
  try {
    const { groupId } = await params;
    const result = await proxyToBackend(`/api/admin/user-groups/${groupId}`, {
      method: 'DELETE',
    });
    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
