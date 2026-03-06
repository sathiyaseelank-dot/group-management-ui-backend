import { useMemo } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

type PolicyView = 'resource' | 'signin' | 'devices';

function viewFromPath(pathname: string): PolicyView {
  if (pathname.includes('/dashboard/policy/sign-in')) return 'signin';
  if (pathname.includes('/dashboard/policy/device-profiles')) return 'devices';
  return 'resource';
}

function pathFromView(view: PolicyView): string {
  switch (view) {
    case 'signin':
      return '/dashboard/policy/sign-in';
    case 'devices':
      return '/dashboard/policy/device-profiles';
    case 'resource':
    default:
      return '/dashboard/policy/resource-policies';
  }
}

export function PolicyTypeDropdown() {
  const navigate = useNavigate();
  const { pathname } = useLocation();

  const value = useMemo(() => viewFromPath(pathname), [pathname]);

  return (
    <Select
      value={value}
      onValueChange={(v) => navigate(pathFromView(v as PolicyView))}
    >
      <SelectTrigger className="min-w-56">
        <SelectValue placeholder="Select policy type" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="resource">Resource Policies</SelectItem>
        <SelectItem value="signin">Sign In Policy</SelectItem>
        <SelectItem value="devices">Device Profiles</SelectItem>
      </SelectContent>
    </Select>
  );
}

