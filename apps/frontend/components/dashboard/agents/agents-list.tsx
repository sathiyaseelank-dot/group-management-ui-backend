'use client';

import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Agent } from '@/lib/types';
import { deleteAgent } from '@/lib/mock-api';
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
import { ArrowRight, Cable, CircleDotDashed, CircleDot, Loader2, Ban, Trash2 } from 'lucide-react';
import { toast } from 'sonner';

interface AgentsListProps {
  agents: Agent[];
  onRevoked?: (id: string) => void;
}

export function AgentsList({ agents, onRevoked }: AgentsListProps) {
  const [revokingId, setRevokingId] = useState<string | null>(null);

  const handleRevoke = async (agent: Agent) => {
    if (!window.confirm(`Delete agent "${agent.name}"? This will remove it from the controller.`)) return;
    setRevokingId(agent.id);
    try {
      await deleteAgent(agent.id);
      toast.success(`Agent "${agent.name}" deleted.`);
      onRevoked?.(agent.id);
    } catch {
      toast.error('Failed to delete agent. Check that the backend is running.');
    } finally {
      setRevokingId(null);
    }
  };

  if (agents.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-12 text-center">
        <p className="text-muted-foreground">No agents found</p>
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
            <TableHead className="font-semibold">Remote Network</TableHead>
            <TableHead className="text-right font-semibold">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {agents.map((agent) => (
            <TableRow key={agent.id}>
              <TableCell className="font-medium">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-full bg-muted">
                    <Cable className="h-4 w-4 text-muted-foreground" />
                  </div>
                  <span>{agent.name}</span>
                </div>
              </TableCell>
              <TableCell>
                <Badge variant="outline" className="gap-1">
                  {agent.status === 'revoked' ? (
                    <Ban className="h-3 w-3 text-red-500" />
                  ) : agent.status === 'online' ? (
                    <CircleDot className="h-3 w-3 fill-green-500 text-green-500" />
                  ) : (
                    <CircleDotDashed className="h-3 w-3 fill-muted-foreground text-muted-foreground" />
                  )}
                  {agent.status === 'revoked' ? 'Revoked' : agent.status === 'online' ? 'Online' : 'Offline'}
                </Badge>
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {agent.version || '—'}
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {agent.hostname || '—'}
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {agent.remoteNetworkId ? (
                  <Link to={`/dashboard/remote-networks/${agent.remoteNetworkId}`}>
                    <Button variant="link" size="sm" className="px-0">
                      {agent.remoteNetworkId}
                    </Button>
                  </Link>
                ) : (
                  '—'
                )}
              </TableCell>
              <TableCell className="text-right">
                <div className="flex items-center justify-end gap-2">
                  <Link to={`/dashboard/agents/${agent.id}`}>
                    <Button variant="ghost" size="sm" className="gap-2">
                      Manage
                      <ArrowRight className="h-4 w-4" />
                    </Button>
                  </Link>
                  <Button
                    variant="destructive"
                    size="sm"
                    className="gap-1"
                    disabled={revokingId === agent.id}
                    onClick={() => handleRevoke(agent)}
                  >
                    {revokingId === agent.id ? (
                      <Loader2 className="h-3 w-3 animate-spin" />
                    ) : (
                      <Trash2 className="h-3 w-3" />
                    )}
                    Delete
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
