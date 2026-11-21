'use client';

import React, { useState, useEffect } from 'react';
import { OrganizeMode, ThemeMode, YtDlpVersionInfo } from '@/types';
import {
  fetchStorageSettings,
  updateStorageSettings,
  organizeGalleryLibrary,
  fetchYtDlpVersion,
  updateYtDlpChannel,
  fetchWorkers,
  setWorkers,
  SPEED_LIMIT_STORAGE_KEY,
} from '@/lib/api';
import { Section } from '@astryxdesign/core/Section';
import { Card } from '@astryxdesign/core/Card';
import { Button } from '@astryxdesign/core/Button';
import { TextInput } from '@astryxdesign/core/TextInput';
import { Selector } from '@astryxdesign/core/Selector';
import { SegmentedControl, SegmentedControlItem } from '@astryxdesign/core/SegmentedControl';
import { Banner } from '@astryxdesign/core/Banner';
import { Badge } from '@astryxdesign/core/Badge';
import {
  Check,
  Cpu,
  FolderCog,
  FolderSync,
  HardDrive,
  Laptop,
  Moon,
  RefreshCw,
  Save,
  Sun,
} from 'lucide-react';

interface SettingsViewProps {
  theme: ThemeMode;
  onSetTheme: (t: ThemeMode) => void;
}

const ORGANIZE_OPTIONS: { label: string; value: OrganizeMode; description: string }[] = [
  {
    label: 'Category / Channel',
    value: 'category_channel',
    description: 'Videos/ChannelName/Group/file.mp4 (Recommended)',
  },
  {
    label: 'Channel / Category',
    value: 'channel_category',
    description: 'ChannelName/Videos/file.mp4',
  },
  {
    label: 'Platform / Channel',
    value: 'platform_channel',
    description: 'YouTube/ChannelName/file.mp4',
  },
  {
    label: 'Flat by Category',
    value: 'flat_category',
    description: 'Videos/file.mp4, Music/file.mp3',
  },
];

const WORKER_OPTIONS = ['1', '2', '3', '4', '5', '6', '8', '10'].map((n) => ({
  label: `${n} Concurrent Worker${n === '1' ? '' : 's'}`,
  value: n,
}));

const SPEED_OPTIONS = [
  { label: 'Unlimited', value: '' },
  { label: '10 MB/s (10M)', value: '10M' },
  { label: '5 MB/s (5M)', value: '5M' },
  { label: '2 MB/s (2M)', value: '2M' },
  { label: '1 MB/s (1M)', value: '1M' },
  { label: '500 KB/s (500K)', value: '500K' },
];

const CHANNEL_OPTIONS = [
  { label: 'Stable (Default)', value: 'stable' },
  { label: 'Nightly (Latest Fixes)', value: 'nightly' },
  { label: 'Master (Bleeding Edge)', value: 'master' },
];

