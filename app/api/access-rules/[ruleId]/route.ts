import { NextResponse } from 'next/server';
import { deleteAccessRule } from '@/lib/data';

export const runtime = 'nodejs';

export async function DELETE(
  _req: Request,
  { params }: { params: Promise<{ ruleId: string }> }
) {
  const { ruleId } = await params;
  deleteAccessRule(ruleId);
  return NextResponse.json({ ok: true });
}
