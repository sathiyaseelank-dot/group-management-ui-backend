'use client';

import { useEffect, useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Loader2, Plus } from 'lucide-react';
import { getResources, addGroupResources } from '@/lib/mock-api'; // Assuming addGroupResources will be implemented
import { Resource } from '@/lib/types';
import { Input } from '@/components/ui/input';

interface AddResourcesModalProps {
  groupId: string;
  isOpen: boolean;
  onClose: () => void;
  onResourcesAdded: () => void;
}

export function AddResourcesModal({
  groupId,
  isOpen,
  onClose,
  onResourcesAdded,
}: AddResourcesModalProps) {
  const [availableResources, setAvailableResources] = useState<Resource[]>([]);
  const [selectedResourceIds, setSelectedResourceIds] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);
  const [isAdding, setIsAdding] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');

  useEffect(() => {
    if (isOpen) {
      const loadResources = async () => {
        setLoading(true);
        try {
          const data = await getResources();
          setAvailableResources(data);
        } catch (error) {
          console.error('Failed to load resources:', error);
        } finally {
          setLoading(false);
        }
      };
      loadResources();
    }
  }, [isOpen]);

  const handleCheckboxChange = (resourceId: string, isChecked: boolean) => {
    setSelectedResourceIds((prev) => {
      const newSet = new Set(prev);
      if (isChecked) {
        newSet.add(resourceId);
      } else {
        newSet.delete(resourceId);
      }
      return newSet;
    });
  };

  const handleAddResources = async () => {
    setIsAdding(true);
    try {
      await addGroupResources(groupId, Array.from(selectedResourceIds));
      onResourcesAdded();
      onClose();
      setSelectedResourceIds(new Set()); // Clear selection
    } catch (error) {
      console.error('Failed to add resources to group:', error);
      // TODO: Show an error toast
    } finally {
      setIsAdding(false);
    }
  };

  const filteredResources = availableResources.filter((resource) =>
    resource.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    resource.address.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>Add Resources to Group</DialogTitle>
          <DialogDescription>
            Select resources to grant access to this group.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          <Input
            placeholder="Search resources..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="col-span-3"
          />
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : (
            <ScrollArea className="h-72 w-full rounded-md border">
              <div className="p-4">
                {filteredResources.length === 0 ? (
                  <p className="text-sm text-muted-foreground text-center">No resources found.</p>
                ) : (
                  filteredResources.map((resource) => (
                    <div key={resource.id} className="flex items-center space-x-2 py-2">
                      <Checkbox
                        id={resource.id}
                        checked={selectedResourceIds.has(resource.id)}
                        onCheckedChange={(checked) =>
                          handleCheckboxChange(resource.id, checked as boolean)
                        }
                      />
                      <Label htmlFor={resource.id} className="flex flex-col text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
                        {resource.name}
                        <span className="text-xs text-muted-foreground">{resource.address}</span>
                      </Label>
                    </div>
                  ))
                )}
              </div>
            </ScrollArea>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={isAdding}>
            Cancel
          </Button>
          <Button
            onClick={handleAddResources}
            disabled={selectedResourceIds.size === 0 || isAdding}
          >
            {isAdding ? (
              <>
                <Plus className="mr-2 h-4 w-4 animate-spin" /> Adding...
              </>
            ) : (
              `Add ${selectedResourceIds.size} Resource(s)`
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
