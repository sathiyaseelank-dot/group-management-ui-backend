import { useEffect, useState } from 'react';
import { getAgents } from '@/lib/mock-api';
import { Agent } from '@/lib/types';
import { AgentsList } from '@/components/dashboard/agents/agents-list';
import { AddAgentModal } from '@/components/dashboard/agents/add-tunneler-modal';
import { Loader2, Plus } from 'lucide-react';
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
      <div className="flex items-center justify-center p-12">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Agents</h1>
          <p className="text-sm text-muted-foreground">
            Manage resource agents for secure access to network resources
          </p>
        </div>
        <Button className="gap-2" onClick={() => setShowAddModal(true)}>
          <Plus className="h-4 w-4" />
          Add Agent
        </Button>
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
