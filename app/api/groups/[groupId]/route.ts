import { NextResponse } from 'next/server';
import { getGroupDetail } from '@/lib/data';

export const runtime = 'nodejs';

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ groupId: string }> }
) {
  const { groupId } = await params;
  const data = getGroupDetail(groupId);
  return NextResponse.json(data);
}
