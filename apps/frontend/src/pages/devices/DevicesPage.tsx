import { useEffect, useState } from 'react';
import { getDevices } from '@/lib/mock-api';
import { Device } from '@/lib/types';
import { Loader2, Monitor, Shield, ShieldCheck, ShieldAlert } from 'lucide-react';

function SecurityBadge({ label, secure }: { label: string; secure?: boolean }) {
  return (
    <span className={`inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset ${
      secure !== false
        ? 'bg-secure/10 text-secure ring-secure/20'
        : 'bg-muted text-muted-foreground ring-border'
    }`}>
      {secure !== false ? (
        <ShieldCheck className="h-3 w-3" />
      ) : (
        <ShieldAlert className="h-3 w-3" />
      )}
      {label}
    </span>
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
      <div className="flex items-center justify-center p-16">
        <div className="flex flex-col items-center gap-3">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
          <p className="text-xs text-muted-foreground font-mono tracking-wider">Scanning devices...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-primary/10 ring-1 ring-primary/20">
            <Monitor className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h1 className="font-display text-xl font-bold uppercase tracking-wide">Devices</h1>
            <p className="text-xs text-muted-foreground mt-0.5">
              Enrolled client devices and security posture
            </p>
          </div>
        </div>
        <div className="flex items-center gap-1.5 rounded-lg bg-muted/60 px-3 py-1.5 ring-1 ring-border/30">
          <Shield className="h-3 w-3 text-primary/70" />
          <span className="text-[11px] font-mono text-muted-foreground">{devices.length} enrolled</span>
        </div>
      </div>

      {devices.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border/40 bg-muted/10 p-16 text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-xl bg-muted/60 ring-1 ring-border/30">
            <Monitor className="h-7 w-7 text-muted-foreground/60" />
          </div>
          <p className="mt-5 font-display text-base font-semibold uppercase tracking-wide">No devices enrolled</p>
          <p className="mt-1.5 text-xs text-muted-foreground max-w-sm">
            Users must authenticate via the ZTNA client to register their device.
          </p>
        </div>
      ) : (
        <div className="rounded-xl border border-border/40 overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border/40 bg-muted/20">
                <th className="px-4 py-3 text-left font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/70">Owner</th>
                <th className="px-4 py-3 text-left font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/70">Device</th>
                <th className="px-4 py-3 text-left font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/70">Serial</th>
                <th className="px-4 py-3 text-left font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/70">OS</th>
                <th className="px-4 py-3 text-left font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/70">Client</th>
                <th className="px-4 py-3 text-left font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/70">Hostname</th>
                <th className="px-4 py-3 text-left font-mono text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/70">Security Posture</th>
              </tr>
            </thead>
            <tbody>
              {devices.map((device) => (
                <tr key={device.deviceId} className="border-b border-border/20 last:border-0 transition-colors hover:bg-primary/[0.02]">
                  <td className="px-4 py-3">
                    <div className="font-medium text-sm">{device.ownerName || device.ownerEmail || device.userId || '—'}</div>
                    {device.ownerEmail && device.ownerName && (
                      <div className="text-[11px] text-muted-foreground">{device.ownerEmail}</div>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <div className="text-sm">{device.deviceName || device.deviceModel || '—'}</div>
                    {device.deviceMake && (
                      <div className="text-[11px] text-muted-foreground">{device.deviceMake}</div>
                    )}
                  </td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{device.serialNumber || '—'}</td>
                  <td className="px-4 py-3">
                    <div className="text-sm">{device.osType || '—'}</div>
                    {device.osVersion && (
                      <div className="text-[11px] text-muted-foreground">{device.osVersion}</div>
                    )}
                  </td>
                  <td className="px-4 py-3 font-mono text-xs">{device.clientVersion || '—'}</td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{device.hostname || '—'}</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1.5">
                      {device.firewallEnabled && <SecurityBadge label="Firewall" />}
                      {device.diskEncrypted && <SecurityBadge label="Encrypted" />}
                      {device.screenLockEnabled && <SecurityBadge label="Lock" />}
                      {!device.firewallEnabled && !device.diskEncrypted && !device.screenLockEnabled && (
                        <span className="inline-flex items-center gap-1 text-[11px] text-muted-foreground/50">
                          <ShieldAlert className="h-3 w-3" />
                          No posture data
                        </span>
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
