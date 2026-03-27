import { useEffect, useState } from 'react';
import { getConnectors } from '@/lib/mock-api';
import { Connector } from '@/lib/types';
import { ConnectorsList } from '@/components/dashboard/connectors/connectors-list';
import { Loader2, Plus, Globe, Zap } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { AddConnectorModal } from '@/components/dashboard/connectors/add-connector-modal';

export default function ConnectorsPage() {
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [loading, setLoading] = useState(true);
  const [isAddOpen, setIsAddOpen] = useState(false);

  useEffect(() => {
    void loadConnectors();
  }, []);

  const loadConnectors = async () => {
    setLoading(true);
    try {
      const data = await getConnectors();
      setConnectors(data);
    } catch (error) {
      console.error('Failed to load connectors:', error);
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center p-16">
        <div className="flex flex-col items-center gap-3">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
          <p className="text-xs text-muted-foreground font-mono tracking-wider">Loading connectors...</p>
        </div>
      </div>
    );
  }

  const onlineCount = connectors.filter(c => c.status === 'online').length;

  return (
    <div className="space-y-6 p-6">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-primary/10 ring-1 ring-primary/20">
            <Globe className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h1 className="font-display text-xl font-bold uppercase tracking-wide">Connectors</h1>
            <p className="text-xs text-muted-foreground mt-0.5">
              Network gateways providing access to remote networks
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5 rounded-lg bg-muted/60 px-3 py-1.5 ring-1 ring-border/30">
            <Zap className="h-3 w-3 text-secure" />
            <span className="text-[11px] font-mono text-muted-foreground">{onlineCount} online</span>
          </div>
          <div className="flex items-center gap-1.5 rounded-lg bg-muted/60 px-3 py-1.5 ring-1 ring-border/30">
            <span className="text-[11px] font-mono text-muted-foreground">{connectors.length} total</span>
          </div>
          <Button className="gap-2 font-display font-semibold uppercase tracking-wider text-[12px]" size="sm" onClick={() => setIsAddOpen(true)}>
            <Plus className="h-4 w-4" />
            Add Connector
          </Button>
        </div>
      </div>

      {/* Connectors List */}
      <ConnectorsList connectors={connectors} onConnectorDeleted={loadConnectors} />

      <AddConnectorModal
        isOpen={isAddOpen}
        onClose={() => setIsAddOpen(false)}
        onConnectorAdded={loadConnectors}
      />
    </div>
  );
}
