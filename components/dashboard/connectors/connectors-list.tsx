'use client';

import Link from 'next/link';
import { Connector } from '@/lib/types';
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
import { ArrowRight, Plug, CircleDotDashed, CircleDot } from 'lucide-react';

interface ConnectorsListProps {
  connectors: Connector[];
}

export function ConnectorsList({ connectors }: ConnectorsListProps) {
  if (connectors.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-12 text-center">
        <p className="text-muted-foreground">No connectors found</p>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border bg-card">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead className="font-semibold">Name</TableHead>
            <TableHead className="font-semibold">Status</TableHead>
            <TableHead className="font-semibold">Version</TableHead>
            <TableHead className="font-semibold">Hostname</TableHead>
            <TableHead className="text-right font-semibold">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {connectors.map((connector) => (
            <TableRow key={connector.id}>
              <TableCell className="font-medium">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-full bg-muted">
                    <Plug className="h-4 w-4 text-muted-foreground" />
                  </div>
                  <span>{connector.name}</span>
                </div>
              </TableCell>
              <TableCell>
                <Badge variant="outline" className="gap-1">
                  {!connector.installed ? (
                    <CircleDotDashed className="h-3 w-3 fill-muted-foreground text-muted-foreground" />
                  ) : connector.status === 'online' ? (
                    <CircleDot className="h-3 w-3 fill-green-500 text-green-500" />
                  ) : (
                    <CircleDotDashed className="h-3 w-3 fill-muted-foreground text-muted-foreground" />
                  )}
                  {!connector.installed
                    ? 'Not installed'
                    : connector.status === 'online'
                      ? 'Online'
                      : 'Offline'}
                </Badge>
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {connector.installed ? connector.version : '—'}
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {connector.installed ? connector.hostname : '—'}
              </TableCell>
              <TableCell className="text-right">
                <Link href={`/dashboard/connectors/${connector.id}`}>
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
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
