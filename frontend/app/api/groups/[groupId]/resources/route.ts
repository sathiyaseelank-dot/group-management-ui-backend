import { NextResponse } from 'next/server';

export const runtime = 'nodejs';

export async function POST(
  req: Request,
  { params }: { params: Promise<{ groupId: string }> }
) {
  // Backend doesn't have an endpoint to add resources to a group
  // The inverse operation exists: /api/admin/resources/{id}/assign_principal
  // For now, return ok - the frontend will need to use the resource's access rules instead
  return NextResponse.json({ ok: true, message: 'Use resource access rules instead' });
}
