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
    const resources = await proxyToBackend<any[]>('/api/admin/resources');
    const resource = resources.find((r: any) => r.ID === resourceId);
    
    if (!resource) {
      return NextResponse.json({ error: 'Resource not found' }, { status: 404 });
    }
    
    // Transform to frontend format
    const formattedResource = {
      id: resource.ID,
      name: resource.Name,
      type: resource.Type,
      address: resource.Address,
      ports: resource.Ports || '',
      alias: resource.Alias,
      description: resource.Description || '',
      remoteNetworkId: resource.RemoteNetwork,
      protocol: resource.Protocol || 'TCP',
      portFrom: resource.PortFrom,
      portTo: resource.PortTo,
    };
    
    // Get access rules (principals) for this resource
    const accessRules = [];
    if (resource.Authorizations) {
      for (const auth of resource.Authorizations) {
        accessRules.push({
          id: `rule_${resource.ID}_${auth.PrincipalSPIFFE}`,
          name: `${auth.PrincipalSPIFFE} access`,
          resourceId: resource.ID,
          allowedGroups: [auth.PrincipalSPIFFE],
          enabled: true,
          createdAt: resource.CreatedAt,
          updatedAt: resource.UpdatedAt || resource.CreatedAt,
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
    
    // The backend doesn't have a PUT for resources
    // We need to delete and recreate, or add an endpoint
    // For now, return not implemented
    return NextResponse.json({ error: 'Update not implemented - use backend API directly' }, { status: 501 });
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
