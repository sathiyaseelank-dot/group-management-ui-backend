import { Link, useLocation } from 'react-router-dom';
import { useEffect, useState } from 'react';
import { cn } from '@/lib/utils';
import {
  Users,
  Shield,
  Database,
  Globe,
  FileText,
  ChevronDown,
  ScrollText,
  Settings,
  Activity,
  Monitor,
  Lock,
  Cpu,
} from 'lucide-react';

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
    children: [
      { label: 'All Resources', href: '/dashboard/resources' },
      { label: 'Network Discovery', href: '/dashboard/discovery' },
      { label: 'Agent Discovery', href: '/dashboard/agent-discovery' },
    ],
  },
  {
    label: 'Remote Networks',
    href: '/dashboard/remote-networks',
    icon: Globe,
    description: 'Manage remote network connectivity',
    children: [
      { label: 'Networks', href: '/dashboard/remote-networks' },
      { label: 'Connectors', href: '/dashboard/connectors' },
      { label: 'Agents', href: '/dashboard/agents' },
    ],
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
    label: 'Devices',
    href: '/dashboard/devices',
    icon: Monitor,
    description: 'View enrolled client devices and posture',
  },
  {
    label: 'Audit Logs',
    href: '/dashboard/audit-logs',
    icon: ScrollText,
    description: 'View admin audit log entries',
  },
  {
    label: 'Network Diagnostics',
    href: '/dashboard/diagnostics',
    icon: Activity,
    description: 'Monitor connector health and trace access paths',
  },
  {
    label: 'Workspace Settings',
    href: '/dashboard/workspace/settings',
    icon: Settings,
    description: 'Manage workspace configuration and members',
  },
];

export function Sidebar() {
  const { pathname } = useLocation();
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

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
    <aside className="flex w-[260px] flex-col bg-sidebar border-r border-sidebar-border">
      {/* Brand */}
      <div className="flex items-center gap-3 px-5 py-5 border-b border-sidebar-border">
        <div className="relative flex h-9 w-9 items-center justify-center rounded-lg bg-sidebar-primary/15 ring-1 ring-sidebar-primary/30">
          <Shield className="h-5 w-5 text-sidebar-primary" />
          <div className="absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-secure pulse-secure" />
        </div>
        <div className="flex flex-col">
          <h1 className="font-display text-[15px] font-bold uppercase tracking-wide text-sidebar-foreground">ZTNA</h1>
          <p className="text-[10px] text-sidebar-foreground/40 font-mono uppercase tracking-widest">Zero Trust Access</p>
        </div>
      </div>

      {/* Section Label */}
      <div className="px-5 pt-5 pb-2">
        <p className="font-mono text-[9px] uppercase tracking-[0.2em] text-sidebar-foreground/25">Navigation</p>
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto px-3 pb-4 space-y-0.5">
        {navItems.map((item) => {
          const Icon = item.icon;
          const hasChildren = Array.isArray(item.children) && item.children.length > 0;
          const isExpanded = expanded[item.href] ?? false;

          const isActive = hasChildren
            ? item.children!.some(
                (c) => pathname === c.href || pathname.startsWith(c.href + '/')
              )
            : pathname === item.href || pathname.startsWith(item.href + '/');

          return (
            <div key={item.href}>
              {hasChildren ? (
                <button
                  type="button"
                  className={cn(
                    'flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-[13px] font-medium transition-all duration-200 cursor-pointer',
                    isActive
                      ? 'bg-sidebar-primary/10 text-sidebar-primary'
                      : 'text-sidebar-foreground/55 hover:bg-sidebar-accent/60 hover:text-sidebar-foreground'
                  )}
                  title={item.description}
                  onClick={() => toggleExpanded(item.href)}
                >
                  {Icon && (
                    <Icon className={cn(
                      'h-[18px] w-[18px] shrink-0 transition-colors',
                      isActive ? 'text-sidebar-primary' : 'text-sidebar-foreground/35'
                    )} />
                  )}
                  <span className="flex-1 text-left">{item.label}</span>
                  <ChevronDown
                    className={cn(
                      'h-3.5 w-3.5 transition-transform duration-200 text-sidebar-foreground/25',
                      isExpanded && 'rotate-180'
                    )}
                  />
                </button>
              ) : (
                <Link
                  to={item.href}
                  className={cn(
                    'flex items-center gap-3 rounded-lg px-3 py-2.5 text-[13px] font-medium transition-all duration-200',
                    isActive
                      ? 'bg-sidebar-primary/10 text-sidebar-primary'
                      : 'text-sidebar-foreground/55 hover:bg-sidebar-accent/60 hover:text-sidebar-foreground'
                  )}
                  title={item.description}
                >
                  {Icon && (
                    <Icon className={cn(
                      'h-[18px] w-[18px] shrink-0 transition-colors',
                      isActive ? 'text-sidebar-primary' : 'text-sidebar-foreground/35'
                    )} />
                  )}
                  <span className="flex-1">{item.label}</span>
                  {isActive && (
                    <div className="h-1.5 w-1.5 rounded-full bg-sidebar-primary" />
                  )}
                </Link>
              )}

              {hasChildren && isExpanded && (
                <div className="mt-1 ml-[22px] space-y-0.5 border-l border-sidebar-border/60 pl-3">
                  {item.children!.map((child) => {
                    const childActive =
                      pathname === child.href || pathname.startsWith(child.href + '/');
                    return (
                      <Link
                        key={child.href}
                        to={child.href}
                        className={cn(
                          'block rounded-md px-3 py-2 text-[13px] transition-all duration-200',
                          childActive
                            ? 'text-sidebar-primary font-medium bg-sidebar-primary/8'
                            : 'text-sidebar-foreground/45 hover:text-sidebar-foreground hover:bg-sidebar-accent/30'
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

      {/* Footer */}
      <div className="border-t border-sidebar-border px-4 py-3">
        <div className="flex items-center gap-2">
          <Lock className="h-3 w-3 text-sidebar-primary/50" />
          <p className="text-[10px] text-sidebar-foreground/30 font-mono tracking-wider">
            Encrypted Control Plane
          </p>
        </div>
      </div>
    </aside>
  );
}
