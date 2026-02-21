'use client';

import { useEffect, useState } from 'react';
import { getRemoteNetworks } from '@/lib/mock-api';
import { RemoteNetwork } from '@/lib/types';
import { RemoteNetworksList } from '@/components/dashboard/remote-networks/remote-networks-list';
import { Loader2, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { AddRemoteNetworkModal } from '@/components/dashboard/remote-networks/add-remote-network-modal';

export default function RemoteNetworksPage() {
  const [networks, setNetworks] = useState<RemoteNetwork[]>([]);
  const [loading, setLoading] = useState(true);
  const [isAddModalOpen, setIsAddModalOpen] = useState(false);

  useEffect(() => {
    loadNetworks();
  }, []);

  const loadNetworks = async () => {
    try {
      const data = await getRemoteNetworks();
      setNetworks(data);
    } catch (error) {
      console.error('Failed to load remote networks:', error);
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center p-12">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Remote Networks</h1>
          <p className="text-sm text-muted-foreground">
            Manage secure connectivity to remote networks and VPCs via Connectors
          </p>
        </div>
        <Button className="gap-2" onClick={() => setIsAddModalOpen(true)}>
          <Plus className="h-4 w-4" />
          Add Remote Network
        </Button>
      </div>

      {/* Remote Networks List */}
      <RemoteNetworksList networks={networks} />

      <AddRemoteNetworkModal
        isOpen={isAddModalOpen}
        onClose={() => setIsAddModalOpen(false)}
        onNetworkAdded={loadNetworks}
      />
    </div>
  );
}
