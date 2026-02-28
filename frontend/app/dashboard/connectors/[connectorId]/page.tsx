'use client';

import { useEffect, useRef, useState } from 'react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import { createEnrollmentToken, getConnector, simulateConnectorHeartbeat } from '@/lib/mock-api';
import { Connector, RemoteNetwork } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Loader2, ArrowLeft, RefreshCw, AlertTriangle, Terminal, Copy, HeartPulse, CheckCircle } from 'lucide-react';
import { ConnectorInfoSection } from '@/components/dashboard/connectors/connector-info-section';
import { ConnectorLogs } from '@/components/dashboard/connectors/connector-logs';
import { toast } from 'sonner';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { Label } from '@/components/ui/label';

interface LogEntry {
  id: number;
  timestamp: string;
  message: string;
}

export default function ConnectorDetailPage() {
  const { connectorId } = useParams();
  const [connector, setConnector] = useState<Connector | null>(null);
  const [network, setNetwork] = useState<RemoteNetwork | undefined>(undefined);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [isSimulatingHeartbeat, setIsSimulatingHeartbeat] = useState(false);
  const [enrollmentToken, setEnrollmentToken] = useState<string>('');
  const [autoHeartbeatSent, setAutoHeartbeatSent] = useState(false);

  const INSTALL_COMMAND = `curl -fsSL https://raw.githubusercontent.com/sathiyaseelank-dot/grpccontroller/main/scripts/setup.sh | sudo \\
  CONTROLLER_ADDR="127.0.0.1:8443" \\
  CONNECTOR_ID="${connectorId ?? 'connector-local-01'}" \\
  ENROLLMENT_TOKEN="${enrollmentToken || 'fetching_enrollment_token'}" \\
  CONTROLLER_CA_PATH="/home/inkyank-01/Downloads/group-management-ui/backend/controller/ca/ca.crt" \\
  bash`;

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
  useEffect(() => {
    if (didFetchToken.current) return;
    didFetchToken.current = true;
    let active = true;
    const loadEnrollmentToken = async () => {
      try {
        const { token } = await createEnrollmentToken();
        if (active) {
          setEnrollmentToken(token);
        }
      } catch (error) {
        console.error('Failed to create enrollment token:', error);
      }
    };
    loadEnrollmentToken();
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (!connectorId || !connector || !connector.installed) return;
    if (connector.status === 'online') return;
    if (!enrollmentToken || autoHeartbeatSent) return;
    const sendHeartbeat = async () => {
      setAutoHeartbeatSent(true);
      try {
        await simulateConnectorHeartbeat(connectorId as string, enrollmentToken);
        await loadConnectorData({ silent: true });
      } catch (error) {
        console.error('Failed to auto-send heartbeat:', error);
      }
    };
    sendHeartbeat();
  }, [autoHeartbeatSent, connector, connectorId, enrollmentToken]);

  useEffect(() => {
    if (!connectorId) return;
    if (connector?.installed) return;
    const interval = setInterval(() => {
      loadConnectorData({ silent: true });
    }, 5000);
    return () => clearInterval(interval);
  }, [connector?.installed, connectorId]);

  const handleRevoke = () => {
    toast.warning('This is a placeholder action.', {
      description: `In a real application, this would revoke the connector's keys.`,
    });
  };

  const handleCopyCommand = () => {
    navigator.clipboard.writeText(INSTALL_COMMAND);
    toast.success('Installation command copied to clipboard!');
  };

  const handleSimulateHeartbeat = async () => {
    if (!connectorId) return;
    setIsSimulatingHeartbeat(true);
    try {
      await simulateConnectorHeartbeat(connectorId as string, enrollmentToken);
      toast.success('Connector status updated to online!');
      loadConnectorData({ silent: true }); // Reload data to reflect changes
    } catch (error) {
      toast.error('Failed to simulate heartbeat.');
    } finally {
      setIsSimulatingHeartbeat(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center p-12">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // If connector is not found, show missing state
  if (!connector) {
    return (
      <div className="space-y-6 p-6">
        <Link href="/dashboard/connectors">
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
          
          <Card className="mt-8 mx-auto max-w-2xl">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Terminal className="h-5 w-5" />
                Installation Command
              </CardTitle>
              <CardDescription>
                Run the following command on your server to install and activate the connector.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex justify-end pb-2">
                <Button variant="ghost" size="sm" className="gap-2" onClick={handleCopyCommand}>
                  <Copy className="h-4 w-4" />
                  Copy command
                </Button>
              </div>
              <div className="relative rounded-md bg-muted p-4 text-left font-mono text-sm text-foreground overflow-x-auto">
                <pre>{INSTALL_COMMAND}</pre>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  // Connector is added but not installed yet
  if (!connector.installed) {
    return (
      <div className="space-y-6 p-6">
        <Link href="/dashboard/connectors">
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

          <Card className="mt-8 mx-auto max-w-2xl">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Terminal className="h-5 w-5" />
                Installation Command
              </CardTitle>
              <CardDescription>
                Run the following command on your server to install and activate the connector.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex justify-end pb-2">
                <Button variant="ghost" size="sm" className="gap-2" onClick={handleCopyCommand}>
                  <Copy className="h-4 w-4" />
                  Copy command
                </Button>
              </div>
              <div className="relative rounded-md bg-muted p-4 text-left font-mono text-sm text-foreground overflow-x-auto">
                <pre>{INSTALL_COMMAND}</pre>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  // Connector is installed
  return (
    <div className="space-y-6 p-6">
      {/* Breadcrumb & Header */}
      <div className="flex items-center justify-between">
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Link href="/dashboard/connectors" className="hover:text-foreground">
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
          {connector.status === 'offline' && (
            <Button
              variant="outline"
              className="gap-2 text-green-500 border-green-500 hover:text-green-600 hover:border-green-600"
              onClick={handleSimulateHeartbeat}
              disabled={isSimulatingHeartbeat}
            >
              {isSimulatingHeartbeat ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <HeartPulse className="h-4 w-4" />
              )}
              Simulate Heartbeat / Go Online
            </Button>
          )}
          <Button variant="destructive" className="gap-2" onClick={handleRevoke}>
            <AlertTriangle className="h-4 w-4" />
            Revoke
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
