import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ networkId: string }> }
) {
  try {
    const { networkId } = await params;
    const network = await proxyToBackend(`/api/remote-networks/${networkId}`);
    return NextResponse.json(network);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
