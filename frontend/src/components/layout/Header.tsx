'use client';

import React from 'react';
import { ThemeMode, ActiveTab } from '@/types';
import { Sun, Moon, Laptop, Cookie, Zap, Activity } from 'lucide-react';
import { IconButton } from '@astryxdesign/core/IconButton';
import { Button } from '@astryxdesign/core/Button';
import { StatusDot } from '@astryxdesign/core/StatusDot';
import { SegmentedControl, SegmentedControlItem } from '@astryxdesign/core/SegmentedControl';

interface HeaderProps {
  activeTab: ActiveTab;
  totalTasks: number;
  isConnected: boolean;
  globalSpeed: string;
  theme: ThemeMode;
  onSetTheme: (mode: ThemeMode) => void;
  onOpenCookies: () => void;
}

const tabTitles: Record<ActiveTab, { title: string; subtitle: string }> = {
  'download-center': {
    title: 'Download Center',
    subtitle: 'Grouped transfers, channel batches & format control',
  },
  browser: {
    title: 'In-App Browser',
    subtitle: 'DNS-resilient proxy, stream preview & media sniffer',
  },
  history: {
    title: 'Media Gallery',
    subtitle: 'Organized folders, sorting, ZIP archives & playback',
  },
  settings: {
    title: 'Settings',
    subtitle: 'Storage organization, engine workers & appearance',
  },
};

export function Header({
  activeTab,
  totalTasks,
  isConnected,
  globalSpeed,
  theme,
  onSetTheme,
  onOpenCookies,
}: HeaderProps) {
  const info = tabTitles[activeTab];
  const nextQuickTheme: ThemeMode =
    theme === 'dark' ? 'light' : theme === 'light' ? 'system' : 'dark';

  return (
    <header className="sticky top-0 z-30 shrink-0 px-3.5 py-2.5 sm:px-6 border-b border-[var(--color-border)] bg-[var(--color-background-surface)]">
      <div className="flex items-center justify-between gap-3">
        {/* Left: View Title & Mobile Brand */}
        <div className="flex items-center gap-2.5 min-w-0">
          <div className="md:hidden w-8 h-8 rounded-lg bg-[var(--color-accent)] flex items-center justify-center shrink-0">
            <Zap className="w-4 h-4 text-[var(--color-on-accent)] fill-current" />
          </div>
          <div className="flex items-center gap-2 min-w-0">
            <h1 className="text-sm sm:text-base font-bold tracking-tight text-[var(--color-text-primary)] truncate">
              {info.title}
            </h1>
            {activeTab === 'download-center' && totalTasks > 0 && (
              <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold font-mono tabular-nums bg-[var(--color-accent-muted)] text-[var(--color-text-accent)]">
                {totalTasks}
              </span>
            )}
            <span className="hidden lg:inline text-xs text-[var(--color-text-secondary)] truncate">
              · {info.subtitle}
            </span>
          </div>
        </div>

        {/* Right: Speed Meter, Cookies, Theme Switcher */}
        <div className="flex items-center gap-2 shrink-0">
          <div
            className="flex items-center gap-1.5 px-2.5 py-1 rounded-full border border-[var(--color-border)] bg-[var(--color-background-muted)] text-xs font-medium"
            title={isConnected ? 'Live connection active' : 'Reconnecting'}
          >
            <StatusDot
              variant={isConnected ? 'success' : 'error'}
              label={isConnected ? 'Live connection active' : 'Disconnected'}
              isPulsing={isConnected}
            />
            <Activity className="w-3.5 h-3.5 text-[var(--color-icon-secondary)] hidden sm:block" />
            <span className="font-mono tabular-nums text-[var(--color-text-primary)] text-[11px] sm:text-xs">
              {globalSpeed || '0 KB/s'}
            </span>
          </div>

          <Button
            label="Cookies"
            size="sm"
            variant="secondary"
            icon={<Cookie className="w-3.5 h-3.5" />}
            onClick={onOpenCookies}
            tooltip="Manage authentication cookies"
          />

          <div className="hidden sm:block">
            <SegmentedControl
              label="Color theme"
              size="sm"
              value={theme}
              onChange={(val) => onSetTheme(val as ThemeMode)}
            >
              <SegmentedControlItem
                value="light"
                label="Light"
                isLabelHidden
                icon={<Sun className="w-3.5 h-3.5" />}
              />
              <SegmentedControlItem
                value="dark"
                label="Dark"
                isLabelHidden
                icon={<Moon className="w-3.5 h-3.5" />}
              />
              <SegmentedControlItem
                value="system"
                label="System"
                isLabelHidden
                icon={<Laptop className="w-3.5 h-3.5" />}
              />
            </SegmentedControl>
          </div>

          <div className="sm:hidden">
            <IconButton
              label={`Switch theme (current: ${theme})`}
              size="sm"
              variant="secondary"
              icon={
                theme === 'dark' ? (
                  <Moon className="w-4 h-4" />
                ) : theme === 'light' ? (
                  <Sun className="w-4 h-4" />
                ) : (
                  <Laptop className="w-4 h-4" />
                )
              }
              onClick={() => onSetTheme(nextQuickTheme)}
            />
          </div>
        </div>
      </div>
    </header>
  );
}
