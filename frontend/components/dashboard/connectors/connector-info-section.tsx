'use client';

import { Connector, RemoteNetwork } from '@/lib/types';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Plug, CircleDot, CircleDotDashed, Globe } from 'lucide-react';
import { Label } from '@/components/ui/label';
import { Button } from '@/components/ui/button';
import Link from 'next/link';

interface ConnectorInfoSectionProps {
  connector: Connector;
  network?: RemoteNetwork;
}

export function ConnectorInfoSection({ connector, network }: ConnectorInfoSectionProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Plug className="h-5 w-5" />
          Connector Details
        </CardTitle>
        <CardDescription>
          Information about this connector.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="flex flex-col space-y-1.5">
          <Label htmlFor="name">Name</Label>
          <p id="name" className="font-semibold">{connector.name}</p>
        </div>
        <div className="flex flex-col space-y-1.5">
          <Label htmlFor="status">Status</Label>
          <p id="status">
            <Badge variant="outline" className="gap-1">
              {connector.status === 'online' ? (
                <CircleDot className="h-3 w-3 fill-green-500 text-green-500" />
              ) : (
                <CircleDotDashed className="h-3 w-3 fill-muted-foreground text-muted-foreground" />
              )}
              {connector.status === 'online' ? 'Online' : 'Offline'}
            </Badge>
          </p>
        </div>
        <div className="flex flex-col space-y-1.5">
          <Label htmlFor="version">Version</Label>
          <p id="version" className="font-semibold">{connector.version}</p>
        </div>
        <div className="flex flex-col space-y-1.5">
          <Label htmlFor="lastSeen">Last Seen</Label>
          <p id="lastSeen" className="text-sm text-muted-foreground">
            {new Date(connector.lastSeen).toLocaleString()}
          </p>
        </div>
        <div className="flex flex-col space-y-1.5">
          <Label htmlFor="network">Remote Network</Label>
          {network ? (
            <Link href={`/dashboard/remote-networks/${network.id}`}>
              <Button variant="link" className="p-0 h-auto justify-start gap-1">
                <Globe className="h-3 w-3" />
                {network.name}
              </Button>
            </Link>
          ) : (
            <p className="text-muted-foreground text-sm">N/A</p>
          )}
        </div>
        <div className="flex flex-col space-y-1.5 col-span-2">
          <Label htmlFor="hostname">Hostname</Label>
          <p id="hostname" className="font-mono text-xs text-muted-foreground">{connector.hostname}</p>
        </div>
      </CardContent>
    </Card>
  );
}
