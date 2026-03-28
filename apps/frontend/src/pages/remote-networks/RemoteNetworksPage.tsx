import { useEffect, useState } from 'react';
import { getRemoteNetworks } from '@/lib/mock-api';
import { RemoteNetwork } from '@/lib/types';
import { RemoteNetworksList } from '@/components/dashboard/remote-networks/remote-networks-list';
import { Loader2, Plus, Globe, Network } from 'lucide-react';
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
      <div className="flex items-center justify-center p-16">
        <div className="flex flex-col items-center gap-3">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
          <p className="text-xs text-muted-foreground font-mono tracking-wider">Loading networks...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-primary/10 ring-1 ring-primary/20">
            <Globe className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h1 className="font-display text-xl font-bold uppercase tracking-wide">Remote Networks</h1>
            <p className="text-xs text-muted-foreground mt-0.5">
              Secure connectivity to remote networks via connectors
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5 rounded-lg bg-muted/60 px-3 py-1.5 ring-1 ring-border/30">
            <Network className="h-3 w-3 text-primary/70" />
            <span className="text-[11px] font-mono text-muted-foreground">{networks.length} networks</span>
          </div>
          <Button size="sm" className="gap-2 font-display font-semibold uppercase tracking-wider text-[12px]" onClick={() => setIsAddModalOpen(true)}>
            <Plus className="h-4 w-4" />
            Add Network
          </Button>
        </div>
      </div>

      {/* Remote Networks List */}
      <RemoteNetworksList networks={networks} onNetworkDeleted={loadNetworks} />

      <AddRemoteNetworkModal
        isOpen={isAddModalOpen}
        onClose={() => setIsAddModalOpen(false)}
        onNetworkAdded={loadNetworks}
      />
    </div>
  );
}
