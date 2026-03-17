'use client';

import { useEffect, useMemo, useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { addAgent, getConnectors, getRemoteNetworks } from '@/lib/mock-api';
import { Connector, RemoteNetwork } from '@/lib/types';

interface AddAgentModalProps {
  isOpen: boolean;
  onClose: () => void;
  onAgentAdded: () => void;
}

export function AddAgentModal({
  isOpen,
  onClose,
  onAgentAdded,
}: AddAgentModalProps) {
  const [name, setName] = useState('');
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [connectorId, setConnectorId] = useState<string>('');
  const [remoteNetworks, setRemoteNetworks] = useState<RemoteNetwork[]>([]);
  const [remoteNetworkId, setRemoteNetworkId] = useState<string>('');
  const [loadingData, setLoadingData] = useState(false);
  const [isAdding, setIsAdding] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const trimmedName = useMemo(() => name.trim(), [name]);

  const filteredConnectors = useMemo(
    () => connectors.filter((c) => c.remoteNetworkId === remoteNetworkId),
    [connectors, remoteNetworkId]
  );

  useEffect(() => {
    if (!isOpen) return;
    setError(null);
    setLoadingData(true);
    Promise.all([getConnectors(), getRemoteNetworks()])
      .then(([connList, netList]) => {
        setConnectors(connList);
        setRemoteNetworks(netList);
      })
      .catch((e) => {
        console.error('Failed to load data:', e);
        setConnectors([]);
        setRemoteNetworks([]);
      })
      .finally(() => setLoadingData(false));
  }, [isOpen]);

  const handleAdd = async () => {
    setError(null);
    if (!trimmedName) {
      setError('Agent name is required.');
      return;
    }
    setIsAdding(true);
    try {
      await addAgent({
        name: trimmedName,
        connectorId: connectorId || undefined,
        remoteNetworkId: remoteNetworkId || undefined,
      });
      onAgentAdded();
      onClose();
      setName('');
      setConnectorId('');
      setRemoteNetworkId('');
      setError(null);
    } catch (e) {
      console.error('Failed to add agent:', e);
      setError((e as Error).message || 'Failed to add agent.');
    } finally {
      setIsAdding(false);
    }
  };

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>Add Agent</DialogTitle>
          <DialogDescription>
            Register an agent and optionally assign it to a connector and remote network.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label htmlFor="agentName">Name</Label>
            <Input
              id="agentName"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g., AWS Prod Agent"
            />
          </div>

          <div className="grid gap-2">
            <Label>Remote Network</Label>
            <Select
              value={remoteNetworkId || '__none__'}
              onValueChange={(v) => {
                const next = v === '__none__' ? '' : v;
                setRemoteNetworkId(next);
                setConnectorId('');
              }}
              disabled={loadingData}
            >
              <SelectTrigger className="w-full">
                <SelectValue
                  placeholder={
                    loadingData ? 'Loading networks...' : 'Select a remote network'
                  }
                />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__none__">None</SelectItem>
                {remoteNetworks.map((n) => (
                  <SelectItem key={n.id} value={n.id}>
                    {n.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {remoteNetworkId && (
            <div className="grid gap-2">
              <Label>Connector</Label>
              <Select
                value={connectorId || '__none__'}
                onValueChange={(v) => setConnectorId(v === '__none__' ? '' : v)}
                disabled={loadingData}
              >
                <SelectTrigger className="w-full">
                  <SelectValue
                    placeholder={
                      loadingData
                        ? 'Loading connectors...'
                        : filteredConnectors.length === 0
                          ? 'No connectors in this network'
                          : 'Select a connector (optional)'
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__none__">None</SelectItem>
                  {filteredConnectors.map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {c.name}
                      {c.privateIp ? (
                        <span className="ml-2 text-muted-foreground font-mono text-xs">
                          ({c.privateIp})
                        </span>
                      ) : null}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {!loadingData && filteredConnectors.length === 0 && (
                <p className="text-xs text-muted-foreground">
                  No connectors found in this remote network.
                </p>
              )}
            </div>
          )}

          {error && (
            <p className="text-sm text-destructive" role="alert">
              {error}
            </p>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={isAdding}>
            Cancel
          </Button>
          <Button onClick={handleAdd} disabled={isAdding || !trimmedName}>
            {isAdding ? 'Adding...' : 'Add Agent'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
