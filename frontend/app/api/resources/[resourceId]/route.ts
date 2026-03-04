import { NextResponse } from 'next/server';
import { proxyToBackend } from '@/lib/proxy';

export const runtime = 'nodejs';

function mapResource(resource: any) {
  return {
    id: resource?.id ?? resource?.ID ?? '',
    name: resource?.name ?? resource?.Name ?? '',
    type: resource?.type ?? resource?.Type ?? '',
    address: resource?.address ?? resource?.Address ?? '',
    ports: resource?.ports ?? resource?.Ports ?? '',
    alias: resource?.alias ?? resource?.Alias,
    description: resource?.description ?? resource?.Description ?? '',
    remoteNetworkId: resource?.remoteNetworkId ?? resource?.remote_network_id ?? resource?.RemoteNetwork,
    protocol: resource?.protocol ?? resource?.Protocol ?? 'TCP',
    portFrom: resource?.portFrom ?? resource?.port_from ?? resource?.PortFrom,
    portTo: resource?.portTo ?? resource?.port_to ?? resource?.PortTo,
  };
}

function mapAccessRule(rule: any) {
  return {
    id: rule?.id ?? rule?.ID ?? '',
    name: rule?.name ?? rule?.Name ?? '',
    resourceId: rule?.resourceId ?? rule?.resource_id ?? rule?.ResourceID ?? '',
    allowedGroups: rule?.allowedGroups ?? rule?.allowed_groups ?? rule?.AllowedGroups ?? [],
    enabled: rule?.enabled ?? rule?.Enabled ?? false,
    createdAt: rule?.createdAt ?? rule?.created_at ?? rule?.CreatedAt ?? '',
    updatedAt: rule?.updatedAt ?? rule?.updated_at ?? rule?.UpdatedAt ?? '',
  };
}

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ resourceId: string }> }
) {
  try {
    const { resourceId } = await params;

    const payload = await proxyToBackend<any>(`/api/resources/${resourceId}`);
    const resource = payload?.resource ?? payload?.Resource ?? payload?.resource;
    const accessRules = Array.isArray(payload?.accessRules)
      ? payload.accessRules
      : Array.isArray(payload?.AccessRules)
        ? payload.AccessRules
        : [];

    if (!resource) {
      return NextResponse.json({ error: 'Resource not found' }, { status: 404 });
    }

    return NextResponse.json({
      resource: mapResource(resource),
      accessRules: accessRules.map(mapAccessRule),
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
