'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { cn } from '@/lib/utils';
import { Users, User, Server, Shield, Database } from 'lucide-react';

const navItems = [
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
    label: 'Service Accounts',
    href: '/dashboard/service-accounts',
    icon: Server,
    description: 'Manage service accounts',
  },

  {
    label: 'Resources',
    href: '/dashboard/resources',
    icon: Database,
    description: 'Manage network resources',
  },
];

export function Sidebar() {
  const pathname = usePathname();

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

          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                'flex items-center gap-3 rounded-lg px-4 py-3 text-sm font-medium transition-colors',
                isActive
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              )}
              title={item.description}
            >
              <Icon className="h-5 w-5" />
              <span>{item.label}</span>
            </Link>
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
