import { useEffect, useState } from 'react';
import { getAgents } from '@/lib/mock-api';
import { Agent } from '@/lib/types';
import { AgentsList } from '@/components/dashboard/agents/agents-list';
import { AddAgentModal } from '@/components/dashboard/agents/add-tunneler-modal';
import { Loader2, Plus, Shield, Cpu } from 'lucide-react';
import { Button } from '@/components/ui/button';

export default function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAddModal, setShowAddModal] = useState(false);

  useEffect(() => {
    const loadAgents = async () => {
      try {
        const data = await getAgents();
        setAgents(data);
      } catch (error) {
        console.error('Failed to load agents:', error);
      } finally {
        setLoading(false);
      }
    };

    loadAgents();
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center p-16">
        <div className="flex flex-col items-center gap-3">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
          <p className="text-xs text-muted-foreground font-mono tracking-wider">Loading agents...</p>
        </div>
      </div>
    );
  }

  const onlineCount = agents.filter(a => a.status === 'online').length;

  return (
    <div className="space-y-6 p-6">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-primary/10 ring-1 ring-primary/20">
            <Cpu className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h1 className="font-display text-xl font-bold uppercase tracking-wide">Agents</h1>
            <p className="text-xs text-muted-foreground mt-0.5">
              Resource agents for secure network access
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5 rounded-lg bg-muted/60 px-3 py-1.5 ring-1 ring-border/30">
            <Shield className="h-3 w-3 text-secure" />
            <span className="text-[11px] font-mono text-muted-foreground">{onlineCount} online</span>
          </div>
          <Button size="sm" className="gap-2 font-display font-semibold uppercase tracking-wider text-[12px]" onClick={() => setShowAddModal(true)}>
            <Plus className="h-4 w-4" />
            Add Agent
          </Button>
        </div>
      </div>

      {/* Agents List */}
      <AgentsList
        agents={agents}
        onRevoked={(id) => setAgents((prev) => prev.filter((t) => t.id !== id))}
      />

      <AddAgentModal
        isOpen={showAddModal}
        onClose={() => setShowAddModal(false)}
        onAgentAdded={async () => {
          const data = await getAgents();
          setAgents(data);
        }}
      />
    </div>
  );
}
