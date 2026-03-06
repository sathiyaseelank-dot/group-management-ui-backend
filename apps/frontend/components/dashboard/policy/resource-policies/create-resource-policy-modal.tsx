'use client';

import { useMemo, useState } from 'react';
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
import { createResourcePolicy } from '@/lib/resource-policies';

interface CreateResourcePolicyModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreated: () => void;
}

export function CreateResourcePolicyModal({
  isOpen,
  onClose,
  onCreated,
}: CreateResourcePolicyModalProps) {
  const [name, setName] = useState('');
  const [isCreating, setIsCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const trimmed = useMemo(() => name.trim(), [name]);

  const handleCreate = async () => {
    setError(null);
    if (!trimmed) {
      setError('Policy name is required.');
      return;
    }
    setIsCreating(true);
    try {
      createResourcePolicy(trimmed);
      setName('');
      onCreated();
    } catch (e) {
      setError((e as Error).message || 'Failed to create policy.');
    } finally {
      setIsCreating(false);
    }
  };

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(open) => {
        if (!open) {
          setError(null);
          onClose();
        }
      }}
    >
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>Create Policy</DialogTitle>
          <DialogDescription>
            Enter a policy name. The policy will be added to the list immediately.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3 py-2">
          <div className="grid gap-2">
            <Label htmlFor="policyName">Policy Name</Label>
            <Input
              id="policyName"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g., Contractors Policy"
              autoFocus
            />
            {error && (
              <p className="text-sm text-destructive" role="alert">
                {error}
              </p>
            )}
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={isCreating}>
            Cancel
          </Button>
          <Button onClick={handleCreate} disabled={isCreating || !trimmed}>
            {isCreating ? 'Creating...' : 'Create'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

