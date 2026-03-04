import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

export async function GET() {
  try {
    // Get all resources with their assigned principals
    const resources = await proxyToBackend<any[]>('/api/admin/resources');
    
    // Transform to access rules format
    // The backend returns resources with Authorizations (principals)
    const accessRules: any[] = [];
    
    for (const resource of resources) {
      if (resource.Authorizations) {
        for (const auth of resource.Authorizations) {
          accessRules.push({
            id: `rule_${resource.ID}_${auth.PrincipalSPIFFE}`,
            name: `${auth.PrincipalSPIFFE} access to ${resource.Name}`,
            resourceId: resource.ID,
            allowedGroups: [auth.PrincipalSPIFFE], // This is actually SPIFFE ID, not group ID
            enabled: true,
            createdAt: resource.CreatedAt,
            updatedAt: resource.UpdatedAt || resource.CreatedAt,
          });
        }
      }
    }
    
    return NextResponse.json(accessRules);
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}

export async function POST(req: Request) {
  try {
    const body = await req.json();
    const { resourceId, name, groupIds, enabled } = body;
    
    if (!resourceId || !name || !Array.isArray(groupIds)) {
      return NextResponse.json({ error: 'resourceId, name, and groupIds are required' }, { status: 400 });
    }
    
    // For each group, assign it to the resource
    // The backend uses SPIFFE IDs, but we'll use group IDs as identifiers
    for (const groupId of groupIds) {
      try {
        await proxyToBackend(`/api/admin/resources/${resourceId}/assign_principal`, {
          method: 'POST',
          body: JSON.stringify({
            principal_spiffe: groupId, // Using group ID as SPIFFE ID substitute
            filters: [],
          }),
        });
      } catch (e) {
        // Continue even if one fails
      }
    }
    
    return NextResponse.json({ ok: true });
  } catch (error) {
    return NextResponse.json({ error: (error as Error).message }, { status: 500 });
  }
}
