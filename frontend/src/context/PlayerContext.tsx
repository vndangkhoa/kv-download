'use client';

import React, { createContext, useContext, useState, useRef, useEffect, useCallback } from 'react';
import { DownloadTask, GalleryItem } from '@/types';

export interface PlayerMedia {
  id: string;
  title: string;
  channel?: string;
  humanSize?: string;
  url?: string;
  streamUrl: string;
  downloadUrl: string;
  thumbnail?: string;
  isAudio: boolean;
}

interface PlayerContextType {
  currentMedia: PlayerMedia | null;
  isPlaying: boolean;
  currentTime: number;
  duration: number;
  volume: number;
  isExpanded: boolean;
  setIsExpanded: (expanded: boolean) => void;
  playMedia: (item: DownloadTask | GalleryItem) => void;
  playStream: (
    streamUrl: string,
    title: string,
    opts?: {
      thumbnail?: string;
      isAudio?: boolean;
      channel?: string;
      downloadUrl?: string;
    }
  ) => void;
  togglePlay: () => void;
  seek: (time: number) => void;
  skip: (seconds: number) => void;
  setVolume: (vol: number) => void;
  closePlayer: () => void;
  onTimeUpdate: (time: number, dur: number) => void;
  videoRef: React.RefObject<HTMLVideoElement | null>;
  audioRef: React.RefObject<HTMLAudioElement | null>;
}

const PlayerContext = createContext<PlayerContextType | undefined>(undefined);

function isAudioItem(name?: string, format?: string, itemType?: string): boolean {
  if (itemType === 'audio') return true;
  if (format === 'audio') return true;
  const lower = (name || '').toLowerCase();
  return (
    lower.endsWith('.mp3') ||
    lower.endsWith('.m4a') ||
    lower.endsWith('.flac') ||
    lower.endsWith('.wav') ||
    lower.endsWith('.ogg') ||
    lower.endsWith('.opus')
  );
}

