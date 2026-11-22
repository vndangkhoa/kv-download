import {
  DownloadTask,
  EnqueueItemInput,
  GalleryResponse,
  OrganizeMode,
  SniffedMedia,
  StorageSettings,
  YtDlpVersionInfo,
} from '@/types';

const BASE_API = '';
export const COOKIES_STORAGE_KEY = 'kv_user_cookies';
export const SPEED_LIMIT_STORAGE_KEY = 'kv_speed_limit';

export function getSavedCookies(): string {
  if (typeof window === 'undefined') return '';
  try {
    return localStorage.getItem(COOKIES_STORAGE_KEY) || '';
  } catch {
    return '';
  }
}

export function setSavedCookies(content: string): void {
  if (typeof window === 'undefined') return;
  try {
    const trimmed = content.trim();
    if (trimmed) {
      localStorage.setItem(COOKIES_STORAGE_KEY, trimmed);
    } else {
      localStorage.removeItem(COOKIES_STORAGE_KEY);
    }
  } catch {
    // Ignore storage failures.
  }
}

export function getSavedRateLimit(): string {
  if (typeof window === 'undefined') return '';
  try {
    return localStorage.getItem(SPEED_LIMIT_STORAGE_KEY) || '';
  } catch {
    return '';
  }
}

export async function fetchQueue(): Promise<DownloadTask[]> {
  const res = await fetch(`${BASE_API}/api/queue`);
  if (!res.ok) throw new Error('Failed to fetch queue');
  const data = await res.json();
  return Array.isArray(data) ? data : data.tasks || [];
}

export async function enqueueUrls(
  urls: string[],
  format: string = 'highest',
  meta?: {
    title?: string;
    channel?: string;
    groupId?: string;
    groupTitle?: string;
    groupType?: string;
    cookies?: string;
    rateLimit?: string;
  }
): Promise<{ success: boolean; tasks?: DownloadTask[]; error?: string }> {
  const cookies = meta?.cookies ?? getSavedCookies();
  const rateLimit = meta?.rateLimit ?? getSavedRateLimit();
  const res = await fetch(`${BASE_API}/api/queue/add`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      urls,
      format,
      playlistTitle: meta?.title,
      channel: meta?.channel,
      groupId: meta?.groupId,
      groupTitle: meta?.groupTitle,
      groupType: meta?.groupType,
      ...(cookies ? { cookies } : {}),
      ...(rateLimit ? { rateLimit } : {}),
    }),
  });
  return res.json();
}

export async function enqueueItems(
  items: EnqueueItemInput[],
  format: string = 'highest',
  opts?: {
    groupId?: string;
    groupTitle?: string;
    groupType?: string;
    playlistTitle?: string;
    channel?: string;
    cookies?: string;
    rateLimit?: string;
  }
): Promise<{ success: boolean; tasks?: DownloadTask[]; error?: string }> {
  const cookies = opts?.cookies ?? getSavedCookies();
  const rateLimit = opts?.rateLimit ?? getSavedRateLimit();
  const res = await fetch(`${BASE_API}/api/queue/add`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      items,
      format,
      groupId: opts?.groupId,
      groupTitle: opts?.groupTitle,
      groupType: opts?.groupType,
      playlistTitle: opts?.playlistTitle || opts?.groupTitle,
      channel: opts?.channel,
      ...(cookies ? { cookies } : {}),
      ...(rateLimit ? { rateLimit } : {}),
    }),
  });
  return res.json();
}

export async function cancelTask(id: string): Promise<void> {
  await fetch(`${BASE_API}/api/queue/cancel?id=${encodeURIComponent(id)}`, { method: 'POST' });
}

export async function retryTask(id: string): Promise<void> {
  await fetch(`${BASE_API}/api/queue/retry?id=${encodeURIComponent(id)}`, { method: 'POST' });
}

export async function deleteTask(id: string, deleteFile: boolean = false): Promise<void> {
  await fetch(
    `${BASE_API}/api/queue/item?id=${encodeURIComponent(id)}&deleteFile=${deleteFile}`,
    { method: 'DELETE' }
  );
}

