'use client';

import Link from 'next/link';
import { Resource } from '@/lib/types';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ArrowRight, Database } from 'lucide-react';

interface ResourcesListProps {
  resources: Resource[];
}

export function ResourcesList({ resources }: ResourcesListProps) {
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
            <TableHead className="font-semibold">Description</TableHead>
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
