'use client';

import { useEffect, useState } from 'react';
import { getResources, getRemoteNetworks } from '@/lib/mock-api';
import { Resource, RemoteNetwork } from '@/lib/types';
import { ResourcesList } from '@/components/dashboard/resources/resources-list';
import { Loader2, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { AddResourceModal } from '@/components/dashboard/resources/add-resource-modal';
import { EditResourceModal } from '@/components/dashboard/resources/edit-resource-modal';

export default function ResourcesPage() {
  const [resources, setResources] = useState<Resource[]>([]);
  const [remoteNetworks, setRemoteNetworks] = useState<RemoteNetwork[]>([]);
  const [loading, setLoading] = useState(true);
  const [isAddModalOpen, setIsAddModalOpen] = useState(false);
  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [editingResource, setEditingResource] = useState<Resource | null>(null);

  const loadData = async () => {
    setLoading(true);
    try {
      const [resourcesData, networksData] = await Promise.all([
        getResources(),
        getRemoteNetworks(),
      ]);
      setResources(resourcesData);
      setRemoteNetworks(networksData);
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
          <h1 className="text-2xl font-bold">Resources</h1>
          <p className="text-sm text-muted-foreground">
            Manage protected network resources and their access policies
          </p>
        </div>
        <Button className="gap-2" onClick={() => setIsAddModalOpen(true)}>
          <Plus className="h-4 w-4" />
          Add Resource
        </Button>
      </div>

      {/* Resources List */}
      <ResourcesList
        resources={resources}
        remoteNetworks={remoteNetworks}
        onEdit={handleEditClick}
      />

      {/* Add Resource Modal */}
      <AddResourceModal
        isOpen={isAddModalOpen}
        onClose={() => setIsAddModalOpen(false)}
        onResourceAdded={loadData}
      />
      
      {/* Edit Resource Modal */}
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