export function PlayerProvider({ children }: { children: React.ReactNode }) {
  const [currentMedia, setCurrentMedia] = useState<PlayerMedia | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolumeState] = useState(1);
  const [isExpanded, setIsExpanded] = useState(false);

  const videoRef = useRef<HTMLVideoElement | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);

  const getActiveElement = useCallback((): HTMLMediaElement | null => {
    if (!currentMedia) return null;
    return currentMedia.isAudio ? audioRef.current : videoRef.current;
  }, [currentMedia]);

  const onTimeUpdate = useCallback((time: number, dur: number) => {
    setCurrentTime(time);
    if (dur && !isNaN(dur) && isFinite(dur)) {
      setDuration(dur);
      if ('mediaSession' in navigator && navigator.mediaSession.setPositionState) {
        try {
          navigator.mediaSession.setPositionState({
            duration: dur,
            playbackRate: 1,
            position: Math.min(time, dur),
          });
        } catch (_) {}
      }
    }
  }, []);

  const updateMediaSessionMetadata = useCallback((media: PlayerMedia) => {
    if ('mediaSession' in navigator && typeof MediaMetadata !== 'undefined') {
      try {
        navigator.mediaSession.metadata = new MediaMetadata({
          title: media.title,
          artist: media.channel || 'KV Download',
          album: 'KV Download Studio',
          artwork: media.thumbnail ? [{ src: media.thumbnail, sizes: '512x512', type: 'image/jpeg' }] : [],
        });
      } catch (_) {}
    }
  }, []);

  const playMedia = useCallback(
    (item: DownloadTask | GalleryItem) => {
      const isGallery = 'path' in item && !('status' in item);
      const id = item.id;
      const title =
        item.title ||
        ('mediaName' in item && item.mediaName) ||
        ('name' in item && item.name) ||
        'Media File';
      const channel = item.channel || ('uploader' in item ? item.uploader : undefined);
      const humanSize = item.humanSize;
      const thumbnail =
        item.thumbnail ||
        (isGallery
          ? `/thumbnail?path=${encodeURIComponent((item as GalleryItem).path)}`
          : `/thumbnail?id=${encodeURIComponent(id)}`);
      const isAudio = isAudioItem(
        'name' in item ? item.name : 'mediaName' in item ? item.mediaName : title,
        'format' in item ? item.format : undefined,
        'type' in item ? item.type : undefined
      );

      const streamUrl = isGallery
        ? `/download?file=${encodeURIComponent((item as GalleryItem).path)}&inline=true`
        : `/download?id=${encodeURIComponent(id)}&inline=true`;
      const downloadUrl = isGallery
        ? `/download?file=${encodeURIComponent((item as GalleryItem).path)}`
        : `/download?id=${encodeURIComponent(id)}`;

      const media: PlayerMedia = {
        id,
        title,
        channel,
        humanSize,
        url: item.url,
        streamUrl,
        downloadUrl,
        thumbnail,
        isAudio,
      };

      setCurrentMedia(media);
      setCurrentTime(0);
      setDuration(0);
      setIsPlaying(true);
      if (!isAudio) {
        setIsExpanded(true);
      }
      updateMediaSessionMetadata(media);
    },
    [updateMediaSessionMetadata]
  );

  const playStream = useCallback(
    (
      streamUrl: string,
      title: string,
      opts?: {
        thumbnail?: string;
        isAudio?: boolean;
        channel?: string;
        downloadUrl?: string;
      }
    ) => {
      const proxied =
        streamUrl.startsWith('/api/') || streamUrl.startsWith('/download')
          ? streamUrl
          : `/api/browser/stream?url=${encodeURIComponent(streamUrl)}`;
      const safeId = `stream-${Math.abs(
        streamUrl.split('').reduce((acc, ch) => (acc * 31 + ch.charCodeAt(0)) | 0, 7)
      )}`;
      const isAudio = Boolean(opts?.isAudio);

      const media: PlayerMedia = {
        id: safeId,
        title: title || 'Live Stream Preview',
        channel: opts?.channel || 'Live Stream',
        url: streamUrl,
        streamUrl: proxied,
        downloadUrl: opts?.downloadUrl || proxied,
        thumbnail: opts?.thumbnail,
        isAudio,
      };

      setCurrentMedia(media);
      setCurrentTime(0);
      setDuration(0);
      setIsPlaying(true);
      if (!isAudio) {
        setIsExpanded(true);
      }
      updateMediaSessionMetadata(media);
    },
    [updateMediaSessionMetadata]
  );

  const togglePlay = useCallback(() => {
    const el = getActiveElement();
    if (el) {
      if (el.paused) {
        el.play().catch(console.error);
        setIsPlaying(true);
      } else {
        el.pause();
        setIsPlaying(false);
      }
    }
  }, [getActiveElement]);

  const seek = useCallback(
    (time: number) => {
      const el = getActiveElement();
      if (el && !isNaN(time)) {
        el.currentTime = time;
        setCurrentTime(time);
      }
    },
    [getActiveElement]
  );

  const skip = useCallback(
    (seconds: number) => {
      const el = getActiveElement();
      if (el) {
        const targetTime = Math.max(0, Math.min(el.duration || 0, el.currentTime + seconds));
        el.currentTime = targetTime;
        setCurrentTime(targetTime);
      }
    },
    [getActiveElement]
  );

  const setVolume = useCallback(
    (vol: number) => {
      const v = Math.max(0, Math.min(1, vol));
      setVolumeState(v);
      const el = getActiveElement();
      if (el) el.volume = v;
    },
    [getActiveElement]
  );

  const closePlayer = useCallback(() => {
    if (videoRef.current) {
      videoRef.current.pause();
      videoRef.current.src = '';
    }
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current.src = '';
    }
    setCurrentMedia(null);
    setIsPlaying(false);
    setIsExpanded(false);
    setCurrentTime(0);
    setDuration(0);
    if ('mediaSession' in navigator) {
      navigator.mediaSession.playbackState = 'none';
    }
  }, []);

  useEffect(() => {
    if (!('mediaSession' in navigator)) return;

    try {
      navigator.mediaSession.setActionHandler('play', () => {
        const el = getActiveElement();
        if (el && el.paused) {
          el.play().catch(console.error);
          setIsPlaying(true);
        }
      });

      navigator.mediaSession.setActionHandler('pause', () => {
        const el = getActiveElement();
        if (el && !el.paused) {
          el.pause();
          setIsPlaying(false);
        }
      });

      navigator.mediaSession.setActionHandler('seekbackward', (details) => {
        skip(-(details.seekOffset || 10));
      });

      navigator.mediaSession.setActionHandler('seekforward', (details) => {
        skip(details.seekOffset || 10);
      });

      navigator.mediaSession.setActionHandler('seekto', (details) => {
        if (details.seekTime !== undefined) {
          seek(details.seekTime);
        }
      });

      navigator.mediaSession.setActionHandler('stop', () => {
        closePlayer();
      });
    } catch (e) {
      console.log('MediaSession action handler error:', e);
    }
  }, [getActiveElement, skip, seek, closePlayer]);

  useEffect(() => {
    if ('mediaSession' in navigator) {
      navigator.mediaSession.playbackState = isPlaying ? 'playing' : 'paused';
    }
  }, [isPlaying]);

  return (
    <PlayerContext.Provider
      value={{
        currentMedia,
        isPlaying,
        currentTime,
        duration,
        volume,
        isExpanded,
        setIsExpanded,
        playMedia,
        playStream,
        togglePlay,
        seek,
        skip,
        setVolume,
        closePlayer,
        onTimeUpdate,
        videoRef,
        audioRef,
      }}
    >
      {children}
    </PlayerContext.Provider>
  );
}

export function usePlayer() {
  const context = useContext(PlayerContext);
  if (!context) {
    throw new Error('usePlayer must be used within a PlayerProvider');
  }
  return context;
}
