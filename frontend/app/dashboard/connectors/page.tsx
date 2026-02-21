'use client';

import { useEffect, useState } from 'react';
import { getConnectors } from '@/lib/mock-api';
import { Connector } from '@/lib/types';
import { ConnectorsList } from '@/components/dashboard/connectors/connectors-list';
import { Loader2, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';

export default function ConnectorsPage() {
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadConnectors = async () => {
      try {
        const data = await getConnectors();
        setConnectors(data);
      } catch (error) {
        console.error('Failed to load connectors:', error);
      } finally {
        setLoading(false);
      }
    };

    loadConnectors();
  }, []);

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
          <h1 className="text-2xl font-bold">Connectors</h1>
          <p className="text-sm text-muted-foreground">
            Manage network connectors that provide access to remote networks
          </p>
        </div>
        <Button className="gap-2">
          <Plus className="h-4 w-4" />
          Add Connector
        </Button>
      </div>

      {/* Connectors List */}
      <ConnectorsList connectors={connectors} />
    </div>
  );
}
