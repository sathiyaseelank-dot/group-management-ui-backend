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
  members?: number;
  Members?: number;
  resource_count?: number;
  ResourceCount?: number;
  created_at?: string;
  CreatedAt?: string;
  updated_at?: string;
  UpdatedAt?: string;
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
    memberCount: group.members ?? group.Members ?? 0,
    resourceCount: group.resource_count ?? group.ResourceCount ?? 0,
    createdAt: group.created_at ?? group.CreatedAt ?? '',
    updatedAt: group.updated_at ?? group.UpdatedAt ?? '',
  };
}

export async function GET() {
  try {
    const groups = await proxyToBackend<BackendGroup[]>('/api/admin/user-groups');
    return NextResponse.json(groups.map(mapBackendGroup));
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function POST(req: Request) {
  try {
    const body = await req.json();
    const group = await proxyToBackend<BackendGroup>('/api/admin/user-groups', {
      method: 'POST',
      body: JSON.stringify(body),
    });
    return NextResponse.json(mapBackendGroup(group));
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
