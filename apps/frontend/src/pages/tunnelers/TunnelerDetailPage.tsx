import { useParams } from 'react-router-dom';
import { TunnelerInstall } from '@/components/dashboard/tunnelers/tunneler-install';

export default function TunnelerDetailPage() {
  const { tunnelerId } = useParams();
  return <TunnelerInstall initialTunnelerId={tunnelerId} />;
}
