'use client';

import { useEffect, useState } from 'react';
import { getTunnelers } from '@/lib/mock-api';
import { Tunneler } from '@/lib/types';
import { TunnelersList } from '@/components/dashboard/tunnelers/tunnelers-list';
import { Loader2, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';

export default function TunnelersPage() {
  const [tunnelers, setTunnelers] = useState<Tunneler[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadTunnelers = async () => {
      try {
        const data = await getTunnelers();
        setTunnelers(data);
      } catch (error) {
        console.error('Failed to load tunnelers:', error);
      } finally {
        setLoading(false);
      }
    };

    loadTunnelers();
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
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Tunnelers</h1>
          <p className="text-sm text-muted-foreground">
            Manage resource tunnelers for secure access to network resources
          </p>
        </div>
        <Button className="gap-2">
          <Plus className="h-4 w-4" />
          Add Tunneler
        </Button>
      </div>

      {/* Tunnelers List */}
      <TunnelersList tunnelers={tunnelers} />
    </div>
  );
}
