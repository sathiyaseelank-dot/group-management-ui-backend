import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ connectorId: string }> }
) {
  try {
    const { connectorId } = await params;
    const connector = await proxyToBackend(`/api/connectors/${connectorId}`);
    return NextResponse.json(connector);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function DELETE(
  _req: Request,
  { params }: { params: Promise<{ connectorId: string }> }
) {
  try {
    const { connectorId } = await params;
    const result = await proxyToBackend(`/api/admin/connectors/${connectorId}`, {
      method: 'DELETE',
    });
    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
