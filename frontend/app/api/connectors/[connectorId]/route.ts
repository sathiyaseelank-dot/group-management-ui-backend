import { NextResponse } from 'next/server';
import { getConnectorDetail } from '@/lib/data';

export const runtime = 'nodejs';

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ connectorId: string }> }
) {
  const { connectorId } = await params;
  const data = getConnectorDetail(connectorId);
  return NextResponse.json(data);
}
