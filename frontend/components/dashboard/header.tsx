'use client';

export function Header() {
  return (
    <header className="flex items-center justify-between border-b bg-background/95 px-6 py-4 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="flex flex-col">
        <h2 className="text-lg font-semibold">Identity & Access Control</h2>
        <p className="text-xs text-muted-foreground">
          Manage groups, users, and resource access policies
        </p>
      </div>
    </header>
  );
}
