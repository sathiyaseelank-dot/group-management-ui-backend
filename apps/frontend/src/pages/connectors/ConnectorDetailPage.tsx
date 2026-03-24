import { useEffect, useMemo, useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Link } from 'react-router-dom';
import { createEnrollmentToken, deleteConnector, getConnector, grantConnector, revokeConnector } from '@/lib/mock-api';
import { Connector, RemoteNetwork } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loader2, ArrowLeft, AlertTriangle, Terminal, Copy, CheckCircle, RefreshCw, ShieldOff, ShieldCheck, Trash2 } from 'lucide-react';
import { ConnectorInfoSection } from '@/components/dashboard/connectors/connector-info-section';
import { ConnectorLogs } from '@/components/dashboard/connectors/connector-logs';
import { toast } from 'sonner';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { getWorkspaceClaims } from '@/lib/jwt';

function copyToClipboard(text: string) {
  if (navigator.clipboard?.writeText) {
    navigator.clipboard.writeText(text).catch(() => fallbackCopy(text));
  } else {
    fallbackCopy(text);
  }
}

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

interface LogEntry {
  id: number;
  timestamp: string;
  message: string;
}

export default function ConnectorDetailPage() {
  const { connectorId } = useParams();
  const navigate = useNavigate();
  const [connector, setConnector] = useState<Connector | null>(null);
  const [network, setNetwork] = useState<RemoteNetwork | undefined>(undefined);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [enrollmentToken, setEnrollmentToken] = useState<string>('');
  const [tokenLoading, setTokenLoading] = useState(false);
  const [tokenError, setTokenError] = useState<string | null>(null);

  // Get workspace slug from JWT token for enrollment
  const token = localStorage.getItem('authToken');
  const claims = token ? getWorkspaceClaims(token) : null;
  const workspaceSlug = claims?.wslug || '';

  // Auto-detect controller IP from the browser's current hostname.
  // When accessed from another machine (e.g. 192.168.1.x), this gives the correct LAN IP.
  const detectedHost = window.location.hostname || '127.0.0.1';
  const [controllerAddr, setControllerAddr] = useState(`${detectedHost}:8443`);
  const [controllerHttpAddr, setControllerHttpAddr] = useState(`${detectedHost}:8081`);
  const [deviceTunnelAddr, setDeviceTunnelAddr] = useState('0.0.0.0:9444');
  const [controllerTrustDomain, setControllerTrustDomain] = useState('mycorp.internal');
  const [agentTrustDomain, setAgentTrustDomain] = useState(
    workspaceSlug ? `${workspaceSlug}.zerotrust.com` : 'mycorp.internal',
  );
  const connectorInstallScript = 'https://raw.githubusercontent.com/vairabarath/zero-trust/feature/quic-device-tunnel-control-plane-merge/scripts/setup.sh';
  const INSTALL_COMMAND = useMemo(() => {
    if (!enrollmentToken) return null;
    return (
      `curl -fsSL ${connectorInstallScript} | sudo \\\n` +
      `  CONTROLLER_ADDR="${controllerAddr || '127.0.0.1:8443'}" \\\n` +
      `  CONTROLLER_HTTP_URL="http://${controllerHttpAddr || '127.0.0.1:8081'}" \\\n` +
      `  CONNECTOR_ID="${connectorId ?? 'connector-local-01'}" \\\n` +
      `  ENROLLMENT_TOKEN="${enrollmentToken}" \\\n` +
      `  CONTROLLER_TRUST_DOMAIN="${controllerTrustDomain || 'mycorp.internal'}" \\\n` +
      `  AGENT_TRUST_DOMAIN="${agentTrustDomain || 'mycorp.internal'}" \\\n` +
      (workspaceSlug ? `  WORKSPACE_SLUG="${workspaceSlug}" \\\n` : '') +
      `  DEVICE_TUNNEL_ADDR="${deviceTunnelAddr || '0.0.0.0:9444'}" \\\n` +
      `  bash`
    );
  }, [
    connectorInstallScript,
    enrollmentToken,
    controllerAddr,
    controllerHttpAddr,
    connectorId,
    controllerTrustDomain,
    agentTrustDomain,
    deviceTunnelAddr,
    workspaceSlug,
  ]);

  const loadConnectorData = async (opts?: { silent?: boolean }) => {
    if (!opts?.silent) {
      setLoading(true);
    }
    try {
      const { connector: fetchedConnector, network: fetchedNetwork, logs: fetchedLogs } = await getConnector(connectorId as string);
      setConnector(fetchedConnector);
      setNetwork(fetchedNetwork);
      setLogs(fetchedLogs);
    } catch (error) {
      console.error('Failed to load connector details:', error);
    } finally {
      if (!opts?.silent) {
        setLoading(false);
      }
    }
  };

  useEffect(() => {
    if (connectorId) {
      loadConnectorData();
    }
  }, [connectorId]);

  const didFetchToken = useRef(false);

  const fetchEnrollmentToken = async () => {
    setTokenLoading(true);
    setTokenError(null);
    try {
      const { token } = await createEnrollmentToken(workspaceSlug || undefined);
      setEnrollmentToken(token);
    } catch (error) {
      console.error('Failed to create enrollment token:', error);
      setTokenError('Failed to generate enrollment token. Check that the backend is running.');
    } finally {
      setTokenLoading(false);
    }
  };

  useEffect(() => {
    if (didFetchToken.current) return;
    didFetchToken.current = true;
    fetchEnrollmentToken();
  }, []);

  useEffect(() => {
    if (!connectorId) return;
    // Poll every 5s until installed, then every 10s to track live online/offline status.
    const delay = connector?.installed ? 10000 : 5000;
    const interval = setInterval(() => {
      loadConnectorData({ silent: true });
    }, delay);
    return () => clearInterval(interval);
  }, [connector?.installed, connectorId]);

  const [isRevoking, setIsRevoking] = useState(false);
  const [isRevokingAccess, setIsRevokingAccess] = useState(false);


  const handleDelete = async () => {
    if (!connectorId) return;
    if (!window.confirm(`Delete connector "${connector?.name ?? connectorId}"? This will remove it from the controller.`)) return;
    setIsRevoking(true);
    try {
      await deleteConnector(connectorId);
      toast.success('Connector deleted successfully.');
      navigate('/dashboard/connectors');
    } catch (error) {
      toast.error('Failed to delete connector. Check that the backend is running.');
    } finally {
      setIsRevoking(false);
    }
  };

  const handleRevoke = async () => {
    if (!connectorId) return;
    setIsRevokingAccess(true);
    try {
      await revokeConnector(connectorId);
      toast.success('Connector access revoked.');
      await loadConnectorData({ silent: true });
    } catch (error) {
      toast.error('Failed to revoke connector access.');
    } finally {
      setIsRevokingAccess(false);
    }
  };

  const handleGrant = async () => {
    if (!connectorId) return;
    setIsRevokingAccess(true);
    try {
      await grantConnector(connectorId);
      toast.success('Connector access granted.');
      await loadConnectorData({ silent: true });
    } catch (error) {
      toast.error('Failed to grant connector access.');
    } finally {
      setIsRevokingAccess(false);
    }
  };

  const handleCopyCommand = () => {
    if (!INSTALL_COMMAND) return;
    copyToClipboard(INSTALL_COMMAND);
    toast.success('Installation command copied to clipboard!');
  };

  // Shared install card used in both the "not found" and "not installed" states
  const installCard = (
    <Card className="mt-8 mx-auto max-w-2xl text-left">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Terminal className="h-5 w-5" />
          Installation Command
        </CardTitle>
        <CardDescription>
          Fill in the fields below, then copy and run the command on your server.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="controllerAddr">Controller gRPC Address</Label>
            <Input
              id="controllerAddr"
              value={controllerAddr}
              onChange={(e) => setControllerAddr(e.target.value)}
              placeholder="127.0.0.1:8443"
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
          <div className="space-y-2">
            <Label htmlFor="deviceTunnelAddr">Device Tunnel Address</Label>
            <Input
              id="deviceTunnelAddr"
              value={deviceTunnelAddr}
              onChange={(e) => setDeviceTunnelAddr(e.target.value)}
              placeholder="0.0.0.0:9444"
            />
            <p className="text-xs text-muted-foreground">
              Address the connector binds for device tunnel traffic.
            </p>
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
            <Label htmlFor="agentTrustDomain">Agent Trust Domain</Label>
            <Input
              id="agentTrustDomain"
              value={agentTrustDomain}
              onChange={(e) => setAgentTrustDomain(e.target.value)}
              placeholder="asdf.zerotrust.com"
            />
          </div>
        </div>

        {tokenLoading && (
          <div className="flex items-center gap-2 py-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Generating enrollment token...
          </div>
        )}
        {tokenError && (
          <div className="space-y-2 py-2">
            <p className="text-sm text-destructive">{tokenError}</p>
            <Button variant="outline" size="sm" className="gap-2" onClick={fetchEnrollmentToken}>
              <RefreshCw className="h-4 w-4" />
              Retry
            </Button>
          </div>
        )}
        {INSTALL_COMMAND && (
          <>
            <div className="flex justify-end">
              <Button variant="ghost" size="sm" className="gap-2" onClick={handleCopyCommand}>
                <Copy className="h-4 w-4" />
                Copy command
              </Button>
            </div>
            <div className="relative rounded-md bg-muted p-4 font-mono text-sm text-foreground overflow-x-auto">
              <pre>{INSTALL_COMMAND}</pre>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );

  if (loading) {
    return (
      <div className="flex items-center justify-center p-12">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!connector) {
    return (
      <div className="space-y-6 p-6">
        <Link to="/dashboard/connectors">
          <Button variant="ghost" className="gap-2">
            <ArrowLeft className="h-4 w-4" />
            Back to Connectors
          </Button>
        </Link>
        <div className="text-center py-20">
          <AlertTriangle className="mx-auto h-16 w-16 text-destructive" />
          <h2 className="mt-4 text-2xl font-bold">Connector Not Found</h2>
          <p className="mt-2 text-muted-foreground">
            It looks like this connector is not registered.
          </p>
          {installCard}
        </div>
      </div>
    );
  }

  if (!connector.installed && connector.status !== 'revoked') {
    return (
      <div className="space-y-6 p-6">
        <Link to="/dashboard/connectors">
          <Button variant="ghost" className="gap-2">
            <ArrowLeft className="h-4 w-4" />
            Back to Connectors
          </Button>
        </Link>
        <div className="text-center py-20">
          <AlertTriangle className="mx-auto h-16 w-16 text-muted-foreground" />
          <h2 className="mt-4 text-2xl font-bold">Connector Added, Not Installed</h2>
          <p className="mt-2 text-muted-foreground">
            This connector is registered but not installed yet. Run the command below on your server.
          </p>
          {installCard}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      {/* Breadcrumb & Header */}
      <div className="flex items-center justify-between">
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Link to="/dashboard/connectors" className="hover:text-foreground">
              Connectors
            </Link>
            <span>/</span>
            <span>{connector.name}</span>
          </div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold">{connector.name}</h1>
            {connector.status === 'online' && (
              <Badge variant="outline" className="gap-1">
                <CheckCircle className="h-3 w-3 text-green-500" />
                Online
              </Badge>
            )}
          </div>
        </div>
        <div className="flex gap-2">
          {connector.status !== 'revoked' && (
            <Button
              variant="outline"
              className="gap-2 text-orange-500 border-orange-500 hover:text-orange-600 hover:border-orange-600"
              onClick={handleRevoke}
              disabled={isRevokingAccess}
            >
              {isRevokingAccess ? <Loader2 className="h-4 w-4 animate-spin" /> : <ShieldOff className="h-4 w-4" />}
              Revoke
            </Button>
          )}
          {connector.status === 'revoked' && (
            <Button
              variant="outline"
              className="gap-2 text-green-500 border-green-500 hover:text-green-600 hover:border-green-600"
              onClick={handleGrant}
              disabled={isRevokingAccess}
            >
              {isRevokingAccess ? <Loader2 className="h-4 w-4 animate-spin" /> : <ShieldCheck className="h-4 w-4" />}
              Grant
            </Button>
          )}
          <Button variant="destructive" className="gap-2" onClick={handleDelete} disabled={isRevoking}>
            {isRevoking ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
            Delete
          </Button>
        </div>
      </div>

      {/* Connector Info Section */}
      <ConnectorInfoSection connector={connector} network={network} />

      {/* Logs Section */}
      <ConnectorLogs logs={logs} />
    </div>
  );
}
