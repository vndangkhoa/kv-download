'use client';

import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { SniffedMedia } from '@/types';
import {
  ChromiumActionPayload,
  enqueueItems,
  enqueueUrls,
  fetchBrowserSniff,
  fetchChromiumCookies,
  fetchChromiumStatus,
  getSavedCookies,
  sendChromiumAction,
  startChromiumSession,
  stopChromiumSession,
} from '@/lib/api';
import { usePlayer } from '@/context/PlayerContext';
import { Button } from '@astryxdesign/core/Button';
import { IconButton } from '@astryxdesign/core/IconButton';
import { TextInput } from '@astryxdesign/core/TextInput';
import { Selector } from '@astryxdesign/core/Selector';
import { TabList, Tab } from '@astryxdesign/core/TabList';
import { Badge } from '@astryxdesign/core/Badge';
import { Banner } from '@astryxdesign/core/Banner';
import {
  ArrowLeft,
  ArrowRight,
  Check,
  Chrome,
  Compass,
  Download,
  ExternalLink,
  Film,
  Globe,
  Home,
  Image as ImageIcon,
  Keyboard,
  ListChecks,
  Loader2,
  Maximize2,
  Minimize2,
  Music,
  Play,
  Power,
  Radar,
  RotateCw,
  Shield,
  Video,
  X,
} from 'lucide-react';
import { cn } from '@/lib/utils';

interface BrowserViewProps {
  onOpenAlbumModal?: (url: string) => void;
  onQueued?: () => void;
}

type SniffFilter = 'video' | 'audio' | 'image' | 'all';
type EngineMode = 'chromium' | 'proxy';

const QUALITY_OPTIONS = [
  { label: 'Highest Quality', value: 'highest' },
  { label: '1080p MP4', value: '1080p' },
  { label: '720p MP4', value: '720p' },
  { label: 'Audio (MP3)', value: 'audio' },
];

const QUICK_BOOKMARKS = [
  { name: 'Google', url: 'https://www.google.com', desc: 'Search & open any website in Chromium Core' },
  { name: 'YouTube', url: 'https://www.youtube.com', desc: 'Browse & download HD/4K videos & shorts' },
  { name: 'TikTok', url: 'https://www.tiktok.com', desc: 'Browse creator feeds & grab clean videos' },
  { name: 'AV01 Media', url: 'https://av01.media', desc: 'DNS-resilient proxy & HLS M3U8 streams' },
  { name: 'Tinhte.vn', url: 'https://tinhte.vn', desc: 'Tech news & embedded videos' },
  { name: 'SoundCloud', url: 'https://soundcloud.com', desc: 'Audio tracks & albums' },
];

function normalizeInputUrl(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) return '';
  if (trimmed.startsWith('http://') || trimmed.startsWith('https://')) return trimmed;
  if (
    /\s/.test(trimmed) ||
    (!trimmed.includes('.') && !trimmed.toLowerCase().startsWith('localhost'))
  ) {
    return `https://www.google.com/search?q=${encodeURIComponent(trimmed)}`;
  }
  return `https://${trimmed}`;
}

function getKeyModifiers(e: React.KeyboardEvent): number {
  let m = 0;
  if (e.altKey) m |= 1;
  if (e.ctrlKey) m |= 2;
  if (e.metaKey) m |= 4;
  if (e.shiftKey) m |= 8;
  return m;
}

