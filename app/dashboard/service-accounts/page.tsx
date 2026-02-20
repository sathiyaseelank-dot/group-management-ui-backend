'use client';

import { useEffect, useState } from 'react';
import { getServiceAccounts } from '@/lib/mock-api';
import { ServiceAccount } from '@/lib/types';
import { ServiceAccountsList } from '@/components/dashboard/service-accounts/service-accounts-list';
import { Loader2 } from 'lucide-react';

export default function ServiceAccountsPage() {
  const [serviceAccounts, setServiceAccounts] = useState<ServiceAccount[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadServiceAccounts = async () => {
      try {
        const data = await getServiceAccounts();
        setServiceAccounts(data);
      } catch (error) {
        console.error('Failed to load service accounts:', error);
      } finally {
        setLoading(false);
      }
    };

    loadServiceAccounts();
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center p-12">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold">Service Accounts</h1>
        <p className="text-sm text-muted-foreground">
          Manage service account subjects for automated systems and CI/CD pipelines
        </p>
      </div>

      {/* Service Accounts List */}
      <ServiceAccountsList serviceAccounts={serviceAccounts} />
    </div>
  );
}
