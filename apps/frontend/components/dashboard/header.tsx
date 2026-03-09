import { LogOut } from 'lucide-react';
import { Button } from '@/components/ui/button';

async function logout() {
  try {
    await fetch('/api/auth/logout', { method: 'POST' });
  } catch {
    // Best-effort
  }
  localStorage.removeItem('authToken');
  window.location.href = '/login';
}

export function Header() {
  return (
    <header className="flex items-center justify-between border-b bg-background/95 px-6 py-4 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="flex flex-col">
        <h2 className="text-lg font-semibold">Identity & Access Control</h2>
        <p className="text-xs text-muted-foreground">
          Manage groups, users, and resource access policies
        </p>
      </div>
      <Button variant="ghost" size="sm" className="gap-2 text-muted-foreground" onClick={logout}>
        <LogOut className="h-4 w-4" />
        Sign out
      </Button>
    </header>
  );
}
