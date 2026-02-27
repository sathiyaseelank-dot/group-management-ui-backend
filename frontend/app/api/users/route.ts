import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

interface BackendUser {
  id?: string;
  ID?: string;
  name?: string;
  Name?: string;
  email?: string;
  Email?: string;
  status?: string;
  Status?: string;
  certificate_identity?: string;
  CertificateIdentity?: string;
  created_at?: string;
  CreatedAt?: string;
  updated_at?: string;
  UpdatedAt?: string;
}

function mapBackendUser(user: BackendUser) {
  const name = user.name ?? user.Name ?? '';
  const email = user.email ?? user.Email ?? '';
  const status = (user.status ?? user.Status ?? 'active').toLowerCase();
  const certificateIdentity = user.certificate_identity ?? user.CertificateIdentity ?? undefined;
  const createdAt = user.created_at ?? user.CreatedAt ?? '';

  return {
    id: user.id ?? user.ID ?? '',
    name,
    email,
    status,
    certificateIdentity,
    groups: [],
    createdAt,
    type: 'USER',
    displayLabel: `User: ${name || email || 'Unknown'}`,
  };
}

export async function GET() {
  try {
    const users = await proxyToBackend<BackendUser[]>('/api/admin/users');
    const formattedUsers = users.map(mapBackendUser);
    return NextResponse.json(formattedUsers);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function POST(req: Request) {
  try {
    const body = await req.json();
    const user = await proxyToBackend('/api/admin/users', {
      method: 'POST',
      body: JSON.stringify(body),
    });
    return NextResponse.json(mapBackendUser(user as BackendUser));
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