export function SettingsView({ theme, onSetTheme }: SettingsViewProps) {
  const [downloadDir, setDownloadDir] = useState('');
  const [organizeMode, setOrganizeMode] = useState<OrganizeMode>('category_channel');
  const [isSavingStorage, setIsSavingStorage] = useState(false);
  const [isOrganizing, setIsOrganizing] = useState(false);

  const [ytdlpInfo, setYtdlpInfo] = useState<YtDlpVersionInfo | null>(null);
  const [channel, setChannel] = useState('stable');
  const [isUpdatingEngine, setIsUpdatingEngine] = useState(false);

  const [maxWorkers, setMaxWorkers] = useState('3');
  const [speedLimit, setSpeedLimit] = useState('');

  const [feedback, setFeedback] = useState<{
    status: 'success' | 'error' | 'info';
    title: string;
    desc?: string;
  } | null>(null);

  useEffect(() => {
    fetchStorageSettings()
      .then((res) => {
        setDownloadDir(res.downloadDir || res.path || '');
        if (res.organizeMode) setOrganizeMode(res.organizeMode);
      })
      .catch(() => {});

    fetchYtDlpVersion()
      .then((res) => {
        setYtdlpInfo(res);
        if (res.channel) setChannel(res.channel);
      })
      .catch(() => {});

    fetchWorkers()
      .then((res) => {
        if (res?.maxWorkers) setMaxWorkers(String(res.maxWorkers));
      })
      .catch(() => {});

    try {
      setSpeedLimit(localStorage.getItem(SPEED_LIMIT_STORAGE_KEY) || '');
    } catch {
      // Ignore storage access failures.
    }
  }, []);

  const handleSaveStorage = async () => {
    setIsSavingStorage(true);
    setFeedback(null);
    try {
      const updated = await updateStorageSettings(downloadDir.trim(), organizeMode);
      setDownloadDir(updated.downloadDir);
      if (updated.organizeMode) setOrganizeMode(updated.organizeMode);
      setFeedback({
        status: 'success',
        title: 'Storage & Organization Settings Saved',
        desc: `Destination: ${updated.downloadDir} · Layout: ${updated.organizeMode}`,
      });
    } catch (err: any) {
      setFeedback({
        status: 'error',
        title: 'Failed to save storage settings',
        desc: err?.message,
      });
    } finally {
      setIsSavingStorage(false);
    }
  };

  const handleOrganizeNow = async () => {
    setIsOrganizing(true);
    setFeedback(null);
    try {
      await updateStorageSettings(downloadDir.trim(), organizeMode);
      const res = await organizeGalleryLibrary();
      setFeedback({
        status: 'success',
        title: 'Library Re-organized Successfully',
        desc: `Relocated ${res.movedFiles ?? 0} files into ${res.organizeMode || organizeMode} layout (${res.totalItems ?? 0} total items).`,
      });
    } catch (err: any) {
      setFeedback({
        status: 'error',
        title: 'Failed to organize existing library',
        desc: err?.message,
      });
    } finally {
      setIsOrganizing(false);
    }
  };

  const handleUpdateYtdlp = async () => {
    setIsUpdatingEngine(true);
    setFeedback(null);
    try {
      const res = await updateYtDlpChannel(channel);
      const nextVer = await fetchYtDlpVersion().catch(() => null);
      if (nextVer) setYtdlpInfo(nextVer);
      setFeedback({
        status: 'success',
        title: 'Extraction Engine Updated',
        desc: res.message,
      });
    } catch (err: any) {
      setFeedback({
        status: 'error',
        title: 'Engine update failed',
        desc: err?.message,
      });
    } finally {
      setIsUpdatingEngine(false);
    }
  };

  const handleWorkersChange = async (val: string) => {
    setMaxWorkers(val);
    const count = parseInt(val, 10) || 3;
    try {
      await setWorkers(count);
      setFeedback({
        status: 'info',
        title: `Concurrent workers updated to ${count}`,
      });
    } catch {
      // Ignore transient error.
    }
  };

  const handleSpeedChange = (val: string) => {
    setSpeedLimit(val);
    try {
      if (val) localStorage.setItem(SPEED_LIMIT_STORAGE_KEY, val);
      else localStorage.removeItem(SPEED_LIMIT_STORAGE_KEY);
      setFeedback({
        status: 'info',
        title: val ? `Rate limit set to ${val}/s` : 'Rate limit set to Unlimited',
      });
    } catch {
      // Ignore storage failures.
    }
  };

  return (
    <div className="max-w-4xl mx-auto space-y-5">
      {feedback && (
        <Banner
          status={feedback.status}
          title={feedback.title}
          description={feedback.desc}
          isDismissable
          onDismiss={() => setFeedback(null)}
        />
      )}

      {/* Storage & Folder Organization */}
      <Section variant="section" padding={4}>
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2.5">
              <div className="w-9 h-9 rounded-lg bg-[var(--color-accent-muted)] text-[var(--color-icon-accent)] flex items-center justify-center">
                <FolderCog className="w-4.5 h-4.5" />
              </div>
              <div>
                <h2 className="text-base font-bold text-[var(--color-text-primary)]">
                  Storage & Folder Organization
                </h2>
                <p className="text-xs text-[var(--color-text-secondary)]">
                  Configure download directory and automatic channel/group folder layout
                </p>
              </div>
            </div>
          </div>

          <Card padding={4}>
            <div className="space-y-4">
              <TextInput
                label="Server Download Directory"
                startIcon={HardDrive}
                width="100%"
                value={downloadDir}
                onChange={(val) => setDownloadDir(val)}
                placeholder="/path/to/downloads"
                description="Completed media files are stored under this path on the server."
              />

              <Selector
                label="Folder Organization Mode"
                width="100%"
                value={organizeMode}
                onChange={(val) => setOrganizeMode(val as OrganizeMode)}
                options={ORGANIZE_OPTIONS}
                description="Controls how channels, playlists, platforms, and media categories map into subfolders."
              />

              <div className="flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-[var(--color-border)]">
                <Button
                  label="Auto-Organize Existing Library"
                  size="sm"
                  variant="secondary"
                  isLoading={isOrganizing}
                  icon={<FolderSync className="w-3.5 h-3.5" />}
                  onClick={handleOrganizeNow}
                />
                <Button
                  label="Save Storage Settings"
                  size="sm"
                  variant="primary"
                  isLoading={isSavingStorage}
                  icon={<Save className="w-3.5 h-3.5" />}
                  onClick={handleSaveStorage}
                />
              </div>
            </div>
          </Card>
        </div>
      </Section>

      {/* Extraction & Queue Engine */}
      <Section variant="section" padding={4}>
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2.5">
              <div className="w-9 h-9 rounded-lg bg-[var(--color-accent-muted)] text-[var(--color-icon-accent)] flex items-center justify-center">
                <Cpu className="w-4.5 h-4.5" />
              </div>
              <div>
                <h2 className="text-base font-bold text-[var(--color-text-primary)]">
                  Extraction & Transfer Engine
                </h2>
                <p className="text-xs text-[var(--color-text-secondary)]">
                  Manage yt-dlp release channel, concurrent workers, and bandwidth cap
                </p>
              </div>
            </div>

            {ytdlpInfo?.version && (
              <Badge
                label={`yt-dlp ${ytdlpInfo.version}`}
                variant="success"
                icon={<Check className="w-3 h-3" />}
              />
            )}
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Card padding={4}>
              <div className="space-y-3">
                <h3 className="text-sm font-semibold text-[var(--color-text-primary)]">
                  yt-dlp Release Channel
                </h3>
                <Selector
                  label="Release channel"
                  isLabelHidden
                  width="100%"
                  value={channel}
                  onChange={(val) => setChannel(val)}
                  options={CHANNEL_OPTIONS}
                />
                <Button
                  label="Update yt-dlp"
                  size="sm"
                  variant="secondary"
                  isLoading={isUpdatingEngine}
                  icon={<RefreshCw className="w-3.5 h-3.5" />}
                  onClick={handleUpdateYtdlp}
                />
              </div>
            </Card>

            <Card padding={4}>
              <div className="space-y-3">
                <Selector
                  label="Concurrent Download Workers"
                  width="100%"
                  value={maxWorkers}
                  onChange={handleWorkersChange}
                  options={WORKER_OPTIONS}
                />
                <Selector
                  label="Per-Task Bandwidth Limit"
                  width="100%"
                  value={speedLimit}
                  onChange={handleSpeedChange}
                  options={SPEED_OPTIONS}
                />
              </div>
            </Card>
          </div>
        </div>
      </Section>

      {/* Appearance */}
      <Section variant="section" padding={4}>
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div>
            <h2 className="text-base font-bold text-[var(--color-text-primary)]">Appearance</h2>
            <p className="text-xs text-[var(--color-text-secondary)]">
              Dual-polarity Astryx surface tokens for Light, Dark, and System modes
            </p>
          </div>

          <SegmentedControl
            label="Theme preference"
            size="md"
            value={theme}
            onChange={(val) => onSetTheme(val as ThemeMode)}
          >
            <SegmentedControlItem
              value="light"
              label="Light"
              icon={<Sun className="w-4 h-4" />}
            />
            <SegmentedControlItem
              value="dark"
              label="Dark"
              icon={<Moon className="w-4 h-4" />}
            />
            <SegmentedControlItem
              value="system"
              label="System"
              icon={<Laptop className="w-4 h-4" />}
            />
          </SegmentedControl>
        </div>
      </Section>
    </div>
  );
}
