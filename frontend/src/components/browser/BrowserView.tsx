'use client';

import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { SniffedMedia } from '@/types';
import { enqueueItems, enqueueUrls, fetchBrowserSniff } from '@/lib/api';
import { usePlayer } from '@/context/PlayerContext';
import { Button } from '@astryxdesign/core/Button';
import { IconButton } from '@astryxdesign/core/IconButton';
import { TextInput } from '@astryxdesign/core/TextInput';
import { Selector } from '@astryxdesign/core/Selector';
import { TabList, Tab } from '@astryxdesign/core/TabList';
import { Badge } from '@astryxdesign/core/Badge';
import { Card } from '@astryxdesign/core/Card';
import { Banner } from '@astryxdesign/core/Banner';
import {
  ArrowLeft,
  ArrowRight,
  Check,
  Compass,
  Download,
  ExternalLink,
  Film,
  Globe,
  Home,
  Image as ImageIcon,
  ListChecks,
  Loader2,
  Music,
  Play,
  Radar,
  RotateCw,
  Video,
  X,
} from 'lucide-react';
import { cn } from '@/lib/utils';

interface BrowserViewProps {
  onOpenAlbumModal?: (url: string) => void;
  onQueued?: () => void;
}

type SniffFilter = 'video' | 'audio' | 'image' | 'all';

const QUALITY_OPTIONS = [
  { label: 'Highest Quality', value: 'highest' },
  { label: '1080p MP4', value: '1080p' },
  { label: '720p MP4', value: '720p' },
  { label: 'Audio (MP3)', value: 'audio' },
];

const QUICK_BOOKMARKS = [
  { name: 'YouTube', url: 'https://www.youtube.com', desc: 'Videos, playlists & channels' },
  { name: 'Tinhte.vn', url: 'https://tinhte.vn', desc: 'Tech news & embedded videos' },
  { name: 'AV01 Media', url: 'https://av01.media', desc: 'DNS-resilient proxy stream host' },
  { name: 'SoundCloud', url: 'https://soundcloud.com', desc: 'Audio tracks & albums' },
  { name: 'TikTok', url: 'https://www.tiktok.com', desc: 'Short-form feeds' },
  { name: 'Vimeo', url: 'https://vimeo.com', desc: 'Studio & HD reels' },
];

function normalizeInputUrl(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) return '';
  if (trimmed.startsWith('http://') || trimmed.startsWith('https://')) return trimmed;
  return `https://${trimmed}`;
}

