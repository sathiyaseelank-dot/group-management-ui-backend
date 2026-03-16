'use client';

import { useState } from 'react';
import { Link } from 'react-router-dom';
import { RemoteNetwork } from '@/lib/types';
import { deleteRemoteNetwork } from '@/lib/mock-api';
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
import { ArrowRight, Globe, Loader2, MoreVertical, ShieldCheck, ShieldAlert, Trash2 } from 'lucide-react';
import { toast } from 'sonner';

interface RemoteNetworksListProps {
  networks: RemoteNetwork[];
  onNetworkDeleted?: () => void;
}

function formatDate(value: string | undefined): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

export function RemoteNetworksList({ networks, onNetworkDeleted }: RemoteNetworksListProps) {
  const [deleteTarget, setDeleteTarget] = useState<RemoteNetwork | null>(null);
  const [deleting, setDeleting] = useState(false);

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await deleteRemoteNetwork(deleteTarget.id);
      toast.success(`Remote network "${deleteTarget.name}" deleted.`);
      onNetworkDeleted?.();
    } catch (error) {
      console.error('Failed to delete remote network:', error);
      toast.error('Failed to delete remote network.');
    } finally {
      setDeleting(false);
      setDeleteTarget(null);
    }
  };

  if (networks.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-12 text-center">
        <p className="text-muted-foreground">No remote networks found</p>
      </div>
    );
  }

  return (
    <>
      <div className="overflow-hidden rounded-lg border bg-card">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="font-semibold">Network Name</TableHead>
              <TableHead className="font-semibold">Location</TableHead>
              <TableHead className="text-center font-semibold">Connectors</TableHead>
              <TableHead className="text-center font-semibold">Resources</TableHead>
              <TableHead className="text-right font-semibold">Updated</TableHead>
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
                    {formatDate(network.updatedAt)}
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
                        <Link to={`/dashboard/remote-networks/${network.id}`}>
                          <DropdownMenuItem>
                            <ArrowRight className="mr-2 h-4 w-4" />
                            Manage
                          </DropdownMenuItem>
                        </Link>
                        <DropdownMenuItem
                          className="text-destructive focus:text-destructive"
                          onClick={() => setDeleteTarget(network)}
                        >
                          <Trash2 className="mr-2 h-4 w-4" />
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </div>

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Remote Network</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete <strong>{deleteTarget?.name}</strong>? All connectors,
              agents, resources, and access rules in this network will be permanently removed.
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
