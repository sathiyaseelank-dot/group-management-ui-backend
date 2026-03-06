'use client';

import { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { addRemoteNetwork } from '@/lib/mock-api';
import { Loader2 } from 'lucide-react';

interface AddRemoteNetworkModalProps {
  isOpen: boolean;
  onClose: () => void;
  onNetworkAdded: () => void;
}

const LOCATION_OPTIONS = [
  { value: 'AWS', label: 'AWS' },
  { value: 'GCP', label: 'GCP' },
  { value: 'AZURE', label: 'Azure' },
  { value: 'ON_PREM', label: 'On-Prem' },
  { value: 'OTHER', label: 'Other' },
];

export function AddRemoteNetworkModal({
  isOpen,
  onClose,
  onNetworkAdded,
}: AddRemoteNetworkModalProps) {
  const [name, setName] = useState('');
  const [location, setLocation] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const resetForm = () => {
    setName('');
    setLocation('');
  };

  const handleClose = () => {
    resetForm();
    onClose();
  };

  const canSubmit = name.trim() && location;

  const handleSubmit = async () => {
    if (!canSubmit) return;
    setIsSubmitting(true);
    try {
      await addRemoteNetwork({ name: name.trim(), location });
      onNetworkAdded();
      handleClose();
    } catch (error) {
      console.error('Failed to add remote network:', error);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[460px]">
        <DialogHeader>
          <DialogTitle>Add Remote Network</DialogTitle>
          <DialogDescription>
            Create a new remote network to attach connectors and resources.
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
              placeholder="e.g., Production AWS"
            />
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="location" className="text-right">
              Location
            </Label>
            <Select value={location} onValueChange={setLocation}>
              <SelectTrigger className="col-span-3">
                <SelectValue placeholder="Select a location" />
              </SelectTrigger>
              <SelectContent>
                {LOCATION_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isSubmitting}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={!canSubmit || isSubmitting}>
            {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Add Network
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
