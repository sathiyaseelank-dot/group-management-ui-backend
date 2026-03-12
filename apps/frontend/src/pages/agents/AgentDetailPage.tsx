import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Link } from 'react-router-dom';
import { getAgent, deleteAgent, revokeAgent, grantAgent } from '@/lib/mock-api';
import { Agent, RemoteNetwork } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Loader2, ArrowLeft, AlertTriangle, ShieldOff, ShieldCheck, Trash2 } from 'lucide-react';
import { AgentInfoSection } from '@/components/dashboard/agents/tunneler-info-section';
import { ConnectorLogs } from '@/components/dashboard/connectors/connector-logs';
import { AgentInstall } from '@/components/dashboard/agents/agent-install';
import { toast } from 'sonner';

interface LogEntry {
  id: number;
  timestamp: string;
  message: string;
}

export default function AgentDetailPage() {
  const { agentId } = useParams();
  const navigate = useNavigate();
  const [agent, setAgent] = useState<Agent | null>(null);
  const [network, setNetwork] = useState<RemoteNetwork | undefined>(undefined);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [isRevoking, setIsRevoking] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  const loadAgentData = async (opts?: { silent?: boolean }) => {
    if (!opts?.silent) {
      setLoading(true);
    }
    try {
      const { agent: fetchedAgent, network: fetchedNetwork, logs: fetchedLogs } = await getAgent(agentId as string);
      setAgent(fetchedAgent);
      setNetwork(fetchedNetwork);
      setLogs(fetchedLogs);
    } catch (error) {
      console.error('Failed to load agent details:', error);
    } finally {
      if (!opts?.silent) {
        setLoading(false);
      }
    }
  };

  useEffect(() => {
    if (agentId) {
      loadAgentData();
    }
  }, [agentId]);

  const handleDelete = async () => {
    if (!agentId) return;
    if (!window.confirm(`Delete agent "${agent?.name ?? agentId}"? This will remove it from the controller.`)) return;
    setIsDeleting(true);
    try {
      await deleteAgent(agentId);
      toast.success('Agent deleted successfully.');
      navigate('/dashboard/agents');
    } catch (error) {
      toast.error('Failed to delete agent. Check that the backend is running.');
    } finally {
      setIsDeleting(false);
    }
  };

  const handleRevoke = async () => {
    if (!agentId) return;
    setIsRevoking(true);
    try {
      await revokeAgent(agentId);
      toast.success('Agent access revoked.');
      await loadAgentData({ silent: true });
    } catch (error) {
      toast.error('Failed to revoke agent access.');
    } finally {
      setIsRevoking(false);
    }
  };

  const handleGrant = async () => {
    if (!agentId) return;
    setIsRevoking(true);
    try {
      await grantAgent(agentId);
      toast.success('Agent access granted.');
      await loadAgentData({ silent: true });
    } catch (error) {
      toast.error('Failed to grant agent access.');
    } finally {
      setIsRevoking(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center p-12">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="space-y-6 p-6">
        <Link to="/dashboard/agents">
          <Button variant="ghost" className="gap-2">
            <ArrowLeft className="h-4 w-4" />
            Back to Agents
          </Button>
        </Link>
        <div className="text-center py-20">
          <AlertTriangle className="mx-auto h-16 w-16 text-muted-foreground" />
          <h2 className="mt-4 text-2xl font-bold">Agent Not Found</h2>
          <p className="mt-2 text-muted-foreground">
            This agent is not registered yet. Use the install command below to enroll it.
          </p>
        </div>
        <AgentInstall initialAgentId={agentId} />
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      {/* Breadcrumb & Header */}
      <div className="flex items-center justify-between">
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Link to="/dashboard/agents" className="hover:text-foreground">
              Agents
            </Link>
            <span>/</span>
            <span>{agent.name}</span>
          </div>
          <h1 className="text-2xl font-bold">{agent.name}</h1>
        </div>
        <div className="flex gap-2">
          {agent.status !== 'revoked' && (
            <Button
              variant="outline"
              className="gap-2 text-orange-500 border-orange-500 hover:text-orange-600 hover:border-orange-600"
              onClick={handleRevoke}
              disabled={isRevoking}
            >
              {isRevoking ? <Loader2 className="h-4 w-4 animate-spin" /> : <ShieldOff className="h-4 w-4" />}
              Revoke
            </Button>
          )}
          {agent.status === 'revoked' && (
            <Button
              variant="outline"
              className="gap-2 text-green-500 border-green-500 hover:text-green-600 hover:border-green-600"
              onClick={handleGrant}
              disabled={isRevoking}
            >
              {isRevoking ? <Loader2 className="h-4 w-4 animate-spin" /> : <ShieldCheck className="h-4 w-4" />}
              Grant
            </Button>
          )}
          <Button variant="destructive" className="gap-2" onClick={handleDelete} disabled={isDeleting}>
            {isDeleting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
            Delete
          </Button>
        </div>
      </div>

      {/* Agent Info Section */}
      <AgentInfoSection agent={agent} network={network} />

      {/* Logs Section */}
      <ConnectorLogs logs={logs} />

      {/* Install card when not installed */}
      {!agent.installed && (
        <AgentInstall
          initialAgentId={agentId}
          initialConnectorId={agent.connectorId}
        />
      )}
    </div>
  );
}
