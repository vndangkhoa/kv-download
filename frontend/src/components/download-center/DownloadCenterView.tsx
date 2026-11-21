'use client';

import React, { useMemo, useState } from 'react';
import { DownloadTask, TaskGroup, TaskStatus, ViewMode } from '@/types';
import { TaskCard } from './TaskCard';
import { TaskGroupCard } from './TaskGroupCard';
import { enqueueUrls, clearCompletedTasks, retryFailedTasks } from '@/lib/api';
import { detectPlatform } from '@/lib/utils';
import { Card } from '@astryxdesign/core/Card';
import { Button } from '@astryxdesign/core/Button';
import { TextArea } from '@astryxdesign/core/TextArea';
import { TextInput } from '@astryxdesign/core/TextInput';
import { Selector } from '@astryxdesign/core/Selector';
import { SegmentedControl, SegmentedControlItem } from '@astryxdesign/core/SegmentedControl';
import { TabList, Tab } from '@astryxdesign/core/TabList';
import { Badge } from '@astryxdesign/core/Badge';
import { EmptyState } from '@astryxdesign/core/EmptyState';
import {
  Clipboard,
  Download,
  FolderKanban,
  LayoutGrid,
  List,
  ListChecks,
  RotateCcw,
  Rows3,
  Search,
  Sparkles,
  Trash2,
} from 'lucide-react';
import { cn } from '@/lib/utils';

interface DownloadCenterViewProps {
  tasks: DownloadTask[];
  onRefresh: () => void;
  onOpenAlbumModal: (url: string) => void;
  onOpenCookies?: () => void;
}

type FilterStatus = 'all' | 'active' | TaskStatus;
type OrganizationView = 'grouped' | 'flat';
type SortOption = 'newest' | 'oldest' | 'status' | 'group' | 'title';

const FORMAT_OPTIONS = [
  { label: 'Max / 8K / 4K (Highest)', value: 'highest' },
  { label: '1080p Full HD MP4', value: '1080p' },
  { label: '720p HD MP4', value: '720p' },
  { label: 'Audio Only (MP3)', value: 'audio' },
];

const SORT_OPTIONS = [
  { label: 'Newest First', value: 'newest' },
  { label: 'Oldest First', value: 'oldest' },
  { label: 'By Status', value: 'status' },
  { label: 'By Group / Channel', value: 'group' },
  { label: 'Title A-Z', value: 'title' },
];

const STATUS_WEIGHT: Record<TaskStatus, number> = {
  downloading: 0,
  queued: 1,
  failed: 2,
  cancelled: 3,
  completed: 4,
};

function buildGroupsAndSingles(tasks: DownloadTask[]): {
  groups: TaskGroup[];
  singles: DownloadTask[];
} {
  const buckets = new Map<string, DownloadTask[]>();
  const singles: DownloadTask[] = [];

  for (const task of tasks) {
    const key =
      (task.groupId && task.groupId.trim()) ||
      (task.playlistTitle && `pl:${task.playlistTitle.trim()}`) ||
      (task.channel && `ch:${task.platform || detectPlatform(task.url).name}:${task.channel.trim()}`) ||
      '';
    if (!key) {
      singles.push(task);
      continue;
    }
    const existing = buckets.get(key);
    if (existing) existing.push(task);
    else buckets.set(key, [task]);
  }

  const groups: TaskGroup[] = [];

  for (const [key, list] of buckets.entries()) {
    // Only collapse into a group card if there are >= 2 tasks or explicit groupId/playlistTitle
    const hasExplicitGroup = Boolean(list[0]?.groupId || list[0]?.playlistTitle);
    if (list.length === 1 && !hasExplicitGroup) {
      singles.push(list[0]);
      continue;
    }

    let completedCount = 0;
    let downloadingCount = 0;
    let queuedCount = 0;
    let failedCount = 0;
    let cancelledCount = 0;
    let totalBytes = 0;
    let pctSum = 0;
    let latest = list[0]?.createdAt || '';

    for (const t of list) {
      if (t.status === 'completed') {
        completedCount++;
        pctSum += 100;
      } else if (t.status === 'downloading') {
        downloadingCount++;
        pctSum += Math.max(0, Math.min(100, t.percent || 0));
      } else if (t.status === 'queued') {
        queuedCount++;
      } else if (t.status === 'failed') {
        failedCount++;
      } else if (t.status === 'cancelled') {
        cancelledCount++;
      }
      totalBytes += t.totalBytes || 0;
      if (t.createdAt > latest) latest = t.createdAt;
    }

    const first = list[0];
    const title =
      first.groupTitle ||
      first.playlistTitle ||
      first.channel ||
      `Batch (${list.length} items)`;
    const groupType =
      first.groupType ||
      (first.playlistTitle ? 'playlist' : first.channel ? 'channel' : 'batch');
    const platform = first.platform || detectPlatform(first.url).name;

    groups.push({
      groupId: first.groupId || key,
      title,
      groupType,
      platform,
      tasks: list,
      completedCount,
      downloadingCount,
      queuedCount,
      failedCount,
      cancelledCount,
      totalBytes,
      overallPercent: list.length > 0 ? Math.round(pctSum / list.length) : 0,
      latestCreatedAt: latest,
    });
  }

  return { groups, singles };
}

