'use client';

import React from 'react';
import { ActiveTab } from '@/types';
import { Download, Compass, Film, Settings } from 'lucide-react';
import { cn } from '@/lib/utils';

interface MobileNavProps {
  activeTab: ActiveTab;
  setActiveTab: (tab: ActiveTab) => void;
  totalTasks: number;
}

const navItems: {
  id: ActiveTab;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
}[] = [
  { id: 'download-center', label: 'Queue', icon: Download },
  { id: 'browser', label: 'Browser', icon: Compass },
  { id: 'history', label: 'Gallery', icon: Film },
  { id: 'settings', label: 'Settings', icon: Settings },
];

export function MobileNav({ activeTab, setActiveTab, totalTasks }: MobileNavProps) {
  return (
    <nav
      aria-label="Mobile Navigation"
      className="md:hidden fixed bottom-0 left-0 right-0 z-40 px-2 pb-[max(0.5rem,env(safe-area-inset-bottom))] pt-1.5 border-t border-[var(--color-border)] bg-[var(--color-background-surface)]"
    >
      <div className="flex items-center justify-around max-w-md mx-auto">
        {navItems.map((item) => {
          const Icon = item.icon;
          const isActive = activeTab === item.id;
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => setActiveTab(item.id)}
              aria-current={isActive ? 'page' : undefined}
              className={cn(
                'relative flex flex-col items-center justify-center gap-0.5 py-1.5 px-3 rounded-lg transition-colors min-w-[68px]',
                isActive
                  ? 'bg-[var(--color-accent-muted)] text-[var(--color-text-accent)] font-semibold'
                  : 'text-[var(--color-text-secondary)] hover:text-[var(--color-text-primary)]'
              )}
            >
              <Icon className="w-5 h-5" />
              <span className="text-[10px] tracking-tight leading-none">{item.label}</span>
              {item.id === 'download-center' && totalTasks > 0 && (
                <span className="absolute top-0.5 right-2 min-w-[16px] h-4 px-1 flex items-center justify-center rounded-full text-[9px] font-bold font-mono tabular-nums bg-[var(--color-accent)] text-[var(--color-on-accent)]">
                  {totalTasks > 99 ? '99+' : totalTasks}
                </span>
              )}
            </button>
          );
        })}
      </div>
    </nav>
  );
}
