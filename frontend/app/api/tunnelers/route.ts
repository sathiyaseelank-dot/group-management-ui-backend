import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

interface BackendAdminTunneler {
  id: string;
  status: 'ONLINE' | 'OFFLINE' | string;
  connector_id: string;
  last_seen: string;
}

export async function GET() {
  try {
    const tunnelers = await proxyToBackend<BackendAdminTunneler[]>('/api/admin/tunnelers');
    const formatted = (Array.isArray(tunnelers) ? tunnelers : []).map((t) => ({
      id: t.id,
      name: t.id,
      status: String(t.status || '').toLowerCase() === 'online' ? 'online' : 'offline',
      version: '—',
      hostname: '—',
      remoteNetworkId: '',
    }));
    return NextResponse.json(formatted);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
