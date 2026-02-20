'use client';

import { useState, useCallback } from 'react';
import { AccessRule } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { deleteAccessRule } from '@/lib/mock-api';
import { Lock, Trash2 } from 'lucide-react';
import { toast } from 'sonner';

interface AccessRulesTableProps {
  resourceId: string;
  accessRules: AccessRule[];
  onRulesChange: (rules: AccessRule[]) => void;
  onAddRule: () => void;
}

export function AccessRulesTable({
  resourceId,
  accessRules,
  onRulesChange,
  onAddRule,
}: AccessRulesTableProps) {
  const [deleting, setDeleting] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  const handleDeleteRule = useCallback(
    async (ruleId: string) => {
      setDeleting(ruleId);
      try {
        await deleteAccessRule(ruleId);
        onRulesChange(accessRules.filter((r) => r.id !== ruleId));
        setConfirmDelete(null);
        toast.success('Access rule deleted');
      } catch (error) {
        toast.error('Failed to delete access rule');
      } finally {
        setDeleting(null);
      }
    },
    [accessRules, onRulesChange]
  );

  return (
    <>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <CardTitle className="flex items-center gap-2">
                <Lock className="h-5 w-5" />
                Access Rules
              </CardTitle>
              <CardDescription>
                Define which subjects (users, groups, services) can access this resource
              </CardDescription>
            </div>
            <Button onClick={onAddRule}>
              Add Access Rule
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {accessRules.length === 0 ? (
            <div className="rounded-lg border border-dashed py-8 text-center">
              <p className="text-sm text-muted-foreground">
                No access rules configured yet
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                Add your first access rule to grant subjects access to this resource
              </p>
            </div>
          ) : (
            <div className="overflow-hidden rounded-lg border">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="font-semibold">Subject</TableHead>
                    <TableHead className="font-semibold">Type</TableHead>
                    <TableHead className="font-semibold">Effect</TableHead>
                    <TableHead className="text-right font-semibold">Created</TableHead>
                    <TableHead className="text-right font-semibold">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {accessRules.map((rule) => (
                    <TableRow key={rule.id}>
                      <TableCell className="font-medium">
                        {rule.subjectName}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline">{rule.subjectType}</Badge>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            rule.effect === 'ALLOW' ? 'default' : 'destructive'
                          }
                          className={
                            rule.effect === 'ALLOW'
                              ? 'bg-green-100 text-green-800'
                              : ''
                          }
                        >
                          {rule.effect}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">
                        {rule.createdAt}
                      </TableCell>
                      <TableCell className="text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setConfirmDelete(rule.id)}
                          disabled={deleting === rule.id}
                          className="text-destructive hover:text-destructive hover:bg-destructive/10"
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={!!confirmDelete} onOpenChange={(open) => !open && setConfirmDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Access Rule</AlertDialogTitle>
            <AlertDialogDescription>
              This will remove access to this resource for the selected subject. This action cannot
              be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={() => confirmDelete && handleDeleteRule(confirmDelete)}
            disabled={deleting === confirmDelete}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {deleting === confirmDelete ? 'Deleting...' : 'Delete'}
          </AlertDialogAction>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
