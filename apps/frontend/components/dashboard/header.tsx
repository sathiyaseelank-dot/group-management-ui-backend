import { LogOut, Shield, Moon, Sun } from 'lucide-react';
import { useTheme } from 'next-themes';
import { Button } from '@/components/ui/button';
import { WorkspaceSwitcher } from './workspace-switcher';

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
  const { theme, setTheme } = useTheme();

  return (
    <header className="flex items-center justify-between border-b border-border/40 bg-background/80 px-6 py-3 backdrop-blur-md supports-[backdrop-filter]:bg-background/50">
      <div className="flex items-center gap-3">
        <div className="h-5 w-0.5 rounded-full bg-primary/40" />
        <div className="flex flex-col">
          <h2 className="font-display text-sm font-semibold uppercase tracking-wide">Security Control Center</h2>
          <p className="text-[10px] text-muted-foreground font-mono tracking-wider">
            Identity & Access Management
          </p>
        </div>
      </div>
      <div className="flex items-center gap-2">
        <WorkspaceSwitcher />
        <Button
          variant="ghost"
          size="icon-sm"
          className="text-muted-foreground hover:text-foreground"
          onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
        >
          {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </Button>
        <div className="h-5 w-px bg-border/40" />
        <Button
          variant="ghost"
          size="sm"
          className="gap-2 text-muted-foreground hover:text-foreground text-xs"
          onClick={logout}
        >
          <LogOut className="h-3.5 w-3.5" />
          Sign out
        </Button>
      </div>
    </header>
  );
}
