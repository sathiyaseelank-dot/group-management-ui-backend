'use client';

import { useEffect, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { RemoteNetwork, ResourceType } from '@/lib/types';
import { getRemoteNetworks, addResource } from '@/lib/mock-api';
import { toast } from 'sonner';
import { Loader2 } from 'lucide-react';

interface AddResourceModalProps {
  isOpen: boolean;
  onClose: () => void;
  onResourceAdded: () => void;
  defaultNetworkId?: string;
  lockNetwork?: boolean;
}

export function AddResourceModal({
  isOpen,
  onClose,
  onResourceAdded,
  defaultNetworkId,
  lockNetwork = false,
}: AddResourceModalProps) {
  const [networks, setNetworks] = useState<RemoteNetwork[]>([]);
  const [loadingNetworks, setLoadingNetworks] = useState(true);

  // Form state
  const [networkId, setNetworkId] = useState<string>(defaultNetworkId ?? '');
  const [name, setName] = useState('');
  const [resourceType, setResourceType] = useState<ResourceType>('STANDARD');
  const [address, setAddress] = useState('');
  const [protocol, setProtocol] = useState<'TCP' | 'UDP'>('TCP');
  const [portFrom, setPortFrom] = useState('');
  const [portTo, setPortTo] = useState('');
  const [alias, setAlias] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (isOpen) {
      const fetchNetworks = async () => {
        setLoadingNetworks(true);
        try {
          const data = await getRemoteNetworks();
          setNetworks(data);
        } catch (error) {
          toast.error('Failed to load networks');
        } finally {
          setLoadingNetworks(false);
        }
      };
      fetchNetworks();
      if (!networkId && defaultNetworkId) {
        setNetworkId(defaultNetworkId);
      }
    }
  }, [isOpen, defaultNetworkId, networkId]);

  const resetForm = () => {
    setNetworkId(defaultNetworkId ?? '');
    setName('');
    setResourceType('STANDARD');
    setAddress('');
    setProtocol('TCP');
    setPortFrom('');
    setPortTo('');
    setAlias('');
  };

  const handleClose = () => {
    resetForm();
    onClose();
  };

  const canSubmit = networkId && name && address && protocol;

  const handleSubmit = async () => {
    if (!canSubmit) return;

    setIsSubmitting(true);
    try {
      await addResource({
        network_id: networkId,
        name,
        type: resourceType,
        address,
        protocol,
        port_from: portFrom ? Number(portFrom) : null,
        port_to: portTo ? Number(portTo) : null,
        alias: alias || undefined,
      });
      toast.success('Resource created');
      onResourceAdded();
      handleClose();
    } catch (error) {
      toast.error('Failed to create resource');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Create Resource</DialogTitle>
          <DialogDescription>
            Define a private service that users can securely access.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="network" className="text-right">
              Network
            </Label>
            <Select
              value={networkId}
              onValueChange={setNetworkId}
              disabled={loadingNetworks || lockNetwork}
            >
              <SelectTrigger className="col-span-3">
                <SelectValue placeholder={loadingNetworks ? 'Loading networks...' : 'Select a network'} />
              </SelectTrigger>
              <SelectContent>
                {networks.map((net) => (
                  <SelectItem key={net.id} value={net.id}>
                    {net.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="name" className="text-right">
              Label
            </Label>
            <Input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="col-span-3"
              maxLength={120}
              placeholder="Human-readable name"
            />
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label className="text-right">Type</Label>
            <div className="col-span-3 flex w-full rounded-md border p-1">
              <Button variant={resourceType === 'STANDARD' ? 'secondary' : 'ghost'} onClick={() => setResourceType('STANDARD')} className="flex-1 text-xs">STANDARD</Button>
              <Button variant={resourceType === 'BROWSER' ? 'secondary' : 'ghost'} onClick={() => setResourceType('BROWSER')} className="flex-1 text-xs">BROWSER</Button>
              <Button variant={resourceType === 'BACKGROUND' ? 'secondary' : 'ghost'} onClick={() => setResourceType('BACKGROUND')} className="flex-1 text-xs">BACKGROUND</Button>
            </div>
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="address" className="text-right">
              Address
            </Label>
            <Input
              id="address"
              value={address}
              onChange={(e) => setAddress(e.target.value.replace(/\s/g, ''))}
              className="col-span-3"
              placeholder="db.internal.local or 10.0.0.15"
            />
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label className="text-right">Protocol</Label>
            <Select value={protocol} onValueChange={(v) => setProtocol(v as 'TCP' | 'UDP')}>
              <SelectTrigger className="col-span-3">
                <SelectValue placeholder="Select protocol" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="TCP">TCP</SelectItem>
                <SelectItem value="UDP">UDP</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="portFrom" className="text-right">
              Port From
            </Label>
            <Input
              id="portFrom"
              value={portFrom}
              onChange={(e) => setPortFrom(e.target.value.replace(/[^0-9]/g, ''))}
              className="col-span-3"
              placeholder="e.g., 443"
              inputMode="numeric"
            />
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="portTo" className="text-right">
              Port To
            </Label>
            <Input
              id="portTo"
              value={portTo}
              onChange={(e) => setPortTo(e.target.value.replace(/[^0-9]/g, ''))}
              className="col-span-3"
              placeholder="Optional"
              inputMode="numeric"
            />
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="alias" className="text-right">
              Alias (Optional)
            </Label>
            <Input
              id="alias"
              value={alias}
              onChange={(e) => setAlias(e.target.value)}
              className="col-span-3"
              placeholder="e.g., jira.company.com"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isSubmitting}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={!canSubmit || isSubmitting}>
            {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Create Resource
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
