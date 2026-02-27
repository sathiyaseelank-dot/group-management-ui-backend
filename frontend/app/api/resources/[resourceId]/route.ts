import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ resourceId: string }> }
) {
  try {
    const { resourceId } = await params;
    
    // Get all resources and find the one we want
    const resources = await proxyToBackend<any[]>('/api/resources');
    const resource = Array.isArray(resources)
      ? resources.find((r: any) => (r.id ?? r.ID) === resourceId)
      : undefined;
    
    if (!resource) {
      return NextResponse.json({ error: 'Resource not found' }, { status: 404 });
    }
    
    // Transform to frontend format
    const formattedResource = {
      id: resource.id ?? resource.ID,
      name: resource.name ?? resource.Name,
      type: resource.type ?? resource.Type,
      address: resource.address ?? resource.Address,
      ports: resource.ports ?? resource.Ports ?? '',
      alias: resource.alias ?? resource.Alias,
      description: resource.description ?? resource.Description ?? '',
      remoteNetworkId: resource.remoteNetworkId ?? resource.remote_network_id ?? resource.RemoteNetwork,
      protocol: resource.protocol ?? resource.Protocol ?? 'TCP',
      portFrom: resource.portFrom ?? resource.port_from ?? resource.PortFrom,
      portTo: resource.portTo ?? resource.port_to ?? resource.PortTo,
    };
    
    // Get access rules (principals) for this resource
    const accessRules = [];
    if (resource.Authorizations) {
      for (const auth of resource.Authorizations) {
        accessRules.push({
          id: `rule_${formattedResource.id}_${auth.PrincipalSPIFFE}`,
          name: `${auth.PrincipalSPIFFE} access`,
          resourceId: formattedResource.id,
          allowedGroups: [auth.PrincipalSPIFFE],
          enabled: true,
          createdAt: resource.CreatedAt ?? resource.created_at ?? '',
          updatedAt: resource.UpdatedAt ?? resource.updated_at ?? resource.CreatedAt ?? '',
        });
      }
    }
    
    return NextResponse.json({
      resource: formattedResource,
      accessRules,
    });
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function PUT(
  req: Request,
  { params }: { params: Promise<{ resourceId: string }> }
) {
  try {
    const { resourceId } = await params;
    const body = await req.json();
    const result = await proxyToBackend(`/api/resources/${resourceId}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    });
    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
