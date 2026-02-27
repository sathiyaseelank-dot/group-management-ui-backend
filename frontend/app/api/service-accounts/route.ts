import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function GET() {
  try {
    const serviceAccounts = await proxyToBackend<any[]>('/api/admin/service-accounts');
    const formatted = serviceAccounts.map((s: any) => ({
      id: s.ID,
      name: s.Name,
      type: 'SERVICE',
      displayLabel: `Service: ${s.Name}`,
      status: s.Status,
      associatedResourceCount: s.AssociatedResourceCount,
      createdAt: s.CreatedAt,
    }));
    return NextResponse.json(formatted);
  } catch (error) {
    // Return empty array if endpoint doesn't exist
    return NextResponse.json([]);
  }
}
