'use client';

import { useState, useCallback } from 'react';
import { AccessRule, SelectedSubject } from '@/lib/types';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { SubjectPicker } from '@/components/subjects/subject-picker';
import { createAccessRule } from '@/lib/mock-api';
import { toast } from 'sonner';

interface AddAccessRuleModalProps {
  resourceId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onRuleCreated: (rule: AccessRule) => void;
}

export function AddAccessRuleModal({
  resourceId,
  open,
  onOpenChange,
  onRuleCreated,
}: AddAccessRuleModalProps) {
  const [selectedSubjects, setSelectedSubjects] = useState<SelectedSubject[]>([]);
  const [effect, setEffect] = useState<'ALLOW' | 'DENY'>('ALLOW');
  const [saving, setSaving] = useState(false);

  const handleSave = useCallback(async () => {
    if (selectedSubjects.length === 0) {
      toast.error('Please select at least one subject');
      return;
    }

    if (!effect) {
      toast.error('Please select an effect');
      return;
    }

    setSaving(true);
    try {
      await createAccessRule(resourceId, selectedSubjects, effect);

      // Create rule objects for the callback
      selectedSubjects.forEach((subject) => {
        const newRule: AccessRule = {
          id: `rule_${Date.now()}_${subject.id}`,
          resourceId,
          subjectId: subject.id,
          subjectType: subject.type,
          subjectName: subject.label.split(': ')[1] || subject.label,
          effect,
          createdAt: new Date().toISOString().split('T')[0],
        };
        onRuleCreated(newRule);
      });

      setSelectedSubjects([]);
      setEffect('ALLOW');
      onOpenChange(false);
      toast.success(`Created access rule(s) for ${selectedSubjects.length} subject(s)`);
    } catch (error) {
      toast.error('Failed to create access rule');
    } finally {
      setSaving(false);
    }
  }, [resourceId, selectedSubjects, effect, onRuleCreated, onOpenChange]);

  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      setSelectedSubjects([]);
      setEffect('ALLOW');
    }
    onOpenChange(newOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Add Access Rule</DialogTitle>
          <DialogDescription>
            Define which subjects can access this resource and whether to allow or deny access.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-4">
          {/* Subject Picker */}
          <div className="space-y-2">
            <label className="text-sm font-semibold">
              Select Subjects
              <span className="text-xs font-normal text-muted-foreground">
                {' '}
                (Users, Groups, or Service Accounts)
              </span>
            </label>
            <SubjectPicker
              selectedSubjects={selectedSubjects}
              onSelectionChange={setSelectedSubjects}
              subjectTypes={['USER', 'GROUP', 'SERVICE']}
            />
          </div>

          {/* Effect Selector */}
          <div className="space-y-2">
            <label className="text-sm font-semibold">Effect</label>
            <div className="flex w-full rounded-md border p-1">
              <Button
                variant={effect === 'ALLOW' ? 'secondary' : 'ghost'}
                onClick={() => setEffect('ALLOW')}
                className="flex-1"
              >
                Allow Access
              </Button>
              <Button
                variant={effect === 'DENY' ? 'secondary' : 'ghost'}
                onClick={() => setEffect('DENY')}
                className="flex-1"
              >
                Deny Access
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              {effect === 'ALLOW'
                ? 'These subjects will be granted access to this resource'
                : 'These subjects will be denied access to this resource'}
            </p>
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => handleOpenChange(false)}
            disabled={saving}
          >
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={saving || selectedSubjects.length === 0}>
            {saving ? 'Creating...' : 'Create Access Rule'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
