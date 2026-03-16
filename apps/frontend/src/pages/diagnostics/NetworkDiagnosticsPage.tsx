import { useEffect, useState } from 'react';
import { getDiagnostics } from '@/lib/mock-api';
import { DiagnosticsData } from '@/lib/types';
import { ConnectivityPanel } from '@/components/dashboard/diagnostics/connectivity-panel';
import { AccessTracePanel } from '@/components/dashboard/diagnostics/access-trace-panel';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Loader2 } from 'lucide-react';

export default function NetworkDiagnosticsPage() {
  const [data, setData] = useState<DiagnosticsData | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    void getDiagnostics()
      .then(setData)
      .catch((err) => console.error('Failed to load diagnostics:', err))
      .finally(() => setLoading(false));
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
      <div>
        <h1 className="text-2xl font-bold">Network Diagnostics</h1>
        <p className="text-sm text-muted-foreground">
          Monitor connector health and trace access paths across your network
        </p>
      </div>

      <Tabs defaultValue="connectivity">
        <TabsList>
          <TabsTrigger value="connectivity">Connectivity</TabsTrigger>
          <TabsTrigger value="trace">Access Trace</TabsTrigger>
        </TabsList>

        <TabsContent value="connectivity" className="mt-4">
          <ConnectivityPanel
            connectors={data?.connectors ?? []}
            tunnelers={data?.tunnelers ?? []}
          />
        </TabsContent>

        <TabsContent value="trace" className="mt-4">
          <AccessTracePanel />
        </TabsContent>
      </Tabs>
    </div>
  );
}
