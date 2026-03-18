import { useEffect, useState } from 'react';
import { Trash2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { ALL_PLATFORMS, Platform, setApprovedOS, getDeviceProfiles } from '@/lib/device-profiles';
import {
  getTrustedProfiles,
  createTrustedProfile,
  deleteTrustedProfile,
  getDevicePosture,
} from '@/lib/mock-api';
import { TrustedProfile, DevicePostureSnapshot } from '@/lib/types';

export default function DeviceProfilesPage() {
  const [profiles, setProfiles] = useState<TrustedProfile[]>([]);
  const [posture, setPosture] = useState<DevicePostureSnapshot[]>([]);
  const [approvedOS, setApprovedOSState] = useState(getDeviceProfiles().approvedOS);
  const [loading, setLoading] = useState(true);

  // New profile form state
  const [newName, setNewName] = useState('');
  const [newFirewall, setNewFirewall] = useState(false);
  const [newDiskEncryption, setNewDiskEncryption] = useState(false);
  const [newScreenLock, setNewScreenLock] = useState(false);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    Promise.all([getTrustedProfiles(), getDevicePosture()])
      .then(([p, pos]) => {
        setProfiles(p);
        setPosture(pos);
      })
      .finally(() => setLoading(false));
  }, []);

  const handleCreateProfile = async () => {
    if (!newName.trim()) return;
    setCreating(true);
    try {
      const created = await createTrustedProfile({
        name: newName.trim(),
        requireFirewall: newFirewall,
        requireDiskEncryption: newDiskEncryption,
        requireScreenLock: newScreenLock,
      });
      setProfiles((prev) => [...prev, created]);
      setNewName('');
      setNewFirewall(false);
      setNewDiskEncryption(false);
      setNewScreenLock(false);
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteProfile = async (id: string) => {
    await deleteTrustedProfile(id);
    setProfiles((prev) => prev.filter((p) => p.id !== id));
  };

  const handleToggleOS = (platform: Platform, enabled: boolean) => {
    setApprovedOS(platform, enabled);
    setApprovedOSState((prev) => ({ ...prev, [platform]: enabled }));
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Trusted Device Profiles</CardTitle>
          <CardDescription>
            Create profiles with posture requirements. Groups linked to a profile will only allow
            users whose device meets the requirements.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {loading ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : profiles.length === 0 ? (
            <p className="text-sm text-muted-foreground">No trusted device profiles created yet.</p>
          ) : (
            <div className="space-y-2">
              {profiles.map((p) => (
                <div key={p.id} className="flex items-center justify-between rounded-md border px-3 py-2">
                  <div className="space-y-1">
                    <p className="text-sm font-medium">{p.name}</p>
                    <div className="flex flex-wrap gap-1">
                      {p.requireFirewall && <Badge variant="secondary">Firewall</Badge>}
                      {p.requireDiskEncryption && <Badge variant="secondary">Disk Encryption</Badge>}
                      {p.requireScreenLock && <Badge variant="secondary">Screen Lock</Badge>}
                      {!p.requireFirewall && !p.requireDiskEncryption && !p.requireScreenLock && (
                        <span className="text-xs text-muted-foreground">No requirements</span>
                      )}
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleDeleteProfile(p.id)}
                    aria-label={`Delete ${p.name}`}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>
          )}

          <div className="rounded-md border p-3 space-y-3">
            <p className="text-sm font-medium">Create New Profile</p>
            <Input
              placeholder="Profile name"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
            />
            <div className="flex flex-wrap gap-4">
              <div className="flex items-center gap-2">
                <Checkbox
                  id="new-firewall"
                  checked={newFirewall}
                  onCheckedChange={(v) => setNewFirewall(Boolean(v))}
                />
                <Label htmlFor="new-firewall" className="text-sm">Require Firewall</Label>
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id="new-disk-encryption"
                  checked={newDiskEncryption}
                  onCheckedChange={(v) => setNewDiskEncryption(Boolean(v))}
                />
                <Label htmlFor="new-disk-encryption" className="text-sm">Require Disk Encryption</Label>
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id="new-screen-lock"
                  checked={newScreenLock}
                  onCheckedChange={(v) => setNewScreenLock(Boolean(v))}
                />
                <Label htmlFor="new-screen-lock" className="text-sm">Require Screen Lock</Label>
              </div>
            </div>
            <Button
              size="sm"
              onClick={handleCreateProfile}
              disabled={creating || !newName.trim()}
            >
              {creating ? 'Creating…' : 'Create Profile'}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Approved Operating Systems</CardTitle>
          <CardDescription>
            Toggle which operating systems are approved for access.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {ALL_PLATFORMS.map((p) => (
            <div key={p} className="flex items-center justify-between">
              <Label className="text-sm">{p}</Label>
              <Switch
                checked={approvedOS[p]}
                onCheckedChange={(v) => handleToggleOS(p, Boolean(v))}
                aria-label={`Approve ${p}`}
              />
            </div>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Live Device Posture</CardTitle>
          <CardDescription>
            Real-time posture data reported by enrolled agents.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {posture.length === 0 ? (
            <p className="text-sm text-muted-foreground">No device posture data available yet.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="pb-2 pr-4">Hostname</th>
                    <th className="pb-2 pr-4">OS</th>
                    <th className="pb-2 pr-4">Version</th>
                    <th className="pb-2 pr-4">Firewall</th>
                    <th className="pb-2 pr-4">Encrypted</th>
                    <th className="pb-2 pr-4">Screen Lock</th>
                    <th className="pb-2">Last Seen</th>
                  </tr>
                </thead>
                <tbody>
                  {posture.map((d) => (
                    <tr key={d.deviceId} className="border-b last:border-0">
                      <td className="py-2 pr-4">{d.hostname || d.deviceId}</td>
                      <td className="py-2 pr-4">{d.osType}</td>
                      <td className="py-2 pr-4 text-muted-foreground">{d.osVersion}</td>
                      <td className="py-2 pr-4">{d.firewallEnabled ? '✓' : '✗'}</td>
                      <td className="py-2 pr-4">{d.diskEncrypted ? '✓' : '✗'}</td>
                      <td className="py-2 pr-4">{d.screenLockEnabled ? '✓' : '✗'}</td>
                      <td className="py-2 text-muted-foreground">{d.reportedAt}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
