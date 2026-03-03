'use client';

import { useEffect, useMemo, useState } from 'react';
import { ChevronDown } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { addTrustedProfile, ALL_PLATFORMS, getDeviceProfiles, Platform, setApprovedOS } from '@/lib/device-profiles';

export default function DeviceProfilesPage() {
  const [refreshKey, setRefreshKey] = useState(0);

  const state = useMemo(() => getDeviceProfiles(), [refreshKey]);

  useEffect(() => {
    setRefreshKey((k) => k + 1);
  }, []);

  const handleAddProfile = (platform: Platform) => {
    addTrustedProfile(platform);
    setRefreshKey((k) => k + 1);
  };

  const handleToggle = (platform: Platform, enabled: boolean) => {
    setApprovedOS(platform, enabled);
    setRefreshKey((k) => k + 1);
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <div className="space-y-1">
            <CardTitle className="text-base">Trusted Device Profiles</CardTitle>
            <CardDescription>
              Create profiles for platforms you consider trusted.
            </CardDescription>
          </div>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" className="gap-2">
                Create
                <ChevronDown className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {ALL_PLATFORMS.map((p) => (
                <DropdownMenuItem key={p} onClick={() => handleAddProfile(p)}>
                  {p}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </CardHeader>
        <CardContent>
          {state.trustedProfiles.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No trusted device profiles created yet.
            </p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {state.trustedProfiles.map((p) => (
                <Badge key={p} variant="secondary">
                  {p}
                </Badge>
              ))}
            </div>
          )}
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
                checked={state.approvedOS[p]}
                onCheckedChange={(v) => handleToggle(p, Boolean(v))}
                aria-label={`Approve ${p}`}
              />
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}

