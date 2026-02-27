'use client';

import { useState, useCallback } from 'react';
import { GroupMember, User } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { AddMembersModal } from './add-members-modal';
import { Users, MoreHorizontal } from 'lucide-react';
import { deactivateUser, deleteUser, getUser, removeGroupMember } from '@/lib/mock-api';
import { toast } from 'sonner';
import { EditUserModal } from '@/components/dashboard/users/edit-user-modal';

interface GroupMembersSectionProps {
  groupId: string;
  members: GroupMember[];
  onMembersChange: (members: GroupMember[]) => void;
  showAddModal: boolean;
  onAddModalChange: (show: boolean) => void;
}

export function GroupMembersSection({
  groupId,
  members,
  onMembersChange,
  showAddModal,
  onAddModalChange,
}: GroupMembersSectionProps) {
  const [deleting, setDeleting] = useState<string | null>(null);
  const [isEditOpen, setIsEditOpen] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);

  const handleRemoveMember = useCallback(
    async (userId: string) => {
      setDeleting(userId);
      try {
        await removeGroupMember(groupId, userId);
        onMembersChange(members.filter((m) => m.userId !== userId));
        toast.success('Member removed from group');
      } catch (error) {
        toast.error('Failed to remove member');
      } finally {
        setDeleting(null);
      }
    },
    [groupId, members, onMembersChange]
  );

  const handleEditUser = useCallback(async (userId: string) => {
    try {
      const user = await getUser(userId);
      setEditingUser(user);
      setIsEditOpen(true);
    } catch (error) {
      toast.error('Failed to load user details');
    }
  }, []);

  const handleDeactivateUser = useCallback(
    async (userId: string, userName: string) => {
      const confirmed = window.confirm(`Deactivate ${userName}?`);
      if (!confirmed) return;
      try {
        await deactivateUser(userId);
        toast.success('User deactivated');
      } catch (error) {
        toast.error('Failed to deactivate user');
      }
    },
    []
  );

  const handleDeleteUser = useCallback(
    async (userId: string, userName: string) => {
      const confirmed = window.confirm(`Delete ${userName}? This cannot be undone.`);
      if (!confirmed) return;
      try {
        await deleteUser(userId);
        onMembersChange(members.filter((m) => m.userId !== userId));
        toast.success('User deleted');
      } catch (error) {
        toast.error('Failed to delete user');
      }
    },
    [members, onMembersChange]
  );

  const handleUserUpdated = useCallback(async () => {
    if (!editingUser) return;
    try {
      const updated = await getUser(editingUser.id);
      onMembersChange(
        members.map((m) =>
          m.userId === updated.id
            ? { ...m, userName: updated.name, email: updated.email }
            : m
        )
      );
    } catch (error) {
      toast.error('Failed to refresh user details');
    }
  }, [editingUser, members, onMembersChange]);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="space-y-1">
            <CardTitle className="flex items-center gap-2">
              <Users className="h-5 w-5" />
              Members
            </CardTitle>
            <CardDescription>
              Users in this group inherit all resource permissions
            </CardDescription>
          </div>
          <Button onClick={() => onAddModalChange(true)}>
            Add Users
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {members.length === 0 ? (
          <div className="rounded-lg border border-dashed py-8 text-center">
            <p className="text-sm text-muted-foreground">No members yet</p>
          </div>
        ) : (
          <div className="overflow-hidden rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="font-semibold">Name</TableHead>
                  <TableHead className="font-semibold">Email</TableHead>
                  <TableHead className="text-right font-semibold">Activity</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {members.map((member) => (
                  <TableRow key={member.userId}>
                    <TableCell className="font-medium">{member.userName}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {member.email}
                    </TableCell>
                    <TableCell className="text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon" aria-label="Manage user">
                            <MoreHorizontal className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => handleEditUser(member.userId)}>
                            Edit
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() => handleDeactivateUser(member.userId, member.userName)}
                          >
                            Deactivate
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() => handleRemoveMember(member.userId)}
                            disabled={deleting === member.userId}
                          >
                            Remove from group
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() => handleDeleteUser(member.userId, member.userName)}
                          >
                            Delete
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>

      {/* Add Members Modal */}
      <AddMembersModal
        groupId={groupId}
        open={showAddModal}
        onOpenChange={onAddModalChange}
        currentMembers={members}
        onMembersUpdated={onMembersChange}
      />

      <EditUserModal
        isOpen={isEditOpen}
        user={editingUser}
        onClose={() => {
          setIsEditOpen(false);
          setEditingUser(null);
        }}
        onUserUpdated={handleUserUpdated}
      />
    </Card>
  );
}
