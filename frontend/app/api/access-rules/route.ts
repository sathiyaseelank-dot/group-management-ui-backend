import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function GET() {
  try {
    return NextResponse.json([]);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function POST(req: Request) {
  try {
    const body = await req.json();
    const { resourceId, name, groupIds, enabled } = body;
    
    if (!resourceId || !name || !Array.isArray(groupIds)) {
      return NextResponse.json({ error: 'resourceId, name, and groupIds are required' }, { status: 400 });
    }

    const rule = await proxyToBackend('/api/access-rules', {
      method: 'POST',
      body: JSON.stringify({ resourceId, name, groupIds, enabled }),
    });

    return NextResponse.json(rule);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
