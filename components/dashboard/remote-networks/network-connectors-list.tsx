'use client';

import { Connector } from '@/lib/types';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { Plug, CircleDotDashed, CircleDot, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import Link from 'next/link';

interface NetworkConnectorsListProps {
  connectors: Connector[];
  remoteNetworkId: string;
  onAddConnectorClick: () => void;
}

export function NetworkConnectorsList({ connectors, remoteNetworkId, onAddConnectorClick }: NetworkConnectorsListProps) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2">
            <Plug className="h-5 w-5" />
            Connectors
          </CardTitle>
          <CardDescription>
            Connectors deployed in this remote network.
          </CardDescription>
        </div>
        <Button variant="outline" size="sm" className="gap-2" onClick={onAddConnectorClick}>
          <Plus className="h-4 w-4" />
          Add Connector
        </Button>
      </CardHeader>
      <CardContent>
        {connectors.length === 0 ? (
          <div className="text-center py-8 border border-dashed rounded-lg">
            <p className="text-muted-foreground">No connectors found for this network.</p>
            <Button variant="link" onClick={onAddConnectorClick}>Add Connector</Button>
          </div>
        ) : (
          <div className="overflow-hidden rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Version</TableHead>
                  <TableHead>Hostname</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {connectors.map((connector) => (
                  <TableRow key={connector.id}>
                    <TableCell className="font-medium">
                      <Link href={`/dashboard/connectors/${connector.id}`} className="hover:underline">
                        {connector.name}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline" className="gap-1">
                        {connector.status === 'online' ? (
                          <CircleDot className="h-3 w-3 fill-green-500 text-green-500" />
                        ) : (
                          <CircleDotDashed className="h-3 w-3 fill-muted-foreground text-muted-foreground" />
                        )}
                        {connector.status === 'online' ? 'Online' : 'Offline'}
                      </Badge>
                    </TableCell>
                    <TableCell>{connector.version}</TableCell>
                    <TableCell className="font-mono text-xs">{connector.hostname}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
