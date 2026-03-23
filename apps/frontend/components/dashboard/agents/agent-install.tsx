'use client';

import { Link, useNavigate } from 'react-router-dom';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ArrowLeft, Check, Copy, KeyRound, Loader2, Terminal, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { createEnrollmentToken, deleteAgent, getConnectors } from '@/lib/mock-api';
import { Connector } from '@/lib/types';
import { toast } from 'sonner';
import { getWorkspaceClaims } from '@/lib/jwt';

function fallbackCopy(text: string) {
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  document.execCommand('copy');
  document.body.removeChild(textarea);
}

export function AgentInstall({
  initialAgentId,
  initialConnectorId,
}: {
  initialAgentId?: string;
  initialConnectorId?: string;
}) {
  const navigate = useNavigate();
  const [token, setToken] = useState<string>('');
  const [tokenLoading, setTokenLoading] = useState(false);
  const [deleting, setDeleting] = useState(false);

  // Get workspace slug from JWT token for enrollment
  const authToken = typeof window !== 'undefined' ? localStorage.getItem('authToken') : null;
  const claims = authToken ? getWorkspaceClaims(authToken) : null;
  const workspaceSlug = claims?.wslug || '';

  // Auto-detect controller IP from the browser's current hostname.
  const detectedHost = window.location.hostname || '127.0.0.1';
  const [controllerAddr, setControllerAddr] = useState(`${detectedHost}:8443`);
  const [controllerHttpAddr, setControllerHttpAddr] = useState(`${detectedHost}:8081`);
  const [connectorAddr, setConnectorAddr] = useState(`${detectedHost}:9443`);
  const [controllerTrustDomain, setControllerTrustDomain] = useState('mycorp.internal');
  const [connectorTrustDomain, setConnectorTrustDomain] = useState(
    workspaceSlug ? `${workspaceSlug}.zerotrust.com` : 'mycorp.internal',
  );
  const [agentId, setAgentId] = useState(initialAgentId || 'agent-local-01');

  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [selectedConnectorId, setSelectedConnectorId] = useState<string>('');

  const didFetchToken = useRef(false);
  useEffect(() => {
    if (didFetchToken.current) return;
    didFetchToken.current = true;
    void handleCreateToken();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    getConnectors()
      .then((list) => {
        const installed = list.filter((c) => c.installed);
        setConnectors(installed.length > 0 ? installed : list);
        if (initialConnectorId) {
          const found = (installed.length > 0 ? installed : list).find((c) => c.id === initialConnectorId);
          if (found) {
            setSelectedConnectorId(found.id);
            if (found.privateIp) setConnectorAddr(`${found.privateIp}:9443`);
          }
        }
      })
      .catch(() => {});
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (selectedConnectorId || connectors.length === 0) return;
    const preferred = connectors.find((c) => c.privateIp) ?? connectors[0];
    if (!preferred) return;
    setSelectedConnectorId(preferred.id);
    if (preferred.privateIp) {
      setConnectorAddr(`${preferred.privateIp}:9443`);
    }
  }, [connectors, selectedConnectorId]);

  useEffect(() => {
    if (!initialAgentId) return;
    setAgentId(initialAgentId);
  }, [initialAgentId]);

  useEffect(() => {
    if (!workspaceSlug) return;
    setConnectorTrustDomain(`${workspaceSlug}.zerotrust.com`);
  }, [workspaceSlug]);

  const handleConnectorSelect = (id: string) => {
    setSelectedConnectorId(id);
    const connector = connectors.find((c) => c.id === id);
    if (connector?.privateIp) {
      setConnectorAddr(`${connector.privateIp}:9443`);
    } else {
      setConnectorAddr('');
    }
  };

  const handleDelete = async () => {
    if (!initialAgentId) return;
    if (!window.confirm(`Delete agent "${initialAgentId}"? This will remove it from the controller.`)) return;
    setDeleting(true);
    try {
      await deleteAgent(initialAgentId);
      toast.success(`Agent "${initialAgentId}" deleted.`);
      navigate('/dashboard/agents');
    } catch {
      toast.error('Failed to delete agent. Check that the backend is running.');
    } finally {
      setDeleting(false);
    }
  };

  const handleCreateToken = async () => {
    setTokenLoading(true);
    try {
      const resp = await createEnrollmentToken(workspaceSlug || undefined);
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
      `curl -fsSL https://raw.githubusercontent.com/vairabarath/zero-trust/feature/quic-device-tunnel-control-plane-merge/scripts/agent-setup.sh | sudo \\\n` +
      `  CONTROLLER_ADDR="${controllerAddr || '127.0.0.1:8443'}" \\\n` +
      `  CONTROLLER_HTTP_ADDR="${controllerHttpAddr || '127.0.0.1:8081'}" \\\n` +
      `  CONNECTOR_ADDR="${connectorAddr || 'CONNECTOR_ADDR_HERE'}" \\\n` +
      `  AGENT_ID="${agentId || 'agent-local-01'}" \\\n` +
      `  ENROLLMENT_TOKEN="${safeToken}" \\\n` +
      `  CONTROLLER_TRUST_DOMAIN="${controllerTrustDomain || 'mycorp.internal'}" \\\n` +
      `  CONNECTOR_TRUST_DOMAIN="${connectorTrustDomain || 'mycorp.internal'}" \\\n` +
      (workspaceSlug ? `  WORKSPACE_SLUG="${workspaceSlug}" \\\n` : '') +
      `  bash`
    );
  }, [
    connectorAddr,
    controllerAddr,
    controllerHttpAddr,
    controllerTrustDomain,
    connectorTrustDomain,
    token,
    agentId,
    workspaceSlug,
  ]);

  const handleCopyCommand = () => {
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(installCommand).catch(() => fallbackCopy(installCommand));
    } else {
      fallbackCopy(installCommand);
    }
    toast.success('Installation command copied to clipboard!');
  };

  return (
    <div className="space-y-6 p-6">
      <Link to="/dashboard/agents">
        <Button variant="ghost" className="gap-2">
          <ArrowLeft className="h-4 w-4" />
          Back to Agents
        </Button>
      </Link>

      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{initialAgentId ? `Agent: ${initialAgentId}` : 'Add Agent'}</h1>
          <p className="text-sm text-muted-foreground">
            Generate an enrollment token and copy the install command.
          </p>
        </div>
        {initialAgentId && (
          <Button
            variant="destructive"
            className="gap-2"
            disabled={deleting}
            onClick={handleDelete}
          >
            {deleting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
            Delete Agent
          </Button>
        )}
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <KeyRound className="h-5 w-5" />
            Enrollment Token
          </CardTitle>
          <CardDescription>
            This token is required for one-time agent enrollment.
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
              <Label htmlFor="connectorSelect">Connector</Label>
              {connectors.length > 0 ? (
                <>
                  <Select value={selectedConnectorId} onValueChange={handleConnectorSelect}>
                    <SelectTrigger id="connectorSelect">
                      <SelectValue placeholder="Select a connector..." />
                    </SelectTrigger>
                    <SelectContent>
                      {connectors.map((c) => (
                        <SelectItem key={c.id} value={c.id}>
                          {c.name}
                          {c.privateIp ? (
                            <span className="ml-2 text-muted-foreground font-mono text-xs">
                              ({c.privateIp}:9443)
                            </span>
                          ) : null}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {connectorAddr && (
                    <p className="text-xs text-muted-foreground font-mono">{connectorAddr}</p>
                  )}
                </>
              ) : (
                <Input
                  id="connectorAddr"
                  value={connectorAddr}
                  onChange={(e) => setConnectorAddr(e.target.value)}
                  placeholder="127.0.0.1:9443"
                />
              )}
              <p className="text-xs text-muted-foreground">
                The connector this agent will connect to.
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="agentId">Agent ID</Label>
              <Input
                id="agentId"
                value={agentId}
                onChange={(e) => setAgentId(e.target.value)}
                placeholder="agent-local-01"
                disabled={!!initialAgentId}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="controllerTrustDomain">Controller Trust Domain</Label>
              <Input
                id="controllerTrustDomain"
                value={controllerTrustDomain}
                onChange={(e) => setControllerTrustDomain(e.target.value)}
                placeholder="mycorp.internal"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="connectorTrustDomain">Connector Trust Domain</Label>
              <Input
                id="connectorTrustDomain"
                value={connectorTrustDomain}
                onChange={(e) => setConnectorTrustDomain(e.target.value)}
                placeholder="workspace.zerotrust.com"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="controllerHttpAddr">Controller HTTP Address</Label>
              <Input
                id="controllerHttpAddr"
                value={controllerHttpAddr}
                onChange={(e) => setControllerHttpAddr(e.target.value)}
                placeholder="127.0.0.1:8081"
              />
              <p className="text-xs text-muted-foreground">
                The CA certificate is fetched automatically from this address.
              </p>
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
