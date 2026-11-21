'use client';

import React, { useState, useEffect, useMemo, useCallback } from 'react';
import {
  GalleryFolder,
  GalleryItem,
  GallerySortMode,
  MediaType,
  ViewMode,
} from '@/types';
import { GalleryCard } from './GalleryCard';
import {
  deleteMultipleGalleryFiles,
  fetchGallery,
  getFolderZipUrl,
  organizeGalleryLibrary,
} from '@/lib/api';
import { detectPlatform, formatBytes, cn } from '@/lib/utils';
import { Card } from '@astryxdesign/core/Card';
import { Button } from '@astryxdesign/core/Button';
import { TextInput } from '@astryxdesign/core/TextInput';
import { Selector } from '@astryxdesign/core/Selector';
import { SegmentedControl, SegmentedControlItem } from '@astryxdesign/core/SegmentedControl';
import { TabList, Tab } from '@astryxdesign/core/TabList';
import { Badge } from '@astryxdesign/core/Badge';
import { Banner } from '@astryxdesign/core/Banner';
import { EmptyState } from '@astryxdesign/core/EmptyState';
import { AlertDialog } from '@astryxdesign/core/AlertDialog';
import {
  Film,
  Folder,
  FolderArchive,
  FolderKanban,
  FolderSync,
  Image as ImageIcon,
  Layers,
  LayoutGrid,
  List,
  Loader2,
  Music,
  Rows3,
  Search,
  Trash2,
} from 'lucide-react';

type GroupViewMode = 'flat' | 'folder' | 'platform';

const SORT_OPTIONS = [
  { label: 'Newest First', value: 'newest' },
  { label: 'Oldest First', value: 'oldest' },
  { label: 'Largest Size', value: 'size_desc' },
  { label: 'Smallest Size', value: 'size_asc' },
  { label: 'Title A-Z', value: 'name_asc' },
  { label: 'Title Z-A', value: 'name_desc' },
  { label: 'Channel / Folder', value: 'channel_asc' },
];