export async function cancelTaskGroup(groupId: string): Promise<void> {
  await fetch(`${BASE_API}/api/queue/group/cancel?groupId=${encodeURIComponent(groupId)}`, {
    method: 'POST',
  });
}

export async function retryTaskGroup(groupId: string): Promise<void> {
  await fetch(`${BASE_API}/api/queue/group/retry?groupId=${encodeURIComponent(groupId)}`, {
    method: 'POST',
  });
}

export async function deleteTaskGroup(groupId: string, deleteFiles: boolean = false): Promise<void> {
  await fetch(
    `${BASE_API}/api/queue/group?groupId=${encodeURIComponent(groupId)}&deleteFiles=${deleteFiles}`,
    { method: 'DELETE' }
  );
}

export function getGroupZipUrl(groupId: string): string {
  return `${BASE_API}/download/zip?groupId=${encodeURIComponent(groupId)}`;
}

export function getFolderZipUrl(folder: string): string {
  return `${BASE_API}/download/zip?folder=${encodeURIComponent(folder)}`;
}

export async function clearCompletedTasks(): Promise<void> {
  await fetch(`${BASE_API}/api/queue/clear-completed`, { method: 'POST' });
}

export async function retryFailedTasks(): Promise<void> {
  await fetch(`${BASE_API}/api/queue/retry-failed`, { method: 'POST' });
}

export async function fetchGallery(params?: {
  sort?: string;
  category?: string;
  folder?: string;
  search?: string;
}): Promise<GalleryResponse> {
  const sp = new URLSearchParams();
  if (params?.sort) sp.set('sort', params.sort);
  if (params?.category && params.category !== 'all') sp.set('category', params.category);
  if (params?.folder) sp.set('folder', params.folder);
  if (params?.search) sp.set('search', params.search);
  const qs = sp.toString();
  const res = await fetch(`${BASE_API}/api/gallery${qs ? `?${qs}` : ''}`);
  if (!res.ok) throw new Error('Failed to load gallery');
  return res.json();
}

export async function organizeGalleryLibrary(): Promise<{
  success?: boolean;
  movedFiles?: number;
  totalItems?: number;
  organizeMode?: string;
}> {
  const res = await fetch(`${BASE_API}/api/gallery/organize`, {
    method: 'POST',
  });
  if (!res.ok) throw new Error('Failed to organize gallery library');
  return res.json();
}

