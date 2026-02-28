import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ connectorId: string }> }
) {
  try {
    const { connectorId } = await params;
    const policy = await proxyToBackend(`/api/policy/compile/${connectorId}`);
    return NextResponse.json(policy);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
