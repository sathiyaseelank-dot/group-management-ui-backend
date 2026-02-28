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

interface BackendGroup {
  id?: string;
  ID?: string;
}

interface BackendGroupMember {
  user_id?: string;
  userId?: string;
}

function mapBackendUser(user: BackendUser, groups: string[]) {
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
    groups,
    createdAt,
    type: 'USER',
    displayLabel: `User: ${name || email || 'Unknown'}`,
  };
}

export async function GET() {
  try {
    const [users, groups] = await Promise.all([
      proxyToBackend<BackendUser[]>('/api/admin/users'),
      proxyToBackend<BackendGroup[]>('/api/admin/user-groups'),
    ]);

    const membershipMap = new Map<string, Set<string>>();
    await Promise.all(
      groups.map(async (group) => {
        const groupId = group.id ?? group.ID;
        if (!groupId) return;
        const members = await proxyToBackend<BackendGroupMember[]>(
          `/api/admin/user-groups/${encodeURIComponent(groupId)}/members`
        );
        members.forEach((member) => {
          const userId = member.user_id ?? member.userId;
          if (!userId) return;
          if (!membershipMap.has(userId)) {
            membershipMap.set(userId, new Set());
          }
          membershipMap.get(userId)?.add(groupId);
        });
      })
    );

    const formattedUsers = users.map((user) => {
      const userId = user.id ?? user.ID ?? '';
      const groupsForUser = Array.from(membershipMap.get(userId) ?? []);
      return mapBackendUser(user, groupsForUser);
    });
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
    return NextResponse.json(mapBackendUser(user as BackendUser, []));
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
