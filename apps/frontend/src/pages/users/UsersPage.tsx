import { useEffect, useState } from 'react';
import { deleteUser, deactivateUser, getUsers } from '@/lib/mock-api';
import { User } from '@/lib/types';
import { UsersList } from '@/components/dashboard/users/users-list';
import { AddUserModal } from '@/components/dashboard/users/add-user-modal';
import { EditUserModal } from '@/components/dashboard/users/edit-user-modal';
import { InviteUserModal } from '@/components/dashboard/users/invite-user-modal';
import { Button } from '@/components/ui/button';
import { Loader2, Mail, Plus, Users, UserCheck } from 'lucide-react';

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [isEditOpen, setIsEditOpen] = useState(false);
  const [isInviteOpen, setIsInviteOpen] = useState(false);
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
      <div className="flex items-center justify-center p-16">
        <div className="flex flex-col items-center gap-3">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
          <p className="text-xs text-muted-foreground font-mono tracking-wider">Loading users...</p>
        </div>
      </div>
    );
  }

  const activeCount = users.filter(u => u.status === 'active').length;

  return (
    <div className="space-y-6 p-6">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-primary/10 ring-1 ring-primary/20">
            <Users className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h1 className="font-display text-xl font-bold uppercase tracking-wide">Users</h1>
            <p className="text-xs text-muted-foreground mt-0.5">
              Identity subjects for access control
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5 rounded-lg bg-muted/60 px-3 py-1.5 ring-1 ring-border/30">
            <UserCheck className="h-3 w-3 text-secure" />
            <span className="text-[11px] font-mono text-muted-foreground">{activeCount} active</span>
          </div>
          <Button variant="outline" size="sm" className="gap-2 text-[12px]" onClick={() => setIsInviteOpen(true)}>
            <Mail className="h-4 w-4" />
            Invite
          </Button>
          <Button size="sm" className="gap-2 font-display font-semibold uppercase tracking-wider text-[12px]" onClick={() => setIsModalOpen(true)}>
            <Plus className="h-4 w-4" />
            Add User
          </Button>
        </div>
      </div>

      {/* Users List */}
      <UsersList
        users={users}
        onEditUser={handleEditUser}
        onDeactivateUser={handleDeactivateUser}
        onDeleteUser={handleDeleteUser}
      />

      {/* Modals */}
      <AddUserModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onUserAdded={loadUsers}
      />

      <InviteUserModal
        isOpen={isInviteOpen}
        onClose={() => setIsInviteOpen(false)}
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
