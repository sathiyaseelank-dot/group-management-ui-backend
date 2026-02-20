'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { getGroups } from '@/lib/mock-api';
import { Group } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { GroupsList } from '@/components/dashboard/groups/groups-list';
import { Loader2 } from 'lucide-react';

export default function GroupsPage() {
  const [groups, setGroups] = useState<Group[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadGroups = async () => {
      try {
        const data = await getGroups();
        setGroups(data);
      } catch (error) {
        console.error('Failed to load groups:', error);
      } finally {
        setLoading(false);
      }
    };

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
      </div>

      {/* Groups List */}
      <GroupsList groups={groups} />
    </div>
  );
}
