import { NextResponse } from 'next/server';
import { listServiceAccounts } from '@/lib/data';

export const runtime = 'nodejs';

export async function GET() {
  const serviceAccounts = listServiceAccounts();
  return NextResponse.json(serviceAccounts);
}
