'use client';

import React, { useState } from 'react';
import { DownloadTask, ViewMode } from '@/types';
import { detectPlatform, cn } from '@/lib/utils';
import { usePlayer } from '@/context/PlayerContext';
import { cancelTask, retryTask, deleteTask } from '@/lib/api';
import { Button } from '@astryxdesign/core/Button';
import { IconButton } from '@astryxdesign/core/IconButton';
import { Badge } from '@astryxdesign/core/Badge';
import { ProgressBar } from '@astryxdesign/core/ProgressBar';
import { AlertDialog } from '@astryxdesign/core/AlertDialog';
import {
  Play,
  RotateCcw,
  Trash2,
  X,
  Download,
  Cookie,
  Film,
  Music,
  AlertCircle,
} from 'lucide-react';

interface TaskCardProps {
  task: DownloadTask;
  viewMode: ViewMode;
  onRefresh: () => void;
  onOpenCookies?: () => void;
}

function getStatusBadge(
  status: DownloadTask['status']
): { label: string; variant: 'success' | 'info' | 'warning' | 'error' | 'neutral' } {
  switch (status) {
    case 'completed':
      return { label: 'Completed', variant: 'success' };
    case 'downloading':
      return { label: 'Downloading', variant: 'info' };
    case 'queued':
      return { label: 'Queued', variant: 'neutral' };
    case 'failed':
      return { label: 'Failed', variant: 'error' };
    case 'cancelled':
      return { label: 'Cancelled', variant: 'warning' };
    default:
      return { label: status, variant: 'neutral' };
  }
}

function needsCookieHint(errMsg?: string): boolean {
  if (!errMsg) return false;
  return /cookie|sign in|login|auth|403|bot|age|members-only/i.test(errMsg);
}

