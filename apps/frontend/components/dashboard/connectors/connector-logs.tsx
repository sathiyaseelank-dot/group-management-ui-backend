'use client';

import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { ScrollArea } from '@/components/ui/scroll-area';
import { BookText } from 'lucide-react';

interface LogEntry {
  id: number;
  timestamp: string;
  message: string;
}

interface ConnectorLogsProps {
  logs: LogEntry[];
}

export function ConnectorLogs({ logs }: ConnectorLogsProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <BookText className="h-5 w-5" />
          Recent Events
        </CardTitle>
        <CardDescription>
          A stream of recent events from the connector.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <ScrollArea className="h-64 w-full rounded-md border bg-muted/20">
          <div className="p-4 text-sm">
            {logs.length === 0 ? (
              <p className="text-muted-foreground p-4 text-center">No recent events.</p>
            ) : (
              <div className="space-y-2">
                {logs.map((log) => (
                  <div key={log.id} className="flex gap-4 font-mono text-xs">
                    <span className="text-muted-foreground">{log.timestamp}</span>
                    <span>{log.message}</span>
                  </div>
                )).reverse()}
              </div>
            )}
          </div>
        </ScrollArea>
      </CardContent>
    </Card>
  );
}
