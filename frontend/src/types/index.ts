export type TaskStatus = 'queued' | 'downloading' | 'completed' | 'failed' | 'cancelled';

export interface FormatChoice {
  label: string;
  value: string;
  desc?: string;
}

export interface DownloadTask {
  id: string;
  url: string;
  status: TaskStatus;
  percent: number;
  speed?: string;
  eta?: string;
  title?: string;
  mediaName?: string;
  channel?: string;
  uploader?: string;
  format?: string;
  humanSize?: string;
  totalBytes?: number;
  mediaId?: string;
  thumbnail?: string;
  errorMessage?: string;
  createdAt: string;
  playlistTitle?: string;
  infoJsonPath?: string;
  groupId?: string;
  groupTitle?: string;
  groupType?: string;
  platform?: string;
  duration?: string;
}

export interface TaskGroup {
  groupId: string;
  title: string;
  groupType: string;
  platform: string;
  tasks: DownloadTask[];
  completedCount: number;
  downloadingCount: number;
  queuedCount: number;
  failedCount: number;
  cancelledCount: number;
  totalBytes: number;
  overallPercent: number;
  latestCreatedAt: string;
}

export type MediaType = 'video' | 'audio' | 'photo' | 'other';

export interface GalleryItem {
  id: string;
  name: string;
  path: string;
  title?: string;
  channel?: string;
  group?: string;
  platform?: string;
  duration?: string;
  category?: string;
  humanSize: string;
  sizeInBytes: number;
  modTime: string;
  type: MediaType;
  thumbnail?: string;
  url?: string;
}

export interface GalleryFolder {
  name: string;
  fileCount: number;
  category?: string;
  platform?: string;
  subfolders?: string[];
  totalBytes?: number;
  humanSize?: string;
  thumbnail?: string;
  modTime?: string;
}

export interface GalleryResponse {
  items: GalleryItem[];
  folders: GalleryFolder[];
  totalItems: number;
  totalSize: string;
}

export type OrganizeMode =
  | 'category_channel'
  | 'channel_category'
  | 'platform_channel'
  | 'flat_category';

export type GallerySortMode =
  | 'newest'
  | 'oldest'
  | 'size_desc'
  | 'size_asc'
  | 'name_asc'
  | 'name_desc'
  | 'channel_asc';

export interface StorageSettings {
  downloadDir: string;
  path?: string;
  organizeMode?: OrganizeMode;
}

export interface YtDlpVersionInfo {
  version: string;
  latest?: string;
  channel?: string;
  updateAvailable?: boolean;
}

export interface AlbumEntry {
  id: string;
  url: string;
  title: string;
  duration?: string;
  thumbnail?: string;
  uploader?: string;
  channel?: string;
  infoJsonPath?: string;
  isPlaylist?: boolean;
  entries?: AlbumEntry[];
}

export interface SniffedMedia {
  id: string;
  url: string;
  type: 'video' | 'audio' | 'image';
  format?: string;
  title: string;
  size?: string;
  thumbnail?: string;
  source?: 'dom' | 'network' | 'yt-dlp';
  duration?: string;
  resolution?: string;
  channel?: string;
  isStream?: boolean;
}

export interface EnqueueItemInput {
  url: string;
  title?: string;
  thumbnail?: string;
  uploader?: string;
  channel?: string;
  duration?: string;
  infoJsonPath?: string;
}

export type ThemeMode = 'system' | 'dark' | 'light';
export type ViewMode = 'standard' | 'compact' | 'list';
export type ActiveTab = 'download-center' | 'browser' | 'history' | 'settings';