export function TaskCard({
  task,
  viewMode,
  onRefresh,
  onOpenCookies,
}: TaskCardProps) {
  const { playMedia } = usePlayer();
  const [imgError, setImgError] = useState(false);
  const [isActionLoading, setIsActionLoading] = useState(false);
  const [showDiskDeleteDialog, setShowDiskDeleteDialog] = useState(false);

  const platformInfo = detectPlatform(task.url);
  const platformName = task.platform || platformInfo.name;
  const displayTitle = task.title || task.mediaName || task.url;
  const displaySubtitle = task.channel || task.uploader || platformName;
  const mediaIdQuery = task.mediaId ? `&mediaId=${encodeURIComponent(task.mediaId)}` : '';
  const thumbUrl = task.thumbnail || `/thumbnail?id=${encodeURIComponent(task.id)}${mediaIdQuery}`;
  const downloadHref = `/download?id=${encodeURIComponent(task.id)}${mediaIdQuery}`;
  const isCompleted = task.status === 'completed';
  const isDownloading = task.status === 'downloading';
  const isRetryable = task.status === 'failed' || task.status === 'cancelled';
  const statusBadge = getStatusBadge(task.status);
  const isAudio = task.format === 'audio';

  const handleRetry = async () => {
    setIsActionLoading(true);
    try {
      await retryTask(task.id);
      onRefresh();
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleCancel = async () => {
    setIsActionLoading(true);
    try {
      await cancelTask(task.id);
      onRefresh();
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleRemoveQueue = async () => {
    setIsActionLoading(true);
    try {
      await deleteTask(task.id, false);
      onRefresh();
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleConfirmDeleteDisk = async () => {
    setIsActionLoading(true);
    try {
      await deleteTask(task.id, true);
      setShowDiskDeleteDialog(false);
      onRefresh();
    } finally {
      setIsActionLoading(false);
    }
  };

  const renderThumbnail = (sizeClass: string) => (
    <div
      className={cn(
        'relative rounded-lg overflow-hidden border border-[var(--color-border)] bg-[var(--color-background-muted)] shrink-0 flex items-center justify-center',
        sizeClass
      )}
    >
      {!imgError && thumbUrl ? (
        <img
          src={thumbUrl}
          alt={displayTitle}
          loading="lazy"
          className="w-full h-full object-cover"
          onError={() => setImgError(true)}
        />
      ) : isAudio ? (
        <Music className="w-5 h-5 text-[var(--color-icon-accent)]" />
      ) : (
        <Film className="w-5 h-5 text-[var(--color-icon-secondary)]" />
      )}
      {task.duration && (
        <span className="absolute bottom-1 right-1 px-1 py-0.5 rounded bg-black/75 text-white text-[10px] font-mono tabular-nums leading-none">
          {task.duration}
        </span>
      )}
    </div>
  );

  // Compact list row view
  if (viewMode === 'list') {
    return (
      <>
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 p-2.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-background-card)] hover:border-[var(--color-border-emphasized)] transition-colors">
          <div className="flex items-center gap-3 min-w-0 flex-1">
            {renderThumbnail('w-14 h-9')}

            <div className="flex-1 min-w-0 space-y-0.5">
              <div className="flex items-center gap-2 min-w-0">
                <p className="text-sm font-semibold text-[var(--color-text-primary)] truncate">
                  {displayTitle}
                </p>
                <Badge label={statusBadge.label} variant={statusBadge.variant} />
              </div>

              <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--color-text-secondary)]">
                <span className="truncate max-w-[180px]">{displaySubtitle}</span>
                {task.format && (
                  <span className="font-mono uppercase text-[11px]">{task.format}</span>
                )}
                {task.humanSize && (
                  <span className="font-mono tabular-nums">{task.humanSize}</span>
                )}
                {isDownloading && (
                  <span className="font-mono tabular-nums text-[var(--color-text-info)]">
                    {Math.round(task.percent)}% {task.speed ? `· ${task.speed}` : ''}
                    {task.eta ? ` · ETA ${task.eta}` : ''}
                  </span>
                )}
              </div>

              {task.errorMessage && (
                <div className="flex items-center gap-1.5 text-xs text-[var(--color-text-error)]">
                  <AlertCircle className="w-3.5 h-3.5 shrink-0" />
                  <span className="truncate">{task.errorMessage}</span>
                  {needsCookieHint(task.errorMessage) && onOpenCookies && (
                    <button
                      type="button"
                      onClick={onOpenCookies}
                      className="underline font-semibold shrink-0 hover:opacity-80"
                    >
                      Cookies
                    </button>
                  )}
                </div>
              )}
            </div>
          </div>

          <div className="flex items-center gap-1.5 shrink-0 justify-end">
            {isCompleted && (
              <>
                <Button
                  label="Play"
                  size="sm"
                  variant="primary"
                  icon={<Play className="w-3.5 h-3.5 fill-current" />}
                  onClick={() => playMedia(task)}
                />
                <a
                  href={downloadHref}
                  download
                  className="inline-flex items-center justify-center p-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-background-muted)] text-[var(--color-text-primary)] hover:bg-[var(--color-overlay-hover)]"
                  title="Save file to device"
                >
                  <Download className="w-3.5 h-3.5" />
                </a>
                <IconButton
                  label="Delete file from disk"
                  size="sm"
                  variant="ghost"
                  icon={<Trash2 className="w-4 h-4 text-[var(--color-text-error)]" />}
                  onClick={() => setShowDiskDeleteDialog(true)}
                />
              </>
            )}

            {isRetryable && (
              <Button
                label="Retry"
                size="sm"
                variant="secondary"
                isLoading={isActionLoading}
                icon={<RotateCcw className="w-3.5 h-3.5" />}
                onClick={handleRetry}
              />
            )}

            {(task.status === 'downloading' || task.status === 'queued') && (
              <IconButton
                label="Cancel task"
                size="sm"
                variant="ghost"
                isLoading={isActionLoading}
                icon={<X className="w-4 h-4" />}
                onClick={handleCancel}
              />
            )}

            <IconButton
              label="Remove from queue"
              size="sm"
              variant="ghost"
              tooltip="Remove from queue"
              isLoading={isActionLoading}
              icon={<X className="w-3.5 h-3.5" />}
              onClick={handleRemoveQueue}
            />
          </div>
        </div>

        <AlertDialog
          isOpen={showDiskDeleteDialog}
          onOpenChange={setShowDiskDeleteDialog}
          title="Delete File from Server Disk?"
          description={`Permanently delete "${displayTitle}" from storage and remove this task from your queue?`}
          actionLabel="Delete File"
          actionVariant="destructive"
          isActionLoading={isActionLoading}
          onAction={handleConfirmDeleteDisk}
        />
      </>
    );
  }

  // Standard and Compact card layout
  return (
    <>
      <div
        className={cn(
          'flex flex-col justify-between rounded-xl border border-[var(--color-border)] bg-[var(--color-background-card)] transition-colors hover:border-[var(--color-border-emphasized)]',
          viewMode === 'compact' ? 'p-2.5 gap-2' : 'p-3.5 gap-3'
        )}
      >
        {/* Top Metadata */}
        <div className="flex items-start gap-3 min-w-0">
          {renderThumbnail(viewMode === 'compact' ? 'w-16 h-11' : 'w-24 h-14')}

          <div className="flex-1 min-w-0 space-y-1">
            <h4
              className={cn(
                'font-semibold text-[var(--color-text-primary)] leading-snug',
                viewMode === 'compact' ? 'text-xs line-clamp-1' : 'text-sm line-clamp-2'
              )}
              title={displayTitle}
            >
              {displayTitle}
            </h4>

            <p className="text-xs text-[var(--color-text-secondary)] truncate">
              {displaySubtitle}
              {task.humanSize ? ` · ${task.humanSize}` : ''}
            </p>

            <div className="flex flex-wrap items-center gap-1">
              <Badge label={platformName} variant="neutral" />
              {task.format && (
                <Badge label={task.format.toUpperCase()} variant="purple" />
              )}
              <Badge label={statusBadge.label} variant={statusBadge.variant} />
            </div>
          </div>
        </div>

        {/* Active Progress Bar (omitted once completed) */}
        {!isCompleted && !task.errorMessage && (
          <div className="space-y-1">
            <ProgressBar
              label={`Download progress for ${displayTitle}`}
              isLabelHidden
              value={Math.max(0, Math.min(100, task.percent || 0))}
              max={100}
              hasValueLabel
              variant={
                isDownloading
                  ? 'accent'
                  : task.status === 'cancelled'
                    ? 'warning'
                    : 'neutral'
              }
            />
            {isDownloading && (
              <div className="flex items-center justify-between text-[11px] font-mono tabular-nums text-[var(--color-text-secondary)]">
                <span>{task.speed || 'Starting...'}</span>
                <span>{task.eta ? `ETA ${task.eta}` : ''}</span>
              </div>
            )}
          </div>
        )}

        {/* Error Box across all view modes */}
        {task.errorMessage && (
          <div className="rounded-lg px-2.5 py-2 bg-[var(--color-error-muted)] text-[var(--color-on-error-muted)] text-xs space-y-1.5">
            <div className="flex items-start gap-1.5">
              <AlertCircle className="w-3.5 h-3.5 shrink-0 mt-0.5" />
              <span className="line-clamp-2">{task.errorMessage}</span>
            </div>
            {needsCookieHint(task.errorMessage) && onOpenCookies && (
              <Button
                label="Open Cookie Manager"
                size="sm"
                variant="secondary"
                icon={<Cookie className="w-3.5 h-3.5" />}
                onClick={onOpenCookies}
              />
            )}
          </div>
        )}

        {/* Action Row */}
        <div className="flex items-center justify-between gap-2 pt-2 border-t border-[var(--color-border)]">
          <div className="flex items-center gap-1.5">
            {isCompleted && (
              <>
                <Button
                  label="Play"
                  size="sm"
                  variant="primary"
                  icon={<Play className="w-3.5 h-3.5 fill-current" />}
                  onClick={() => playMedia(task)}
                />
                <a
                  href={downloadHref}
                  download
                  className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg text-xs font-semibold border border-[var(--color-border)] bg-[var(--color-background-muted)] text-[var(--color-text-primary)] hover:bg-[var(--color-overlay-hover)] transition-colors"
                >
                  <Download className="w-3.5 h-3.5" />
                  Save
                </a>
              </>
            )}

            {isRetryable && (
              <Button
                label="Retry"
                size="sm"
                variant="secondary"
                isLoading={isActionLoading}
                icon={<RotateCcw className="w-3.5 h-3.5" />}
                onClick={handleRetry}
              />
            )}

            {(task.status === 'downloading' || task.status === 'queued') && (
              <Button
                label="Cancel"
                size="sm"
                variant="secondary"
                isLoading={isActionLoading}
                icon={<X className="w-3.5 h-3.5" />}
                onClick={handleCancel}
              />
            )}
          </div>

          <div className="flex items-center gap-1">
            {isCompleted && (
              <IconButton
                label="Delete downloaded file"
                tooltip="Delete file from disk"
                size="sm"
                variant="ghost"
                icon={<Trash2 className="w-3.5 h-3.5 text-[var(--color-text-error)]" />}
                onClick={() => setShowDiskDeleteDialog(true)}
              />
            )}
            <IconButton
              label="Remove from queue"
              tooltip="Remove from queue"
              size="sm"
              variant="ghost"
              isLoading={isActionLoading}
              icon={<X className="w-3.5 h-3.5" />}
              onClick={handleRemoveQueue}
            />
          </div>
        </div>
      </div>

      <AlertDialog
        isOpen={showDiskDeleteDialog}
        onOpenChange={setShowDiskDeleteDialog}
        title="Delete File from Server Disk?"
        description={`Permanently delete "${displayTitle}" from storage and remove this task from your queue?`}
        actionLabel="Delete File"
        actionVariant="destructive"
        isActionLoading={isActionLoading}
        onAction={handleConfirmDeleteDisk}
      />
    </>
  );
}
