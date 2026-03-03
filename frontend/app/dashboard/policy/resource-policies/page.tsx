'use client';

import Link from 'next/link';
import { useEffect, useMemo, useState } from 'react';
import { Plus, Trash2, Lock } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card } from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { CreateResourcePolicyModal } from '@/components/dashboard/policy/resource-policies/create-resource-policy-modal';
import {
  deleteResourcePolicy,
  ensureDefaultResourcePolicy,
  listResourcePolicies,
  ResourcePolicy,
} from '@/lib/resource-policies';

export default function ResourcePoliciesPage() {
  const [policies, setPolicies] = useState<ResourcePolicy[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);

  const load = () => {
    ensureDefaultResourcePolicy();
    setPolicies(listResourcePolicies());
  };

  useEffect(() => {
    load();
  }, []);

  const sorted = useMemo(() => {
    const copy = [...policies];
    copy.sort((a, b) => {
      if (a.isDefault && !b.isDefault) return -1;
      if (!a.isDefault && b.isDefault) return 1;
      return a.name.localeCompare(b.name);
    });
    return copy;
  }, [policies]);

  const handleDelete = async (policy: ResourcePolicy) => {
    if (policy.isDefault) return;
    const ok = window.confirm(`Delete "${policy.name}"? This cannot be undone.`);
    if (!ok) return;
    deleteResourcePolicy(policy.id);
    load();
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Resource Policies</h2>
          <p className="text-sm text-muted-foreground">
            Manage how users access protected resources.
          </p>
        </div>
        <Button className="gap-2" onClick={() => setIsCreateOpen(true)}>
          <Plus className="h-4 w-4" />
          Create
        </Button>
      </div>

      <Card className="overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="font-semibold">Policy Name</TableHead>
              <TableHead className="font-semibold">Type</TableHead>
              <TableHead className="text-right font-semibold">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sorted.map((p) => (
              <TableRow key={p.id}>
                <TableCell className="font-medium">
                  <Link
                    href={`/dashboard/policy/resource-policies/${encodeURIComponent(p.id)}`}
                    className="hover:underline"
                  >
                    {p.name}
                  </Link>
                </TableCell>
                <TableCell>
                  {p.isDefault ? (
                    <Badge variant="secondary">Default</Badge>
                  ) : (
                    <Badge variant="outline">Custom</Badge>
                  )}
                </TableCell>
                <TableCell className="text-right">
                  {p.isDefault ? (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="inline-flex">
                          <Button variant="ghost" size="icon" disabled aria-label="Default policy locked">
                            <Lock className="h-4 w-4" />
                          </Button>
                        </span>
                      </TooltipTrigger>
                      <TooltipContent sideOffset={6}>
                        Default policy cannot be deleted.
                      </TooltipContent>
                    </Tooltip>
                  ) : (
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleDelete(p)}
                      aria-label={`Delete ${p.name}`}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>

      <CreateResourcePolicyModal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        onCreated={() => {
          setIsCreateOpen(false);
          load();
        }}
      />
    </div>
  );
}

