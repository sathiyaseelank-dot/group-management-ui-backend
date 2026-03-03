'use client';

import { useParams } from 'next/navigation';
import { TunnelerInstall } from '@/components/dashboard/tunnelers/tunneler-install';

export default function TunnelerDetailPage() {
  const { tunnelerId } = useParams();
  const id = Array.isArray(tunnelerId) ? tunnelerId[0] : (tunnelerId as string | undefined);
  return <TunnelerInstall initialTunnelerId={id} />;
}

