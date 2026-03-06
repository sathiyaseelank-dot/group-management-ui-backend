import { Link, useLocation } from 'react-router-dom';
import { useEffect, useState } from 'react';
import { cn } from '@/lib/utils';
import { Users, Shield, Database, Globe, FileText, ChevronDown, Activity } from 'lucide-react';

type NavItem = {
  label: string;
  href: string;
  icon?: any;
  description?: string;
  children?: Array<{ label: string; href: string }>;
};

const navItems: NavItem[] = [
  {
    label: 'Team',
    href: '/dashboard/team',
    icon: Users,
    description: 'Manage users and groups',
    children: [
      { label: 'Users', href: '/dashboard/users' },
      { label: 'Groups', href: '/dashboard/groups' },
    ],
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
    children: [
      { label: 'Networks', href: '/dashboard/remote-networks' },
      { label: 'Connectors', href: '/dashboard/connectors' },
      { label: 'Tunnelers', href: '/dashboard/tunnelers' },
    ],
  },

  {
    label: 'Network Diagnostics',
    href: '/dashboard/diagnostics',
    icon: Activity,
    description: 'Monitor connectivity and trace access paths',
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

];

export function Sidebar() {
  const { pathname } = useLocation();
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  // Auto-expand parent when a child route is active
  useEffect(() => {
    const updates: Record<string, boolean> = {};
    navItems.forEach((item) => {
      if (item.children) {
        const childActive = item.children.some(
          (c) => pathname === c.href || pathname.startsWith(c.href + '/')
        );
        if (childActive) {
          updates[item.href] = true;
        }
      }
    });
    if (Object.keys(updates).length > 0) {
      setExpanded((prev) => ({ ...prev, ...updates }));
    }
  }, [pathname]);

  const toggleExpanded = (href: string) => {
    setExpanded((prev) => ({ ...prev, [href]: !prev[href] }));
  };

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
          const hasChildren = Array.isArray(item.children) && item.children.length > 0;
          const isExpanded = expanded[item.href] ?? false;

          // A parent is active if its own href matches OR any child is active
          const isActive = hasChildren
            ? item.children!.some(
                (c) => pathname === c.href || pathname.startsWith(c.href + '/')
              )
            : pathname === item.href || pathname.startsWith(item.href + '/');

          return (
            <div key={item.href} className="space-y-1">
              {hasChildren ? (
                <button
                  type="button"
                  className={cn(
                    'flex w-full items-center gap-3 rounded-lg px-4 py-3 text-sm font-medium transition-colors',
                    isActive
                      ? 'bg-primary text-primary-foreground'
                      : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                  )}
                  title={item.description}
                  onClick={() => toggleExpanded(item.href)}
                >
                  {Icon && <Icon className="h-5 w-5" />}
                  <span className="flex-1 text-left">{item.label}</span>
                  <ChevronDown
                    className={cn(
                      'h-4 w-4 transition-transform',
                      isExpanded && 'rotate-180'
                    )}
                  />
                </button>
              ) : (
                <Link
                  to={item.href}
                  className={cn(
                    'flex items-center gap-3 rounded-lg px-4 py-3 text-sm font-medium transition-colors',
                    isActive
                      ? 'bg-primary text-primary-foreground'
                      : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                  )}
                  title={item.description}
                >
                  {Icon && <Icon className="h-5 w-5" />}
                  <span className="flex-1">{item.label}</span>
                </Link>
              )}

              {hasChildren && isExpanded && (
                <div className="ml-7 space-y-1">
                  {item.children!.map((child) => {
                    const childActive =
                      pathname === child.href || pathname.startsWith(child.href + '/');
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
