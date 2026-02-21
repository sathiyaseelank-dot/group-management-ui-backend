'use client';

import Link from 'next/link';
import { RemoteNetwork } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ArrowRight, Globe, ShieldCheck, ShieldAlert } from 'lucide-react';

interface RemoteNetworksListProps {
  networks: RemoteNetwork[];
}

export function RemoteNetworksList({ networks }: RemoteNetworksListProps) {
  if (networks.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-12 text-center">
        <p className="text-muted-foreground">No remote networks found</p>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border bg-card">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead className="font-semibold">Network Name</TableHead>
            <TableHead className="font-semibold">Location</TableHead>
            <TableHead className="text-center font-semibold">Connectors</TableHead>
            <TableHead className="text-center font-semibold">Resources</TableHead>
            <TableHead className="text-right font-semibold">Created</TableHead>
            <TableHead className="text-right font-semibold">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {networks.map((network) => {
            const allOnline = network.onlineConnectorCount === network.connectorCount;
            const noneOnline = network.onlineConnectorCount === 0;
            
            return (
              <TableRow key={network.id}>
                <TableCell className="font-medium">
                  <div className="flex items-center gap-3">
                    <div className="flex h-8 w-8 items-center justify-center rounded-full bg-muted">
                      <Globe className="h-4 w-4 text-muted-foreground" />
                    </div>
                    <div className="flex flex-col">
                      <span>{network.name}</span>
                      <span className="text-xs text-muted-foreground">ID: {network.id}</span>
                    </div>
                  </div>
                </TableCell>
                <TableCell>
                  <Badge variant="outline" className="font-mono text-[10px]">
                    {network.location}
                  </Badge>
                </TableCell>
                <TableCell className="text-center">
                  <div className="flex flex-col items-center gap-1">
                    <div className="flex items-center gap-1.5">
                      {allOnline ? (
                        <ShieldCheck className="h-4 w-4 text-green-500" />
                      ) : noneOnline ? (
                        <ShieldAlert className="h-4 w-4 text-destructive" />
                      ) : (
                        <ShieldAlert className="h-4 w-4 text-yellow-500" />
                      )}
                      <span className="text-sm font-medium">
                        {network.onlineConnectorCount} / {network.connectorCount} Online
                      </span>
                    </div>
                  </div>
                </TableCell>
                <TableCell className="text-center">
                  <Badge variant="secondary" className="px-2.5 py-0.5">
                    {network.resourceCount} Resources
                  </Badge>
                </TableCell>
                <TableCell className="text-right text-sm text-muted-foreground">
                  {network.createdAt}
                </TableCell>
                <TableCell className="text-right">
                  <Link href={`/dashboard/remote-networks/${network.id}`}>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="gap-2"
                    >
                      Manage
                      <ArrowRight className="h-4 w-4" />
                    </Button>
                  </Link>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}
