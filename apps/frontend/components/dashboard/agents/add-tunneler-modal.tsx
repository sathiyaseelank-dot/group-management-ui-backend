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

interface AddTunnelerModalProps {
  isOpen: boolean;
  onClose: () => void;
  onTunnelerAdded: () => void;
}

export function AddTunnelerModal({
  isOpen,
  onClose,
  onTunnelerAdded,
}: AddTunnelerModalProps) {
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
      setError('Tunneler name is required.');
      return;
    }
    setIsAdding(true);
    try {
      await addAgent({
        name: trimmedName,
        connectorId: connectorId || undefined,
        remoteNetworkId: remoteNetworkId || undefined,
      });
      onTunnelerAdded();
      onClose();
      setName('');
      setConnectorId('');
      setRemoteNetworkId('');
      setError(null);
    } catch (e) {
      console.error('Failed to add tunneler:', e);
      setError((e as Error).message || 'Failed to add tunneler.');
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
          <DialogTitle>Add Tunneler</DialogTitle>
          <DialogDescription>
            Register a tunneler and optionally assign it to a connector and remote network.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label htmlFor="tunnelerName">Name</Label>
            <Input
              id="tunnelerName"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g., AWS Prod Tunneler"
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
            {isAdding ? 'Adding...' : 'Add Tunneler'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
