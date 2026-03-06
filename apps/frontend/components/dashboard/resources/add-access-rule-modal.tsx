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
import { SubjectPicker } from '@/components/subjects/subject-picker';
import { createAccessRule } from '@/lib/mock-api';
import { toast } from 'sonner';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';

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
  const [selectedGroups, setSelectedGroups] = useState<SelectedSubject[]>([]);
  const [ruleName, setRuleName] = useState('');
  const [enabled, setEnabled] = useState(true);
  const [saving, setSaving] = useState(false);

  const handleSave = useCallback(async () => {
    if (!ruleName.trim()) {
      toast.error('Please provide a rule name');
      return;
    }
    if (selectedGroups.length === 0) {
      toast.error('Please select at least one group');
      return;
    }

    setSaving(true);
    try {
      const rule = await createAccessRule(resourceId, {
        name: ruleName.trim(),
        groupIds: selectedGroups.map((g) => g.id),
        enabled,
      });

      onRuleCreated(rule as AccessRule);

      setSelectedGroups([]);
      setRuleName('');
      setEnabled(true);
      onOpenChange(false);
      toast.success('Created access rule');
    } catch (error) {
      toast.error('Failed to create access rule');
    } finally {
      setSaving(false);
    }
  }, [resourceId, selectedGroups, ruleName, enabled, onRuleCreated, onOpenChange]);

  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      setSelectedGroups([]);
      setRuleName('');
      setEnabled(true);
    }
    onOpenChange(newOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Add Access Rule</DialogTitle>
          <DialogDescription>
            Define which groups can access this resource.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-4">
          <div className="space-y-2">
            <label className="text-sm font-semibold">Rule Name</label>
            <Input
              value={ruleName}
              onChange={(e) => setRuleName(e.target.value)}
              placeholder="e.g., Engineering access"
              maxLength={120}
            />
          </div>
          {/* Subject Picker */}
          <div className="space-y-2">
            <label className="text-sm font-semibold">
              Select Groups
              <span className="text-xs font-normal text-muted-foreground">
                {' '}
                (Groups only)
              </span>
            </label>
            <SubjectPicker
              selectedSubjects={selectedGroups}
              onSelectionChange={setSelectedGroups}
              subjectTypes={['GROUP']}
            />
          </div>

          <div className="flex items-center justify-between rounded-md border px-3 py-2">
            <div className="space-y-1">
              <p className="text-sm font-semibold">Enabled</p>
              <p className="text-xs text-muted-foreground">
                Disabled rules are ignored during policy compilation.
              </p>
            </div>
            <Switch checked={enabled} onCheckedChange={setEnabled} />
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
          <Button onClick={handleSave} disabled={saving || selectedGroups.length === 0 || !ruleName.trim()}>
            {saving ? 'Creating...' : 'Create Access Rule'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
