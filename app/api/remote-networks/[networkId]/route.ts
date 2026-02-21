import { NextResponse } from 'next/server';
import { getRemoteNetworkDetail } from '@/lib/data';

export const runtime = 'nodejs';

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ networkId: string }> }
) {
  const { networkId } = await params;
  const data = getRemoteNetworkDetail(networkId);
  return NextResponse.json(data);
}
