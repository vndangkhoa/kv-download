'use client';

import React, { useState } from 'react';
import { GalleryItem, ViewMode } from '@/types';
import { usePlayer } from '@/context/PlayerContext';
import { deleteGalleryFile } from '@/lib/api';
import { detectPlatform, cn } from '@/lib/utils';
import { Button } from '@astryxdesign/core/Button';
import { IconButton } from '@astryxdesign/core/IconButton';
import { Badge } from '@astryxdesign/core/Badge';
import { AlertDialog } from '@astryxdesign/core/AlertDialog';
import { Check, Download, Film, Image as ImageIcon, Music, Play, Trash2 } from 'lucide-react';

interface GalleryCardProps {
  item: GalleryItem;
  viewMode: ViewMode;
  isSelected: boolean;
  onToggleSelect: (path: string) => void;
  onDeleted: () => void;
}

export function GalleryCard({
  item,
  viewMode,
  isSelected,
  onToggleSelect,
  onDeleted,
}: GalleryCardProps) {
  const { playMedia } = usePlayer();
  const [imgError, setImgError] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  const displayTitle = item.title || item.name;
  const platformName = item.platform || detectPlatform(item.url || '').name;
  const isPhoto = item.type === 'photo';
  const isAudio = item.type === 'audio';

  const streamUrl = `/download?file=${encodeURIComponent(item.path)}&inline=true`;
  const downloadUrl = `/download?file=${encodeURIComponent(item.path)}`;
  const thumbSrc = isPhoto
    ? streamUrl
    : item.thumbnail || `/thumbnail?path=${encodeURIComponent(item.path)}`;

  const handleConfirmDelete = async () => {
    setIsDeleting(true);
    try {
      await deleteGalleryFile(item.path);
      setShowDeleteDialog(false);
      onDeleted();
    } finally {
      setIsDeleting(false);
    }
  };

  if (viewMode === 'list') {
    return (
      <>
        <div
          className={cn(
            'group flex items-center justify-between gap-3 p-2.5 rounded-lg border transition-colors',
            isSelected
              ? 'bg-[var(--color-accent-muted)] border-[var(--color-accent)]'
              : 'bg-[var(--color-background-card)] border-[var(--color-border)] hover:border-[var(--color-border-emphasized)]'
          )}
        >
          <div className="flex items-center gap-3 min-w-0 flex-1">
            <button
              type="button"
              onClick={() => onToggleSelect(item.path)}
              aria-label={`Select ${displayTitle}`}
              className={cn(
                'w-4.5 h-4.5 rounded border flex items-center justify-center shrink-0 transition-opacity',
                isSelected
                  ? 'bg-[var(--color-accent)] border-[var(--color-accent)] text-[var(--color-on-accent)] opacity-100'
                  : 'border-[var(--color-border-emphasized)] bg-[var(--color-background-surface)] opacity-75 group-hover:opacity-100'
              )}
            >
              {isSelected && <Check className="w-3 h-3" />}
            </button>

            <div className="w-14 h-9 rounded overflow-hidden border border-[var(--color-border)] bg-[var(--color-background-muted)] shrink-0 flex items-center justify-center">
              {!imgError ? (
                <img
                  src={thumbSrc}
                  alt={displayTitle}
                  loading="lazy"
                  className="w-full h-full object-cover"
                  onError={() => setImgError(true)}
                />
              ) : isAudio ? (
                <Music className="w-4 h-4 text-[var(--color-icon-accent)]" />
              ) : (
                <Film className="w-4 h-4 text-[var(--color-icon-secondary)]" />
              )}
            </div>

            <div className="flex-1 min-w-0">
              <p className="text-sm font-semibold text-[var(--color-text-primary)] truncate">
                {displayTitle}
              </p>
              <p className="text-xs text-[var(--color-text-secondary)] truncate">
                {item.channel || 'General'}
                {item.group ? ` · ${item.group}` : ''}
                {platformName && platformName !== 'Web' ? ` · ${platformName}` : ''}
                {' · '}
                <span className="font-mono tabular-nums">{item.humanSize}</span>
              </p>
            </div>
          </div>

          <div className="flex items-center gap-1.5 shrink-0">
            {!isPhoto ? (
              <Button
                label="Play"
                size="sm"
                variant="primary"
                icon={<Play className="w-3.5 h-3.5 fill-current" />}
                onClick={() => playMedia(item)}
              />
            ) : (
              <a
                href={streamUrl}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg text-xs font-semibold border border-[var(--color-border)] bg-[var(--color-background-muted)] text-[var(--color-text-primary)] hover:bg-[var(--color-overlay-hover)]"
              >
                <ImageIcon className="w-3.5 h-3.5" />
                View
              </a>
            )}
            <a
              href={downloadUrl}
              download={item.name}
              className="inline-flex items-center justify-center p-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-background-muted)] text-[var(--color-text-primary)] hover:bg-[var(--color-overlay-hover)]"
              title="Download to device"
            >
              <Download className="w-3.5 h-3.5" />
            </a>
            <IconButton
              label={`Delete ${displayTitle}`}
              size="sm"
              variant="ghost"
              icon={<Trash2 className="w-4 h-4 text-[var(--color-text-error)]" />}
              onClick={() => setShowDeleteDialog(true)}
            />
          </div>
        </div>

        <AlertDialog
          isOpen={showDeleteDialog}
          onOpenChange={setShowDeleteDialog}
          title="Delete Media File?"
          description={`Permanently delete "${displayTitle}" (${item.humanSize}) from server storage?`}
          actionLabel="Delete File"
          actionVariant="destructive"
          isActionLoading={isDeleting}
          onAction={handleConfirmDelete}
        />
      </>
    );
  }

  return (
    <>
      <article
        className={cn(
          'group flex flex-col justify-between rounded-xl border overflow-hidden transition-colors',
          isSelected
            ? 'bg-[var(--color-accent-muted)] border-[var(--color-accent)]'
            : 'bg-[var(--color-background-card)] border-[var(--color-border)] hover:border-[var(--color-border-emphasized)]'
        )}
      >
        {/* Thumbnail Preview */}
        <div
          className={cn(
            'relative w-full bg-[var(--color-background-muted)] overflow-hidden',
            viewMode === 'compact' ? 'aspect-[16/10]' : 'aspect-video'
          )}
        >
          {!imgError ? (
            <img
              src={thumbSrc}
              alt={displayTitle}
              loading="lazy"
              className="w-full h-full object-cover"
              onError={() => setImgError(true)}
            />
          ) : (
            <div className="w-full h-full flex items-center justify-center text-[var(--color-icon-secondary)]">
              {isAudio ? (
                <Music className="w-10 h-10 text-[var(--color-icon-accent)]" />
              ) : isPhoto ? (
                <ImageIcon className="w-10 h-10" />
              ) : (
                <Film className="w-10 h-10" />
              )}
            </div>
          )}

          {/* Hover / Focus Selection Checkbox */}
          <button
            type="button"
            onClick={() => onToggleSelect(item.path)}
            aria-label={`Select ${displayTitle}`}
            className={cn(
              'absolute top-2 left-2 w-6 h-6 rounded-md border flex items-center justify-center transition-opacity shadow-sm',
              isSelected
                ? 'bg-[var(--color-accent)] border-[var(--color-accent)] text-[var(--color-on-accent)] opacity-100'
                : 'bg-black/60 border-white/50 text-white opacity-0 group-hover:opacity-100 focus-visible:opacity-100'
            )}
          >
            {isSelected && <Check className="w-3.5 h-3.5" />}
          </button>

          {item.channel && (
            <span className="absolute top-2 right-2 max-w-[65%] truncate px-2 py-0.5 rounded-md bg-black/70 text-white text-[11px] font-medium">
              {item.channel}
            </span>
          )}

          <div className="absolute bottom-2 right-2 flex items-center gap-1">
            {item.duration && (
              <span className="px-1.5 py-0.5 rounded bg-black/75 text-white text-[10px] font-mono tabular-nums">
                {item.duration}
              </span>
            )}
            <span className="px-1.5 py-0.5 rounded bg-black/75 text-white text-[10px] font-mono tabular-nums">
              {item.humanSize}
            </span>
          </div>
        </div>

        {/* Metadata & Actions */}
        <div className={cn('flex flex-col flex-1 justify-between gap-2.5', viewMode === 'compact' ? 'p-2.5' : 'p-3')}>
          <div className="space-y-1">
            <h4
              className={cn(
                'font-semibold text-[var(--color-text-primary)] leading-snug',
                viewMode === 'compact' ? 'text-xs line-clamp-1' : 'text-sm line-clamp-2'
              )}
              title={displayTitle}
            >
              {displayTitle}
            </h4>

            <div className="flex flex-wrap items-center gap-1">
              {platformName && <Badge label={platformName} variant="neutral" />}
              {item.group && item.group !== item.channel && (
                <Badge label={item.group} variant="purple" />
              )}
            </div>
          </div>

          <div className="flex items-center justify-between gap-1.5 pt-2 border-t border-[var(--color-border)]">
            {!isPhoto ? (
              <Button
                label="Play"
                size="sm"
                variant="primary"
                icon={<Play className="w-3.5 h-3.5 fill-current" />}
                onClick={() => playMedia(item)}
              />
            ) : (
              <a
                href={streamUrl}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-semibold border border-[var(--color-border)] bg-[var(--color-background-muted)] text-[var(--color-text-primary)] hover:bg-[var(--color-overlay-hover)]"
              >
                <ImageIcon className="w-3.5 h-3.5" />
                Open
              </a>
            )}

            <div className="flex items-center gap-1">
              <a
                href={downloadUrl}
                download={item.name}
                className="inline-flex items-center justify-center p-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-background-muted)] text-[var(--color-text-primary)] hover:bg-[var(--color-overlay-hover)]"
                title="Save to device"
              >
                <Download className="w-3.5 h-3.5" />
              </a>
              <IconButton
                label={`Delete ${displayTitle}`}
                size="sm"
                variant="ghost"
                icon={<Trash2 className="w-3.5 h-3.5 text-[var(--color-text-error)]" />}
                onClick={() => setShowDeleteDialog(true)}
              />
            </div>
          </div>
        </div>
      </article>

      <AlertDialog
        isOpen={showDeleteDialog}
        onOpenChange={setShowDeleteDialog}
        title="Delete Media File?"
        description={`Permanently delete "${displayTitle}" (${item.humanSize}) from server storage?`}
        actionLabel="Delete File"
        actionVariant="destructive"
        isActionLoading={isDeleting}
        onAction={handleConfirmDelete}
      />
    </>
  );
}
