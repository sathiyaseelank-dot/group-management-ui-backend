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
import { addConnector, getRemoteNetworks } from '@/lib/mock-api';
import { RemoteNetwork } from '@/lib/types';

interface AddConnectorModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConnectorAdded: () => void;
}

export function AddConnectorModal({
  isOpen,
  onClose,
  onConnectorAdded,
}: AddConnectorModalProps) {
  const [name, setName] = useState('');
  const [remoteNetworks, setRemoteNetworks] = useState<RemoteNetwork[]>([]);
  const [remoteNetworkId, setRemoteNetworkId] = useState<string>('');
  const [loadingNetworks, setLoadingNetworks] = useState(false);
  const [isAdding, setIsAdding] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const trimmedName = useMemo(() => name.trim(), [name]);

  useEffect(() => {
    if (!isOpen) return;
    setError(null);
    setLoadingNetworks(true);
    getRemoteNetworks()
      .then((nets) => setRemoteNetworks(nets))
      .catch((e) => {
        console.error('Failed to load remote networks:', e);
        setRemoteNetworks([]);
      })
      .finally(() => setLoadingNetworks(false));
  }, [isOpen]);

  const handleAdd = async () => {
    setError(null);
    if (!trimmedName) {
      setError('Connector name is required.');
      return;
    }
    if (!remoteNetworkId) {
      setError('Remote network is required.');
      return;
    }
    setIsAdding(true);
    try {
      await addConnector({ name: trimmedName, remoteNetworkId });
      onConnectorAdded();
      onClose();
      setName('');
      setRemoteNetworkId('');
    } catch (e) {
      console.error('Failed to add connector:', e);
      setError((e as Error).message || 'Failed to add connector.');
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
          <DialogTitle>Add Connector</DialogTitle>
          <DialogDescription>
            Register a connector and assign it to a remote network.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label htmlFor="connectorName">Name</Label>
            <Input
              id="connectorName"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g., AWS Prod Connector"
            />
          </div>

          <div className="grid gap-2">
            <Label>Remote Network</Label>
            <Select
              value={remoteNetworkId}
              onValueChange={(v) => setRemoteNetworkId(v)}
              disabled={loadingNetworks || remoteNetworks.length === 0}
            >
              <SelectTrigger className="w-full">
                <SelectValue
                  placeholder={
                    loadingNetworks
                      ? 'Loading networks...'
                      : remoteNetworks.length === 0
                        ? 'No remote networks found'
                        : 'Select a remote network'
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {remoteNetworks.map((n) => (
                  <SelectItem key={n.id} value={n.id}>
                    {n.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

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
          <Button onClick={handleAdd} disabled={isAdding || !trimmedName || !remoteNetworkId}>
            {isAdding ? 'Adding...' : 'Add Connector'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

