'use client';

import Link from 'next/link';
import { Resource, RemoteNetwork } from '@/lib/types';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ArrowRight, Database, Globe, MoreVertical } from 'lucide-react';

interface ResourcesListProps {
  resources: Resource[];
  remoteNetworks: RemoteNetwork[];
  onEdit: (resource: Resource) => void;
}

export function ResourcesList({ resources, remoteNetworks, onEdit }: ResourcesListProps) {
  const handleDelete = (resourceId: string) => {
    console.log('Delete resource:', resourceId);
    // Implement delete logic, e.g., open a confirmation dialog
  };

  const getNetworkName = (networkId: string) => {
    const network = remoteNetworks.find((net) => net.id === networkId);
    return network ? network.name : networkId;
  };

  if (resources.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-12 text-center">
        <p className="text-muted-foreground">No resources found</p>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border bg-card">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead className="font-semibold">Resource</TableHead>
            <TableHead className="font-semibold">Address</TableHead>
            <TableHead className="font-semibold">Remote Network</TableHead>
            <TableHead className="text-right font-semibold">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {resources.map((resource) => (
            <TableRow key={resource.id}>
              <TableCell className="font-medium flex items-center gap-2">
                <Database className="h-4 w-4 text-muted-foreground" />
                {resource.name}
              </TableCell>
              <TableCell className="text-sm font-mono text-muted-foreground">
                {resource.address}
              </TableCell>
              <TableCell>
                {resource.remoteNetworkId ? (
                  <Link href={`/dashboard/remote-networks/${resource.remoteNetworkId}`}>
                    <Button variant="link" size="sm" className="px-0 gap-2">
                      <Globe className="h-3 w-3" />
                      {getNetworkName(resource.remoteNetworkId)}
                    </Button>
                  </Link>
                ) : (
                  <span className="text-sm text-muted-foreground">-</span>
                )}
              </TableCell>
              <TableCell className="text-right">
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" className="h-8 w-8 p-0">
                      <span className="sr-only">Open menu</span>
                      <MoreVertical className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onClick={() => onEdit(resource)}>Edit</DropdownMenuItem>
                    <Link href={`/dashboard/resources/${resource.id}`}>
                      <DropdownMenuItem>Manage Access</DropdownMenuItem>
                    </Link>
                    <DropdownMenuItem onClick={() => handleDelete(resource.id)}>Delete</DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
