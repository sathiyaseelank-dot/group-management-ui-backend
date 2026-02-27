import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

interface BackendResource {
  ID: string;
  Name: string;
  Type: string;
  Address: string;
  Ports?: string;
  Alias?: string;
  Description?: string;
  RemoteNetwork?: string;
  Protocol?: string;
  PortFrom?: number;
  PortTo?: number;
  Authorizations?: any[];
}

export async function GET() {
  try {
    const resources = await proxyToBackend<any[]>('/api/resources');
    
    // Handle different response formats from backend
    let resourceList: any[] = [];
    if (Array.isArray(resources)) {
      resourceList = resources;
    } else if (resources?.Resources) {
      resourceList = resources.Resources;
    }
    
    // Transform to frontend format
    const formatted = resourceList.map((r: any) => ({
      id: r.id ?? r.ID,
      name: r.name ?? r.Name,
      type: r.type ?? r.Type,
      address: r.address ?? r.Address,
      ports: r.ports ?? r.Ports ?? '',
      alias: r.alias ?? r.Alias,
      description: r.description ?? r.Description ?? '',
      remoteNetworkId: r.remoteNetworkId ?? r.remote_network_id ?? r.RemoteNetwork,
      protocol: r.protocol ?? r.Protocol ?? 'TCP',
      portFrom: r.portFrom ?? r.port_from ?? r.PortFrom,
      portTo: r.portTo ?? r.port_to ?? r.PortTo,
    }));
    
    return NextResponse.json(formatted);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function POST(req: Request) {
  try {
    const body = await req.json();
    const resource = await proxyToBackend('/api/resources', {
      method: 'POST',
      body: JSON.stringify(body),
    });
    return NextResponse.json(resource);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
