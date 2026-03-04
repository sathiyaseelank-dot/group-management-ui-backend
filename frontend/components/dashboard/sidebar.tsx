import { Link, useLocation } from 'react-router-dom';
import { useEffect, useMemo, useState } from 'react';
import { cn } from '@/lib/utils';
import { Users, User, Shield, Database, Globe, Plug, Cable, FileText, ChevronDown } from 'lucide-react';

type NavItem = {
  label: string;
  href: string;
  icon?: any;
  description?: string;
  children?: Array<{ label: string; href: string }>;
};

const navItems: NavItem[] = [
  {
    label: 'Users',
    href: '/dashboard/users',
    icon: Users,
    description: 'View all users',
  },

  {
    label: 'Groups',
    href: '/dashboard/groups',
    icon: User,
    description: 'Manage identity groups',
  },
  
  {
    label: 'Resources',
    href: '/dashboard/resources',
    icon: Database,
    description: 'Manage network resources',
  },

  {
    label: 'Remote Networks',
    href: '/dashboard/remote-networks',
    icon: Globe,
    description: 'Manage remote network connectivity',
  },

  {
    label: 'Connectors',
    href: '/dashboard/connectors',
    icon: Plug,
    description: 'Manage network connectors',
  },
  {
    label: 'Policy',
    href: '/dashboard/policy',
    icon: FileText,
    description: 'Manage access and device policies',
    children: [
      { label: 'Resource Policies', href: '/dashboard/policy/resource-policies' },
      { label: 'Sign In Policy', href: '/dashboard/policy/sign-in' },
      { label: 'Device Profiles', href: '/dashboard/policy/device-profiles' },
    ],
  },

  {
    label: 'Tunnelers',
    href: '/dashboard/tunnelers',
    icon: Cable,
    description: 'Manage resource tunnelers',
  },
];

export function Sidebar() {
  const { pathname } = useLocation();
  const policyActive = useMemo(
    () => pathname === '/dashboard/policy' || pathname.startsWith('/dashboard/policy/'),
    [pathname],
  );
  const [policyExpanded, setPolicyExpanded] = useState(false);

  useEffect(() => {
    if (policyActive) setPolicyExpanded(true);
  }, [policyActive]);

  return (
    <aside className="flex w-64 flex-col border-r bg-muted/30">
      {/* Logo / Brand */}
      <div className="flex items-center gap-2 border-b px-6 py-6">
        <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary">
          <Shield className="h-5 w-5 text-primary-foreground" />
        </div>
        <div className="flex flex-col">
          <h1 className="text-sm font-bold">Identity Provider</h1>
          <p className="text-xs text-muted-foreground">Zero-Trust ACL</p>
        </div>
      </div>

      {/* Navigation Links */}
      <nav className="flex-1 space-y-2 px-3 py-6">
        {navItems.map((item) => {
          const Icon = item.icon;
          const isActive = pathname === item.href || pathname.startsWith(item.href + '/');
          const hasChildren = Array.isArray(item.children) && item.children.length > 0;
          const expanded = item.href === '/dashboard/policy' ? policyExpanded : false;

          return (
            <div key={item.href} className="space-y-1">
              <Link
                to={item.href}
                className={cn(
                  'flex items-center gap-3 rounded-lg px-4 py-3 text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                )}
                title={item.description}
                onClick={() => {
                  if (item.href === '/dashboard/policy') {
                    setPolicyExpanded(true);
                  }
                }}
              >
                {Icon && <Icon className="h-5 w-5" />}
                <span className="flex-1">{item.label}</span>
                {hasChildren && (
                  <button
                    type="button"
                    className={cn(
                      'rounded-md p-1 transition-colors',
                      isActive
                        ? 'hover:bg-primary-foreground/10'
                        : 'hover:bg-muted'
                    )}
                    aria-label={expanded ? 'Collapse policy menu' : 'Expand policy menu'}
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      setPolicyExpanded((v) => !v);
                    }}
                  >
                    <ChevronDown className={cn('h-4 w-4 transition-transform', expanded && 'rotate-180')} />
                  </button>
                )}
              </Link>

              {hasChildren && expanded && (
                <div className="ml-7 space-y-1">
                  {item.children!.map((child) => {
                    const childActive = pathname === child.href || pathname.startsWith(child.href + '/');
                    return (
                      <Link
                        key={child.href}
                        to={child.href}
                        className={cn(
                          'block rounded-md px-3 py-2 text-sm transition-colors',
                          childActive
                            ? 'bg-muted text-foreground'
                            : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                        )}
                      >
                        {child.label}
                      </Link>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}
      </nav>

      {/* Footer Info */}
      <div className="border-t px-6 py-4">
        <p className="text-xs text-muted-foreground">
          Security configuration panel for Zero-Trust Resource Access Control
        </p>
      </div>
    </aside>
  );
}
