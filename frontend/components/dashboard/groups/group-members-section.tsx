'use client';

import { useState, useCallback } from 'react';
import { GroupMember } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { AddMembersModal } from './add-members-modal';
import { Users, Trash2 } from 'lucide-react';
import { removeGroupMember } from '@/lib/mock-api';
import { toast } from 'sonner';

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
                  <TableHead className="text-right font-semibold">Actions</TableHead>
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
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleRemoveMember(member.userId)}
                        disabled={deleting === member.userId}
                        className="text-destructive hover:text-destructive hover:bg-destructive/10"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
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
    </Card>
  );
}
