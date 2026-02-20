'use client';

import { useEffect, useState } from 'react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import { getGroup } from '@/lib/mock-api';
import { Group, GroupMember, Resource } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { GroupMembersSection } from '@/components/dashboard/groups/group-members-section';
import { GroupResourcesSection } from '@/components/dashboard/groups/group-resources-section';
import { Loader2, ArrowLeft } from 'lucide-react';

export default function GroupDetailPage() {
  const { groupId } = useParams();
  const [group, setGroup] = useState<Group | null>(null);
  const [members, setMembers] = useState<GroupMember[]>([]);
  const [resources, setResources] = useState<Resource[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAddMembersModal, setShowAddMembersModal] = useState(false);

  useEffect(() => {
    const loadGroupData = async () => {
      try {
        const { group, members, resources } = await getGroup(groupId as string);
        setGroup(group);
        setMembers(members);
        setResources(resources);
      } catch (error) {
        console.error('Failed to load group:', error);
      } finally {
        setLoading(false);
      }
    };

    if (groupId) {
      loadGroupData();
    }
  }, [groupId]);

  if (loading) {
    return (
      <div className="flex items-center justify-center p-12">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!group) {
    return (
      <div className="space-y-4 p-6">
        <Link href="/dashboard/groups">
          <Button variant="ghost" className="gap-2">
            <ArrowLeft className="h-4 w-4" />
            Back to Groups
          </Button>
        </Link>
        <p className="text-muted-foreground">Group not found</p>
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      {/* Breadcrumb & Header */}
      <div className="flex items-center justify-between">
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Link href="/dashboard/groups" className="hover:text-foreground">
              Groups
            </Link>
            <span>/</span>
            <span>{group.name}</span>
          </div>
          <h1 className="text-2xl font-bold">{group.name}</h1>
          <p className="text-sm text-muted-foreground">{group.description}</p>
        </div>
        <Link href="/dashboard/groups">
          <Button variant="outline" className="gap-2">
            <ArrowLeft className="h-4 w-4" />
            Back
          </Button>
        </Link>
      </div>

      {/* Members Section */}
      <GroupMembersSection
        groupId={group.id}
        members={members}
        onMembersChange={setMembers}
        showAddModal={showAddMembersModal}
        onAddModalChange={setShowAddMembersModal}
      />

      {/* Resources Section (Read-only) */}
      <GroupResourcesSection
        groupId={group.id}
        resources={resources}
      />
    </div>
  );
}
