'use client';

import { Link } from 'react-router-dom';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ArrowLeft, Check, Copy, KeyRound, Loader2, Terminal } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { createEnrollmentToken } from '@/lib/mock-api';
import { toast } from 'sonner';

export function TunnelerInstall({ initialTunnelerId }: { initialTunnelerId?: string }) {
  const [token, setToken] = useState<string>('');
  const [tokenLoading, setTokenLoading] = useState(false);

  const [controllerAddr, setControllerAddr] = useState('127.0.0.1:8443');
  const [connectorAddr, setConnectorAddr] = useState('');
  const [tunnelerId, setTunnelerId] = useState(initialTunnelerId || 'tunneler-local-01');
  const [controllerCAPath, setControllerCAPath] = useState('/etc/grpcconnector2/ca.crt');

  const didFetchToken = useRef(false);
  useEffect(() => {
    if (didFetchToken.current) return;
    didFetchToken.current = true;
    void handleCreateToken();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!initialTunnelerId) return;
    setTunnelerId(initialTunnelerId);
  }, [initialTunnelerId]);

  const handleCreateToken = async () => {
    setTokenLoading(true);
    try {
      const resp = await createEnrollmentToken();
      setToken(resp.token);
    } catch (error) {
      console.error('Failed to create enrollment token:', error);
      toast.error('Failed to create enrollment token.');
    } finally {
      setTokenLoading(false);
    }
  };

  const installCommand = useMemo(() => {
    const safeToken = token || 'fetching_enrollment_token';
    return (
      `curl -fsSL https://raw.githubusercontent.com/sathiyaseelank-dot/grpccontroller/main/scripts/tunneler-setup.sh | sudo \\\n` +
      `  CONTROLLER_ADDR="${controllerAddr || '127.0.0.1:8443'}" \\\n` +
      `  CONNECTOR_ADDR="${connectorAddr || 'CONNECTOR_ADDR_HERE'}" \\\n` +
      `  TUNNELER_ID="${tunnelerId || 'tunneler-local-01'}" \\\n` +
      `  ENROLLMENT_TOKEN="${safeToken}" \\\n` +
      `  CONTROLLER_CA_PATH="${controllerCAPath || '/etc/grpcconnector2/ca.crt'}" \\\n` +
      `  bash`
    );
  }, [connectorAddr, controllerAddr, controllerCAPath, token, tunnelerId]);

  const handleCopyCommand = async () => {
    await navigator.clipboard.writeText(installCommand);
    toast.success('Installation command copied to clipboard!');
  };

  return (
    <div className="space-y-6 p-6">
      <Link to="/dashboard/tunnelers">
        <Button variant="ghost" className="gap-2">
          <ArrowLeft className="h-4 w-4" />
          Back to Tunnelers
        </Button>
      </Link>

      <div>
        <h1 className="text-2xl font-bold">Add Tunneler</h1>
        <p className="text-sm text-muted-foreground">
          Generate an enrollment token and copy the install command.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <KeyRound className="h-5 w-5" />
            Enrollment Token
          </CardTitle>
          <CardDescription>
            This token is required for one-time tunneler enrollment.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Button onClick={handleCreateToken} disabled={tokenLoading}>
              {tokenLoading ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Generating...
                </>
              ) : (
                <>
                  <Check className="h-4 w-4" />
                  Generate Token
                </>
              )}
            </Button>
          </div>
          <div className="space-y-2">
            <Label>Token</Label>
            <div className="rounded-md border bg-muted/30 px-3 py-2 font-mono text-sm break-all">
              {token || '—'}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Terminal className="h-5 w-5" />
            Installation Command
          </CardTitle>
          <CardDescription>
            Fill the fields, then copy and run this command on the target host.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="controllerAddr">Controller Address</Label>
              <Input
                id="controllerAddr"
                value={controllerAddr}
                onChange={(e) => setControllerAddr(e.target.value)}
                placeholder="127.0.0.1:8443"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="connectorAddr">Connector Address</Label>
              <Input
                id="connectorAddr"
                value={connectorAddr}
                onChange={(e) => setConnectorAddr(e.target.value)}
                placeholder="127.0.0.1:9443"
              />
              <p className="text-xs text-muted-foreground">
                The connector’s listen address that tunnelers connect to (host:port).
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="tunnelerId">Tunneler ID</Label>
              <Input
                id="tunnelerId"
                value={tunnelerId}
                onChange={(e) => setTunnelerId(e.target.value)}
                placeholder="tunneler-local-01"
                disabled={!!initialTunnelerId}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="controllerCAPath">Controller CA Path</Label>
              <Input
                id="controllerCAPath"
                value={controllerCAPath}
                onChange={(e) => setControllerCAPath(e.target.value)}
                placeholder="/etc/grpcconnector2/ca.crt"
              />
            </div>
          </div>

          <div className="flex justify-end">
            <Button variant="outline" size="sm" className="gap-2" onClick={handleCopyCommand}>
              <Copy className="h-4 w-4" />
              Copy command
            </Button>
          </div>

          <div className="relative rounded-md bg-muted p-4 text-left font-mono text-sm text-foreground overflow-x-auto">
            <pre>{installCommand}</pre>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

