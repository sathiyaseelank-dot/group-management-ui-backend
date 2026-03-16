import { Link, useParams } from 'react-router-dom';
import { useEffect, useMemo, useState } from 'react';
import { ArrowLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { ensureDefaultResourcePolicy, getResourcePolicy, saveResourcePolicy, ResourcePolicy } from '@/lib/resource-policies';
import { getTrustedProfiles } from '@/lib/mock-api';
import { TrustedProfile } from '@/lib/types';

type DeviceSecurityMode = 'any' | 'trusted' | 'custom';

export default function ResourcePolicyDetailPage() {
  const { policyId } = useParams();

  const [policy, setPolicy] = useState<ResourcePolicy | null>(null);
  const [trustedProfiles, setTrustedProfiles] = useState<TrustedProfile[]>([]);

  useEffect(() => {
    ensureDefaultResourcePolicy();
    if (!policyId) return;
    setPolicy(getResourcePolicy(policyId));
    getTrustedProfiles().then(setTrustedProfiles).catch(() => {});
  }, [policyId]);

  const deviceMode: DeviceSecurityMode = useMemo(() => {
    const v = policy?.deviceSecurity?.mode;
    if (v === 'trusted' || v === 'custom') return v;
    return 'any';
  }, [policy?.deviceSecurity?.mode]);

  const setDeviceMode = (mode: DeviceSecurityMode) => {
    if (!policy) return;
    const next: ResourcePolicy = {
      ...policy,
      deviceSecurity: { mode },
    };
    setPolicy(next);
    saveResourcePolicy(next);
  };

  if (!policyId) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Policy Not Found</CardTitle>
          <CardDescription>Missing policy id.</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  if (!policy) {
    return (
      <div className="space-y-4">
        <Link to="/dashboard/policy/resource-policies">
          <Button variant="ghost" className="gap-2">
            <ArrowLeft className="h-4 w-4" />
            Back to Resource Policies
          </Button>
        </Link>
        <Card>
          <CardHeader>
            <CardTitle>Policy Not Found</CardTitle>
            <CardDescription>It looks like this policy does not exist.</CardDescription>
          </CardHeader>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <Link to="/dashboard/policy/resource-policies">
        <Button variant="ghost" className="gap-2">
          <ArrowLeft className="h-4 w-4" />
          Back to Resource Policies
        </Button>
      </Link>

      <div>
        <h2 className="text-xl font-semibold">{policy.name}</h2>
        <p className="text-sm text-muted-foreground">Configure this resource policy.</p>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <div className="space-y-6 lg:col-span-2">
          <Card>
            <CardHeader>
              <CardTitle>Geo Location Options</CardTitle>
              <CardDescription>
                Configure geo-based access rules for this policy.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Add geo location controls here.
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Authentication Requirements</CardTitle>
              <CardDescription>
                Configure authentication requirements for this policy.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Add authentication controls here.
              </p>
            </CardContent>
          </Card>
        </div>

        <Card className="h-fit">
          <CardHeader>
            <CardTitle>Device Security</CardTitle>
            <CardDescription>
              Choose what device posture is required for access.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <RadioGroup value={deviceMode} onValueChange={(v) => setDeviceMode(v as DeviceSecurityMode)}>
              <div className="flex items-start gap-3">
                <RadioGroupItem value="any" id="device-any" />
                <div className="grid gap-1">
                  <Label htmlFor="device-any">Any Device</Label>
                  <p className="text-xs text-muted-foreground">
                    Allow access from any device.
                  </p>
                </div>
              </div>
              <div className="flex items-start gap-3">
                <RadioGroupItem value="trusted" id="device-trusted" />
                <div className="grid gap-1">
                  <Label htmlFor="device-trusted">Only Trusted Devices</Label>
                  <p className="text-xs text-muted-foreground">
                    Require a trusted device posture.
                  </p>
                </div>
              </div>
              {deviceMode === 'trusted' && (
                <div className="ml-6 rounded-md border bg-muted/30 p-3 space-y-2">
                  <p className="text-xs font-medium">Select Trusted Profile</p>
                  {trustedProfiles.length === 0 ? (
                    <p className="text-xs text-muted-foreground">
                      No profiles yet. Create one in Device Profiles.
                    </p>
                  ) : (
                    <select
                      className="w-full rounded border bg-background px-2 py-1 text-sm"
                      value={(policy?.deviceSecurity as any)?.trustedProfileId ?? ''}
                      onChange={(e) => {
                        if (!policy) return;
                        const next: ResourcePolicy = {
                          ...policy,
                          deviceSecurity: { mode: 'trusted', trustedProfileId: e.target.value } as any,
                        };
                        setPolicy(next);
                        saveResourcePolicy(next);
                      }}
                    >
                      <option value="">— None —</option>
                      {trustedProfiles.map((tp) => (
                        <option key={tp.id} value={tp.id}>{tp.name}</option>
                      ))}
                    </select>
                  )}
                </div>
              )}
              <div className="flex items-start gap-3">
                <RadioGroupItem value="custom" id="device-custom" />
                <div className="grid gap-1">
                  <Label htmlFor="device-custom">Custom</Label>
                  <p className="text-xs text-muted-foreground">
                    Define custom device requirements.
                  </p>
                </div>
              </div>
            </RadioGroup>

            {deviceMode === 'custom' && (
              <div className="rounded-md border bg-muted/30 p-3">
                <p className="text-sm text-muted-foreground">
                  Add custom device security controls here.
                </p>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