export async function deleteGalleryFile(path: string): Promise<void> {
  const res = await fetch(`${BASE_API}/api/gallery/file?path=${encodeURIComponent(path)}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error('Failed to delete file');
}

export async function deleteMultipleGalleryFiles(
  paths: string[]
): Promise<{ deletedCount: number }> {
  const res = await fetch(`${BASE_API}/api/gallery/delete-multiple`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ paths }),
  });
  if (!res.ok) throw new Error('Failed to delete files');
  return res.json();
}

export async function fetchStorageSettings(): Promise<StorageSettings> {
  const res = await fetch(`${BASE_API}/api/settings/storage`);
  if (!res.ok) throw new Error('Failed to load settings');
  const data = await res.json();
  const resolvedDir = data.downloadDir || data.path || '';
  return {
    downloadDir: resolvedDir,
    path: resolvedDir,
    organizeMode: (data.organizeMode || 'category_channel') as OrganizeMode,
  };
}

export async function updateStorageSettings(
  downloadDir: string,
  organizeMode?: OrganizeMode
): Promise<StorageSettings> {
  const res = await fetch(`${BASE_API}/api/settings/storage`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      downloadDir,
      path: downloadDir,
      ...(organizeMode ? { organizeMode } : {}),
    }),
  });
  if (!res.ok) throw new Error('Failed to save settings');
  const data = await res.json();
  const resolvedDir = data.downloadDir || data.path || downloadDir;
  return {
    downloadDir: resolvedDir,
    path: resolvedDir,
    organizeMode: (data.organizeMode || organizeMode || 'category_channel') as OrganizeMode,
  };
}

export async function fetchYtDlpVersion(): Promise<YtDlpVersionInfo> {
  const res = await fetch(`${BASE_API}/api/ytdlp/version`);
  if (!res.ok) throw new Error('Failed to get version');
  return res.json();
}

export async function updateYtDlpChannel(channel: string): Promise<{ message: string }> {
  const res = await fetch(
    `${BASE_API}/api/ytdlp/update?channel=${encodeURIComponent(channel)}`,
    {
      method: 'POST',
    }
  );
  if (!res.ok) throw new Error('Update failed');
  return res.json();
}

export async function fetchWorkers(): Promise<{ maxWorkers: number }> {
  const res = await fetch(`${BASE_API}/api/queue/workers`);
  if (!res.ok) throw new Error('Failed to get workers');
  return res.json();
}

export async function setWorkers(count: number): Promise<void> {
  const res = await fetch(`${BASE_API}/api/queue/workers?count=${count}`, {
    method: 'POST',
  });
  if (!res.ok) throw new Error('Failed to update workers');
}

export async function fetchBrowserSniff(url: string): Promise<SniffedMedia[]> {
  const cookies = getSavedCookies();
  const res = await fetch(`${BASE_API}/api/browser/sniff?url=${encodeURIComponent(url)}`, {
    headers: cookies ? { 'X-Cookies': cookies } : undefined,
  });
  if (!res.ok) throw new Error('Sniff request failed');
  const data = await res.json();
  return Array.isArray(data.items) ? data.items : [];
}

export async function scanMediaUrl(
  url: string,
  start: number = 1,
  limit: number = 48
): Promise<any> {
  const cookies = getSavedCookies();
  const res = await fetch(
    `${BASE_API}/api/scan?url=${encodeURIComponent(url)}&start=${start}&limit=${limit}`,
    {
      headers: cookies ? { 'X-Cookies': cookies } : undefined,
    }
  );
  if (!res.ok) throw new Error('Failed to scan URL');
  return res.json();
}

export interface ChromiumStatus {
  available: boolean;
  binaryPath?: string;
  running: boolean;
  currentUrl?: string;
  pageTitle?: string;
  canGoBack: boolean;
  canGoForward: boolean;
  width: number;
  height: number;
  clientCount: number;
  mediaCount: number;
}

export interface ChromiumActionPayload {
  type: string;
  url?: string;
  x?: number;
  y?: number;
  button?: string;
  clickCount?: number;
  deltaX?: number;
  deltaY?: number;
  key?: string;
  code?: string;
  keyCode?: number;
  text?: string;
  modifiers?: number;
  width?: number;
  height?: number;
}

export async function fetchChromiumStatus(): Promise<ChromiumStatus> {
  const res = await fetch(`${BASE_API}/api/browser/chromium/status`);
  if (!res.ok) throw new Error('Failed to query Chromium status');
  return res.json();
}

export async function startChromiumSession(
  url?: string,
  width?: number,
  height?: number
): Promise<{ ok: boolean; status?: ChromiumStatus; error?: string }> {
  const res = await fetch(`${BASE_API}/api/browser/chromium/session`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url, width, height }),
  });
  return res.json();
}

export async function stopChromiumSession(): Promise<void> {
  await fetch(`${BASE_API}/api/browser/chromium/session`, { method: 'DELETE' });
}

export async function sendChromiumAction(action: ChromiumActionPayload): Promise<void> {
  await fetch(`${BASE_API}/api/browser/chromium/action`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(action),
  });
}

export async function fetchChromiumCookies(): Promise<string> {
  try {
    const res = await fetch(`${BASE_API}/api/browser/chromium/cookies`);
    if (!res.ok) return '';
    const data = await res.json();
    return typeof data.cookies === 'string' ? data.cookies : '';
  } catch {
    return '';
  }
}
