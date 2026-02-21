import { NextResponse } from 'next/server';
import { listTunnelers } from '@/lib/data';

export const runtime = 'nodejs';

export async function GET() {
  const tunnelers = listTunnelers();
  return NextResponse.json(tunnelers);
}
