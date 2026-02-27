'use client';

import { useEffect, useState } from 'react';
import { deleteUser, deactivateUser, getUsers } from '@/lib/mock-api';
import { User } from '@/lib/types';
import { UsersList } from '@/components/dashboard/users/users-list';
import { AddUserModal } from '@/components/dashboard/users/add-user-modal';
import { EditUserModal } from '@/components/dashboard/users/edit-user-modal';
import { Button } from '@/components/ui/button';
import { Loader2, Plus } from 'lucide-react';

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [isEditOpen, setIsEditOpen] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);

  const loadUsers = async () => {
    setLoading(true);
    try {
      const data = await getUsers();
      setUsers(data);
    } catch (error) {
      console.error('Failed to load users:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadUsers();
  }, []);

  const handleEditUser = (user: User) => {
    setEditingUser(user);
    setIsEditOpen(true);
  };

  const handleDeactivateUser = async (user: User) => {
    if (user.status === 'inactive') return;
    const confirmed = window.confirm(`Deactivate ${user.name}?`);
    if (!confirmed) return;
    try {
      await deactivateUser(user.id);
      await loadUsers();
    } catch (error) {
      console.error('Failed to deactivate user:', error);
    }
  };

  const handleDeleteUser = async (user: User) => {
    const confirmed = window.confirm(`Delete ${user.name}? This cannot be undone.`);
    if (!confirmed) return;
    try {
      await deleteUser(user.id);
      await loadUsers();
    } catch (error) {
      console.error('Failed to delete user:', error);
    }
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
          <h1 className="text-2xl font-bold">Users</h1>
          <p className="text-sm text-muted-foreground">
            View all user subjects available for identity and access control
          </p>
        </div>
        <Button className="gap-2" onClick={() => setIsModalOpen(true)}>
          <Plus className="h-4 w-4" />
          Add User
        </Button>
      </div>

      {/* Users List */}
      <UsersList
        users={users}
        onEditUser={handleEditUser}
        onDeactivateUser={handleDeactivateUser}
        onDeleteUser={handleDeleteUser}
      />

      {/* Add User Modal */}
      <AddUserModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onUserAdded={loadUsers}
      />

      <EditUserModal
        isOpen={isEditOpen}
        user={editingUser}
        onClose={() => {
          setIsEditOpen(false);
          setEditingUser(null);
        }}
        onUserUpdated={loadUsers}
      />
    </div>
  );
}
