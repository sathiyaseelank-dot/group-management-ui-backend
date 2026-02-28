import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function POST() {
  try {
    const token = await proxyToBackend('/api/admin/tokens', {
      method: 'POST',
    });
    return NextResponse.json(token);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
