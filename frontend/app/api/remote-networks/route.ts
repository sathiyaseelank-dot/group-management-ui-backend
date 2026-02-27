import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

interface BackendRemoteNetwork {
  id?: string;
  ID?: string;
  name?: string;
  Name?: string;
  location?: string;
  Location?: string;
  connectorCount?: number;
  ConnectorCount?: number;
  onlineConnectorCount?: number;
  OnlineConnectorCount?: number;
  resourceCount?: number;
  ResourceCount?: number;
  createdAt?: string;
  CreatedAt?: string;
  updatedAt?: string;
  UpdatedAt?: string;
  created_at?: string;
  updated_at?: string;
}

function mapBackendNetwork(n: BackendRemoteNetwork) {
  const createdAt = n.createdAt ?? n.CreatedAt ?? n.created_at ?? '';
  const updatedAt = n.updatedAt ?? n.UpdatedAt ?? n.updated_at ?? '';
  return {
    id: n.id ?? n.ID ?? '',
    name: n.name ?? n.Name ?? '',
    location: (n.location ?? n.Location ?? 'OTHER') as
      | 'AWS'
      | 'GCP'
      | 'AZURE'
      | 'ON_PREM'
      | 'OTHER',
    connectorCount: n.connectorCount ?? n.ConnectorCount ?? 0,
    onlineConnectorCount: n.onlineConnectorCount ?? n.OnlineConnectorCount ?? 0,
    resourceCount: n.resourceCount ?? n.ResourceCount ?? 0,
    createdAt,
    updatedAt: updatedAt || createdAt,
  };
}

export async function GET() {
  try {
    const networks = await proxyToBackend<BackendRemoteNetwork[]>('/api/remote-networks');
    return NextResponse.json(networks.map(mapBackendNetwork));
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function POST(req: Request) {
  try {
    const body = await req.json();
    const network = await proxyToBackend<BackendRemoteNetwork>('/api/remote-networks', {
      method: 'POST',
      body: JSON.stringify(body),
    });
    return NextResponse.json(mapBackendNetwork(network));
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
