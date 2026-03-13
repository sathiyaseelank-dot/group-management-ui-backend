'use client';

import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Connector } from '@/lib/types';
import { deleteConnector } from '@/lib/mock-api';
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { ArrowRight, CircleDot, CircleDotDashed, Loader2, MoreVertical, Plug, Trash2 } from 'lucide-react';
import { toast } from 'sonner';

interface ConnectorsListProps {
  connectors: Connector[];
  onConnectorDeleted?: () => void;
}

export function ConnectorsList({ connectors, onConnectorDeleted }: ConnectorsListProps) {
  const [deleteTarget, setDeleteTarget] = useState<Connector | null>(null);
  const [deleting, setDeleting] = useState(false);

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await deleteConnector(deleteTarget.id);
      toast.success(`Connector "${deleteTarget.name}" deleted.`);
      onConnectorDeleted?.();
    } catch (error) {
      console.error('Failed to delete connector:', error);
      toast.error('Failed to delete connector.');
    } finally {
      setDeleting(false);
      setDeleteTarget(null);
    }
  };

  if (connectors.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-12 text-center">
        <p className="text-muted-foreground">No connectors found</p>
      </div>
    );
  }

  return (
    <>
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
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" className="h-8 w-8 p-0">
                        <span className="sr-only">Open menu</span>
                        <MoreVertical className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <Link to={`/dashboard/connectors/${connector.id}`}>
                        <DropdownMenuItem>
                          <ArrowRight className="mr-2 h-4 w-4" />
                          Manage
                        </DropdownMenuItem>
                      </Link>
                      <DropdownMenuItem
                        className="text-destructive focus:text-destructive"
                        onClick={() => setDeleteTarget(connector)}
                      >
                        <Trash2 className="mr-2 h-4 w-4" />
                        Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Connector</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete <strong>{deleteTarget?.name}</strong>? All agents
              connected through this connector will also be permanently removed.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              disabled={deleting}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
