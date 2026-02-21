'use client';

import Link from 'next/link';
import { Resource } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Database, ExternalLink, Plus } from 'lucide-react';

interface GroupResourcesSectionProps {
  groupId: string;
  resources: Resource[];
  onAddResourcesClick: () => void;
}

export function GroupResourcesSection({
  groupId,
  resources,
  onAddResourcesClick,
}: GroupResourcesSectionProps) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="space-y-1">
            <CardTitle className="flex items-center gap-2">
              <Database className="h-5 w-5" />
              Resource Access
            </CardTitle>
            <CardDescription>
              Resources this group has access to (determined by access policies)
            </CardDescription>
          </div>
          <Button variant="outline" size="sm" className="gap-2" onClick={onAddResourcesClick}>
            <Plus className="h-4 w-4" />
            Add Resources
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {resources.length === 0 ? (
          <div className="rounded-lg border border-dashed py-8 text-center">
            <p className="text-sm text-muted-foreground">
              No resource access policies configured yet
            </p>
            <p className="mt-2 text-xs text-muted-foreground">
              To grant this group resource access, configure access rules on the resource policy page
            </p>
          </div>
        ) : (
          <div className="overflow-hidden rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="font-semibold">Resource</TableHead>
                  <TableHead className="font-semibold">Address</TableHead>
                  <TableHead className="font-semibold">Description</TableHead>
                  <TableHead className="text-right font-semibold">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {resources.map((resource) => (
                  <TableRow key={resource.id}>
                    <TableCell className="font-medium">{resource.name}</TableCell>
                    <TableCell className="text-sm font-mono text-muted-foreground">
                      {resource.address}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {resource.description}
                    </TableCell>
                    <TableCell className="text-right">
                      <Link href={`/dashboard/resources/${resource.id}`}>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="gap-2"
                        >
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
