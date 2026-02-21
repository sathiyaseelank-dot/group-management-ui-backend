'use client';

import { Resource } from '@/lib/types';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Database, ExternalLink, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import Link from 'next/link';

interface NetworkResourcesListProps {
  resources: Resource[];
  remoteNetworkId: string;
  onAddResourceClick: () => void;
}

export function NetworkResourcesList({ resources, remoteNetworkId, onAddResourceClick }: NetworkResourcesListProps) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2">
            <Database className="h-5 w-5" />
            Resources
          </CardTitle>
          <CardDescription>
            Resources available in this remote network.
          </CardDescription>
        </div>
        <Button variant="outline" size="sm" className="gap-2" onClick={onAddResourceClick}>
          <Plus className="h-4 w-4" />
          Add Resource
        </Button>
      </CardHeader>
      <CardContent>
        {resources.length === 0 ? (
          <div className="text-center py-8 border border-dashed rounded-lg">
            <p className="text-muted-foreground">No resources found for this network.</p>
            <Button variant="link" onClick={onAddResourceClick}>Add Resource</Button>
          </div>
        ) : (
          <div className="overflow-hidden rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Address</TableHead>
                  <TableHead>Description</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {resources.map((resource) => (
                  <TableRow key={resource.id}>
                    <TableCell className="font-medium">{resource.name}</TableCell>
                    <TableCell className="font-mono text-xs">{resource.address}</TableCell>
                    <TableCell>{resource.description}</TableCell>
                    <TableCell className="text-right">
                      <Link href={`/dashboard/resources/${resource.id}`}>
                        <Button variant="ghost" size="sm" className="gap-2">
                          Manage Access
                          <ExternalLink className="h-4 w-4" />
                        </Button>
                      </Link>
                    </TableCell>
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
