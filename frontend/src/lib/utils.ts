import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatBytes(bytes?: number): string {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

export function formatTime(seconds?: number): string {
  if (!seconds || isNaN(seconds)) return '0:00';
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins}:${secs < 10 ? '0' : ''}${secs}`;
}

export function extractUrlsFromText(text?: string): string[] {
  if (!text) return [];
  const matches = text.match(/https?:\/\/[^\s<>"']+/gi) || [];
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of matches) {
    const cleaned = raw.replace(/[.,;:!?)\]}]+$/g, '').trim();
    if (cleaned && !seen.has(cleaned)) {
      seen.add(cleaned);
      out.push(cleaned);
    }
  }
  return out;
}

export interface PlatformInfo {
  name: string;
  icon: string;
  color: string;
  isPlaylist?: boolean;
}

export function isPlaylistUrl(url?: string): boolean {
  if (!url) return false;
  const u = url.toLowerCase().trim();

  // TikTok creator / channel profile (e.g. tiktok.com/@username without /video/ or /photo/)
  if (u.includes('tiktok.com/@') && !u.includes('/video/') && !u.includes('/photo/')) {
    return true;
  }

  // Facebook profile / page / group / videos / reels tab
  if (u.includes('facebook.com') || u.includes('fb.com') || u.includes('fb.watch')) {
    const isSingleVideo = /(?:\/videos\/|\/posts\/|\/reel\/|watch\?v=|video\.php\?v=)\d{6,}|\/share\/(?:r|v|p)\/[a-z0-9_-]+|fb\.watch\//i.test(u);
    if (!isSingleVideo) {
      const cleanPath = u.replace(/^https?:\/\/[^\/]+/, '').replace(/\/+$/, '');
      if (cleanPath && cleanPath !== '' && cleanPath !== '/') {
        return true;
      }
    }
  }

  // YouTube channel / user / playlist / mixes / radio
  if (
    u.includes('youtube.com/@') ||
    u.includes('youtube.com/channel/') ||
    u.includes('youtube.com/c/') ||
    u.includes('youtube.com/user/') ||
    u.includes('youtu.be/@') ||
    u.includes('list=') ||
    u.includes('/playlist') ||
    u.includes('/mix') ||
    u.includes('/radio')
  ) {
    return true;
  }

  // SoundCloud sets / albums / profiles
  if (u.includes('soundcloud.com/') && (u.includes('/sets/') || u.includes('/albums/'))) {
    return true;
  }

  // Bilibili series / channel / medialist
  if (u.includes('bilibili.com/medialist') || u.includes('space.bilibili.com/')) {
    return true;
  }

  // Generic albums, galleries and archives
  return (
    u.includes('/album/') ||
    u.includes('/albums/') ||
    u.includes('/sets/') ||
    u.includes('/collection/') ||
    u.includes('imgur.com/a/') ||
    u.includes('imgur.com/gallery/') ||
    u.includes('archive.org/details/') ||
    u.includes('pixeldrain.com/l/')
  );
}

export function detectPlatform(url?: string): PlatformInfo {
  if (!url) return { name: 'Media', icon: 'Film', color: '#a78bfa', isPlaylist: false };
  const u = url.toLowerCase();
  const isPl = isPlaylistUrl(url);

  if (u.includes('tiktok.com')) {
    return {
      name: isPl ? 'TikTok Channel / Profile' : 'TikTok',
      icon: 'Music2',
      color: '#00f2fe',
      isPlaylist: isPl
    };
  }
  if (u.includes('youtube.com') || u.includes('youtu.be')) {
    return {
      name: isPl ? 'YouTube Playlist / Channel' : 'YouTube',
      icon: 'Youtube',
      color: '#ef4444',
      isPlaylist: isPl
    };
  }
  if (u.includes('facebook.com') || u.includes('fb.watch') || u.includes('fb.com')) {
    return {
      name: isPl ? 'Facebook Videos / Reels' : 'Facebook Video',
      icon: 'Facebook',
      color: '#3b82f6',
      isPlaylist: isPl
    };
  }
  if (u.includes('instagram.com')) {
    return {
      name: isPl ? 'Instagram Profile' : 'Instagram',
      icon: 'Instagram',
      color: '#ec4899',
      isPlaylist: isPl
    };
  }
  if (
    u.includes('twitter.com') ||
    u.includes('x.com') ||
    u.includes('fxtwitter.com') ||
    u.includes('vxtwitter.com') ||
    u.includes('fixupx.com') ||
    u.includes('fixvx.com')
  ) {
    return {
      name: 'X / Twitter',
      icon: 'Twitter',
      color: '#38bdf8',
      isPlaylist: isPl
    };
  }
  if (u.includes('.m3u8') || u.includes('/m3u8')) {
    return {
      name: 'HLS Stream',
      icon: 'Radio',
      color: '#f59e0b',
      isPlaylist: false
    };
  }
  if (u.includes('bilibili.com')) {
    return {
      name: isPl ? 'Bilibili Playlist / Channel' : 'Bilibili',
      icon: 'Tv',
      color: '#00aeec',
      isPlaylist: isPl
    };
  }
  if (u.includes('soundcloud.com')) {
    return {
      name: isPl ? 'SoundCloud Album / Set' : 'SoundCloud',
      icon: 'Headphones',
      color: '#ff5500',
      isPlaylist: isPl
    };
  }

  return {
    name: isPl ? 'Album / Playlist' : 'Direct Link',
    icon: 'Globe',
    color: '#a78bfa',
    isPlaylist: isPl
  };
}
