'use client';

import { RemoteNetwork } from '@/lib/types';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Globe, ShieldCheck, ShieldAlert } from 'lucide-react';
import { Label } from '@/components/ui/label';

interface RemoteNetworkInfoSectionProps {
  network: RemoteNetwork;
}

function formatDate(value: string | undefined): string {
  if (!value) return '—';
  // Unix timestamp stored as text (seconds)
  if (/^\d+$/.test(value)) {
    const d = new Date(parseInt(value, 10) * 1000);
    if (!isNaN(d.getTime())) return d.toLocaleString();
  }
  // ISO 8601 with T
  let d = new Date(value);
  if (!isNaN(d.getTime())) return d.toLocaleString();
  // PostgreSQL "2026-03-11 10:15:30+00" — replace space with T
  d = new Date(value.replace(' ', 'T'));
  if (!isNaN(d.getTime())) return d.toLocaleString();
  return value;
}

export function RemoteNetworkInfoSection({ network }: RemoteNetworkInfoSectionProps) {
  const allOnline = network.onlineConnectorCount === network.connectorCount;
  const noneOnline = network.onlineConnectorCount === 0;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Globe className="h-5 w-5" />
          Network Details
        </CardTitle>
        <CardDescription>
          Information about this remote network.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="flex flex-col space-y-1.5">
          <Label htmlFor="name">Name</Label>
          <p id="name" className="font-semibold">{network.name}</p>
        </div>
        <div className="flex flex-col space-y-1.5">
          <Label htmlFor="location">Location</Label>
          <p id="location"><Badge variant="outline" className="font-mono">{network.location}</Badge></p>
        </div>
        <div className="flex flex-col space-y-1.5">
          <Label htmlFor="connectors">Connectors</Label>
          <div id="connectors" className="flex items-center gap-2">
            {allOnline ? (
              <ShieldCheck className="h-5 w-5 text-green-500" />
            ) : noneOnline ? (
              <ShieldAlert className="h-5 w-5 text-destructive" />
            ) : (
              <ShieldAlert className="h-5 w-5 text-yellow-500" />
            )}
            <span className="font-semibold">
              {network.onlineConnectorCount} / {network.connectorCount} Online
            </span>
          </div>
        </div>
        <div className="flex flex-col space-y-1.5">
          <Label htmlFor="resources">Resources</Label>
          <p id="resources" className="font-semibold">{network.resourceCount} resources</p>
        </div>
        <div className="flex flex-col space-y-1.5">
          <Label htmlFor="id">Network ID</Label>
          <p id="id" className="font-mono text-xs text-muted-foreground">{network.id}</p>
        </div>
        <div className="flex flex-col space-y-1.5">
          <Label htmlFor="created">Created</Label>
          <p id="created" className="text-sm text-muted-foreground">{formatDate(network.createdAt)}</p>
        </div>
      </CardContent>
    </Card>
  );
}