export function GalleryView() {
  const [items, setItems] = useState<GalleryItem[]>([]);
  const [folders, setFolders] = useState<GalleryFolder[]>([]);
  const [totalSize, setTotalSize] = useState('');
  const [isLoading, setIsLoading] = useState(true);
  const [isOrganizing, setIsOrganizing] = useState(false);
  const [organizeMessage, setOrganizeMessage] = useState<string | null>(null);

  const [activeCategory, setActiveCategory] = useState<'all' | MediaType>('all');
  const [selectedFolder, setSelectedFolder] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [sortBy, setSortBy] = useState<GallerySortMode>('newest');
  const [groupMode, setGroupMode] = useState<GroupViewMode>('folder');
  const [viewMode, setViewMode] = useState<ViewMode>('standard');

  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set());
  const [showBatchDeleteDialog, setShowBatchDeleteDialog] = useState(false);
  const [isBatchDeleting, setIsBatchDeleting] = useState(false);

  const loadGallery = useCallback(async () => {
    setIsLoading(true);
    try {
      const data = await fetchGallery({ sort: sortBy });
      setItems(data.items || []);
      setFolders(data.folders || []);
      setTotalSize(data.totalSize || '');
    } catch (err) {
      console.error('Failed to load gallery:', err);
    } finally {
      setIsLoading(false);
    }
  }, [sortBy]);

  useEffect(() => {
    loadGallery();
  }, [loadGallery]);

  const handleAutoOrganize = async () => {
    setIsOrganizing(true);
    setOrganizeMessage(null);
    try {
      const res = await organizeGalleryLibrary();
      setOrganizeMessage(
        `Organized library using "${res.organizeMode || 'configured'}" layout (${res.movedFiles ?? 0} files relocated, ${res.totalItems ?? items.length} indexed).`
      );
      await loadGallery();
    } catch (err: any) {
      setOrganizeMessage(err?.message || 'Failed to auto-organize library');
    } finally {
      setIsOrganizing(false);
    }
  };

  const toggleSelect = (path: string) => {
    setSelectedPaths((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  };

  const handleConfirmBatchDelete = async () => {
    if (selectedPaths.size === 0) return;
    setIsBatchDeleting(true);
    try {
      await deleteMultipleGalleryFiles(Array.from(selectedPaths));
      setSelectedPaths(new Set());
      setShowBatchDeleteDialog(false);
      await loadGallery();
    } finally {
      setIsBatchDeleting(false);
    }
  };

  const filteredItems = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    const list = items.filter((item) => {
      if (activeCategory !== 'all' && item.type !== activeCategory) return false;
      if (selectedFolder) {
        const ch = (item.channel || '').toLowerCase();
        const grp = (item.group || '').toLowerCase();
        const pth = (item.path || '').toLowerCase();
        const fLower = selectedFolder.toLowerCase();
        if (ch !== fLower && grp !== fLower && !pth.includes(`/${fLower}/`) && !pth.startsWith(`${fLower}/`)) {
          return false;
        }
      }
      if (q) {
        const hay = [item.title, item.name, item.channel, item.group, item.platform, item.path]
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
          return a.modTime.localeCompare(b.modTime);
        case 'size_desc':
          return (b.sizeInBytes || 0) - (a.sizeInBytes || 0);
        case 'size_asc':
          return (a.sizeInBytes || 0) - (b.sizeInBytes || 0);
        case 'name_asc':
          return (a.title || a.name).localeCompare(b.title || b.name);
        case 'name_desc':
          return (b.title || b.name).localeCompare(a.title || a.name);
        case 'channel_asc':
          return (a.channel || 'zzz').localeCompare(b.channel || 'zzz');
        case 'newest':
        default:
          return b.modTime.localeCompare(a.modTime);
      }
    });
    return copy;
  }, [items, activeCategory, selectedFolder, searchQuery, sortBy]);

  const groupedSections = useMemo(() => {
    if (groupMode === 'flat' || selectedFolder) {
      return [{ key: 'all', title: selectedFolder || 'All Media', items: filteredItems }];
    }

    const map = new Map<string, GalleryItem[]>();
    for (const item of filteredItems) {
      const key =
        groupMode === 'platform'
          ? item.platform || detectPlatform(item.url || '').name || 'Local'
          : item.channel || item.group || 'General';
      const arr = map.get(key);
      if (arr) arr.push(item);
      else map.set(key, [item]);
    }

    return Array.from(map.entries())
      .map(([key, sectionItems]) => ({
        key,
        title: key,
        items: sectionItems,
      }))
      .sort((a, b) => a.title.localeCompare(b.title));
  }, [filteredItems, groupMode, selectedFolder]);

  return (
    <div className="max-w-7xl mx-auto space-y-4">
      {/* Top Toolbar: Search, Category, Organize, Folder ZIP */}
      <Card padding={3} elevation="low">
        <div className="space-y-3">
          <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-2.5">
            <div className="flex-1 max-w-md">
              <TextInput
                label="Search library"
                isLabelHidden
                size="sm"
                width="100%"
                startIcon={Search}
                value={searchQuery}
                onChange={(val) => setSearchQuery(val)}
                hasClear
                placeholder="Search title, channel, folder, or platform..."
              />
            </div>

            <div className="flex flex-wrap items-center gap-2">
              {selectedFolder && (
                <a
                  href={getFolderZipUrl(selectedFolder)}
                  download
                  className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold border border-[var(--color-border)] bg-[var(--color-background-muted)] text-[var(--color-text-primary)] hover:bg-[var(--color-overlay-hover)]"
                >
                  <FolderArchive className="w-3.5 h-3.5 text-[var(--color-icon-accent)]" />
                  Download Folder ZIP
                </a>
              )}

              <Button
                label="Auto-Organize"
                size="sm"
                variant="secondary"
                isLoading={isOrganizing}
                icon={<FolderSync className="w-3.5 h-3.5" />}
                onClick={handleAutoOrganize}
              />

              <Selector
                label="Sort gallery"
                isLabelHidden
                size="sm"
                variant="secondary"
                value={sortBy}
                onChange={(val) => setSortBy(val as GallerySortMode)}
                options={SORT_OPTIONS}
              />
            </div>
          </div>

          <div className="flex flex-col md:flex-row md:items-center justify-between gap-2.5 pt-2 border-t border-[var(--color-border)]">
            <div className="overflow-x-auto scrollbar-none">
              <TabList
                value={activeCategory}
                onChange={(val) => setActiveCategory(val as 'all' | MediaType)}
                size="sm"
              >
                <Tab value="all" label="All Media" />
                <Tab value="video" label="Videos" icon={<Film className="w-3.5 h-3.5" />} />
                <Tab value="audio" label="Audio" icon={<Music className="w-3.5 h-3.5" />} />
                <Tab
                  value="photo"
                  label="Photos"
                  icon={<ImageIcon className="w-3.5 h-3.5" />}
                />
              </TabList>
            </div>

            <div className="flex flex-wrap items-center gap-2">
              <SegmentedControl
                label="Library grouping"
                size="sm"
                value={groupMode}
                onChange={(val) => setGroupMode(val as GroupViewMode)}
              >
                <SegmentedControlItem
                  value="folder"
                  label="By Folder"
                  icon={<FolderKanban className="w-3.5 h-3.5" />}
                />
                <SegmentedControlItem
                  value="platform"
                  label="Platform"
                  icon={<Layers className="w-3.5 h-3.5" />}
                />
                <SegmentedControlItem value="flat" label="Flat" />
              </SegmentedControl>

              <SegmentedControl
                label="Gallery card size"
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

          {/* Horizontal Folder Filter Strip */}
          {folders.length > 0 && (
            <div className="flex items-center gap-1.5 overflow-x-auto scrollbar-none pt-1">
              <button
                type="button"
                onClick={() => setSelectedFolder('')}
                className={cn(
                  'shrink-0 inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium border transition-colors',
                  !selectedFolder
                    ? 'bg-[var(--color-accent)] border-[var(--color-accent)] text-[var(--color-on-accent)]'
                    : 'bg-[var(--color-background-surface)] border-[var(--color-border)] text-[var(--color-text-secondary)] hover:text-[var(--color-text-primary)]'
                )}
              >
                All ({items.length})
                {totalSize ? ` · ${totalSize}` : ''}
              </button>

              {folders.map((folder) => {
                const isActive = selectedFolder === folder.name;
                return (
                  <button
                    key={folder.name}
                    type="button"
                    onClick={() => setSelectedFolder(isActive ? '' : folder.name)}
                    className={cn(
                      'shrink-0 inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium border transition-colors',
                      isActive
                        ? 'bg-[var(--color-accent)] border-[var(--color-accent)] text-[var(--color-on-accent)]'
                        : 'bg-[var(--color-background-surface)] border-[var(--color-border)] text-[var(--color-text-secondary)] hover:text-[var(--color-text-primary)]'
                    )}
                  >
                    <Folder className="w-3 h-3" />
                    <span>{folder.name}</span>
                    <span className="font-mono tabular-nums opacity-80">
                      ({folder.fileCount})
                    </span>
                  </button>
                );
              })}
            </div>
          )}
        </div>
      </Card>

      {organizeMessage && (
        <Banner
          status="success"
          title="Library Organization Complete"
          description={organizeMessage}
          isDismissable
          onDismiss={() => setOrganizeMessage(null)}
        />
      )}

      {/* Batch Selection Bar */}
      {selectedPaths.size > 0 && (
        <Card padding={2} variant="muted">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <Badge label={`${selectedPaths.size} selected`} variant="purple" />
              <Button
                label="Select All Visible"
                size="sm"
                variant="ghost"
                onClick={() =>
                  setSelectedPaths(new Set(filteredItems.map((i) => i.path)))
                }
              />
              <Button
                label="Clear Selection"
                size="sm"
                variant="ghost"
                onClick={() => setSelectedPaths(new Set())}
              />
            </div>

            <Button
              label={`Delete Selected (${selectedPaths.size})`}
              size="sm"
              variant="destructive"
              icon={<Trash2 className="w-3.5 h-3.5" />}
              onClick={() => setShowBatchDeleteDialog(true)}
            />
          </div>
        </Card>
      )}

      {/* Content */}
      {isLoading ? (
        <div className="py-16 flex flex-col items-center justify-center gap-3">
          <Loader2 className="w-7 h-7 animate-spin text-[var(--color-icon-accent)]" />
          <span className="text-sm text-[var(--color-text-secondary)]">
            Scanning organized media library...
          </span>
        </div>
      ) : filteredItems.length === 0 ? (
        <Card padding={6}>
          <EmptyState
            icon={<Film className="w-8 h-8 text-[var(--color-icon-accent)]" />}
            title="No media files found"
            description="Completed downloads and organized channel folders will appear here automatically."
          />
        </Card>
      ) : (
        <div className="space-y-6">
          {groupedSections.map((section) => {
            const sectionBytes = section.items.reduce(
              (sum, it) => sum + (it.sizeInBytes || 0),
              0
            );
            return (
              <section key={section.key} className="space-y-3">
                {groupMode !== 'flat' && !selectedFolder && (
                  <div className="flex items-center justify-between gap-2 pb-1.5 border-b border-[var(--color-border)]">
                    <div className="flex items-center gap-2 min-w-0">
                      <Folder className="w-4 h-4 text-[var(--color-icon-accent)] shrink-0" />
                      <h3 className="text-sm font-bold text-[var(--color-text-primary)] truncate">
                        {section.title}
                      </h3>
                      <Badge label={`${section.items.length}`} variant="neutral" />
                      <span className="text-xs font-mono tabular-nums text-[var(--color-text-secondary)]">
                        {formatBytes(sectionBytes)}
                      </span>
                    </div>

                    {groupMode === 'folder' && section.title !== 'General' && (
                      <a
                        href={getFolderZipUrl(section.title)}
                        download
                        className="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium text-[var(--color-text-accent)] hover:bg-[var(--color-accent-muted)]"
                      >
                        <FolderArchive className="w-3.5 h-3.5" />
                        ZIP
                      </a>
                    )}
                  </div>
                )}

                <div
                  className={cn(
                    viewMode === 'list'
                      ? 'flex flex-col gap-2'
                      : viewMode === 'compact'
                        ? 'grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3'
                        : 'grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4'
                  )}
                >
                  {section.items.map((item) => (
                    <GalleryCard
                      key={item.id || item.path}
                      item={item}
                      viewMode={viewMode}
                      isSelected={selectedPaths.has(item.path)}
                      onToggleSelect={toggleSelect}
                      onDeleted={loadGallery}
                    />
                  ))}
                </div>
              </section>
            );
          })}
        </div>
      )}

      <AlertDialog
        isOpen={showBatchDeleteDialog}
        onOpenChange={setShowBatchDeleteDialog}
        title="Delete Selected Media Files?"
        description={`Permanently delete ${selectedPaths.size} selected file(s) from server storage? This cannot be undone.`}
        actionLabel="Delete All Selected"
        actionVariant="destructive"
        isActionLoading={isBatchDeleting}
        onAction={handleConfirmBatchDelete}
      />
    </div>
  );
}
