'use client';

import { useEffect, useState } from 'react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import { getResource } from '@/lib/mock-api';
import { Resource, AccessRule } from '@/lib/types';
import { Button } from '@/components/ui/button';
import { ResourceInfoSection } from '@/components/dashboard/resources/resource-info-section';
import { AccessRulesTable } from '@/components/dashboard/resources/access-rules-table';
import { AddAccessRuleModal } from '@/components/dashboard/resources/add-access-rule-modal';
import { Loader2, ArrowLeft } from 'lucide-react';

export default function ResourceAccessEditorPage() {
  const { resourceId } = useParams();
  const [resource, setResource] = useState<Resource | null>(null);
  const [accessRules, setAccessRules] = useState<AccessRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAddRuleModal, setShowAddRuleModal] = useState(false);

  useEffect(() => {
    const loadResourceData = async () => {
      try {
        const { resource, accessRules } = await getResource(resourceId as string);
        setResource(resource);
        setAccessRules(accessRules);
      } catch (error) {
        console.error('Failed to load resource:', error);
      } finally {
        setLoading(false);
      }
    };

    if (resourceId) {
      loadResourceData();
    }
  }, [resourceId]);

  if (loading) {
    return (
      <div className="flex items-center justify-center p-12">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!resource) {
    return (
      <div className="space-y-4 p-6">
        <Link href="/dashboard/groups">
          <Button variant="ghost" className="gap-2">
            <ArrowLeft className="h-4 w-4" />
            Back
          </Button>
        </Link>
        <p className="text-muted-foreground">Resource not found</p>
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      {/* Breadcrumb & Header */}
      <div className="flex items-center justify-between">
        <div className="space-y-2">
          <div className="text-sm text-muted-foreground">
            Resource Access Policy Editor
          </div>
          <h1 className="text-2xl font-bold">{resource.name}</h1>
        </div>
        <Link href="/dashboard/groups">
          <Button variant="outline" className="gap-2">
            <ArrowLeft className="h-4 w-4" />
            Back
          </Button>
        </Link>
      </div>

      {/* Resource Information */}
      <ResourceInfoSection resource={resource} />

      {/* Access Rules */}
      <AccessRulesTable
        resourceId={resource.id}
        accessRules={accessRules}
        onRulesChange={setAccessRules}
        onAddRule={() => setShowAddRuleModal(true)}
      />

      {/* Add Access Rule Modal */}
      <AddAccessRuleModal
        resourceId={resource.id}
        open={showAddRuleModal}
        onOpenChange={setShowAddRuleModal}
        onRuleCreated={(newRule) => {
          setAccessRules([...accessRules, newRule]);
          setShowAddRuleModal(false);
        }}
      />
    </div>
  );
}
