'use client';

import { ServiceAccount } from '@/lib/types';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

interface ServiceAccountsListProps {
  serviceAccounts: ServiceAccount[];
}

export function ServiceAccountsList({
  serviceAccounts,
}: ServiceAccountsListProps) {
  if (serviceAccounts.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-12 text-center">
        <p className="text-muted-foreground">No service accounts found</p>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border bg-card">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead className="font-semibold">Name</TableHead>
            <TableHead className="font-semibold">Type</TableHead>
            <TableHead className="font-semibold">Status</TableHead>
            <TableHead className="text-right font-semibold">Resources</TableHead>
            <TableHead className="text-right font-semibold">Created</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {serviceAccounts.map((account) => (
            <TableRow key={account.id}>
              <TableCell className="font-medium">{account.name}</TableCell>
              <TableCell>
                <Badge variant="outline">{account.type}</Badge>
              </TableCell>
              <TableCell>
                <Badge
                  variant={account.status === 'active' ? 'default' : 'secondary'}
                >
                  {account.status}
                </Badge>
              </TableCell>
              <TableCell className="text-right text-sm">
                {account.associatedResourceCount}
              </TableCell>
              <TableCell className="text-right text-sm text-muted-foreground">
                {account.createdAt}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
