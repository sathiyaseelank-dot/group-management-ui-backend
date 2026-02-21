import { NextResponse } from 'next/server';
import { createAccessRule } from '@/lib/data';

export const runtime = 'nodejs';

export async function POST(req: Request) {
  const body = await req.json();
  if (!body?.resourceId || !Array.isArray(body?.subjects) || !body?.effect) {
    return NextResponse.json({ error: 'resourceId, subjects, and effect are required' }, { status: 400 });
  }
  createAccessRule(body.resourceId, body.subjects, body.effect);
  return NextResponse.json({ ok: true });
}
