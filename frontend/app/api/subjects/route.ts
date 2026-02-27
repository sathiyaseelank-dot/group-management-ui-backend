import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function GET(req: Request) {
  try {
    const url = new URL(req.url);
    const typeParam = url.searchParams.get('type');
    
    const subjects: any[] = [];
    
    // Get users
    if (!typeParam || typeParam === 'USER') {
      const users = await proxyToBackend<any[]>('/api/admin/users');
      users.forEach((u: any) => {
        const id = u.id ?? u.ID;
        const name = u.name ?? u.Name ?? '';
        subjects.push({
          id,
          name,
          type: 'USER',
          displayLabel: `User: ${name || id || 'Unknown'}`,
        });
      });
    }
    
    // Get groups
    if (!typeParam || typeParam === 'GROUP') {
      const groups = await proxyToBackend<any[]>('/api/admin/user-groups');
      groups.forEach((g: any) => {
        const id = g.id ?? g.ID;
        const name = g.name ?? g.Name ?? '';
        subjects.push({
          id,
          name,
          type: 'GROUP',
          displayLabel: `Group: ${name || id || 'Unknown'}`,
        });
      });
    }
    
    // Service accounts - backend might not have this
    if (!typeParam || typeParam === 'SERVICE') {
      try {
        const services = await proxyToBackend<any[]>('/api/admin/service-accounts');
        services.forEach((s: any) => {
          const id = s.id ?? s.ID;
          const name = s.name ?? s.Name ?? '';
          subjects.push({
            id,
            name,
            type: 'SERVICE',
            displayLabel: `Service: ${name || id || 'Unknown'}`,
          });
        });
      } catch (e) {
        // Service accounts endpoint doesn't exist, skip
      }
    }
    
    return NextResponse.json(subjects);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