export function BrowserView({ onOpenAlbumModal, onQueued }: BrowserViewProps) {
  const { playStream } = usePlayer();

  const [engineMode, setEngineMode] = useState<EngineMode>('chromium');
  const [chromiumAvailable, setChromiumAvailable] = useState<boolean>(true);
  const [chromiumRunning, setChromiumRunning] = useState<boolean>(false);
  const [chromiumTransport, setChromiumTransport] = useState<'ws' | 'sse' | null>(null);
  const [chromiumCanBack, setChromiumCanBack] = useState(false);
  const [chromiumCanForward, setChromiumCanForward] = useState(false);
  const [viewportSize, setViewportSize] = useState({ width: 1280, height: 800 });
  const [showMobileTypeBar, setShowMobileTypeBar] = useState(false);
  const [mobileTypeInput, setMobileTypeInput] = useState('');

  const [urlInput, setUrlInput] = useState('');
  const [activeUrl, setActiveUrl] = useState('');
  const [history, setHistory] = useState<string[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const [reloadNonce, setReloadNonce] = useState(0);

  const [isLoadingPage, setIsLoadingPage] = useState(false);
  const [isMaximized, setIsMaximized] = useState(false);
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
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const viewportContainerRef = useRef<HTMLDivElement | null>(null);
  const canvasStageRef = useRef<HTMLDivElement | null>(null);
  const lastRequestedSizeRef = useRef<{ width: number; height: number } | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const sseRef = useRef<EventSource | null>(null);
  const lastMoveSentRef = useRef<number>(0);
  const touchPosRef = useRef<{ x: number; y: number; startY: number; moved: boolean } | null>(null);
  const frameImgRef = useRef<HTMLImageElement | null>(null);
  const activeUrlRef = useRef<string>('');

  useEffect(() => {
    activeUrlRef.current = activeUrl;
  }, [activeUrl]);

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

      next.sort((a, b) => {
        const rank = (m: SniffedMedia) =>
          m.type === 'video' ? (m.isStream ? 0 : 1) : m.type === 'audio' ? 2 : 3;
        return rank(a) - rank(b);
      });
      return next;
    });
  }, []);

  const drawFrameToCanvas = useCallback((base64Jpeg: string, w?: number, h?: number) => {
    const canvas = canvasRef.current;
    if (!canvas || !base64Jpeg) return;
    if (w && h && (w !== canvas.width || h !== canvas.height)) {
      canvas.width = w;
      canvas.height = h;
      setViewportSize({ width: w, height: h });
    }
    if (!frameImgRef.current) {
      frameImgRef.current = new Image();
    }
    const img = frameImgRef.current;
    img.onload = () => {
      const ctx = canvasRef.current?.getContext('2d');
      if (ctx && canvasRef.current) {
        ctx.drawImage(img, 0, 0, canvasRef.current.width, canvasRef.current.height);
      }
    };
    img.src = `data:image/jpeg;base64,${base64Jpeg}`;
  }, []);

  const handleChromiumEvent = useCallback(
    (raw: any) => {
      if (!raw || typeof raw !== 'object') return;
      switch (raw.type) {
        case 'frame':
          setIsLoadingPage(false);
          setChromiumRunning(true);
          if (typeof raw.data === 'string') {
            drawFrameToCanvas(raw.data, raw.width, raw.height);
          }
          break;
        case 'status':
          if (typeof raw.available === 'boolean') setChromiumAvailable(raw.available);
          if (typeof raw.running === 'boolean') setChromiumRunning(raw.running);
          if (raw.width && raw.height) {
            setViewportSize({ width: raw.width, height: raw.height });
          }
          break;
        case 'nav':
          setIsLoadingPage(false);
          if (typeof raw.canGoBack === 'boolean') setChromiumCanBack(raw.canGoBack);
          if (typeof raw.canGoForward === 'boolean') setChromiumCanForward(raw.canGoForward);
          if (typeof raw.url === 'string' && raw.url.startsWith('http')) {
            const changed = raw.url !== activeUrlRef.current;
            setUrlInput(raw.url);
            setActiveUrl(raw.url);
            if (changed) {
              setSniffedItems([]);
              setSelectedUrls(new Set());
              seenUrlsRef.current.clear();
            }
          }
          break;
        case 'media_list':
          if (Array.isArray(raw.items)) {
            mergeSniffedItems(raw.items);
          }
          break;
        case 'media':
          if (raw.item) {
            mergeSniffedItems([raw.item]);
          }
          break;
        case 'error':
          setIsLoadingPage(false);
          if (typeof raw.error === 'string' && raw.error) {
            setStatusNotice(`Chromium Core: ${raw.error}`);
          }
          break;
      }
    },
    [drawFrameToCanvas, mergeSniffedItems]
  );

  const disconnectChromiumStream = useCallback(() => {
    if (wsRef.current) {
      try {
        wsRef.current.close();
      } catch (_) {}
      wsRef.current = null;
    }
    if (sseRef.current) {
      try {
        sseRef.current.close();
      } catch (_) {}
      sseRef.current = null;
    }
    setChromiumTransport(null);
  }, []);

  const measureStageSize = useCallback(() => {
    const el = canvasStageRef.current ?? viewportContainerRef.current;
    if (!el) return { width: 1280, height: 800 };
    const rect = el.getBoundingClientRect();
    if (rect.width < 200 || rect.height < 200) return { width: 1280, height: 800 };
    const width = Math.min(1920, Math.max(360, Math.round(rect.width)));
    const height = Math.min(1200, Math.max(320, Math.round(rect.height)));
    return { width, height };
  }, []);

  const buildStreamQuery = useCallback((startUrl?: string, width?: number, height?: number) => {
    const params = new URLSearchParams();
    if (startUrl) params.set('url', startUrl);
    if (width && width > 0) params.set('width', String(width));
    if (height && height > 0) params.set('height', String(height));
    const qs = params.toString();
    return qs ? `?${qs}` : '';
  }, []);

  const connectChromiumSSE = useCallback(
    (startUrl?: string, width?: number, height?: number) => {
      if (sseRef.current) {
        sseRef.current.close();
        sseRef.current = null;
      }
      const qs = buildStreamQuery(startUrl, width, height);
      const es = new EventSource(`/api/browser/chromium/events${qs}`);
      sseRef.current = es;
      setChromiumTransport('sse');

      es.onmessage = (event) => {
        try {
          handleChromiumEvent(JSON.parse(event.data));
        } catch (_) {}
      };
      es.onerror = () => {
        // EventSource automatically retries on transient network hiccups
      };
    },
    [buildStreamQuery, handleChromiumEvent]
  );

  const ensureChromiumConnected = useCallback(
    (startUrl?: string, width?: number, height?: number) => {
      if (typeof window === 'undefined') return;
      if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
        if (startUrl) {
          wsRef.current.send(JSON.stringify({ type: 'navigate', url: startUrl }));
        }
        return;
      }
      if (sseRef.current) {
        if (startUrl) {
          sendChromiumAction({ type: 'navigate', url: startUrl }).catch(() => {});
        }
        return;
      }

      const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const qs = buildStreamQuery(startUrl, width, height);
      let opened = false;
      try {
        const ws = new WebSocket(`${proto}//${window.location.host}/api/browser/chromium/ws${qs}`);
        wsRef.current = ws;

        ws.onopen = () => {
          opened = true;
          setChromiumTransport('ws');
        };
        ws.onmessage = (ev) => {
          try {
            handleChromiumEvent(JSON.parse(ev.data));
          } catch (_) {}
        };
        ws.onerror = () => {
          if (!opened) {
            wsRef.current = null;
            connectChromiumSSE(startUrl, width, height);
          }
        };
        ws.onclose = () => {
          wsRef.current = null;
          if (!opened) {
            connectChromiumSSE(startUrl, width, height);
          } else {
            setChromiumTransport(null);
          }
        };
      } catch {
        connectChromiumSSE(startUrl, width, height);
      }
    },
    [buildStreamQuery, connectChromiumSSE, handleChromiumEvent]
  );

  const dispatchChromiumAction = useCallback(async (action: ChromiumActionPayload) => {
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(action));
      return;
    }
    try {
      await sendChromiumAction(action);
    } catch (_) {}
  }, []);

  // Probe Chromium availability on mount without spinning up Chromium
  useEffect(() => {
    let cancelled = false;
    fetchChromiumStatus()
      .then((st) => {
        if (cancelled) return;
        setChromiumAvailable(st.available);
        setChromiumRunning(st.running);
        setChromiumCanBack(st.canGoBack);
        setChromiumCanForward(st.canGoForward);
        if (!st.available) {
          setEngineMode('proxy');
        } else if (st.running && st.currentUrl) {
          setActiveUrl(st.currentUrl);
          setUrlInput(st.currentUrl);
          const { width, height } = measureStageSize();
          setViewportSize({ width, height });
          lastRequestedSizeRef.current = { width, height };
          ensureChromiumConnected(undefined, width, height);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setChromiumAvailable(false);
          setEngineMode('proxy');
        }
      });

    return () => {
      cancelled = true;
      disconnectChromiumStream();
    };
  }, [disconnectChromiumStream, ensureChromiumConnected, measureStageSize]);

  // Keep Chromium viewport aligned 1:1 with actual inner stage dimensions
  useEffect(() => {
    if (engineMode !== 'chromium' || !activeUrl || !chromiumRunning) return;
    const el = canvasStageRef.current ?? viewportContainerRef.current;
    if (!el || typeof ResizeObserver === 'undefined') return;

    let timer: ReturnType<typeof setTimeout> | null = null;
    const syncSize = () => {
      const rect = el.getBoundingClientRect();
      if (!rect || rect.width < 200 || rect.height < 200) return;
      const targetW = Math.min(1920, Math.max(360, Math.round(rect.width)));
      const targetH = Math.min(1200, Math.max(320, Math.round(rect.height)));
      const prev = lastRequestedSizeRef.current;
      if (prev && Math.abs(prev.width - targetW) < 4 && Math.abs(prev.height - targetH) < 4) {
        return;
      }
      lastRequestedSizeRef.current = { width: targetW, height: targetH };
      dispatchChromiumAction({ type: 'resize', width: targetW, height: targetH });
    };

    const immediateTimer = setTimeout(syncSize, 40);
    const ro = new ResizeObserver(() => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(syncSize, 180);
    });
    ro.observe(el);
    return () => {
      clearTimeout(immediateTimer);
      if (timer) clearTimeout(timer);
      ro.disconnect();
    };
  }, [activeUrl, chromiumRunning, dispatchChromiumAction, engineMode, isMaximized, isSnifferOpen]);

  // Allow Escape to exit maximized browser mode when focus isn't inside page canvas
  useEffect(() => {
    if (!isMaximized) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && document.activeElement !== canvasRef.current) {
        setIsMaximized(false);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [isMaximized]);

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

      if (engineMode === 'chromium' && chromiumAvailable) {
        if (wsRef.current?.readyState === WebSocket.OPEN || sseRef.current) {
          dispatchChromiumAction({ type: 'navigate', url: target });
        } else {
          const { width, height } = measureStageSize();
          setViewportSize({ width, height });
          lastRequestedSizeRef.current = { width, height };
          startChromiumSession(target, width, height)
            .then((res) => {
              if (!res.ok) {
                setStatusNotice(
                  res.error
                    ? `Chromium Core unavailable (${res.error}); using Fast Proxy.`
                    : 'Chromium Core unavailable; using Fast Proxy.'
                );
                setEngineMode('proxy');
                runServerSniff(target);
                return;
              }
              setChromiumRunning(true);
              ensureChromiumConnected(target, width, height);
            })
            .catch(() => {
              setEngineMode('proxy');
              runServerSniff(target);
            });
        }
        return;
      }

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
    [
      chromiumAvailable,
      dispatchChromiumAction,
      engineMode,
      ensureChromiumConnected,
      historyIndex,
      measureStageSize,
      runServerSniff,
    ]
  );

  const handleBack = () => {
    if (engineMode === 'chromium') {
      dispatchChromiumAction({ type: 'back' });
      return;
    }
    if (historyIndex <= 0) return;
    const prevIdx = historyIndex - 1;
    const target = history[prevIdx];
    setHistoryIndex(prevIdx);
    navigateTo(target, false);
  };

  const handleForward = () => {
    if (engineMode === 'chromium') {
      dispatchChromiumAction({ type: 'forward' });
      return;
    }
    if (historyIndex >= history.length - 1) return;
    const nextIdx = historyIndex + 1;
    const target = history[nextIdx];
    setHistoryIndex(nextIdx);
    navigateTo(target, false);
  };

  const handleHome = () => {
    disconnectChromiumStream();
    setActiveUrl('');
    setUrlInput('');
    setSniffedItems([]);
    setSelectedUrls(new Set());
    setIsSnifferOpen(false);
    setStatusNotice(null);
  };

  const handleStopChromium = async () => {
    disconnectChromiumStream();
    try {
      await stopChromiumSession();
    } catch (_) {}
    setChromiumRunning(false);
    setActiveUrl('');
    setUrlInput('');
    setStatusNotice('Chromium Core stopped to free NAS CPU & RAM.');
  };

  const handleReload = () => {
    if (!activeUrl) return;
    setIsLoadingPage(true);
    if (engineMode === 'chromium') {
      dispatchChromiumAction({ type: 'reload' });
      return;
    }
    setReloadNonce((n) => n + 1);
    runServerSniff(activeUrl);
  };

  const handleSwitchEngine = (nextMode: EngineMode) => {
    if (nextMode === engineMode) return;
    if (nextMode === 'proxy') {
      disconnectChromiumStream();
      setEngineMode('proxy');
      if (activeUrl) {
        setIsLoadingPage(true);
        setReloadNonce((n) => n + 1);
        runServerSniff(activeUrl);
      }
    } else if (chromiumAvailable) {
      setEngineMode('chromium');
      if (activeUrl) {
        setIsLoadingPage(true);
        const { width, height } = measureStageSize();
        setViewportSize({ width, height });
        lastRequestedSizeRef.current = { width, height };
        startChromiumSession(activeUrl, width, height)
          .then((res) => {
            if (res.ok) {
              setChromiumRunning(true);
              ensureChromiumConnected(activeUrl, width, height);
            } else {
              setEngineMode('proxy');
            }
          })
          .catch(() => setEngineMode('proxy'));
      }
    }
  };

  // Listen to proxy iframe messages for link navigation & live DOM/fetch/XHR media capture
  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      if (engineMode !== 'proxy') return;
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
  }, [engineMode, mergeSniffedItems, runServerSniff]);

  const toCanvasCoords = (clientX: number, clientY: number) => {
    const canvas = canvasRef.current;
    if (!canvas) return { x: 0, y: 0 };
    const rect = canvas.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return { x: 0, y: 0 };
    const x = Math.round(((clientX - rect.left) / rect.width) * viewportSize.width);
    const y = Math.round(((clientY - rect.top) / rect.height) * viewportSize.height);
    return {
      x: Math.max(0, Math.min(viewportSize.width, x)),
      y: Math.max(0, Math.min(viewportSize.height, y)),
    };
  };

  const handleCanvasMouseDown = (e: React.MouseEvent<HTMLCanvasElement>) => {
    e.currentTarget.focus();
    const { x, y } = toCanvasCoords(e.clientX, e.clientY);
    const button = e.button === 2 ? 'right' : e.button === 1 ? 'middle' : 'left';
    dispatchChromiumAction({ type: 'mousedown', x, y, button, clickCount: 1 });
  };

  const handleCanvasMouseUp = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const { x, y } = toCanvasCoords(e.clientX, e.clientY);
    const button = e.button === 2 ? 'right' : e.button === 1 ? 'middle' : 'left';
    dispatchChromiumAction({ type: 'mouseup', x, y, button, clickCount: 1 });
  };

  const handleCanvasMouseMove = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const now = performance.now();
    if (now - lastMoveSentRef.current < 45) return;
    lastMoveSentRef.current = now;
    const { x, y } = toCanvasCoords(e.clientX, e.clientY);
    dispatchChromiumAction({ type: 'mousemove', x, y });
  };

  const handleCanvasWheel = (e: React.WheelEvent<HTMLCanvasElement>) => {
    e.preventDefault();
    const { x, y } = toCanvasCoords(e.clientX, e.clientY);
    dispatchChromiumAction({
      type: 'wheel',
      x,
      y,
      deltaX: Math.round(e.deltaX),
      deltaY: Math.round(e.deltaY),
    });
  };

  const handleCanvasKeyDown = (e: React.KeyboardEvent<HTMLCanvasElement>) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'v') {
      return; // Handled via onPaste so clipboard inserts cleanly
    }
    if (
      [' ', 'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Backspace', 'Tab', 'Enter'].includes(
        e.key
      )
    ) {
      e.preventDefault();
    }
    const modifiers = getKeyModifiers(e);
    dispatchChromiumAction({
      type: 'keydown',
      key: e.key,
      code: e.code,
      keyCode: e.keyCode,
      modifiers,
    });
  };

  const handleCanvasKeyUp = (e: React.KeyboardEvent<HTMLCanvasElement>) => {
    dispatchChromiumAction({
      type: 'keyup',
      key: e.key,
      code: e.code,
      keyCode: e.keyCode,
      modifiers: getKeyModifiers(e),
    });
  };

  const handleCanvasPaste = (e: React.ClipboardEvent<HTMLCanvasElement>) => {
    const pasted = e.clipboardData?.getData('text');
    if (pasted) {
      e.preventDefault();
      dispatchChromiumAction({ type: 'text', text: pasted });
    }
  };

  const handleCanvasTouchStart = (e: React.TouchEvent<HTMLCanvasElement>) => {
    const t = e.touches[0];
    if (!t) return;
    const { x, y } = toCanvasCoords(t.clientX, t.clientY);
    touchPosRef.current = { x, y, startY: y, moved: false };
  };

  const handleCanvasTouchMove = (e: React.TouchEvent<HTMLCanvasElement>) => {
    const t = e.touches[0];
    const prev = touchPosRef.current;
    if (!t || !prev) return;
    const { x, y } = toCanvasCoords(t.clientX, t.clientY);
    const dy = prev.y - y;
    if (Math.abs(y - prev.startY) > 10) {
      prev.moved = true;
    }
    if (Math.abs(dy) >= 6) {
      dispatchChromiumAction({ type: 'wheel', x, y, deltaX: 0, deltaY: dy * 2 });
      prev.x = x;
      prev.y = y;
    }
  };

  const handleCanvasTouchEnd = () => {
    const prev = touchPosRef.current;
    touchPosRef.current = null;
    if (prev && !prev.moved) {
      dispatchChromiumAction({ type: 'click', x: prev.x, y: prev.y, button: 'left', clickCount: 1 });
    }
  };

  const resolveEffectiveCookies = async (): Promise<string | undefined> => {
    const saved = getSavedCookies();
    if (saved) return saved;
    if (engineMode === 'chromium' && chromiumRunning) {
      const liveCookies = await fetchChromiumCookies();
      if (liveCookies) return liveCookies;
    }
    return undefined;
  };

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
      const cookies = await resolveEffectiveCookies();
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
          cookies,
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
      const cookies = await resolveEffectiveCookies();
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
          cookies,
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
      const cookies = await resolveEffectiveCookies();
      await enqueueUrls([activeUrl], quality, {
        channel: getHostLabel(activeUrl),
        cookies,
      });
      setStatusNotice(`Sent ${activeUrl} to download queue.`);
      onQueued?.();
    } finally {
      setIsEnqueuing(false);
    }
  };

  const canGoBack = engineMode === 'chromium' ? chromiumCanBack : historyIndex > 0;
  const canGoForward =
    engineMode === 'chromium' ? chromiumCanForward : historyIndex < history.length - 1;

  const engineStatusTitle =
    engineMode === 'chromium'
      ? `Chromium Core ${chromiumTransport ? `(${chromiumTransport.toUpperCase()})` : ''} · ${chromiumRunning ? 'Active — click canvas to interact · auto-stops when idle' : 'On-demand · launches when navigating'}`
      : 'Fast Proxy Mode · DNS-resilient server proxy';

  return (
    <div
      className={cn(
        'flex flex-col gap-1.5 min-h-0',
        isMaximized
          ? 'fixed inset-0 z-40 bg-[var(--color-background-body)] p-1.5 sm:p-2'
          : 'w-full h-full flex-1'
      )}
    >
      {/* Unified Slim Browser Chrome Bar */}
      <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-background-surface)] px-2 py-1.5 flex items-center gap-1.5 flex-wrap lg:flex-nowrap shrink-0 shadow-xs">
        <div className="flex items-center gap-0.5 shrink-0">
          <IconButton
            label="Back"
            size="sm"
            variant="ghost"
            isDisabled={!canGoBack}
            icon={<ArrowLeft className="w-4 h-4" />}
            onClick={handleBack}
          />
          <IconButton
            label="Forward"
            size="sm"
            variant="ghost"
            isDisabled={!canGoForward}
            icon={<ArrowRight className="w-4 h-4" />}
            onClick={handleForward}
          />
          <IconButton
            label="Reload page"
            size="sm"
            variant="ghost"
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
          <IconButton
            label="Home bookmarks"
            size="sm"
            variant="ghost"
            icon={<Home className="w-4 h-4" />}
            onClick={handleHome}
          />
        </div>

        <div className="flex-1 min-w-[200px] flex items-center gap-1.5">
          <span
            title={engineStatusTitle}
            aria-label={engineStatusTitle}
            className="inline-flex items-center justify-center w-6 h-7 rounded-md bg-[var(--color-background-muted)] border border-[var(--color-border)] shrink-0"
          >
            <span
              className={cn(
                'w-2 h-2 rounded-full',
                engineMode === 'proxy'
                  ? 'bg-[var(--color-accent)]'
                  : chromiumRunning
                    ? 'bg-[var(--color-text-success)]'
                    : 'bg-amber-500'
              )}
            />
          </span>
          <div className="flex-1 min-w-0">
            <TextInput
              label="Web URL or Search"
              isLabelHidden
              type="text"
              size="sm"
              width="100%"
              startIcon={Globe}
              value={urlInput}
              onChange={(val) => setUrlInput(val)}
              onEnter={() => navigateTo(urlInput)}
              enterKeyHint="go"
              hasClear
              placeholder="Search Google or enter URL (youtube.com, tiktok.com, av01.media)..."
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

        <div className="flex items-center gap-1 shrink-0 flex-wrap">
          {chromiumAvailable && (
            <Button
              label={engineMode === 'chromium' ? 'Chromium' : 'Proxy'}
              size="sm"
              variant="secondary"
              icon={
                engineMode === 'chromium' ? (
                  <Chrome className="w-3.5 h-3.5" />
                ) : (
                  <Shield className="w-3.5 h-3.5" />
                )
              }
              onClick={() =>
                handleSwitchEngine(engineMode === 'chromium' ? 'proxy' : 'chromium')
              }
            />
          )}

          {activeUrl && (
            <Button
              label="Download"
              size="sm"
              variant="primary"
              isLoading={isEnqueuing}
              icon={<Download className="w-3.5 h-3.5" />}
              onClick={handleDownloadCurrentPage}
            />
          )}

          <Button
            label={`Sniffer (${sniffedItems.length})`}
            size="sm"
            variant={isSnifferOpen ? 'primary' : 'secondary'}
            isLoading={isSniffing}
            icon={<Radar className="w-3.5 h-3.5" />}
            onClick={() => {
              const nextOpen = !isSnifferOpen;
              setIsSnifferOpen(nextOpen);
              if (nextOpen && engineMode === 'chromium' && activeUrl) {
                dispatchChromiumAction({ type: 'extract' });
              }
            }}
          />

          {activeUrl && onOpenAlbumModal && (
            <IconButton
              label="Scan page or playlist"
              tooltip="Scan page / playlist for batch download"
              size="sm"
              variant="secondary"
              icon={<ListChecks className="w-3.5 h-3.5" />}
              onClick={() => onOpenAlbumModal(activeUrl)}
            />
          )}

          {engineMode === 'chromium' && activeUrl && (
            <IconButton
              label="Toggle virtual text input for mobile"
              tooltip="Virtual text input bar"
              size="sm"
              variant={showMobileTypeBar ? 'primary' : 'secondary'}
              icon={<Keyboard className="w-3.5 h-3.5" />}
              onClick={() => setShowMobileTypeBar((v) => !v)}
            />
          )}

          <IconButton
            label={isMaximized ? 'Restore browser view' : 'Maximize browser view'}
            tooltip={isMaximized ? 'Restore view (Esc)' : 'Maximize browser view'}
            size="sm"
            variant={isMaximized ? 'primary' : 'ghost'}
            icon={
              isMaximized ? (
                <Minimize2 className="w-4 h-4" />
              ) : (
                <Maximize2 className="w-4 h-4" />
              )
            }
            onClick={() => setIsMaximized((m) => !m)}
          />

          {engineMode === 'chromium' && chromiumRunning && (
            <IconButton
              label="Stop Chromium engine"
              tooltip="Stop Chromium to free NAS CPU & RAM"
              size="sm"
              variant="ghost"
              icon={<Power className="w-3.5 h-3.5 text-[var(--color-text-error)]" />}
              onClick={handleStopChromium}
            />
          )}
        </div>
      </div>

      {statusNotice && (
        <Banner
          status="info"
          title={statusNotice}
          isDismissable
          onDismiss={() => setStatusNotice(null)}
        />
      )}

      {/* Workspace Split: Edge-to-Edge Viewport + Collapsible Sniffer Panel */}
      <div className="flex-1 min-h-0 flex gap-2 relative overflow-hidden">
        <div
          ref={viewportContainerRef}
          className="flex-1 min-w-0 min-h-0 rounded-xl border border-[var(--color-border)] bg-[var(--color-background-surface)] overflow-hidden flex flex-col"
        >
          {!activeUrl ? (
            <div className="flex-1 overflow-y-auto p-6 flex flex-col items-center justify-center">
              <div className="max-w-2xl w-full space-y-6">
                <div className="text-center space-y-2">
                  <div className="w-12 h-12 rounded-2xl bg-[var(--color-accent-muted)] text-[var(--color-icon-accent)] flex items-center justify-center mx-auto">
                    {chromiumAvailable ? (
                      <Chrome className="w-6 h-6" />
                    ) : (
                      <Compass className="w-6 h-6" />
                    )}
                  </div>
                  <h2 className="text-lg font-bold text-[var(--color-text-primary)]">
                    {chromiumAvailable
                      ? 'Embedded Chromium Core Browser'
                      : 'DNS-Resilient In-App Browser'}
                  </h2>
                  <p className="text-sm text-[var(--color-text-secondary)]">
                    {chromiumAvailable
                      ? 'On-demand headless Chromium 142 with live interactive 1:1 viewport rendering & network stream capture. Surf Google, YouTube, TikTok & AV01 directly, or tap Download anytime.'
                      : 'Browse pages through the built-in Cloudflare/Google DoH fallback proxy, capture HLS/MP4/MPD video streams live, preview in-app, or batch-download into groups.'}
                  </p>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                  {QUICK_BOOKMARKS.map((site) => (
                    <button
                      key={site.url}
                      type="button"
                      onClick={() => navigateTo(site.url)}
                      className="p-3.5 rounded-xl border border-[var(--color-border)] bg-[var(--color-background-card)] hover:border-[var(--color-accent)] hover:bg-[var(--color-accent-muted)] text-left transition-colors space-y-1"
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

                {chromiumAvailable && (
                  <p className="text-center text-xs text-[var(--color-text-secondary)]">
                    Low-CPU mode active · Chromium stays off at 0% idle until you open a site and shuts down automatically when idle.
                  </p>
                )}
              </div>
            </div>
          ) : engineMode === 'chromium' ? (
            <div className="relative flex-1 min-h-0 w-full h-full flex flex-col overflow-hidden">
              <div
                ref={canvasStageRef}
                className="relative flex-1 min-h-0 w-full h-full bg-[var(--color-background-surface)] overflow-hidden"
              >
                <canvas
                  ref={canvasRef}
                  width={viewportSize.width}
                  height={viewportSize.height}
                  tabIndex={0}
                  aria-label="Interactive Chromium Browser Viewport"
                  onMouseDown={handleCanvasMouseDown}
                  onMouseUp={handleCanvasMouseUp}
                  onMouseMove={handleCanvasMouseMove}
                  onWheel={handleCanvasWheel}
                  onKeyDown={handleCanvasKeyDown}
                  onKeyUp={handleCanvasKeyUp}
                  onPaste={handleCanvasPaste}
                  onTouchStart={handleCanvasTouchStart}
                  onTouchMove={handleCanvasTouchMove}
                  onTouchEnd={handleCanvasTouchEnd}
                  onContextMenu={(e) => e.preventDefault()}
                  className="block w-full h-full outline-none cursor-default focus:ring-1 focus:ring-inset focus:ring-[var(--color-accent)]"
                />
                {isLoadingPage && (
                  <div className="absolute top-2.5 right-2.5 px-2.5 py-1 rounded-full bg-black/75 text-white text-xs flex items-center gap-1.5 pointer-events-none">
                    <Loader2 className="w-3.5 h-3.5 animate-spin" />
                    Rendering...
                  </div>
                )}
              </div>

              {showMobileTypeBar && (
                <div className="px-3 py-2 border-t border-[var(--color-border)] bg-[var(--color-background-surface)] flex items-center gap-2 shrink-0">
                  <div className="flex-1 min-w-0">
                    <TextInput
                      label="Send text to page"
                      isLabelHidden
                      size="sm"
                      width="100%"
                      value={mobileTypeInput}
                      onChange={(v) => setMobileTypeInput(v)}
                      onEnter={async () => {
                        if (mobileTypeInput) {
                          await dispatchChromiumAction({ type: 'text', text: mobileTypeInput });
                          setMobileTypeInput('');
                        }
                        await dispatchChromiumAction({
                          type: 'keydown',
                          key: 'Enter',
                          code: 'Enter',
                          keyCode: 13,
                        });
                      }}
                      placeholder="Type or paste text into focused page input..."
                    />
                  </div>
                  <Button
                    label="Send"
                    size="sm"
                    variant="primary"
                    onClick={async () => {
                      if (!mobileTypeInput) return;
                      await dispatchChromiumAction({ type: 'text', text: mobileTypeInput });
                      setMobileTypeInput('');
                    }}
                  />
                  <Button
                    label="Enter"
                    size="sm"
                    variant="secondary"
                    onClick={() =>
                      dispatchChromiumAction({
                        type: 'keydown',
                        key: 'Enter',
                        code: 'Enter',
                        keyCode: 13,
                      })
                    }
                  />
                </div>
              )}
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
                    onClick={() => {
                      if (engineMode === 'chromium') {
                        dispatchChromiumAction({ type: 'extract' });
                      }
                      runServerSniff(activeUrl);
                    }}
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
                    Play a video inside the browser, open a YouTube/TikTok/AV01 video page, or tap
                    Detect Streams.
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
                        {(item.type === 'video' || item.type === 'audio') &&
                          !/youtube\.com\/watch|youtu\.be\//i.test(item.url) && (
                            <Button
                              label="Preview"
                              size="sm"
                              variant="secondary"
                              icon={<Play className="w-3 h-3 fill-current" />}
                              onClick={() =>
                                playStream(item.url, item.title, {
                                  thumbnail: item.thumbnail,
                                  isAudio: item.type === 'audio',
                                  isHls: Boolean(item.isStream || /\.m3u8(\?|$)/i.test(item.url)),
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
