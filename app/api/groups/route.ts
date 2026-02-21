import { NextResponse } from 'next/server';
import { addGroup, listGroups } from '@/lib/data';

export const runtime = 'nodejs';

export async function GET() {
  const groups = listGroups();
  return NextResponse.json(groups);
}

export async function POST(req: Request) {
  const body = await req.json();
  if (!body?.name || !body?.description) {
    return NextResponse.json({ error: 'name and description are required' }, { status: 400 });
  }
  addGroup({ name: body.name, description: body.description });
  return NextResponse.json({ ok: true });
}
