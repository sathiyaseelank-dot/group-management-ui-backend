import { useEffect, useState } from 'react';
import { getUsers, getResources, traceAccess } from '@/lib/mock-api';
import { User, Resource, AccessTrace, TraceHop } from '@/lib/types';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Loader2, User as UserIcon, Users, Database, Globe, Server, ArrowRight } from 'lucide-react';

function HopIcon({ type }: { type: TraceHop['type'] }) {
  switch (type) {
    case 'user': return <UserIcon className="h-5 w-5" />;
    case 'group': return <Users className="h-5 w-5" />;
    case 'resource': return <Database className="h-5 w-5" />;
    case 'remote_network': return <Globe className="h-5 w-5" />;
    case 'connector': return <Server className="h-5 w-5" />;
  }
}

function HopCard({ hop }: { hop: TraceHop }) {
  return (
    <div
      className={`flex flex-col items-center gap-1 rounded-lg border p-3 min-w-[110px] text-center ${
        hop.healthy ? 'border-green-400 bg-green-50' : 'border-red-300 bg-red-50'
      }`}
    >
      <div className={hop.healthy ? 'text-green-600' : 'text-red-500'}>
        <HopIcon type={hop.type} />
      </div>
      <span className="text-xs font-medium text-muted-foreground capitalize">
        {hop.type.replace('_', ' ')}
      </span>
      <span className="text-sm font-semibold truncate max-w-[100px]" title={hop.name}>
        {hop.name}
      </span>
      <Badge variant={hop.healthy ? 'default' : 'secondary'} className="text-xs">
        {hop.status}
      </Badge>
    </div>
  );
}

export function AccessTracePanel() {
  const [users, setUsers] = useState<User[]>([]);
  const [resources, setResources] = useState<Resource[]>([]);
  const [selectedUser, setSelectedUser] = useState('');
  const [selectedResource, setSelectedResource] = useState('');
  const [tracing, setTracing] = useState(false);
  const [result, setResult] = useState<AccessTrace | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    void Promise.all([getUsers(), getResources()]).then(([u, r]) => {
      setUsers(u);
      setResources(r);
    });
  }, []);

  const handleTrace = async () => {
    if (!selectedUser || !selectedResource) return;
    setTracing(true);
    setError(null);
    setResult(null);
    try {
      const trace = await traceAccess(selectedUser, selectedResource);
      setResult(trace);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Trace failed');
    } finally {
      setTracing(false);
    }
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Trace Access Path</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap items-end gap-4">
          <div className="space-y-1 flex-1 min-w-[200px]">
            <label className="text-sm font-medium">User</label>
            <Select value={selectedUser} onValueChange={setSelectedUser}>
              <SelectTrigger>
                <SelectValue placeholder="Select a user…" />
              </SelectTrigger>
              <SelectContent>
                {users.map((u) => (
                  <SelectItem key={u.id} value={u.id}>
                    {u.name} ({u.email})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1 flex-1 min-w-[200px]">
            <label className="text-sm font-medium">Resource</label>
            <Select value={selectedResource} onValueChange={setSelectedResource}>
              <SelectTrigger>
                <SelectValue placeholder="Select a resource…" />
              </SelectTrigger>
              <SelectContent>
                {resources.map((r) => (
                  <SelectItem key={r.id} value={r.id}>
                    {r.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <Button
            onClick={() => void handleTrace()}
            disabled={!selectedUser || !selectedResource || tracing}
          >
            {tracing ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
                Tracing…
              </>
            ) : (
              'Trace'
            )}
          </Button>
        </CardContent>
      </Card>

      {error && (
        <div className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700">
          {error}
        </div>
      )}

      {result && (
        <div className="space-y-4">
          <div className="flex items-center gap-3">
            <Badge
              className={`text-base px-4 py-1 ${result.allowed ? 'bg-green-600 hover:bg-green-600' : 'bg-red-600 hover:bg-red-600'}`}
            >
              {result.allowed ? 'ACCESS ALLOWED' : 'ACCESS DENIED'}
            </Badge>
            <span className="text-sm text-muted-foreground">{result.reason}</span>
          </div>

          <Card>
            <CardHeader>
              <CardTitle className="text-sm font-medium">Access Path</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex flex-wrap items-center gap-2">
                {result.path.map((hop, idx) => (
                  <div key={`${hop.type}-${hop.id}`} className="flex items-center gap-2">
                    <HopCard hop={hop} />
                    {idx < result.path.length - 1 && (
                      <ArrowRight className="h-5 w-5 text-muted-foreground shrink-0" />
                    )}
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          {result.userGroups.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-medium">User Groups</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-wrap gap-2">
                {result.userGroups.map((g) => (
                  <Badge key={g.id} variant="outline">{g.name}</Badge>
                ))}
              </CardContent>
            </Card>
          )}

          {result.matchedRules.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-medium">Matched Access Rules</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-wrap gap-2">
                {result.matchedRules.map((rule) => (
                  <Badge key={rule.id} variant={rule.enabled ? 'default' : 'secondary'}>
                    {rule.name}
                  </Badge>
                ))}
              </CardContent>
            </Card>
          )}
        </div>
      )}
    </div>
  );
}
