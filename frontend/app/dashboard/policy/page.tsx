'use client';

import { useEffect, useState } from 'react';
import { getConnectors } from '@/lib/mock-api';
import { Connector } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Loader2, RefreshCw } from 'lucide-react';

interface CompiledPolicy {
  snapshot_meta: {
    connector_id: string;
    policy_version: number;
    compiled_at: string;
    policy_hash: string;
  };
  resources: Array<{
    resource_id: string;
    address: string;
    protocol: string;
    port_from: number | null;
    port_to: number | null;
    allowed_identities: string[];
    _note?: string;
  }>;
}

export default function PolicyPage() {
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [loading, setLoading] = useState(true);
  const [compiling, setCompiling] = useState<string | null>(null);
  const [policies, setPolicies] = useState<Record<string, CompiledPolicy>>({});

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

  useEffect(() => {
    loadConnectors();
  }, []);

  const compilePolicy = async (connectorId: string) => {
    setCompiling(connectorId);
    try {
      const res = await fetch(`/api/policy/compile/${connectorId}`);
      if (!res.ok) throw new Error('compile failed');
      const policy = (await res.json()) as CompiledPolicy;
      setPolicies((prev) => ({ ...prev, [connectorId]: policy }));
    } catch (error) {
      console.error('Failed to compile policy:', error);
    } finally {
      setCompiling(null);
    }
  };

  const getIdentityCount = (policy?: CompiledPolicy) => {
    if (!policy) return 0;
    const set = new Set<string>();
    policy.resources.forEach((res) => res.allowed_identities.forEach((id) => set.add(id)));
    return set.size;
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
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Policy</h1>
          <p className="text-sm text-muted-foreground">
            Compile and inspect connector policy snapshots.
          </p>
        </div>
        <Button variant="outline" className="gap-2" onClick={loadConnectors}>
          <RefreshCw className="h-4 w-4" />
          Refresh
        </Button>
      </div>

      <div className="space-y-4">
        {connectors.map((connector) => {
          const policy = policies[connector.id];
          const compiledVersion = policy?.snapshot_meta.policy_version ?? 0;
          const stale = compiledVersion > connector.lastPolicyVersion;
          const identityCount = getIdentityCount(policy);

          return (
            <Card key={connector.id}>
              <CardHeader className="flex flex-row items-center justify-between space-y-0">
                <div className="space-y-1">
                  <CardTitle className="flex items-center gap-2">
                    {connector.name}
                    {stale && (
                      <Badge variant="destructive">Stale</Badge>
                    )}
                  </CardTitle>
                  <CardDescription>
                    Last seen: {connector.lastSeenAt ?? connector.lastSeen}
                  </CardDescription>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="outline">Connector v{connector.lastPolicyVersion}</Badge>
                  <Button
                    onClick={() => compilePolicy(connector.id)}
                    disabled={compiling === connector.id}
                  >
                    {compiling === connector.id ? 'Compiling...' : 'Compile Policy'}
                  </Button>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex flex-wrap gap-6 text-sm">
                  <div>
                    <span className="text-muted-foreground">Compiled version:</span>{' '}
                    <span className="font-semibold">{compiledVersion || '—'}</span>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Identity count:</span>{' '}
                    <span className="font-semibold">{policy ? identityCount : '—'}</span>
                  </div>
                </div>

                {policy && (
                  <details className="rounded-lg border bg-muted/30 p-4">
                    <summary className="cursor-pointer text-sm font-medium">
                      View compiled policy JSON
                    </summary>
                    <pre className="mt-3 overflow-auto text-xs">
                      {JSON.stringify(policy, null, 2)}
                    </pre>
                  </details>
                )}
              </CardContent>
            </Card>
          );
        })}
      </div>
    </div>
  );
}
