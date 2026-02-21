'use client';

import { useEffect, useState } from 'react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import { getRemoteNetwork } from '@/lib/mock-api';
import { RemoteNetwork, Connector, Resource } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Loader2, ArrowLeft, Plus, RefreshCw } from 'lucide-react';
import { RemoteNetworkInfoSection } from '@/components/dashboard/remote-networks/remote-network-info-section';
import { NetworkConnectorsList } from '@/components/dashboard/remote-networks/network-connectors-list';
import { NetworkResourcesList } from '@/components/dashboard/remote-networks/network-resources-list';
import { AddConnectorModal } from '@/components/dashboard/remote-networks/add-connector-modal';
import { AddResourceModal } from '@/components/dashboard/resources/add-resource-modal';

export default function RemoteNetworkDetailPage() {
  const { networkId } = useParams();
  const [network, setNetwork] = useState<RemoteNetwork | null>(null);
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [resources, setResources] = useState<Resource[]>([]);
  const [loading, setLoading] = useState(true);
  const [isAddConnectorModalOpen, setIsAddConnectorModalOpen] = useState(false);
  const [isAddResourceModalOpen, setIsAddResourceModalOpen] = useState(false);

  const loadNetworkData = async () => {
    setLoading(true);
    try {
      const { network: fetchedNetwork, connectors: fetchedConnectors, resources: fetchedResources } = await getRemoteNetwork(networkId as string);
      setNetwork(fetchedNetwork);
      setConnectors(fetchedConnectors);
      setResources(fetchedResources);
    } catch (error) {
      console.error('Failed to load remote network details:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (networkId) {
      loadNetworkData();
    }
  }, [networkId]);

  if (loading) {
    return (
      <div className="flex items-center justify-center p-12">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!network) {
    return (
      <div className="space-y-4 p-6">
        <Link href="/dashboard/remote-networks">
          <Button variant="ghost" className="gap-2">
            <ArrowLeft className="h-4 w-4" />
            Back to Remote Networks
          </Button>
        </Link>
        <p className="text-muted-foreground">Remote network not found</p>
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      {/* Breadcrumb & Header */}
      <div className="flex items-center justify-between">
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Link href="/dashboard/remote-networks" className="hover:text-foreground">
              Remote Networks
            </Link>
            <span>/</span>
            <span>{network.name}</span>
          </div>
          <h1 className="text-2xl font-bold">{network.name}</h1>
        </div>
        <div className="flex gap-2">
          {/* Action buttons will be placed within their respective cards */}
        </div>
      </div>

      {/* Network Info Section */}
      <RemoteNetworkInfoSection network={network} />

      {/* Connectors List */}
      <NetworkConnectorsList
        connectors={connectors}
        remoteNetworkId={network.id}
        onAddConnectorClick={() => setIsAddConnectorModalOpen(true)}
      />
      
      {/* Resources List */}
      <NetworkResourcesList
        resources={resources}
        remoteNetworkId={network.id}
        onAddResourceClick={() => setIsAddResourceModalOpen(true)}
      />

      {/* Add Connector Modal */}
      <AddConnectorModal
        isOpen={isAddConnectorModalOpen}
        onClose={() => setIsAddConnectorModalOpen(false)}
        onConnectorAdded={loadNetworkData}
        remoteNetworkId={network.id}
      />

      {/* Add Resource Modal */}
      <AddResourceModal
        isOpen={isAddResourceModalOpen}
        onClose={() => setIsAddResourceModalOpen(false)}
        onResourceAdded={loadNetworkData}
        defaultNetworkId={network.id}
      />
    </div>
  );
}
