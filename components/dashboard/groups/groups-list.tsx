'use client';

import Link from 'next/link';
import { Group } from '@/lib/types';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ArrowRight } from 'lucide-react';

interface GroupsListProps {
  groups: Group[];
}

export function GroupsList({ groups }: GroupsListProps) {
  if (groups.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-12 text-center">
        <p className="text-muted-foreground">No groups found</p>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border bg-card">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead className="font-semibold">Name</TableHead>
            <TableHead className="font-semibold">Description</TableHead>
            <TableHead className="text-right font-semibold">Members</TableHead>
            <TableHead className="text-right font-semibold">Resources</TableHead>
            <TableHead className="text-right font-semibold">Created</TableHead>
            <TableHead className="text-right font-semibold">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {groups.map((group) => (
            <TableRow key={group.id}>
              <TableCell className="font-medium">{group.name}</TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {group.description}
              </TableCell>
              <TableCell className="text-right text-sm">
                {group.memberCount}
              </TableCell>
              <TableCell className="text-right text-sm">
                {/* Will show count of resources this group has access to */}
                {group.memberCount > 0 ? '-' : '0'}
              </TableCell>
              <TableCell className="text-right text-sm text-muted-foreground">
                {group.createdAt}
              </TableCell>
              <TableCell className="text-right">
                <Link href={`/dashboard/groups/${group.id}`}>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="gap-2"
                  >
                    View Details
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
