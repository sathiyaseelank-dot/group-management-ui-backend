'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Plus } from 'lucide-react';
import { addUser } from '@/lib/mock-api';

interface AddUserModalProps {
  isOpen: boolean;
  onClose: () => void;
  onUserAdded: () => void;
}

export function AddUserModal({ isOpen, onClose, onUserAdded }: AddUserModalProps) {
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [status, setStatus] = useState<'active' | 'inactive'>('active');
  const [isAdding, setIsAdding] = useState(false);

  const handleAddUser = async () => {
    if (!name.trim() || !email.trim()) return;
    setIsAdding(true);
    try {
      await addUser({ name, email, status });
      onUserAdded();
      onClose();
      setName('');
      setEmail('');
      setStatus('active');
    } catch (error) {
      console.error('Failed to add user:', error);
    } finally {
      setIsAdding(false);
    }
  };

  const handleClose = () => {
    setName('');
    setEmail('');
    setStatus('active');
    onClose();
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>Add New User</DialogTitle>
          <DialogDescription>
            Create a new user identity for access control.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="name" className="text-right">
              Name
            </Label>
            <Input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="col-span-3"
              placeholder="e.g., John Doe"
            />
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="email" className="text-right">
              Email
            </Label>
            <Input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="col-span-3"
              placeholder="e.g., john@company.com"
            />
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label className="text-right">Status</Label>
            <div className="col-span-3 flex gap-2">
              <Button
                variant={status === 'active' ? 'secondary' : 'ghost'}
                size="sm"
                onClick={() => setStatus('active')}
              >
                Active
              </Button>
              <Button
                variant={status === 'inactive' ? 'secondary' : 'ghost'}
                size="sm"
                onClick={() => setStatus('inactive')}
              >
                Inactive
              </Button>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isAdding}>
            Cancel
          </Button>
          <Button onClick={handleAddUser} disabled={isAdding || !name.trim() || !email.trim()}>
            {isAdding ? (
              <>
                <Plus className="mr-2 h-4 w-4 animate-spin" /> Adding...
              </>
            ) : (
              'Add User'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
