import { NextResponse } from 'next/server';

export const runtime = 'nodejs';

// Simulate heartbeat - backend doesn't have this exact endpoint
// The connector will call PATCH directly
export async function POST(
  _req: Request,
  { params }: { params: Promise<{ connectorId: string }> }
) {
  const { connectorId } = await params;
  // Return ok - actual heartbeat happens via PATCH from connector
  return NextResponse.json({ ok: true, message: 'Use PATCH for actual heartbeat' });
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