export function DownloadCenterView({
  tasks,
  onRefresh,
  onOpenAlbumModal,
  onOpenCookies,
}: DownloadCenterViewProps) {
  const [urlsText, setUrlsText] = useState('');
  const [format, setFormat] = useState('highest');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const [filterStatus, setFilterStatus] = useState<FilterStatus>('all');
  const [orgView, setOrgView] = useState<OrganizationView>('grouped');
  const [viewMode, setViewMode] = useState<ViewMode>('standard');
  const [sortBy, setSortBy] = useState<SortOption>('newest');
  const [searchQuery, setSearchQuery] = useState('');

  const parsedUrls = useMemo(
    () =>
      urlsText
        .split('\n')
        .map((u) => u.trim())
        .filter((u) => u.startsWith('http://') || u.startsWith('https://')),
    [urlsText]
  );

  const counts = useMemo(() => {
    let active = 0;
    let queued = 0;
    let completed = 0;
    let failed = 0;
    for (const t of tasks) {
      if (t.status === 'downloading') active++;
      else if (t.status === 'queued') queued++;
      else if (t.status === 'completed') completed++;
      else if (t.status === 'failed' || t.status === 'cancelled') failed++;
    }
    return { all: tasks.length, active: active + queued, queued, completed, failed };
  }, [tasks]);

  const handlePaste = async () => {
    try {
      const text = await navigator.clipboard.readText();
      if (text) {
        setUrlsText((prev) => (prev.trim() ? `${prev.trim()}\n${text.trim()}` : text.trim()));
      }
    } catch {
      // Clipboard access denied.
    }
  };

  const handleStartDownload = async () => {
    if (parsedUrls.length === 0 || isSubmitting) return;
    setIsSubmitting(true);
    try {
      const res = await enqueueUrls(parsedUrls, format, {
        groupType: parsedUrls.length > 1 ? 'batch' : undefined,
        groupTitle:
          parsedUrls.length > 1
            ? `${detectPlatform(parsedUrls[0]).name} Batch (${parsedUrls.length} URLs)`
            : undefined,
      });
      if (res.success) {
        setUrlsText('');
        onRefresh();
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  const filteredAndSortedTasks = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    const list = tasks.filter((t) => {
      if (filterStatus === 'active' && t.status !== 'downloading' && t.status !== 'queued') {
        return false;
      }
      if (filterStatus === 'queued' && t.status !== 'queued') return false;
      if (filterStatus === 'completed' && t.status !== 'completed') return false;
      if (
        filterStatus === 'failed' &&
        t.status !== 'failed' &&
        t.status !== 'cancelled'
      ) {
        return false;
      }
      if (q) {
        const hay = [
          t.title,
          t.mediaName,
          t.channel,
          t.uploader,
          t.groupTitle,
          t.playlistTitle,
          t.platform,
          t.url,
        ]
          .filter(Boolean)
          .join(' ')
          .toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    });

    const copy = [...list];
    copy.sort((a, b) => {
      switch (sortBy) {
        case 'oldest':
          return a.createdAt.localeCompare(b.createdAt);
        case 'status':
          return (STATUS_WEIGHT[a.status] ?? 9) - (STATUS_WEIGHT[b.status] ?? 9);
        case 'group':
          return (a.groupTitle || a.channel || 'zzz').localeCompare(
            b.groupTitle || b.channel || 'zzz'
          );
        case 'title':
          return (a.title || a.url).localeCompare(b.title || b.url);
        case 'newest':
        default:
          return b.createdAt.localeCompare(a.createdAt);
      }
    });
    return copy;
  }, [tasks, filterStatus, searchQuery, sortBy]);

  const { groups, singles } = useMemo(() => {
    const res = buildGroupsAndSingles(filteredAndSortedTasks);
    res.groups.sort((a, b) => {
      switch (sortBy) {
        case 'oldest':
          return a.latestCreatedAt.localeCompare(b.latestCreatedAt);
        case 'status':
          return b.downloadingCount - a.downloadingCount || b.failedCount - a.failedCount;
        case 'group':
        case 'title':
          return a.title.localeCompare(b.title);
        case 'newest':
        default:
          return b.latestCreatedAt.localeCompare(a.latestCreatedAt);
      }
    });
    return res;
  }, [filteredAndSortedTasks, sortBy]);

  return (
    <div className="max-w-7xl mx-auto space-y-5">
      {/* URL Input Composer */}
      <Card padding={4} elevation="low">
        <div className="space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <h2 className="text-sm font-bold text-[var(--color-text-primary)]">
                Add Downloads
              </h2>
              {parsedUrls.length > 0 && (
                <Badge
                  label={
                    parsedUrls.length > 1
                      ? `${parsedUrls.length} URLs (auto-grouped)`
                      : '1 URL ready'
                  }
                  variant="purple"
                />
              )}
            </div>
            <span className="text-xs text-[var(--color-text-secondary)]">
              Supports YouTube channels/playlists, TikTok, Facebook, SoundCloud, HLS & direct links
            </span>
          </div>

          <TextArea
            label="Media URLs to download"
            isLabelHidden
            rows={3}
            value={urlsText}
            onChange={(val) => setUrlsText(val)}
            placeholder="Paste one or more media, playlist, or channel URLs (one per line)..."
          />

          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
            <div className="flex flex-wrap items-center gap-2">
              <Button
                label="Paste"
                size="sm"
                variant="secondary"
                icon={<Clipboard className="w-3.5 h-3.5" />}
                onClick={handlePaste}
              />
              <Selector
                label="Download quality"
                isLabelHidden
                size="sm"
                variant="secondary"
                value={format}
                onChange={(val) => setFormat(val)}
                options={FORMAT_OPTIONS}
              />
            </div>

            <div className="flex flex-wrap items-center gap-2 justify-end">
              {parsedUrls.length >= 1 && (
                <Button
                  label="Inspect & Pick Items"
                  size="sm"
                  variant="secondary"
                  icon={<ListChecks className="w-3.5 h-3.5" />}
                  onClick={() => onOpenAlbumModal(parsedUrls[0])}
                />
              )}
              <Button
                label={
                  parsedUrls.length > 1
                    ? `Download Group (${parsedUrls.length})`
                    : 'Download'
                }
                size="sm"
                variant="primary"
                isLoading={isSubmitting}
                isDisabled={parsedUrls.length === 0}
                icon={<Download className="w-3.5 h-3.5" />}
                onClick={handleStartDownload}
              />
            </div>
          </div>
        </div>
      </Card>

      {/* Queue Controls & Tabs */}
      <div className="space-y-3">
        <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-3">
          <div className="overflow-x-auto scrollbar-none">
            <TabList
              value={filterStatus}
              onChange={(val) => setFilterStatus(val as FilterStatus)}
              size="sm"
              hasDivider
            >
              <Tab
                value="all"
                label="All"
                endContent={<Badge label={String(counts.all)} variant="neutral" />}
              />
              <Tab
                value="active"
                label="Active"
                endContent={
                  counts.active > 0 ? (
                    <Badge label={String(counts.active)} variant="info" />
                  ) : undefined
                }
              />
              <Tab
                value="completed"
                label="Completed"
                endContent={
                  counts.completed > 0 ? (
                    <Badge label={String(counts.completed)} variant="success" />
                  ) : undefined
                }
              />
              <Tab
                value="failed"
                label="Failed"
                endContent={
                  counts.failed > 0 ? (
                    <Badge label={String(counts.failed)} variant="error" />
                  ) : undefined
                }
              />
            </TabList>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {counts.failed > 0 && (
              <Button
                label="Retry Failed"
                size="sm"
                variant="secondary"
                icon={<RotateCcw className="w-3.5 h-3.5" />}
                onClick={async () => {
                  await retryFailedTasks();
                  onRefresh();
                }}
              />
            )}
            {counts.completed > 0 && (
              <Button
                label="Clear Done"
                size="sm"
                variant="secondary"
                icon={<Trash2 className="w-3.5 h-3.5" />}
                onClick={async () => {
                  await clearCompletedTasks();
                  onRefresh();
                }}
              />
            )}
          </div>
        </div>

        {/* Search, Grouping, Sort, and Density */}
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-2.5">
          <div className="flex-1 max-w-md">
            <TextInput
              label="Search queue"
              isLabelHidden
              size="sm"
              value={searchQuery}
              onChange={(val) => setSearchQuery(val)}
              placeholder="Search titles, channels, groups, or URLs..."
              startIcon={Search}
              hasClear
              width="100%"
            />
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <SegmentedControl
              label="Queue grouping"
              size="sm"
              value={orgView}
              onChange={(val) => setOrgView(val as OrganizationView)}
            >
              <SegmentedControlItem
                value="grouped"
                label="Grouped"
                icon={<FolderKanban className="w-3.5 h-3.5" />}
              />
              <SegmentedControlItem
                value="flat"
                label="All Items"
                icon={<List className="w-3.5 h-3.5" />}
              />
            </SegmentedControl>

            <Selector
              label="Sort downloads"
              isLabelHidden
              size="sm"
              variant="secondary"
              value={sortBy}
              onChange={(val) => setSortBy(val as SortOption)}
              options={SORT_OPTIONS}
            />

            <SegmentedControl
              label="Card density"
              size="sm"
              value={viewMode}
              onChange={(val) => setViewMode(val as ViewMode)}
            >
              <SegmentedControlItem
                value="standard"
                label="Standard"
                isLabelHidden
                icon={<LayoutGrid className="w-3.5 h-3.5" />}
              />
              <SegmentedControlItem
                value="compact"
                label="Compact"
                isLabelHidden
                icon={<Rows3 className="w-3.5 h-3.5" />}
              />
              <SegmentedControlItem
                value="list"
                label="List"
                isLabelHidden
                icon={<List className="w-3.5 h-3.5" />}
              />
            </SegmentedControl>
          </div>
        </div>
      </div>

      {/* Queue Items */}
      {filteredAndSortedTasks.length === 0 ? (
        <Card padding={6}>
          <EmptyState
            icon={<Sparkles className="w-8 h-8 text-[var(--color-icon-accent)]" />}
            title={
              tasks.length === 0
                ? 'Your download queue is empty'
                : 'No transfers match the current filter'
            }
            description={
              tasks.length === 0
                ? 'Paste media links, channel URLs, or browse with the In-App Browser to begin.'
                : 'Try switching status tabs or clearing your search query.'
            }
          />
        </Card>
      ) : orgView === 'grouped' ? (
        <div className="space-y-4">
          {groups.map((group) => (
            <TaskGroupCard
              key={group.groupId}
              group={group}
              viewMode={viewMode}
              onRefresh={onRefresh}
              onOpenCookies={onOpenCookies}
            />
          ))}

          {singles.length > 0 && (
            <div
              className={cn(
                viewMode === 'list'
                  ? 'flex flex-col gap-2'
                  : viewMode === 'compact'
                    ? 'grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2.5'
                    : 'grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3.5'
              )}
            >
              {singles.map((task) => (
                <TaskCard
                  key={task.id}
                  task={task}
                  viewMode={viewMode}
                  onRefresh={onRefresh}
                  onOpenCookies={onOpenCookies}
                />
              ))}
            </div>
          )}
        </div>
      ) : (
        <div
          className={cn(
            viewMode === 'list'
              ? 'flex flex-col gap-2'
              : viewMode === 'compact'
                ? 'grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2.5'
                : 'grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3.5'
          )}
        >
          {filteredAndSortedTasks.map((task) => (
            <TaskCard
              key={task.id}
              task={task}
              viewMode={viewMode}
              onRefresh={onRefresh}
              onOpenCookies={onOpenCookies}
            />
          ))}
        </div>
      )}
    </div>
  );
}
