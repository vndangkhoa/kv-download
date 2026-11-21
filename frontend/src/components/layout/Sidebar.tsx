'use client';

import React from 'react';
import { ActiveTab } from '@/types';
import { Download, Compass, Film, Settings, Zap } from 'lucide-react';
import { Badge } from '@astryxdesign/core/Badge';
import { StatusDot } from '@astryxdesign/core/StatusDot';
import { cn } from '@/lib/utils';

interface SidebarProps {
  activeTab: ActiveTab;
  setActiveTab: (tab: ActiveTab) => void;
  totalTasks: number;
  isConnected: boolean;
  ytdlpVersion?: string;
}

const navItems: {
  id: ActiveTab;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
}[] = [
  { id: 'download-center', label: 'Download Center', icon: Download },
  { id: 'browser', label: 'In-App Browser', icon: Compass },
  { id: 'history', label: 'Media Gallery', icon: Film },
  { id: 'settings', label: 'Settings', icon: Settings },
];

export function Sidebar({
  activeTab,
  setActiveTab,
  totalTasks,
  isConnected,
  ytdlpVersion,
}: SidebarProps) {
  return (
    <aside className="hidden md:flex flex-col w-[240px] shrink-0 h-screen border-r border-[var(--color-border)] bg-[var(--color-background-surface)] select-none">
      {/* Brand Header */}
      <div className="flex items-center gap-3 px-4 py-4 border-b border-[var(--color-border)]">
        <div className="w-9 h-9 rounded-xl bg-[var(--color-accent)] flex items-center justify-center shrink-0">
          <Zap className="w-4.5 h-4.5 text-[var(--color-on-accent)] fill-current" />
        </div>
        <div className="flex flex-col min-w-0">
          <div className="flex items-center gap-1.5">
            <span className="font-bold text-base tracking-tight text-[var(--color-text-primary)] truncate">
              KV Download
            </span>
            <Badge label="Astryx" variant="purple" />
          </div>
          <span className="text-[11px] font-medium text-[var(--color-text-secondary)] truncate">
            Studio Media Engine
          </span>
        </div>
      </div>

      {/* Navigation Links */}
      <nav aria-label="Primary" className="flex-1 px-2.5 py-3 space-y-1 overflow-y-auto">
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
                'w-full flex items-center justify-between px-3 py-2.5 rounded-lg text-[13px] font-medium transition-colors text-left',
                isActive
                  ? 'bg-[var(--color-accent-muted)] text-[var(--color-text-accent)] font-semibold'
                  : 'text-[var(--color-text-secondary)] hover:text-[var(--color-text-primary)] hover:bg-[var(--color-overlay-hover)]'
              )}
            >
              <div className="flex items-center gap-2.5 min-w-0">
                <Icon
                  className={cn(
                    'w-4 h-4 shrink-0',
                    isActive
                      ? 'text-[var(--color-icon-accent)]'
                      : 'text-[var(--color-icon-secondary)]'
                  )}
                />
                <span className="truncate">{item.label}</span>
              </div>

              {item.id === 'download-center' && totalTasks > 0 && (
                <Badge
                  label={String(totalTasks)}
                  variant={isActive ? 'purple' : 'neutral'}
                />
              )}
            </button>
          );
        })}
      </nav>

      {/* Live Engine Diagnostics */}
      <div className="p-3 border-t border-[var(--color-border)]">
        <div className="rounded-lg px-3 py-2.5 bg-[var(--color-background-muted)] border border-[var(--color-border)] space-y-1.5">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs font-medium text-[var(--color-text-secondary)]">
              SSE Engine
            </span>
            <div className="flex items-center gap-1.5">
              <StatusDot
                variant={isConnected ? 'success' : 'error'}
                label={isConnected ? 'Engine connected' : 'Engine disconnected'}
                isPulsing={isConnected}
              />
              <span
                className={cn(
                  'text-xs font-semibold',
                  isConnected
                    ? 'text-[var(--color-text-success)]'
                    : 'text-[var(--color-text-error)]'
                )}
              >
                {isConnected ? 'Live' : 'Offline'}
              </span>
            </div>
          </div>

          <div className="flex items-center justify-between gap-2 text-[11px]">
            <span className="text-[var(--color-text-secondary)]">yt-dlp</span>
            <span className="font-mono tabular-nums text-[var(--color-text-primary)] truncate">
              {ytdlpVersion || 'Checking...'}
            </span>
          </div>
        </div>
      </div>
    </aside>
  );
}
