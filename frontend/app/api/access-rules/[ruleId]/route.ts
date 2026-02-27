import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function DELETE(
  _req: Request,
  { params }: { params: Promise<{ ruleId: string }> }
) {
  try {
    const { ruleId } = await params;
    
    // Rule ID format: rule_{resourceId}_{principalSPIFFE}
    const parts = ruleId.replace('rule_', '').split('_');
    if (parts.length < 2) {
      return NextResponse.json({ error: 'Invalid rule ID format' }, { status: 400 });
    }
    
    const resourceId = parts[0];
    const principalSPIFFE = parts.slice(1).join('_'); // Rejoin in case SPIFFE has underscores
    
    await proxyToBackend(`/api/admin/resources/${resourceId}/assign_principal/${principalSPIFFE}`, {
      method: 'DELETE',
    });
    
    return NextResponse.json({ ok: true });
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
