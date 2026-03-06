import { useState } from 'react';
import { pingConnector } from '@/lib/mock-api';
import { ConnectorDiagnostic, PingResult } from '@/lib/types';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Loader2, Wifi, WifiOff } from 'lucide-react';

interface Props {
  connectors: ConnectorDiagnostic[];
}

function formatStaleness(seconds: number): string {
  if (seconds <= 0) return 'just now';
  if (seconds < 60) return `${Math.round(seconds)}s ago`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h ago`;
  return `${Math.round(seconds / 86400)}d ago`;
}

export function ConnectivityPanel({ connectors }: Props) {
  const [pingResults, setPingResults] = useState<Record<string, PingResult>>({});
  const [pinging, setPinging] = useState<Record<string, boolean>>({});

  const onlineCount = connectors.filter((c) => c.status === 'online').length;
  const streamActiveCount = Object.values(pingResults).filter((r) => r.streamActive).length;

  const handlePing = async (id: string) => {
    setPinging((prev) => ({ ...prev, [id]: true }));
    try {
      const result = await pingConnector(id);
      setPingResults((prev) => ({ ...prev, [id]: result }));
    } catch (err) {
      console.error('Ping failed:', err);
    } finally {
      setPinging((prev) => ({ ...prev, [id]: false }));
    }
  };

  return (
    <div className="space-y-4">
      {/* Summary */}
      <div className="grid grid-cols-3 gap-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Total Connectors</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">{connectors.length}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Online</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold text-green-600">{onlineCount}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Streams Active (pinged)</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold text-blue-600">{streamActiveCount}</p>
          </CardContent>
        </Card>
      </div>

      {/* Table */}
      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Stream Active</TableHead>
                <TableHead>Staleness</TableHead>
                <TableHead>Last Seen</TableHead>
                <TableHead className="text-right">Action</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {connectors.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-muted-foreground py-8">
                    No connectors found
                  </TableCell>
                </TableRow>
              )}
              {connectors.map((connector) => {
                const result = pingResults[connector.id];
                const isPinging = pinging[connector.id] ?? false;
                const isOnline = connector.status === 'online';
                const staleness = result ? result.stalenessSeconds : connector.stalenessSeconds;
                const lastSeenAt = result ? result.lastSeenAt : connector.lastSeenAt;
                const streamActive = result ? result.streamActive : null;

                return (
                  <TableRow key={connector.id}>
                    <TableCell className="font-medium">{connector.name || connector.id}</TableCell>
                    <TableCell>
                      <Badge variant={isOnline ? 'default' : 'secondary'}>
                        {connector.status}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {streamActive === null ? (
                        <span className="text-muted-foreground text-sm">—</span>
                      ) : streamActive ? (
                        <div className="flex items-center gap-1 text-green-600">
                          <Wifi className="h-4 w-4" />
                          <span className="text-sm">Active</span>
                        </div>
                      ) : (
                        <div className="flex items-center gap-1 text-red-500">
                          <WifiOff className="h-4 w-4" />
                          <span className="text-sm">Inactive</span>
                        </div>
                      )}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {staleness > 0 ? formatStaleness(staleness) : lastSeenAt ? 'just now' : 'Never'}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {lastSeenAt ? new Date(lastSeenAt).toLocaleString() : 'Never'}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => void handlePing(connector.id)}
                        disabled={isPinging}
                      >
                        {isPinging ? (
                          <>
                            <Loader2 className="h-3 w-3 animate-spin mr-1" />
                            Pinging…
                          </>
                        ) : (
                          'Ping'
                        )}
                      </Button>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
