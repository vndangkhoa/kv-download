'use client';

import React, { useState, useEffect, useMemo } from 'react';
import { AlbumEntry } from '@/types';
import { enqueueItems, scanMediaUrl } from '@/lib/api';
import { Dialog, DialogHeader } from '@astryxdesign/core/Dialog';
import { Button } from '@astryxdesign/core/Button';
import { Badge } from '@astryxdesign/core/Badge';
import { TextInput } from '@astryxdesign/core/TextInput';
import { Selector } from '@astryxdesign/core/Selector';
import { Banner } from '@astryxdesign/core/Banner';
import {
  CheckSquare,
  Square,
  Download,
  Loader2,
  ListMusic,
  Check,
  Search,
  Layers,
} from 'lucide-react';
import { cn } from '@/lib/utils';

interface AlbumModalProps {
  url: string;
  onClose: () => void;
  onSuccess: () => void;
}

const FORMAT_OPTIONS = [
  { label: 'Max / 8K / 4K (Highest)', value: 'highest' },
  { label: '1080p Full HD MP4', value: '1080p' },
  { label: '720p HD MP4', value: '720p' },
  { label: 'Audio Only (MP3)', value: 'audio' },
];

export function AlbumModal({ url, onClose, onSuccess }: AlbumModalProps) {
  const [entries, setEntries] = useState<AlbumEntry[]>([]);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [isLoading, setIsLoading] = useState(true);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [format, setFormat] = useState('highest');
  const [searchQuery, setSearchQuery] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [scanTitle, setScanTitle] = useState('Channel / Playlist Batch');
  const [scanChannel, setScanChannel] = useState('');
  const [totalEntries, setTotalEntries] = useState<number>(0);

  useEffect(() => {
    let active = true;
    const load = async () => {
      setIsLoading(true);
      setError(null);
      try {
        const data = await scanMediaUrl(url, 1, 50);
        if (!active) return;

        if (data.error) {
          setError(data.error);
          setIsLoading(false);
          return;
        }

        const rawTitle = data.title || data.playlistTitle || 'Playlist / Channel Selection';
        const rawChannel = data.channel || data.uploader || '';
        setScanTitle(rawTitle);
        setScanChannel(rawChannel);
        setTotalEntries(Number(data.playlistCount || data.totalCount || 0));

        const list: AlbumEntry[] = (data.entries || []).map((e: any, idx: number) => ({
          id: e.id || `item-${idx}`,
          url: e.url || e.webpage_url || url,
          title: e.title || `Track ${idx + 1}`,
          duration: e.durationString || e.duration_string || e.duration || '',
          thumbnail: e.thumbnail,
          uploader: e.uploader || rawChannel,
          channel: e.channel || e.uploader || rawChannel,
          infoJsonPath: e.infoJsonPath || e.info_json_path,
        }));

        if (list.length === 0 && data.webpage_url) {
          list.push({
            id: data.id || 'single',
            url: data.webpage_url,
            title: data.title || url,
            duration: data.durationString || data.duration_string || '',
            thumbnail: data.thumbnail,
            uploader: rawChannel,
            channel: rawChannel,
            infoJsonPath: data.infoJsonPath,
          });
        }

        setEntries(list);
        setSelectedIds(new Set(list.map((it) => it.id)));
        setHasMore(Boolean(data.hasMore) || (Number(data.playlistCount || 0) > list.length));
      } catch (err: any) {
        if (active) setError(err?.message || 'Failed to inspect URL');
      } finally {
        if (active) setIsLoading(false);
      }
    };

    load();
    return () => {
      active = false;
    };
  }, [url]);

  const handleLoadMore = async () => {
    if (isLoadingMore || isLoading) return;
    setIsLoadingMore(true);
    try {
      const start = entries.length + 1;
      const data = await scanMediaUrl(url, start, 50);
      const nextItems: AlbumEntry[] = (data.entries || []).map((e: any, idx: number) => ({
        id: e.id || `item-${start + idx}`,
        url: e.url || e.webpage_url || url,
        title: e.title || `Track ${start + idx}`,
        duration: e.durationString || e.duration_string || e.duration || '',
        thumbnail: e.thumbnail,
        uploader: e.uploader || scanChannel,
        channel: e.channel || e.uploader || scanChannel,
        infoJsonPath: e.infoJsonPath || e.info_json_path,
      }));
      if (nextItems.length > 0) {
        setEntries((prev) => {
          const existing = new Set(prev.map((p) => p.url));
          const unique = nextItems.filter((n) => !existing.has(n.url));
          return [...prev, ...unique];
        });
        setSelectedIds((prev) => {
          const next = new Set(prev);
          nextItems.forEach((n) => next.add(n.id));
          return next;
        });
      }
      setHasMore(Boolean(data.hasMore) && nextItems.length > 0);
    } catch {
      // Keep already scanned items visible.
    } finally {
      setIsLoadingMore(false);
    }
  };

  const filteredEntries = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return entries;
    return entries.filter(
      (e) =>
        e.title.toLowerCase().includes(q) ||
        (e.channel || '').toLowerCase().includes(q) ||
        e.url.toLowerCase().includes(q)
    );
  }, [entries, searchQuery]);

  const toggleSelectAll = () => {
    if (selectedIds.size >= entries.length) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(entries.map((e) => e.id)));
    }
  };

  const toggleItem = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleDownloadSelected = async () => {
    const selected = entries.filter((e) => selectedIds.has(e.id));
    if (selected.length === 0) return;

    setIsSubmitting(true);
    try {
      const res = await enqueueItems(
        selected.map((e) => ({
          url: e.url,
          title: e.title,
          thumbnail: e.thumbnail,
          uploader: e.uploader,
          channel: e.channel || scanChannel,
          duration: e.duration,
          infoJsonPath: e.infoJsonPath,
        })),
        format,
        {
          groupTitle: scanTitle,
          groupType: selected.length > 1 ? 'playlist' : 'single',
          playlistTitle: scanTitle,
          channel: scanChannel || selected[0]?.channel,
        }
      );
      if (!res.success) {
        setError(res.error || 'Failed to enqueue selected items');
        return;
      }
      onSuccess();
      onClose();
    } catch (err: any) {
      setError(err?.message || 'Failed to enqueue selected items');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog
      isOpen={Boolean(url)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      width={720}
      maxHeight="85dvh"
      purpose="form"
    >
      <div className="flex flex-col max-h-[82dvh]">
        <DialogHeader
          title={scanTitle}
          subtitle={
            scanChannel
              ? `${scanChannel} · ${totalEntries > entries.length ? `${entries.length} of ${totalEntries}` : entries.length} items`
              : `${entries.length} items detected`
          }
          onOpenChange={(open) => {
            if (!open) onClose();
          }}
          startContent={
            <div className="w-8 h-8 rounded-lg bg-[var(--color-accent-muted)] flex items-center justify-center text-[var(--color-icon-accent)]">
              <ListMusic className="w-4 h-4" />
            </div>
          }
          endContent={
            <Badge
              label={`${selectedIds.size} selected`}
              variant="purple"
              icon={<Layers className="w-3 h-3" />}
            />
          }
        />

        {/* Toolbar */}
        {!isLoading && entries.length > 0 && (
          <div className="px-4 py-3 border-b border-[var(--color-border)] bg-[var(--color-background-muted)] flex flex-col sm:flex-row sm:items-center gap-2.5">
            <div className="flex-1 min-w-0">
              <TextInput
                label="Filter entries"
                isLabelHidden
                value={searchQuery}
                onChange={(val) => setSearchQuery(val)}
                placeholder="Filter videos or tracks..."
                startIcon={Search}
                hasClear
                size="sm"
                width="100%"
              />
            </div>

            <div className="flex items-center gap-2 shrink-0">
              <Button
                label={selectedIds.size >= entries.length ? 'Clear All' : 'Select All'}
                size="sm"
                variant="secondary"
                icon={
                  selectedIds.size >= entries.length ? (
                    <CheckSquare className="w-3.5 h-3.5" />
                  ) : (
                    <Square className="w-3.5 h-3.5" />
                  )
                }
                onClick={toggleSelectAll}
              />
              <Selector
                label="Quality"
                isLabelHidden
                size="sm"
                variant="secondary"
                value={format}
                onChange={(val) => setFormat(val)}
                options={FORMAT_OPTIONS}
              />
            </div>
          </div>
        )}

        {/* Content List */}
        <div className="flex-1 overflow-y-auto p-4 space-y-2 min-h-[260px]">
          {isLoading ? (
            <div className="flex flex-col items-center justify-center py-14 gap-3 text-center">
              <Loader2 className="w-7 h-7 animate-spin text-[var(--color-icon-accent)]" />
              <div className="space-y-1">
                <p className="text-sm font-semibold text-[var(--color-text-primary)]">
                  Inspecting source playlist or channel...
                </p>
                <p className="text-xs font-mono text-[var(--color-text-secondary)] max-w-md truncate">
                  {url}
                </p>
              </div>
            </div>
          ) : error ? (
            <Banner status="error" title="Unable to inspect source" description={error} />
          ) : filteredEntries.length === 0 ? (
            <div className="py-12 text-center text-sm text-[var(--color-text-secondary)]">
              No matching entries found.
            </div>
          ) : (
            <div className="space-y-2">
              {filteredEntries.map((item, idx) => {
                const isSelected = selectedIds.has(item.id);
                return (
                  <button
                    key={`${item.id}-${idx}`}
                    type="button"
                    onClick={() => toggleItem(item.id)}
                    className={cn(
                      'w-full flex items-center gap-3 p-2.5 rounded-lg border text-left transition-colors',
                      isSelected
                        ? 'bg-[var(--color-accent-muted)] border-[var(--color-accent)]'
                        : 'bg-[var(--color-background-card)] border-[var(--color-border)] hover:bg-[var(--color-overlay-hover)]'
                    )}
                  >
                    <div
                      className={cn(
                        'w-5 h-5 rounded border flex items-center justify-center shrink-0 transition-colors',
                        isSelected
                          ? 'bg-[var(--color-accent)] border-[var(--color-accent)] text-[var(--color-on-accent)]'
                          : 'border-[var(--color-border-emphasized)] bg-[var(--color-background-surface)]'
                      )}
                    >
                      {isSelected && <Check className="w-3.5 h-3.5" />}
                    </div>

                    {item.thumbnail && (
                      <img
                        src={item.thumbnail}
                        alt=""
                        className="w-16 h-10 rounded object-cover border border-[var(--color-border)] shrink-0 bg-[var(--color-background-muted)]"
                      />
                    )}

                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-[var(--color-text-primary)] truncate">
                        {item.title}
                      </p>
                      <p className="text-xs text-[var(--color-text-secondary)] truncate">
                        {item.channel || item.uploader || 'Source item'}
                      </p>
                    </div>

                    {item.duration && (
                      <span className="text-xs font-mono tabular-nums text-[var(--color-text-secondary)] shrink-0">
                        {String(item.duration)}
                      </span>
                    )}
                  </button>
                );
              })}

              {hasMore && (
                <div className="pt-2 flex justify-center">
                  <Button
                    label={isLoadingMore ? 'Loading more...' : 'Load More Entries'}
                    variant="secondary"
                    size="sm"
                    isLoading={isLoadingMore}
                    onClick={handleLoadMore}
                  />
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="px-4 py-3 border-t border-[var(--color-border)] bg-[var(--color-background-surface)] flex items-center justify-between gap-3">
          <span className="text-xs text-[var(--color-text-secondary)]">
            {selectedIds.size > 1
              ? 'Will be grouped automatically as a batch/playlist'
              : 'Ready to enqueue'}
          </span>
          <div className="flex items-center gap-2">
            <Button label="Cancel" variant="secondary" size="sm" onClick={onClose} />
            <Button
              label={`Download Selected (${selectedIds.size})`}
              variant="primary"
              size="sm"
              isLoading={isSubmitting}
              isDisabled={isLoading || isSubmitting || selectedIds.size === 0}
              icon={<Download className="w-3.5 h-3.5" />}
              onClick={handleDownloadSelected}
            />
          </div>
        </div>
      </div>
    </Dialog>
  );
}
