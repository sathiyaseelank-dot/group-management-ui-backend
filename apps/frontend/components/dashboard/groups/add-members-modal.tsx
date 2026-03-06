'use client';

import { useState, useCallback, useMemo } from 'react';
import { GroupMember } from '@/lib/types';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog';
import { SubjectPicker } from '@/components/subjects/subject-picker';
import { SelectedSubject } from '@/lib/types';
import { updateGroupMembers } from '@/lib/mock-api';
import { toast } from 'sonner';

interface AddMembersModalProps {
  groupId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentMembers: GroupMember[];
  onMembersUpdated: (members: GroupMember[]) => void;
}

export function AddMembersModal({
  groupId,
  open,
  onOpenChange,
  currentMembers,
  onMembersUpdated,
}: AddMembersModalProps) {
  const [selectedUsers, setSelectedUsers] = useState<SelectedSubject[]>([]);
  const [saving, setSaving] = useState(false);

  // Exclude already added members
  const excludeUserIds = useMemo(
    () => currentMembers.map((m) => m.userId),
    [currentMembers]
  );

  const handleSave = useCallback(async () => {
    if (selectedUsers.length === 0) {
      toast.error('Please select at least one user');
      return;
    }

    setSaving(true);
    try {
      // Get all member IDs (current + newly selected)
      const allMemberIds = [
        ...currentMembers.map((m) => m.userId),
        ...selectedUsers.map((u) => u.id),
      ];

      await updateGroupMembers(groupId, allMemberIds);

      // Combine current members with new selections for UI update
      const newMembers: GroupMember[] = [
        ...currentMembers,
        ...selectedUsers.map((u) => ({
          userId: u.id,
          userName: u.label.replace('User: ', ''),
          email: '', // Email would come from API in real implementation
        })),
      ];

      onMembersUpdated(newMembers);
      setSelectedUsers([]);
      onOpenChange(false);
      toast.success(`Added ${selectedUsers.length} member(s) to group`);
    } catch (error) {
      toast.error('Failed to update group members');
    } finally {
      setSaving(false);
    }
  }, [groupId, selectedUsers, currentMembers, onMembersUpdated, onOpenChange]);

  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      setSelectedUsers([]);
    }
    onOpenChange(newOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Add Members to Group</DialogTitle>
          <DialogDescription>
            Select users to add to this group. They will inherit all resource permissions.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <SubjectPicker
            selectedSubjects={selectedUsers}
            onSelectionChange={setSelectedUsers}
            subjectTypes={['USER']}
            excludeSubjects={excludeUserIds}
          />
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => handleOpenChange(false)}
            disabled={saving}
          >
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={saving || selectedUsers.length === 0}>
            {saving ? 'Saving...' : 'Add Members'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
