import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function GET() {
  try {
    const connectors = await proxyToBackend('/api/admin/connectors');
    return NextResponse.json(connectors);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
