import { useEffect, useState } from 'react';
import { getDevices } from '@/lib/mock-api';
import { Device } from '@/lib/types';
import { Loader2, Monitor } from 'lucide-react';
import { Badge } from '@/components/ui/badge';

function SecurityBadge({ label }: { label: string }) {
  return (
    <Badge variant="default" className="text-xs">
      {label}
    </Badge>
  );
}

export default function DevicesPage() {
  const [devices, setDevices] = useState<Device[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    getDevices()
      .then(setDevices)
      .catch((err) => console.error('Failed to load devices:', err))
      .finally(() => setLoading(false));
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
      <div>
        <h1 className="text-2xl font-bold">Devices</h1>
        <p className="text-sm text-muted-foreground">
          Client devices that have enrolled via the ZTNA client
        </p>
      </div>

      {devices.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-12 text-center">
          <Monitor className="mb-4 h-10 w-10 text-muted-foreground" />
          <p className="font-medium">No devices have enrolled yet.</p>
          <p className="text-sm text-muted-foreground">
            Users must log in via the ZTNA client to appear here.
          </p>
        </div>
      ) : (
        <div className="rounded-lg border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50">
                <th className="px-4 py-3 text-left font-medium">Owner</th>
                <th className="px-4 py-3 text-left font-medium">Name</th>
                <th className="px-4 py-3 text-left font-medium">Model</th>
                <th className="px-4 py-3 text-left font-medium">Make</th>
                <th className="px-4 py-3 text-left font-medium">Serial</th>
                <th className="px-4 py-3 text-left font-medium">OS</th>
                <th className="px-4 py-3 text-left font-medium">Client Version</th>
                <th className="px-4 py-3 text-left font-medium">Hostname</th>
                <th className="px-4 py-3 text-left font-medium">Internet Security</th>
              </tr>
            </thead>
            <tbody>
              {devices.map((device) => (
                <tr key={device.deviceId} className="border-b last:border-0 hover:bg-muted/30">
                  <td className="px-4 py-3">
                    <div className="font-medium">{device.ownerName || device.ownerEmail || device.userId || '—'}</div>
                    {device.ownerEmail && device.ownerName && (
                      <div className="text-xs text-muted-foreground">{device.ownerEmail}</div>
                    )}
                  </td>
                  <td className="px-4 py-3">{device.deviceName || '—'}</td>
                  <td className="px-4 py-3">{device.deviceModel || '—'}</td>
                  <td className="px-4 py-3">{device.deviceMake || '—'}</td>
                  <td className="px-4 py-3 font-mono text-xs">{device.serialNumber || '—'}</td>
                  <td className="px-4 py-3">
                    <div>{device.osType || '—'}</div>
                    {device.osVersion && (
                      <div className="text-xs text-muted-foreground">{device.osVersion}</div>
                    )}
                  </td>
                  <td className="px-4 py-3">{device.clientVersion || '—'}</td>
                  <td className="px-4 py-3 font-mono text-xs">{device.hostname || '—'}</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {device.firewallEnabled && <SecurityBadge label="Firewall" />}
                      {device.diskEncrypted && <SecurityBadge label="Disk Enc" />}
                      {device.screenLockEnabled && <SecurityBadge label="Screen Lock" />}
                      {!device.firewallEnabled && !device.diskEncrypted && !device.screenLockEnabled && (
                        <span className="text-xs text-muted-foreground">—</span>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
