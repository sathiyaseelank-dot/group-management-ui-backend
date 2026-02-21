'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { getGroups } from '@/lib/mock-api';
import { Group } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { GroupsList } from '@/components/dashboard/groups/groups-list';
import { Loader2, Plus } from 'lucide-react';
import { AddGroupModal } from '@/components/dashboard/groups/add-group-modal';

export default function GroupsPage() {
  const [groups, setGroups] = useState<Group[]>([]);
  const [loading, setLoading] = useState(true);
  const [isModalOpen, setIsModalOpen] = useState(false);

  const loadGroups = async () => {
    setLoading(true);
    try {
      const data = await getGroups();
      setGroups(data);
    } catch (error) {
      console.error('Failed to load groups:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadGroups();
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
          <h1 className="text-2xl font-bold">Groups</h1>
          <p className="text-sm text-muted-foreground">
            Manage identity groups and their members
          </p>
        </div>
        <Button className="gap-2" onClick={() => setIsModalOpen(true)}>
          <Plus className="h-4 w-4" />
          Add Group
        </Button>
      </div>

      {/* Groups List */}
      <GroupsList groups={groups} />

      {/* Add Group Modal */}
      <AddGroupModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onGroupAdded={loadGroups} // Reload groups after a new one is added
      />
    </div>
  );
}
