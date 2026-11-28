'use client';

import React, { useEffect, useState } from 'react';
import { Theme } from '@astryxdesign/core/theme';
import { kvDownloadTheme } from '@/theme';
import { ActiveTab } from '@/types';
import { useSSE } from '@/hooks/useSSE';
import { useTheme } from '@/hooks/useTheme';
import { fetchYtDlpVersion } from '@/lib/api';
import { Header } from '@/components/layout/Header';
import { Sidebar } from '@/components/layout/Sidebar';
import { MobileNav } from '@/components/layout/MobileNav';
import { MiniPlayerBar } from '@/components/layout/MiniPlayerBar';
import { DownloadCenterView } from '@/components/download-center/DownloadCenterView';
import { GalleryView } from '@/components/gallery/GalleryView';
import { BrowserView } from '@/components/browser/BrowserView';
import { SettingsView } from '@/components/settings/SettingsView';
import { AlbumModal } from '@/components/modals/AlbumModal';
import { CookieModal } from '@/components/modals/CookieModal';

export default function App() {
  const [activeTab, setActiveTab] = useState<ActiveTab>('download-center');
  const { taskList, isConnected, globalSpeed, syncQueue } = useSSE();
  const { theme, effectiveTheme, setTheme } = useTheme();
  const [ytdlpVersion, setYtdlpVersion] = useState<string>('');

  // Modals state
  const [albumUrl, setAlbumUrl] = useState<string | null>(null);
  const [showCookies, setShowCookies] = useState(false);

  useEffect(() => {
    fetchYtDlpVersion()
      .then((info) => {
        const ver = info?.version || info?.current;
        if (ver) setYtdlpVersion(ver);
      })
      .catch(() => {});
  }, []);

  return (
    <Theme theme={kvDownloadTheme} mode={effectiveTheme}>
      <div className="flex h-screen w-screen overflow-hidden bg-[var(--color-background-body)] text-[var(--color-text-primary)]">
        {/* Desktop & Tablet Sidebar */}
        <Sidebar
          activeTab={activeTab}
          setActiveTab={setActiveTab}
          totalTasks={taskList.length}
          isConnected={isConnected}
          ytdlpVersion={ytdlpVersion}
        />

        {/* Main Content Area */}
        <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
          <Header
            activeTab={activeTab}
            totalTasks={taskList.length}
            isConnected={isConnected}
            globalSpeed={globalSpeed}
            theme={theme}
            onSetTheme={setTheme}
            onOpenCookies={() => setShowCookies(true)}
          />

          {/* Scrollable View Container */}
          <main
            className={
              activeTab === 'browser'
                ? 'flex-1 min-h-0 overflow-hidden p-1.5 sm:p-2 pb-16 md:pb-2 flex flex-col'
                : 'flex-1 overflow-y-auto px-3.5 py-4 sm:px-6 sm:py-6 pb-32 md:pb-24'
            }
          >
            {activeTab === 'download-center' && (
              <DownloadCenterView
                tasks={taskList}
                onRefresh={syncQueue}
                onOpenAlbumModal={(url) => setAlbumUrl(url)}
                onOpenCookies={() => setShowCookies(true)}
              />
            )}
            {activeTab === 'history' && <GalleryView />}
            {activeTab === 'browser' && (
              <BrowserView
                onOpenAlbumModal={(url) => setAlbumUrl(url)}
                onQueued={() => {
                  syncQueue();
                  setActiveTab('download-center');
                }}
              />
            )}
            {activeTab === 'settings' && (
              <SettingsView theme={theme} onSetTheme={setTheme} />
            )}
          </main>
        </div>

        {/* Floating Mini Player Bar */}
        <MiniPlayerBar />

        {/* Mobile Bottom Navigation Dock */}
        <MobileNav
          activeTab={activeTab}
          setActiveTab={setActiveTab}
          totalTasks={taskList.length}
        />

        {/* Modals */}
        {albumUrl && (
          <AlbumModal
            url={albumUrl}
            onClose={() => setAlbumUrl(null)}
            onSuccess={() => {
              syncQueue();
              setActiveTab('download-center');
            }}
          />
        )}

        <CookieModal isOpen={showCookies} onOpenChange={setShowCookies} />
      </div>
    </Theme>
  );
}
