import { useEffect, useState } from 'react';
import { deleteGroup, getGroups } from '@/lib/mock-api';
import { Group } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { GroupsList } from '@/components/dashboard/groups/groups-list';
import { Loader2, Plus, Users, Shield } from 'lucide-react';
import { AddGroupModal } from '@/components/dashboard/groups/add-group-modal';
import { EditGroupModal } from '@/components/dashboard/groups/edit-group-modal';

export default function GroupsPage() {
  const [groups, setGroups] = useState<Group[]>([]);
  const [loading, setLoading] = useState(true);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [isEditOpen, setIsEditOpen] = useState(false);
  const [editingGroup, setEditingGroup] = useState<Group | null>(null);

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

  const handleEditGroup = (group: Group) => {
    setEditingGroup(group);
    setIsEditOpen(true);
  };

  const handleDeleteGroup = async (group: Group) => {
    const confirmed = window.confirm(`Delete ${group.name}? This cannot be undone.`);
    if (!confirmed) return;
    try {
      await deleteGroup(group.id);
      await loadGroups();
    } catch (error) {
      console.error('Failed to delete group:', error);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center p-16">
        <div className="flex flex-col items-center gap-3">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
          <p className="text-xs text-muted-foreground font-mono tracking-wider">Loading groups...</p>
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
            <Users className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h1 className="font-display text-xl font-bold uppercase tracking-wide">Groups</h1>
            <p className="text-xs text-muted-foreground mt-0.5">
              Identity groups and their members
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5 rounded-lg bg-muted/60 px-3 py-1.5 ring-1 ring-border/30">
            <Shield className="h-3 w-3 text-primary/70" />
            <span className="text-[11px] font-mono text-muted-foreground">{groups.length} groups</span>
          </div>
          <Button size="sm" className="gap-2 font-display font-semibold uppercase tracking-wider text-[12px]" onClick={() => setIsModalOpen(true)}>
            <Plus className="h-4 w-4" />
            Add Group
          </Button>
        </div>
      </div>

      {/* Groups List */}
      <GroupsList groups={groups} onEditGroup={handleEditGroup} onDeleteGroup={handleDeleteGroup} />

      {/* Modals */}
      <AddGroupModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onGroupAdded={loadGroups}
      />

      <EditGroupModal
        isOpen={isEditOpen}
        group={editingGroup}
        onClose={() => {
          setIsEditOpen(false);
          setEditingGroup(null);
        }}
        onGroupUpdated={loadGroups}
      />
    </div>
  );
}
