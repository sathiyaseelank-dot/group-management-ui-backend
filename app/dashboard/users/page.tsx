'use client';

import { useEffect, useState } from 'react';
import { getUsers } from '@/lib/mock-api';
import { User } from '@/lib/types';
import { UsersList } from '@/components/dashboard/users/users-list';
import { Loader2 } from 'lucide-react';

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadUsers = async () => {
      try {
        const data = await getUsers();
        setUsers(data);
      } catch (error) {
        console.error('Failed to load users:', error);
      } finally {
        setLoading(false);
      }
    };

    loadUsers();
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
      <div>
        <h1 className="text-2xl font-bold">Users</h1>
        <p className="text-sm text-muted-foreground">
          View all user subjects available for identity and access control
        </p>
      </div>

      {/* Users List */}
      <UsersList users={users} />
    </div>
  );
}
