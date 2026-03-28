import { useEffect, useState } from 'react';
import { Loader2, ScrollText, Shield, CheckCircle, XCircle } from 'lucide-react';

interface AuditLogEntry {
  id: number;
  timestamp: number;
  actor: string;
  action: string;
  target: string;
  result: string;
}

function ResultBadge({ result }: { result: string }) {
  const lower = result.toLowerCase();
  if (lower === 'success' || lower === 'ok') {
    return (
      <span className="inline-flex items-center gap-1 rounded-md bg-secure/10 px-2 py-0.5 text-[11px] font-medium text-secure ring-1 ring-inset ring-secure/20">
        <CheckCircle className="h-3 w-3" />
        {result}
      </span>
    );
  }
  if (lower === 'failure' || lower === 'denied' || lower === 'error') {
    return (
      <span className="inline-flex items-center gap-1 rounded-md bg-destructive/10 px-2 py-0.5 text-[11px] font-medium text-destructive ring-1 ring-inset ring-destructive/20">
        <XCircle className="h-3 w-3" />
        {result}
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground ring-1 ring-inset ring-border">
      {result}
    </span>
  );
}

export default function AuditLogsPage() {
  const [logs, setLogs] = useState<AuditLogEntry[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const token = typeof window !== 'undefined' ? localStorage.getItem('authToken') : null;
    fetch('/api/audit-logs', {
      credentials: 'same-origin',
      headers: token ? { Authorization: `Bearer ${token}` } : undefined,
    })
      .then((r) => r.json())
      .then((data) => setLogs(Array.isArray(data) ? data : []))
      .catch(console.error)
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center p-16">
        <div className="flex flex-col items-center gap-3">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
          <p className="text-xs text-muted-foreground font-mono tracking-wider">Loading audit trail...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-primary/10 ring-1 ring-primary/20">
            <ScrollText className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h1 className="font-display text-xl font-bold uppercase tracking-wide">Audit Logs</h1>
            <p className="text-xs text-muted-foreground mt-0.5">Admin activity and security events</p>
          </div>
        </div>
        <div className="flex items-center gap-1.5 rounded-lg bg-muted/60 px-3 py-1.5 ring-1 ring-border/30">
          <Shield className="h-3 w-3 text-primary/70" />
          <span className="text-[11px] font-mono text-muted-foreground">{logs.length} events</span>
        </div>
      </div>

      {logs.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border/40 bg-muted/10 p-16 text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-xl bg-muted/60 ring-1 ring-border/30">
            <ScrollText className="h-7 w-7 text-muted-foreground/60" />
          </div>
          <p className="mt-5 font-display text-base font-semibold uppercase tracking-wide">No audit log entries</p>
          <p className="mt-1.5 text-xs text-muted-foreground">Activity will appear here as admin actions are performed.</p>
        </div>
      ) : (
        <div className="rounded-xl border border-border/40 overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border/40 bg-muted/20">
                <th className="px-4 py-3 text-left font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/70">Time</th>
                <th className="px-4 py-3 text-left font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/70">Actor</th>
                <th className="px-4 py-3 text-left font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/70">Action</th>
                <th className="px-4 py-3 text-left font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/70">Target</th>
                <th className="px-4 py-3 text-left font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/70">Result</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((entry) => (
                <tr key={entry.id} className="border-b border-border/20 last:border-0 transition-colors hover:bg-primary/[0.02]">
                  <td className="px-4 py-2.5 text-xs text-muted-foreground font-mono">
                    {new Date(entry.timestamp * 1000).toLocaleString()}
                  </td>
                  <td className="px-4 py-2.5 text-sm">{entry.actor}</td>
                  <td className="px-4 py-2.5 font-mono text-xs text-primary">{entry.action}</td>
                  <td className="px-4 py-2.5 text-sm">{entry.target}</td>
                  <td className="px-4 py-2.5">
                    <ResultBadge result={entry.result} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
