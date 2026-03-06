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
import { listResourcePolicies, ensureDefaultResourcePolicy, ResourcePolicy } from '@/lib/resource-policies';
import { toast } from 'sonner';
import { Loader2 } from 'lucide-react';

type PortMode = 'all' | 'custom' | 'blocked';
type AccessMode = 'manual' | 'auto-lock' | 'requests';

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
  const [policies, setPolicies] = useState<ResourcePolicy[]>([]);

  // Form state
  const [networkId, setNetworkId] = useState<string>(defaultNetworkId ?? '');
  const [name, setName] = useState('');
  const [resourceType, setResourceType] = useState<ResourceType>('STANDARD');
  const [address, setAddress] = useState('');
  const [alias, setAlias] = useState('');

  // Protocol port restrictions
  const [tcpMode, setTcpMode] = useState<PortMode>('all');
  const [tcpPorts, setTcpPorts] = useState('');
  const [udpMode, setUdpMode] = useState<PortMode>('all');
  const [udpPorts, setUdpPorts] = useState('');

  // Policy & access
  const [policyId, setPolicyId] = useState('');
  const [accessMode, setAccessMode] = useState<AccessMode>('manual');

  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (isOpen) {
      const fetchNetworks = async () => {
        setLoadingNetworks(true);
        try {
          const data = await getRemoteNetworks();
          setNetworks(data);
        } catch {
          toast.error('Failed to load networks');
        } finally {
          setLoadingNetworks(false);
        }
      };
      fetchNetworks();
      if (!networkId && defaultNetworkId) setNetworkId(defaultNetworkId);

      ensureDefaultResourcePolicy();
      const loaded = listResourcePolicies();
      setPolicies(loaded);
      if (!policyId && loaded.length > 0) setPolicyId(loaded[0].id);
    }
  }, [isOpen, defaultNetworkId]);

  const resetForm = () => {
    setNetworkId(defaultNetworkId ?? '');
    setName('');
    setResourceType('STANDARD');
    setAddress('');
    setAlias('');
    setTcpMode('all');
    setTcpPorts('');
    setUdpMode('all');
    setUdpPorts('');
    setPolicyId(policies[0]?.id ?? '');
    setAccessMode('manual');
  };

  const handleClose = () => {
    resetForm();
    onClose();
  };

  const canSubmit = networkId && name && address;

  const handleSubmit = async () => {
    if (!canSubmit) return;

    // Derive the primary protocol/port for backend compatibility
    const activeTcp = tcpMode !== 'blocked';
    const protocol = activeTcp ? 'TCP' : 'UDP';

    setIsSubmitting(true);
    try {
      await addResource({
        network_id: networkId,
        name,
        type: resourceType,
        address,
        protocol,
        port_from: null,
        port_to: null,
        alias: alias || undefined,
      });
      toast.success('Resource created');
      onResourceAdded();
      handleClose();
    } catch {
      toast.error('Failed to create resource');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Create Resource</DialogTitle>
          <DialogDescription>
            Define a private service that users can securely access.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 py-4">
          {/* Network */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="network" className="text-right">Network</Label>
            <Select
              value={networkId}
              onValueChange={setNetworkId}
              disabled={loadingNetworks || lockNetwork}
            >
              <SelectTrigger className="col-span-3">
                <SelectValue placeholder={loadingNetworks ? 'Loading...' : 'Select a network'} />
              </SelectTrigger>
              <SelectContent>
                {networks.map((net) => (
                  <SelectItem key={net.id} value={net.id}>{net.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Label */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="name" className="text-right">Label</Label>
            <Input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="col-span-3"
              maxLength={120}
              placeholder="Human-readable name"
            />
          </div>

          {/* Type */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label className="text-right">Type</Label>
            <div className="col-span-3 flex w-full rounded-md border p-1">
              <Button variant={resourceType === 'STANDARD' ? 'secondary' : 'ghost'} onClick={() => setResourceType('STANDARD')} className="flex-1 text-xs">STANDARD</Button>
              <Button variant={resourceType === 'BROWSER' ? 'secondary' : 'ghost'} onClick={() => setResourceType('BROWSER')} className="flex-1 text-xs">BROWSER</Button>
              <Button variant={resourceType === 'BACKGROUND' ? 'secondary' : 'ghost'} onClick={() => setResourceType('BACKGROUND')} className="flex-1 text-xs">BACKGROUND</Button>
            </div>
          </div>

          {/* Address */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="address" className="text-right">Address</Label>
            <Input
              id="address"
              value={address}
              onChange={(e) => setAddress(e.target.value.replace(/\s/g, ''))}
              className="col-span-3"
              placeholder="db.internal.local or 10.0.0.15"
            />
          </div>

          {/* Port Restrictions */}
          <div className="grid grid-cols-4 gap-4">
            <Label className="text-right pt-2">Port Restrictions</Label>
            <div className="col-span-3 space-y-3 rounded-md border p-3 bg-muted/20">

              {/* TCP */}
              <div className="space-y-2">
                <div className="flex items-center gap-3">
                  <span className="w-10 text-sm font-medium">TCP</span>
                  <Select value={tcpMode} onValueChange={(v) => setTcpMode(v as PortMode)}>
                    <SelectTrigger className="flex-1">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All ports</SelectItem>
                      <SelectItem value="custom">Custom</SelectItem>
                      <SelectItem value="blocked">Blocked</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                {tcpMode === 'custom' && (
                  <div className="flex items-center gap-3">
                    <span className="w-10" />
                    <Input
                      value={tcpPorts}
                      onChange={(e) => setTcpPorts(e.target.value)}
                      placeholder="e.g. 80, 443, 8000-9000"
                      className="flex-1"
                    />
                  </div>
                )}
              </div>

              {/* UDP */}
              <div className="space-y-2">
                <div className="flex items-center gap-3">
                  <span className="w-10 text-sm font-medium">UDP</span>
                  <Select value={udpMode} onValueChange={(v) => setUdpMode(v as PortMode)}>
                    <SelectTrigger className="flex-1">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All ports</SelectItem>
                      <SelectItem value="custom">Custom</SelectItem>
                      <SelectItem value="blocked">Blocked</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                {udpMode === 'custom' && (
                  <div className="flex items-center gap-3">
                    <span className="w-10" />
                    <Input
                      value={udpPorts}
                      onChange={(e) => setUdpPorts(e.target.value)}
                      placeholder="e.g. 53, 500, 1194"
                      className="flex-1"
                    />
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Policy */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="policy" className="text-right">Policy</Label>
            <Select value={policyId} onValueChange={setPolicyId}>
              <SelectTrigger className="col-span-3">
                <SelectValue placeholder="Select a policy" />
              </SelectTrigger>
              <SelectContent>
                {policies.map((p) => (
                  <SelectItem key={p.id} value={p.id}>{p.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Access */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="access" className="text-right">Access</Label>
            <Select value={accessMode} onValueChange={(v) => setAccessMode(v as AccessMode)}>
              <SelectTrigger className="col-span-3">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="manual">Manual Access</SelectItem>
                <SelectItem value="auto-lock">Auto-lock</SelectItem>
                <SelectItem value="requests">Access Requests</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Alias */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="alias" className="text-right">Alias</Label>
            <Input
              id="alias"
              value={alias}
              onChange={(e) => setAlias(e.target.value)}
              className="col-span-3"
              placeholder="e.g., jira.company.com (optional)"
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
