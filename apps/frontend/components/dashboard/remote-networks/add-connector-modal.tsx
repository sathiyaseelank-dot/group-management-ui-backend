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
import { Plus, Loader2 } from 'lucide-react';
import { addConnector } from '@/lib/mock-api'; // Will be implemented next
import { toast } from 'sonner';

interface AddConnectorModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConnectorAdded: () => void;
  remoteNetworkId: string;
}

export function AddConnectorModal({
  isOpen,
  onClose,
  onConnectorAdded,
  remoteNetworkId,
}: AddConnectorModalProps) {
  const [name, setName] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleClose = () => {
    setName('');
    onClose();
  };

  const handleSubmit = async () => {
    if (!name.trim()) {
      toast.error('Connector name cannot be empty.');
      return;
    }

    setIsSubmitting(true);
    try {
      await addConnector({ name, remoteNetworkId });
      toast.success('Connector added successfully.');
      onConnectorAdded();
      handleClose();
    } catch (error) {
      console.error('Failed to add connector:', error);
      toast.error('Failed to add connector.');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>Add New Connector</DialogTitle>
          <DialogDescription>
            Deploy a new connector to this remote network.
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
              placeholder="e.g., AWS-Connector-RegionX"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isSubmitting}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={!name.trim() || isSubmitting}>
            {isSubmitting ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Adding...
              </>
            ) : (
              'Add Connector'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
