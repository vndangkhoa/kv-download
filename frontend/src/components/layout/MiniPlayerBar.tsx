'use client';

import React, { useEffect, useState, useRef } from 'react';
import { usePlayer } from '@/context/PlayerContext';
import {
  Play,
  Pause,
  X,
  Maximize2,
  Minimize2,
  SkipBack,
  SkipForward,
  Volume2,
  VolumeX,
  Music,
  Film,
  Download,
} from 'lucide-react';
import { IconButton } from '@astryxdesign/core/IconButton';
import { Button } from '@astryxdesign/core/Button';
import { formatTime, cn } from '@/lib/utils';

export function MiniPlayerBar() {
  const {
    currentMedia,
    isPlaying,
    currentTime,
    duration,
    volume,
    isExpanded,
    setIsExpanded,
    togglePlay,
    seek,
    skip,
    setVolume,
    closePlayer,
    onTimeUpdate,
    videoRef,
    audioRef,
  } = usePlayer();

  const [isHoveredProgress, setIsHoveredProgress] = useState(false);
  const previousVolumeRef = useRef(1);

  useEffect(() => {
    if (!currentMedia) return;
    const el = currentMedia.isAudio ? audioRef.current : videoRef.current;
    if (el && currentMedia.streamUrl) {
      el.src = currentMedia.streamUrl;
      el.volume = volume;
      el.play().catch((err) => console.warn('Autoplay prevented or delayed:', err));
    }
  }, [currentMedia, audioRef, videoRef, volume]);

  if (!currentMedia) return null;

  const handleToggleMute = () => {
    if (volume > 0) {
      previousVolumeRef.current = volume;
      setVolume(0);
    } else {
      setVolume(previousVolumeRef.current || 1);
    }
  };

  const handleSeekChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    seek(parseFloat(e.target.value));
  };

  const progressPercent = duration > 0 ? Math.min(100, (currentTime / duration) * 100) : 0;

  return (
    <>
      <audio
        ref={audioRef}
        preload="metadata"
        onTimeUpdate={(e) => {
          const t = e.currentTarget;
          onTimeUpdate(t.currentTime, t.duration);
        }}
        onEnded={() => closePlayer()}
        className="hidden"
      />

      {/* Expanded Theater Overlay */}
      <div
        role={isExpanded ? 'dialog' : undefined}
        aria-modal={isExpanded ? 'true' : undefined}
        aria-label={isExpanded ? currentMedia.title : undefined}
        className={cn(
          'fixed inset-0 z-50 flex flex-col justify-between bg-[var(--color-background-body)] text-[var(--color-text-primary)] transition-all duration-200',
          isExpanded
            ? 'opacity-100 pointer-events-auto'
            : 'opacity-0 pointer-events-none translate-y-6'
        )}
      >
        <div className="flex items-center justify-between px-4 py-3 sm:px-6 border-b border-[var(--color-border)] bg-[var(--color-background-surface)]">
          <div className="flex items-center gap-3 min-w-0">
            <div className="w-9 h-9 rounded-lg bg-[var(--color-accent-muted)] flex items-center justify-center text-[var(--color-icon-accent)] shrink-0">
              {currentMedia.isAudio ? <Music className="w-4 h-4" /> : <Film className="w-4 h-4" />}
            </div>
            <div className="flex flex-col min-w-0">
              <span className="text-sm font-semibold truncate">{currentMedia.title}</span>
              <span className="text-xs text-[var(--color-text-secondary)] truncate">
                {currentMedia.channel || 'Media Studio'}
                {currentMedia.humanSize ? ` · ${currentMedia.humanSize}` : ''}
              </span>
            </div>
          </div>

          <div className="flex items-center gap-2 shrink-0">
            <a
              href={currentMedia.downloadUrl}
              download
              target="_blank"
              rel="noreferrer"
              className="hidden sm:inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold border border-[var(--color-border)] bg-[var(--color-background-muted)] text-[var(--color-text-primary)] hover:bg-[var(--color-overlay-hover)] transition-colors"
            >
              <Download className="w-3.5 h-3.5" />
              Save File
            </a>
            <IconButton
              label="Minimize player"
              variant="secondary"
              size="sm"
              icon={<Minimize2 className="w-4 h-4" />}
              onClick={() => setIsExpanded(false)}
            />
            <IconButton
              label="Close player"
              variant="secondary"
              size="sm"
              icon={<X className="w-4 h-4" />}
              onClick={closePlayer}
            />
          </div>
        </div>

        {/* Center Stage */}
        <div className="flex-1 flex items-center justify-center p-4 sm:p-6 overflow-hidden relative bg-black/95">
          {currentMedia.isAudio ? (
            <div className="flex flex-col items-center gap-6 text-center max-w-md w-full px-4">
              <div className="relative w-52 h-52 sm:w-64 sm:h-64 rounded-2xl overflow-hidden border border-white/15 bg-slate-900">
                {currentMedia.thumbnail ? (
                  <img
                    src={currentMedia.thumbnail}
                    alt={currentMedia.title}
                    className="w-full h-full object-cover"
                  />
                ) : (
                  <div className="w-full h-full flex items-center justify-center text-violet-400">
                    <Music className="w-20 h-20" />
                  </div>
                )}
              </div>
              <div className="space-y-1">
                <h3 className="text-lg font-bold text-white line-clamp-2">{currentMedia.title}</h3>
                <p className="text-sm text-slate-300">{currentMedia.channel || 'Audio Track'}</p>
              </div>
            </div>
          ) : (
            <video
              ref={videoRef}
              controls
              playsInline
              className="max-h-full max-w-full rounded-lg outline-none"
              onTimeUpdate={(e) => {
                const t = e.currentTarget;
                onTimeUpdate(t.currentTime, t.duration);
              }}
              onPlay={() => {}}
              onPause={() => {}}
              onEnded={() => {}}
            />
          )}
        </div>

        {/* Theater Controls (shown for audio or mobile/tablet unified control) */}
        <div className="px-4 py-4 sm:px-6 border-t border-[var(--color-border)] bg-[var(--color-background-surface)] space-y-3">
          <div className="max-w-3xl mx-auto flex items-center gap-3">
            <span className="text-xs font-mono tabular-nums text-[var(--color-text-secondary)] w-11 text-right shrink-0">
              {formatTime(currentTime)}
            </span>
            <input
              type="range"
              aria-label="Seek timeline"
              min={0}
              max={duration || 100}
              step={0.1}
              value={currentTime}
              onChange={handleSeekChange}
              className="flex-1 h-1.5 rounded-full accent-[var(--color-accent)] cursor-pointer"
            />
            <span className="text-xs font-mono tabular-nums text-[var(--color-text-secondary)] w-11 shrink-0">
              {formatTime(duration)}
            </span>
          </div>

          <div className="flex items-center justify-center gap-4">
            <IconButton
              label="Skip back 10 seconds"
              variant="ghost"
              size="md"
              icon={<SkipBack className="w-4 h-4" />}
              onClick={() => skip(-10)}
            />
            <Button
              label={isPlaying ? 'Pause' : 'Play'}
              variant="primary"
              size="md"
              icon={
                isPlaying ? (
                  <Pause className="w-4 h-4 fill-current" />
                ) : (
                  <Play className="w-4 h-4 fill-current" />
                )
              }
              onClick={togglePlay}
            />
            <IconButton
              label="Skip forward 10 seconds"
              variant="ghost"
              size="md"
              icon={<SkipForward className="w-4 h-4" />}
              onClick={() => skip(10)}
            />
          </div>
        </div>
      </div>

      {/* Floating Compact Mini-Bar */}
      {!isExpanded && (
        <div className="fixed bottom-16 md:bottom-4 left-3 right-3 md:left-[260px] md:right-6 z-40 rounded-xl border border-[var(--color-border-emphasized)] bg-[var(--color-background-card)] shadow-lg overflow-hidden">
          <div
            className="w-full h-1 bg-[var(--color-background-muted)] cursor-pointer relative"
            onMouseEnter={() => setIsHoveredProgress(true)}
            onMouseLeave={() => setIsHoveredProgress(false)}
          >
            <div
              className="h-full bg-[var(--color-accent)] transition-all duration-150"
              style={{ width: `${progressPercent}%` }}
            />
            {isHoveredProgress && (
              <input
                type="range"
                aria-label="Seek media position"
                min={0}
                max={duration || 100}
                step={0.1}
                value={currentTime}
                onChange={handleSeekChange}
                className="absolute inset-0 w-full opacity-0 cursor-pointer"
              />
            )}
          </div>

          <div className="flex items-center justify-between px-3 py-2 gap-3">
            <button
              type="button"
              onClick={() => setIsExpanded(true)}
              className="flex items-center gap-2.5 min-w-0 flex-1 text-left"
            >
              <div className="w-9 h-9 rounded-lg overflow-hidden bg-[var(--color-background-muted)] border border-[var(--color-border)] shrink-0 flex items-center justify-center">
                {currentMedia.thumbnail ? (
                  <img
                    src={currentMedia.thumbnail}
                    alt={currentMedia.title}
                    className="w-full h-full object-cover"
                  />
                ) : currentMedia.isAudio ? (
                  <Music className="w-4 h-4 text-[var(--color-icon-accent)]" />
                ) : (
                  <Film className="w-4 h-4 text-[var(--color-icon-accent)]" />
                )}
              </div>
              <div className="flex flex-col min-w-0">
                <span className="text-xs font-semibold text-[var(--color-text-primary)] truncate">
                  {currentMedia.title}
                </span>
                <span className="text-[11px] font-mono tabular-nums text-[var(--color-text-secondary)] truncate">
                  {formatTime(currentTime)} / {formatTime(duration || 0)}
                  {currentMedia.channel ? ` · ${currentMedia.channel}` : ''}
                </span>
              </div>
            </button>

            <div className="flex items-center gap-1 shrink-0">
              <div className="hidden lg:flex items-center gap-1.5 mr-1">
                <IconButton
                  label={volume > 0 ? 'Mute' : 'Unmute'}
                  variant="ghost"
                  size="sm"
                  icon={
                    volume === 0 ? (
                      <VolumeX className="w-4 h-4" />
                    ) : (
                      <Volume2 className="w-4 h-4" />
                    )
                  }
                  onClick={handleToggleMute}
                />
                <input
                  type="range"
                  aria-label="Volume"
                  min={0}
                  max={1}
                  step={0.05}
                  value={volume}
                  onChange={(e) => setVolume(parseFloat(e.target.value))}
                  className="w-16 h-1 accent-[var(--color-accent)] cursor-pointer"
                />
              </div>

              <IconButton
                label="Back 10 seconds"
                variant="ghost"
                size="sm"
                icon={<SkipBack className="w-4 h-4" />}
                onClick={() => skip(-10)}
              />
              <IconButton
                label={isPlaying ? 'Pause' : 'Play'}
                variant="primary"
                size="sm"
                icon={
                  isPlaying ? (
                    <Pause className="w-4 h-4 fill-current" />
                  ) : (
                    <Play className="w-4 h-4 fill-current" />
                  )
                }
                onClick={togglePlay}
              />
              <IconButton
                label="Forward 10 seconds"
                variant="ghost"
                size="sm"
                icon={<SkipForward className="w-4 h-4" />}
                onClick={() => skip(10)}
              />
              <IconButton
                label="Expand player"
                variant="ghost"
                size="sm"
                icon={<Maximize2 className="w-4 h-4" />}
                onClick={() => setIsExpanded(true)}
              />
              <IconButton
                label="Close player"
                variant="ghost"
                size="sm"
                icon={<X className="w-4 h-4" />}
                onClick={closePlayer}
              />
            </div>
          </div>
        </div>
      )}
    </>
  );
}