export function BrowserView({ onOpenAlbumModal, onQueued }: BrowserViewProps) {
  const { playStream } = usePlayer();

  const [urlInput, setUrlInput] = useState('');
  const [activeUrl, setActiveUrl] = useState('');
  const [history, setHistory] = useState<string[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const [reloadNonce, setReloadNonce] = useState(0);

  const [isLoadingPage, setIsLoadingPage] = useState(false);
  const [isSniffing, setIsSniffing] = useState(false);
  const [isSnifferOpen, setIsSnifferOpen] = useState(false);
  const [sniffFilter, setSniffFilter] = useState<SniffFilter>('video');
  const [quality, setQuality] = useState('highest');
  const [sniffedItems, setSniffedItems] = useState<SniffedMedia[]>([]);
  const [selectedUrls, setSelectedUrls] = useState<Set<string>>(new Set());
  const [statusNotice, setStatusNotice] = useState<string | null>(null);
  const [isEnqueuing, setIsEnqueuing] = useState(false);

  const seenUrlsRef = useRef<Set<string>>(new Set());
  const lastSniffedPageRef = useRef<string>('');

  const mergeSniffedItems = useCallback((incoming: SniffedMedia[]) => {
    if (!incoming || incoming.length === 0) return;
    setSniffedItems((prev) => {
      const next = [...prev];
      const byUrl = new Map<string, number>();
      next.forEach((it, idx) => byUrl.set(it.url, idx));

      for (const item of incoming) {
        if (!item?.url) continue;
        const idx = byUrl.get(item.url);
        if (idx !== undefined) {
          // Prefer higher-confidence metadata if updating existing URL
          const existing = next[idx];
          next[idx] = {
            ...existing,
            ...item,
            title: item.title || existing.title,
            thumbnail: item.thumbnail || existing.thumbnail,
            duration: item.duration || existing.duration,
            resolution: item.resolution || existing.resolution,
          };
        } else {
          seenUrlsRef.current.add(item.url);
          byUrl.set(item.url, next.length);
          next.push(item);
        }
      }

      // Sort videos and streams before audio and images
      next.sort((a, b) => {
        const rank = (m: SniffedMedia) =>
          m.type === 'video' ? (m.isStream ? 0 : 1) : m.type === 'audio' ? 2 : 3;
        return rank(a) - rank(b);
      });
      return next;
    });
  }, []);

  const runServerSniff = useCallback(
    async (targetUrl: string) => {
      if (!targetUrl) return;
      setIsSniffing(true);
      try {
        const items = await fetchBrowserSniff(targetUrl);
        mergeSniffedItems(items);
        if (items.some((i) => i.type === 'video' || i.type === 'audio')) {
          setIsSnifferOpen(true);
        }
      } catch {
        // Ignore transient sniff error while page DOM sniffer runs.
      } finally {
        setIsSniffing(false);
      }
    },
    [mergeSniffedItems]
  );

  const navigateTo = useCallback(
    (rawUrl: string, pushHistory: boolean = true) => {
      const target = normalizeInputUrl(rawUrl);
      if (!target) return;

      setUrlInput(target);
      setActiveUrl(target);
      setIsLoadingPage(true);
      setStatusNotice(null);
      setSniffedItems([]);
      setSelectedUrls(new Set());
      seenUrlsRef.current.clear();
      lastSniffedPageRef.current = target;

      if (pushHistory) {
        setHistory((prev) => {
          const sliced = historyIndex >= 0 ? prev.slice(0, historyIndex + 1) : prev;
          if (sliced[sliced.length - 1] === target) return sliced;
          const updated = [...sliced, target];
          setHistoryIndex(updated.length - 1);
          return updated;
        });
      }

      runServerSniff(target);
    },
    [historyIndex, runServerSniff]
  );

  const handleBack = () => {
    if (historyIndex <= 0) return;
    const prevIdx = historyIndex - 1;
    const target = history[prevIdx];
    setHistoryIndex(prevIdx);
    navigateTo(target, false);
  };

  const handleForward = () => {
    if (historyIndex >= history.length - 1) return;
    const nextIdx = historyIndex + 1;
    const target = history[nextIdx];
    setHistoryIndex(nextIdx);
    navigateTo(target, false);
  };

  const handleHome = () => {
    setActiveUrl('');
    setUrlInput('');
    setSniffedItems([]);
    setSelectedUrls(new Set());
    setIsSnifferOpen(false);
    setStatusNotice(null);
  };

  const handleReload = () => {
    if (!activeUrl) return;
    setIsLoadingPage(true);
    setReloadNonce((n) => n + 1);
    runServerSniff(activeUrl);
  };

  // Listen to proxy iframe messages for link navigation & live DOM/fetch/XHR media capture
  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      const data = event.data;
      if (!data || typeof data !== 'object') return;

      if (data.type === 'kv_browser_navigate' && typeof data.url === 'string') {
        const nextUrl = data.url.trim();
        if (!nextUrl) return;
        setUrlInput(nextUrl);
        setActiveUrl(nextUrl);
        setIsLoadingPage(false);
        if (lastSniffedPageRef.current !== nextUrl) {
          lastSniffedPageRef.current = nextUrl;
          setSniffedItems([]);
          setSelectedUrls(new Set());
          seenUrlsRef.current.clear();
          setHistory((prev) => {
            const updated = [...prev, nextUrl];
            setHistoryIndex(updated.length - 1);
            return updated;
          });
          runServerSniff(nextUrl);
        }
      } else if (data.type === 'kv_sniff_media' && data.payload) {
        const p = data.payload;
        const itemUrl = typeof p.url === 'string' ? p.url.trim() : '';
        if (!itemUrl || itemUrl.startsWith('data:') || itemUrl.startsWith('blob:')) return;
        const mediaType: SniffedMedia['type'] =
          p.mediaType === 'audio'
            ? 'audio'
            : p.mediaType === 'image'
              ? 'image'
              : 'video';
        mergeSniffedItems([
          {
            id: `dom-${Math.abs(itemUrl.split('').reduce((a: number, c: string) => (a * 31 + c.charCodeAt(0)) | 0, 1))}`,
            url: itemUrl,
            type: mediaType,
            format: p.format || (mediaType === 'video' ? 'MP4/HLS' : mediaType.toUpperCase()),
            title: p.title || itemUrl.split('/').pop() || 'Detected Media',
            thumbnail: p.thumbnail,
            source: 'dom',
            isStream: /\.(m3u8|mpd)(\?|$)/i.test(itemUrl),
          },
        ]);
      } else if (data.type === 'kv_browser_error' && typeof data.message === 'string') {
        setStatusNotice(data.message);
        setIsLoadingPage(false);
      }
    };

    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
  }, [mergeSniffedItems, runServerSniff]);

  const counts = useMemo(() => {
    let video = 0;
    let audio = 0;
    let image = 0;
    for (const item of sniffedItems) {
      if (item.type === 'video') video++;
      else if (item.type === 'audio') audio++;
      else image++;
    }
    return { video, audio, image, all: sniffedItems.length };
  }, [sniffedItems]);

  const filteredSniffed = useMemo(() => {
    if (sniffFilter === 'all') return sniffedItems;
    return sniffedItems.filter((it) => it.type === sniffFilter);
  }, [sniffedItems, sniffFilter]);

  const toggleSelectItem = (url: string) => {
    setSelectedUrls((prev) => {
      const next = new Set(prev);
      if (next.has(url)) next.delete(url);
      else next.add(url);
      return next;
    });
  };

  const toggleSelectVisible = () => {
    const visibleUrls = filteredSniffed.map((i) => i.url);
    const allSelected = visibleUrls.length > 0 && visibleUrls.every((u) => selectedUrls.has(u));
    if (allSelected) {
      setSelectedUrls((prev) => {
        const next = new Set(prev);
        visibleUrls.forEach((u) => next.delete(u));
        return next;
      });
    } else {
      setSelectedUrls((prev) => {
        const next = new Set(prev);
        visibleUrls.forEach((u) => next.add(u));
        return next;
      });
    }
  };

  const handleDownloadSingle = async (item: SniffedMedia) => {
    setIsEnqueuing(true);
    try {
      await enqueueItems(
        [
          {
            url: item.url,
            title: item.title,
            thumbnail: item.thumbnail,
            channel: item.channel || getHostLabel(activeUrl),
            duration: item.duration,
          },
        ],
        item.type === 'audio' ? 'audio' : quality,
        {
          channel: item.channel || getHostLabel(activeUrl),
        }
      );
      setStatusNotice(`Queued "${item.title}" for download.`);
      onQueued?.();
    } finally {
      setIsEnqueuing(false);
    }
  };

  const handleDownloadSelectedGroup = async () => {
    const selected = sniffedItems.filter((it) => selectedUrls.has(it.url));
    if (selected.length === 0) return;

    setIsEnqueuing(true);
    try {
      const host = getHostLabel(activeUrl) || 'Browser';
      await enqueueItems(
        selected.map((it) => ({
          url: it.url,
          title: it.title,
          thumbnail: it.thumbnail,
          channel: it.channel || host,
          duration: it.duration,
        })),
        quality,
        {
          groupTitle: `${host} (${selected.length} detected)`,
          groupType: selected.length > 1 ? 'batch' : 'single',
          channel: host,
        }
      );
      setStatusNotice(`Queued ${selected.length} selected items as a download group.`);
      setSelectedUrls(new Set());
      onQueued?.();
    } finally {
      setIsEnqueuing(false);
    }
  };

  const handleDownloadCurrentPage = async () => {
    if (!activeUrl) return;
    setIsEnqueuing(true);
    try {
      await enqueueUrls([activeUrl], quality, {
        channel: getHostLabel(activeUrl),
      });
      setStatusNotice(`Sent ${activeUrl} to download queue.`);
      onQueued?.();
    } finally {
      setIsEnqueuing(false);
    }
  };

  return (
    <div className="max-w-7xl mx-auto flex flex-col gap-3 h-[calc(100dvh-10.5rem)] min-h-[540px]">
      {/* Browser Address & Control Bar */}
      <Card padding={3} elevation="low">
        <div className="flex flex-col lg:flex-row lg:items-center gap-2">
          <div className="flex items-center gap-1 shrink-0">
            <IconButton
              label="Back"
              size="sm"
              variant="secondary"
              isDisabled={historyIndex <= 0}
              icon={<ArrowLeft className="w-4 h-4" />}
              onClick={handleBack}
            />
            <IconButton
              label="Forward"
              size="sm"
              variant="secondary"
              isDisabled={historyIndex >= history.length - 1}
              icon={<ArrowRight className="w-4 h-4" />}
              onClick={handleForward}
            />
            <IconButton
              label="Home bookmarks"
              size="sm"
              variant="secondary"
              icon={<Home className="w-4 h-4" />}
              onClick={handleHome}
            />
            <IconButton
              label="Reload page"
              size="sm"
              variant="secondary"
              isDisabled={!activeUrl}
              icon={
                isLoadingPage ? (
                  <Loader2 className="w-4 h-4 animate-spin" />
                ) : (
                  <RotateCw className="w-4 h-4" />
                )
              }
              onClick={handleReload}
            />
          </div>

          <div className="flex-1 min-w-0 flex items-center gap-2">
            <div className="flex-1 min-w-0">
              <TextInput
                label="Web URL"
                isLabelHidden
                type="url"
                size="sm"
                width="100%"
                startIcon={Globe}
                value={urlInput}
                onChange={(val) => setUrlInput(val)}
                onEnter={() => navigateTo(urlInput)}
                enterKeyHint="go"
                hasClear
                placeholder="Enter a page or stream URL (e.g. https://av01.media, https://tinhte.vn)..."
              />
            </div>
            <Button
              label="Go"
              size="sm"
              variant="primary"
              isDisabled={!urlInput.trim()}
              onClick={() => navigateTo(urlInput)}
            />
          </div>

          <div className="flex flex-wrap items-center gap-1.5 shrink-0">
            {activeUrl && onOpenAlbumModal && (
              <Button
                label="Scan Page"
                size="sm"
                variant="secondary"
                icon={<ListChecks className="w-3.5 h-3.5" />}
                onClick={() => onOpenAlbumModal(activeUrl)}
              />
            )}
            <Button
              label={`Sniffer (${sniffedItems.length})`}
              size="sm"
              variant={isSnifferOpen ? 'primary' : 'secondary'}
              isLoading={isSniffing}
              icon={<Radar className="w-3.5 h-3.5" />}
              onClick={() => setIsSnifferOpen((o) => !o)}
            />
            {activeUrl && (
              <Button
                label="Download Page"
                size="sm"
                variant="secondary"
                isLoading={isEnqueuing}
                icon={<Download className="w-3.5 h-3.5" />}
                onClick={handleDownloadCurrentPage}
              />
            )}
          </div>
        </div>
      </Card>

      {statusNotice && (
        <Banner
          status="info"
          title={statusNotice}
          isDismissable
          onDismiss={() => setStatusNotice(null)}
        />
      )}

      {/* Workspace Split: Proxy Viewport + Collapsible Sniffer Panel */}
      <div className="flex-1 min-h-0 flex gap-3 relative overflow-hidden">
        {/* Browser Viewport */}
        <div className="flex-1 min-w-0 rounded-xl border border-[var(--color-border)] bg-[var(--color-background-card)] overflow-hidden flex flex-col">
          {!activeUrl ? (
            <div className="flex-1 overflow-y-auto p-6 flex flex-col items-center justify-center">
              <div className="max-w-2xl w-full space-y-6">
                <div className="text-center space-y-2">
                  <div className="w-12 h-12 rounded-2xl bg-[var(--color-accent-muted)] text-[var(--color-icon-accent)] flex items-center justify-center mx-auto">
                    <Compass className="w-6 h-6" />
                  </div>
                  <h2 className="text-lg font-bold text-[var(--color-text-primary)]">
                    DNS-Resilient In-App Browser
                  </h2>
                  <p className="text-sm text-[var(--color-text-secondary)]">
                    Browse pages through the built-in Cloudflare/Google DoH fallback proxy, capture
                    HLS/MP4/MPD video streams live, preview in-app, or batch-download into groups.
                  </p>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                  {QUICK_BOOKMARKS.map((site) => (
                    <button
                      key={site.url}
                      type="button"
                      onClick={() => navigateTo(site.url)}
                      className="p-3.5 rounded-xl border border-[var(--color-border)] bg-[var(--color-background-surface)] hover:border-[var(--color-accent)] hover:bg-[var(--color-accent-muted)] text-left transition-colors space-y-1"
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-sm font-semibold text-[var(--color-text-primary)]">
                          {site.name}
                        </span>
                        <ExternalLink className="w-3.5 h-3.5 text-[var(--color-icon-secondary)]" />
                      </div>
                      <p className="text-xs text-[var(--color-text-secondary)] line-clamp-1">
                        {site.desc}
                      </p>
                      <p className="text-[11px] font-mono text-[var(--color-text-accent)] truncate">
                        {site.url}
                      </p>
                    </button>
                  ))}
                </div>
              </div>
            </div>
          ) : (
            <div className="relative flex-1 w-full h-full bg-[var(--color-background-surface)]">
              <iframe
                key={`${activeUrl}-${reloadNonce}`}
                src={`/api/browser/proxy?url=${encodeURIComponent(activeUrl)}`}
                title="KV Download In-App Browser"
                sandbox="allow-scripts allow-forms allow-popups allow-presentation"
                className="w-full h-full border-0 bg-white"
                onLoad={() => setIsLoadingPage(false)}
              />
            </div>
          )}
        </div>

        {/* Live Sniffer Drawer */}
        {isSnifferOpen && (
          <aside
            aria-label="Live Media Sniffer"
            className="w-full sm:w-[380px] lg:w-[420px] shrink-0 rounded-xl border border-[var(--color-border)] bg-[var(--color-background-card)] flex flex-col overflow-hidden absolute sm:static inset-0 z-20 shadow-lg sm:shadow-none"
          >
            <div className="px-3.5 py-2.5 border-b border-[var(--color-border)] bg-[var(--color-background-surface)] flex items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <Radar className="w-4 h-4 text-[var(--color-icon-accent)]" />
                <span className="text-sm font-bold text-[var(--color-text-primary)]">
                  Live Sniffer
                </span>
                <Badge label={`${sniffedItems.length}`} variant="purple" />
              </div>

              <div className="flex items-center gap-1">
                {activeUrl && (
                  <IconButton
                    label="Re-scan current page"
                    size="sm"
                    variant="ghost"
                    isLoading={isSniffing}
                    icon={<RotateCw className="w-3.5 h-3.5" />}
                    onClick={() => runServerSniff(activeUrl)}
                  />
                )}
                <IconButton
                  label="Close sniffer"
                  size="sm"
                  variant="ghost"
                  icon={<X className="w-4 h-4" />}
                  onClick={() => setIsSnifferOpen(false)}
                />
              </div>
            </div>

            {/* Sniffer Controls */}
            <div className="px-3 py-2 border-b border-[var(--color-border)] space-y-2 bg-[var(--color-background-muted)]">
              <div className="overflow-x-auto scrollbar-none">
                <TabList
                  value={sniffFilter}
                  onChange={(val) => setSniffFilter(val as SniffFilter)}
                  size="sm"
                >
                  <Tab
                    value="video"
                    label={`Videos (${counts.video})`}
                    icon={<Video className="w-3.5 h-3.5" />}
                  />
                  <Tab
                    value="audio"
                    label={`Audio (${counts.audio})`}
                    icon={<Music className="w-3.5 h-3.5" />}
                  />
                  <Tab
                    value="image"
                    label={`Images (${counts.image})`}
                    icon={<ImageIcon className="w-3.5 h-3.5" />}
                  />
                  <Tab value="all" label={`All (${counts.all})`} />
                </TabList>
              </div>

              <div className="flex items-center justify-between gap-2">
                <Selector
                  label="Sniffer quality"
                  isLabelHidden
                  size="sm"
                  variant="secondary"
                  value={quality}
                  onChange={(val) => setQuality(val)}
                  options={QUALITY_OPTIONS}
                />

                {filteredSniffed.length > 0 && (
                  <Button
                    label={
                      filteredSniffed.every((it) => selectedUrls.has(it.url))
                        ? 'Clear'
                        : 'Select All'
                    }
                    size="sm"
                    variant="ghost"
                    onClick={toggleSelectVisible}
                  />
                )}
              </div>
            </div>

            {/* Sniffed Media List */}
            <div className="flex-1 overflow-y-auto p-3 space-y-2">
              {filteredSniffed.length === 0 ? (
                <div className="py-12 px-4 text-center space-y-2">
                  <Film className="w-8 h-8 text-[var(--color-icon-secondary)] mx-auto opacity-60" />
                  <p className="text-sm font-semibold text-[var(--color-text-primary)]">
                    {isSniffing ? 'Scanning page for streams...' : 'No media detected in this tab'}
                  </p>
                  <p className="text-xs text-[var(--color-text-secondary)]">
                    Play a video inside the browser or switch filter tabs (Videos / Audio / All).
                  </p>
                </div>
              ) : (
                filteredSniffed.map((item, idx) => {
                  const isSelected = selectedUrls.has(item.url);
                  return (
                    <div
                      key={`${item.id}-${idx}`}
                      className={cn(
                        'rounded-lg border p-2.5 space-y-2 transition-colors',
                        isSelected
                          ? 'bg-[var(--color-accent-muted)] border-[var(--color-accent)]'
                          : 'bg-[var(--color-background-surface)] border-[var(--color-border)]'
                      )}
                    >
                      <div className="flex items-start gap-2.5">
                        <button
                          type="button"
                          onClick={() => toggleSelectItem(item.url)}
                          aria-label={`Select ${item.title}`}
                          className={cn(
                            'mt-0.5 w-4 h-4 rounded border flex items-center justify-center shrink-0',
                            isSelected
                              ? 'bg-[var(--color-accent)] border-[var(--color-accent)] text-[var(--color-on-accent)]'
                              : 'border-[var(--color-border-emphasized)] bg-[var(--color-background-card)]'
                          )}
                        >
                          {isSelected && <Check className="w-3 h-3" />}
                        </button>

                        {item.thumbnail && (
                          <img
                            src={item.thumbnail}
                            alt=""
                            loading="lazy"
                            className="w-16 h-10 rounded object-cover border border-[var(--color-border)] shrink-0 bg-[var(--color-background-muted)]"
                            onError={(e) => {
                              (e.currentTarget as HTMLImageElement).style.display = 'none';
                            }}
                          />
                        )}

                        <div className="flex-1 min-w-0 space-y-1">
                          <p
                            className="text-xs font-semibold text-[var(--color-text-primary)] line-clamp-2"
                            title={item.title}
                          >
                            {item.title}
                          </p>
                          <div className="flex flex-wrap items-center gap-1">
                            {item.format && (
                              <Badge label={item.format.toUpperCase()} variant="neutral" />
                            )}
                            {item.resolution && (
                              <Badge label={item.resolution} variant="purple" />
                            )}
                            {item.duration && (
                              <span className="text-[11px] font-mono tabular-nums text-[var(--color-text-secondary)]">
                                {item.duration}
                              </span>
                            )}
                            {item.isStream && <Badge label="HLS/DASH" variant="info" />}
                          </div>
                        </div>
                      </div>

                      <div className="flex items-center justify-end gap-1.5">
                        {(item.type === 'video' || item.type === 'audio') && (
                          <Button
                            label="Preview"
                            size="sm"
                            variant="secondary"
                            icon={<Play className="w-3 h-3 fill-current" />}
                            onClick={() =>
                              playStream(item.url, item.title, {
                                thumbnail: item.thumbnail,
                                isAudio: item.type === 'audio',
                                channel: item.channel || getHostLabel(activeUrl),
                              })
                            }
                          />
                        )}
                        <Button
                          label="Download"
                          size="sm"
                          variant="primary"
                          isLoading={isEnqueuing}
                          icon={<Download className="w-3 h-3" />}
                          onClick={() => handleDownloadSingle(item)}
                        />
                      </div>
                    </div>
                  );
                })
              )}
            </div>

            {/* Batch Download Selected Footer */}
            {selectedUrls.size > 0 && (
              <div className="p-3 border-t border-[var(--color-border)] bg-[var(--color-background-surface)]">
                <Button
                  label={`Download Selected as Group (${selectedUrls.size})`}
                  size="sm"
                  variant="primary"
                  width="100%"
                  isLoading={isEnqueuing}
                  icon={<Download className="w-3.5 h-3.5" />}
                  onClick={handleDownloadSelectedGroup}
                />
              </div>
            )}
          </aside>
        )}
      </div>
    </div>
  );
}

function getHostLabel(rawUrl: string): string {
  try {
    return new URL(rawUrl).hostname.replace(/^www\./, '');
  } catch {
    return 'Browser';
  }
}
