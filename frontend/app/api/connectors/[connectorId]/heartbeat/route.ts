import { NextResponse } from 'next/server';
import { simulateConnectorHeartbeat } from '@/lib/data';

export const runtime = 'nodejs';

export async function POST(
  _req: Request,
  { params }: { params: Promise<{ connectorId: string }> }
) {
  const { connectorId } = await params;
  simulateConnectorHeartbeat(connectorId);
  return NextResponse.json({ ok: true });
}
