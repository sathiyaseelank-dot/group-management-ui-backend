'use client';

import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Agent, Resource, RemoteNetwork, FirewallStatus } from '@/lib/types';
import { setResourceFirewallStatus } from '@/lib/mock-api';
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
import { BotOff, Database, Globe, MoreVertical, Shield, ShieldOff, Loader2 } from 'lucide-react';

interface ResourcesListProps {
  resources: Resource[];
  remoteNetworks: RemoteNetwork[];
  agents: Agent[];
  onEdit: (resource: Resource) => void;
  onDelete?: (resourceId: string) => void;
  onFirewallStatusChange?: () => void;
}

export function ResourcesList({ resources, remoteNetworks, agents, onEdit, onDelete, onFirewallStatusChange }: ResourcesListProps) {
  const [togglingId, setTogglingId] = useState<string | null>(null);

  const getNetworkName = (networkId: string) => {
    const network = remoteNetworks.find((net) => net.id === networkId);
    return network ? network.name : networkId;
  };

  const getAgentForResource = (resource: Resource): Agent | undefined => {
    return agents.find((a) => a.ip && a.ip === resource.address);
  };

  const handleFirewallToggle = async (resource: Resource) => {
    const newStatus: FirewallStatus =
      resource.firewallStatus === 'protected' ? 'unprotected' : 'protected';
    setTogglingId(resource.id);
    try {
      await setResourceFirewallStatus(resource.id, newStatus);
      onFirewallStatusChange?.();
    } catch (error) {
      console.error('Failed to update firewall status:', error);
    } finally {
      setTogglingId(null);
    }
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
            <TableHead className="font-semibold">Agent</TableHead>
            <TableHead className="font-semibold">Firewall</TableHead>
            <TableHead className="text-right font-semibold">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {resources.map((resource) => {
            const isProtected = resource.firewallStatus === 'protected';
            const isToggling = togglingId === resource.id;
            const matchedAgent = getAgentForResource(resource);

            return (
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
                    <Link to={`/dashboard/remote-networks/${resource.remoteNetworkId}`}>
                      <Button variant="link" size="sm" className="px-0 gap-2">
                        <Globe className="h-3 w-3" />
                        {getNetworkName(resource.remoteNetworkId)}
                      </Button>
                    </Link>
                  ) : (
                    <span className="text-sm text-muted-foreground">-</span>
                  )}
                </TableCell>
                <TableCell>
                  {matchedAgent ? (
                    <div className="flex items-center gap-1.5">
                      <span
                        className={`h-2 w-2 rounded-full flex-shrink-0 ${matchedAgent.status === 'online' ? 'bg-green-500' : 'bg-zinc-400'}`}
                      />
                      <span className="text-sm">{matchedAgent.name}</span>
                    </div>
                  ) : (
                    <span className="text-sm text-muted-foreground">-</span>
                  )}
                </TableCell>
                <TableCell>
                  {!matchedAgent ? (
                    <span className="inline-flex items-center gap-1.5 rounded-full bg-zinc-500/10 px-2.5 py-0.5 text-xs font-medium text-zinc-600 dark:text-zinc-400">
                      <BotOff className="h-3 w-3" />
                      No agent
                    </span>
                  ) : (
                    <div className="flex items-center gap-2">
                      {isProtected ? (
                        <span className="inline-flex items-center gap-1.5 rounded-full bg-green-500/10 px-2.5 py-0.5 text-xs font-medium text-green-700 dark:text-green-400">
                          <Shield className="h-3 w-3" />
                          Protected
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1.5 rounded-full bg-zinc-500/10 px-2.5 py-0.5 text-xs font-medium text-zinc-600 dark:text-zinc-400">
                          <ShieldOff className="h-3 w-3" />
                          Unprotected
                        </span>
                      )}
                      <Button
                        variant={isProtected ? 'outline' : 'default'}
                        size="sm"
                        className="h-7 text-xs"
                        disabled={isToggling}
                        onClick={() => handleFirewallToggle(resource)}
                      >
                        {isToggling ? (
                          <Loader2 className="h-3 w-3 animate-spin" />
                        ) : isProtected ? (
                          'Unprotect'
                        ) : (
                          'Protect'
                        )}
                      </Button>
                    </div>
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
                      <Link to={`/dashboard/resources/${resource.id}`}>
                        <DropdownMenuItem>Manage Access</DropdownMenuItem>
                      </Link>
                      <DropdownMenuItem onClick={() => onDelete?.(resource.id)}>Delete</DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}
