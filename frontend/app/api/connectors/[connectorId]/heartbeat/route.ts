import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

// Simulate heartbeat - backend doesn't have this exact endpoint
// The connector will call PATCH directly
export async function POST(
  req: Request,
  { params }: { params: Promise<{ connectorId: string }> }
) {
  try {
    const { connectorId } = await params;
    let body: unknown = undefined;
    try {
      body = await req.json();
    } catch {
      body = undefined;
    }
    const result = await proxyToBackend(`/api/connectors/${connectorId}/heartbeat`, {
      method: 'POST',
      body: body ? JSON.stringify(body) : undefined,
    });
    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

// Handle policy version updates from connectors
export async function PATCH(
  req: Request,
  { params }: { params: Promise<{ connectorId: string }> }
) {
  try {
    const { connectorId } = await params;
    const body = await req.json();
    
    if (typeof body?.last_policy_version !== 'number') {
      return NextResponse.json({ error: 'last_policy_version is required' }, { status: 400 });
    }
    
    // TODO: Actually update the connector's policy version in the database
    // For now, return that no update is available
    return NextResponse.json({
      update_available: false,
      current_version: body.last_policy_version,
    });
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
