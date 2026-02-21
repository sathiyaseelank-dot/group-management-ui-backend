import { NextResponse } from 'next/server';
import { listUsers } from '@/lib/data';

export const runtime = 'nodejs';

export async function GET() {
  const users = listUsers();
  return NextResponse.json(users);
}
