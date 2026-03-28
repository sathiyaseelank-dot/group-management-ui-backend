import { useEffect, useState } from 'react';
import { getResources, getRemoteNetworks, getAgents, deleteResource } from '@/lib/mock-api';
import { Resource, RemoteNetwork, Agent } from '@/lib/types';
import { ResourcesList } from '@/components/dashboard/resources/resources-list';
import { Loader2, Plus, Database, Shield } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { AddResourceModal } from '@/components/dashboard/resources/add-resource-modal';
import { EditResourceModal } from '@/components/dashboard/resources/edit-resource-modal';

export default function ResourcesPage() {
  const [resources, setResources] = useState<Resource[]>([]);
  const [remoteNetworks, setRemoteNetworks] = useState<RemoteNetwork[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [isAddModalOpen, setIsAddModalOpen] = useState(false);
  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [editingResource, setEditingResource] = useState<Resource | null>(null);

  const loadData = async () => {
    setLoading(true);
    try {
      const [resourcesData, networksData, agentsData] = await Promise.all([
        getResources(),
        getRemoteNetworks(),
        getAgents(),
      ]);
      setResources(resourcesData);
      setRemoteNetworks(networksData);
      setAgents(agentsData);
    } catch (error) {
      console.error('Failed to load data:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleEditClick = (resource: Resource) => {
    setEditingResource(resource);
    setIsEditModalOpen(true);
  };

  const handleDeleteResource = async (resourceId: string) => {
    try {
      await deleteResource(resourceId);
      loadData();
    } catch (error) {
      console.error('Failed to delete resource:', error);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center p-16">
        <div className="flex flex-col items-center gap-3">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
          <p className="text-xs text-muted-foreground font-mono tracking-wider">Loading resources...</p>
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
            <Database className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h1 className="font-display text-xl font-bold uppercase tracking-wide">Resources</h1>
            <p className="text-xs text-muted-foreground mt-0.5">
              Protected network resources and access policies
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5 rounded-lg bg-muted/60 px-3 py-1.5 ring-1 ring-border/30">
            <Shield className="h-3 w-3 text-secure" />
            <span className="text-[11px] font-mono text-muted-foreground">{resources.length} resources</span>
          </div>
          <Button className="gap-2 font-display font-semibold uppercase tracking-wider text-[12px]" size="sm" onClick={() => setIsAddModalOpen(true)}>
            <Plus className="h-4 w-4" />
            Add Resource
          </Button>
        </div>
      </div>

      {/* Resources List */}
      <ResourcesList
        resources={resources}
        remoteNetworks={remoteNetworks}
        agents={agents}
        onEdit={handleEditClick}
        onDelete={handleDeleteResource}
        onFirewallStatusChange={loadData}
      />

      {/* Modals */}
      <AddResourceModal
        isOpen={isAddModalOpen}
        onClose={() => setIsAddModalOpen(false)}
        onResourceAdded={loadData}
      />

      <EditResourceModal
        resource={editingResource}
        isOpen={isEditModalOpen}
        onClose={() => {
          setIsEditModalOpen(false);
          setEditingResource(null);
        }}
        onResourceUpdated={loadData}
      />
    </div>
  );
}
