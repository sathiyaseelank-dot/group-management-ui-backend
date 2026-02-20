'use client';

import { User } from '@/lib/types';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

interface UsersListProps {
  users: User[];
}

export function UsersList({ users }: UsersListProps) {
  if (users.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-12 text-center">
        <p className="text-muted-foreground">No users found</p>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border bg-card">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead className="font-semibold">Name</TableHead>
            <TableHead className="font-semibold">Email</TableHead>
            <TableHead className="font-semibold">Type</TableHead>
            <TableHead className="font-semibold">Status</TableHead>
            <TableHead className="font-semibold">Groups</TableHead>
            <TableHead className="text-right font-semibold">Created</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {users.map((user) => (
            <TableRow key={user.id}>
              <TableCell className="font-medium">{user.name}</TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {user.email}
              </TableCell>
              <TableCell>
                <Badge variant="outline">{user.type}</Badge>
              </TableCell>
              <TableCell>
                <Badge
                  variant={user.status === 'active' ? 'default' : 'secondary'}
                >
                  {user.status}
                </Badge>
              </TableCell>
              <TableCell className="text-sm">
                {user.groups.length > 0
                  ? `${user.groups.length} group(s)`
                  : 'No groups'}
              </TableCell>
              <TableCell className="text-right text-sm text-muted-foreground">
                {user.createdAt}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
