import { useEffect, useState } from 'react';
import { getDiagnostics } from '@/lib/mock-api';
import { DiagnosticsData } from '@/lib/types';
import { ConnectivityPanel } from '@/components/dashboard/diagnostics/connectivity-panel';
import { AccessTracePanel } from '@/components/dashboard/diagnostics/access-trace-panel';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Loader2, Activity, Wifi, Route } from 'lucide-react';

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
      <div className="flex items-center justify-center p-16">
        <div className="flex flex-col items-center gap-3">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
          <p className="text-xs text-muted-foreground font-mono tracking-wider">Running diagnostics...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      {/* Page Header */}
      <div className="flex items-center gap-4">
        <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-primary/10 ring-1 ring-primary/20">
          <Activity className="h-5 w-5 text-primary" />
        </div>
        <div>
          <h1 className="font-display text-xl font-bold uppercase tracking-wide">Network Diagnostics</h1>
          <p className="text-xs text-muted-foreground mt-0.5">
            Monitor health and trace access paths
          </p>
        </div>
      </div>

      <Tabs defaultValue="connectivity">
        <TabsList>
          <TabsTrigger value="connectivity" className="gap-1.5">
            <Wifi className="h-3.5 w-3.5" />
            Connectivity
          </TabsTrigger>
          <TabsTrigger value="trace" className="gap-1.5">
            <Route className="h-3.5 w-3.5" />
            Access Trace
          </TabsTrigger>
        </TabsList>

        <TabsContent value="connectivity" className="mt-4">
          <ConnectivityPanel
            connectors={data?.connectors ?? []}
            agents={data?.agents ?? []}
          />
        </TabsContent>

        <TabsContent value="trace" className="mt-4">
          <AccessTracePanel />
        </TabsContent>
      </Tabs>
    </div>
  );
}
