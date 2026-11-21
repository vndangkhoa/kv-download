'use client';

import React, { useState } from 'react';
import { TaskGroup, ViewMode } from '@/types';
import { TaskCard } from './TaskCard';
import { Card } from '@astryxdesign/core/Card';
import { Button } from '@astryxdesign/core/Button';
import { IconButton } from '@astryxdesign/core/IconButton';
import { Badge } from '@astryxdesign/core/Badge';
import { ProgressBar } from '@astryxdesign/core/ProgressBar';
import { Dialog, DialogHeader } from '@astryxdesign/core/Dialog';
import {
  cancelTaskGroup,
  deleteTaskGroup,
  getGroupZipUrl,
  retryTaskGroup,
} from '@/lib/api';
import {
  ChevronDown,
  ChevronRight,
  FolderArchive,
  Layers,
  RotateCcw,
  StopCircle,
  Trash2,
  X,
} from 'lucide-react';
import { formatBytes, cn } from '@/lib/utils';

interface TaskGroupCardProps {
  group: TaskGroup;
  viewMode: ViewMode;
  onRefresh: () => void;
  onOpenCookies?: () => void;
}

const INITIAL_VISIBLE_TASKS = 12;

function getGroupBadgeLabel(groupType: string): string {
  switch (groupType) {
    case 'channel':
      return 'Channel';
    case 'playlist':
      return 'Playlist';
    case 'album':
      return 'Album';
    case 'batch':
      return 'Batch';
    default:
      return 'Group';
  }
}

export function TaskGroupCard({
  group,
  viewMode,
  onRefresh,
  onOpenCookies,
}: TaskGroupCardProps) {
  const [isExpanded, setIsExpanded] = useState(
    group.downloadingCount > 0 || group.tasks.length <= 4
  );
  const [visibleCount, setVisibleCount] = useState(INITIAL_VISIBLE_TASKS);
  const [isBusy, setIsBusy] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);

  const thumbnails = group.tasks
    .map((t) => t.thumbnail || `/thumbnail?id=${encodeURIComponent(t.id)}`)
    .filter(Boolean)
    .slice(0, 4);

  const hasActive = group.downloadingCount > 0 || group.queuedCount > 0;
  const hasRetryable = group.failedCount > 0 || group.cancelledCount > 0;
  const allCompleted = group.completedCount === group.tasks.length && group.tasks.length > 0;

  const activeSpeedText = group.tasks.find((t) => t.status === 'downloading' && t.speed)?.speed;

  const handleCancelGroup = async () => {
    setIsBusy(true);
    try {
      await cancelTaskGroup(group.groupId);
      onRefresh();
    } finally {
      setIsBusy(false);
    }
  };

  const handleRetryGroup = async () => {
    setIsBusy(true);
    try {
      await retryTaskGroup(group.groupId);
      onRefresh();
    } finally {
      setIsBusy(false);
    }
  };

  const handleDeleteGroup = async (deleteFiles: boolean) => {
    setIsBusy(true);
    try {
      await deleteTaskGroup(group.groupId, deleteFiles);
      setShowDeleteDialog(false);
      onRefresh();
    } finally {
      setIsBusy(false);
    }
  };

  const visibleTasks = group.tasks.slice(0, visibleCount);
  const remainingCount = Math.max(0, group.tasks.length - visibleTasks.length);

  return (
    <Card padding={3} elevation="low">
      <div className="space-y-3">
        {/* Group Header */}
        <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-3">
          <button
            type="button"
            onClick={() => setIsExpanded((prev) => !prev)}
            className="flex items-start sm:items-center gap-3 min-w-0 flex-1 text-left group"
          >
            {/* 2x2 Thumbnail Collage */}
            <div className="w-16 h-12 sm:w-20 sm:h-14 rounded-lg overflow-hidden border border-[var(--color-border)] bg-[var(--color-background-muted)] shrink-0 grid grid-cols-2 grid-rows-2 gap-px">
              {thumbnails.length > 0 ? (
                [0, 1, 2, 3].map((slot) => {
                  const thumb = thumbnails[slot % thumbnails.length];
                  return (
                    <div
                      key={slot}
                      className="w-full h-full bg-[var(--color-background-surface)] overflow-hidden"
                    >
                      <img
                        src={thumb}
                        alt=""
                        loading="lazy"
                        className="w-full h-full object-cover"
                        onError={(e) => {
                          (e.currentTarget as HTMLImageElement).style.display = 'none';
                        }}
                      />
                    </div>
                  );
                })
              ) : (
                <div className="col-span-2 row-span-2 flex items-center justify-center text-[var(--color-icon-accent)]">
                  <Layers className="w-5 h-5" />
                </div>
              )}
            </div>

            <div className="flex-1 min-w-0 space-y-1">
              <div className="flex items-center gap-1.5 min-w-0">
                {isExpanded ? (
                  <ChevronDown className="w-4 h-4 shrink-0 text-[var(--color-icon-secondary)]" />
                ) : (
                  <ChevronRight className="w-4 h-4 shrink-0 text-[var(--color-icon-secondary)]" />
                )}
                <h3 className="text-sm sm:text-base font-bold text-[var(--color-text-primary)] truncate">
                  {group.title}
                </h3>
              </div>

              <div className="flex flex-wrap items-center gap-1.5">
                {group.platform && <Badge label={group.platform} variant="neutral" />}
                <Badge label={getGroupBadgeLabel(group.groupType)} variant="purple" />
                <span className="text-xs font-mono tabular-nums text-[var(--color-text-secondary)]">
                  {group.tasks.length} items
                </span>
                {group.totalBytes > 0 && (
                  <span className="text-xs font-mono tabular-nums text-[var(--color-text-secondary)]">
                    · {formatBytes(group.totalBytes)}
                  </span>
                )}
                {group.completedCount > 0 && (
                  <Badge label={`${group.completedCount} done`} variant="success" />
                )}
                {group.downloadingCount > 0 && (
                  <Badge
                    label={
                      activeSpeedText
                        ? `${group.downloadingCount} active · ${activeSpeedText}`
                        : `${group.downloadingCount} active`
                    }
                    variant="info"
                  />
                )}
                {group.queuedCount > 0 && (
                  <Badge label={`${group.queuedCount} queued`} variant="neutral" />
                )}
                {group.failedCount > 0 && (
                  <Badge label={`${group.failedCount} failed`} variant="error" />
                )}
                {group.cancelledCount > 0 && (
                  <Badge label={`${group.cancelledCount} cancelled`} variant="warning" />
                )}
              </div>
            </div>
          </button>

          {/* Group Actions */}
          <div className="flex flex-wrap items-center gap-1.5 shrink-0 pl-0 lg:pl-2">
            {group.completedCount > 0 && (
              <a
                href={getGroupZipUrl(group.groupId)}
                download
                className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-semibold border border-[var(--color-border)] bg-[var(--color-background-muted)] text-[var(--color-text-primary)] hover:bg-[var(--color-overlay-hover)] transition-colors"
              >
                <FolderArchive className="w-3.5 h-3.5 text-[var(--color-icon-accent)]" />
                ZIP ({group.completedCount})
              </a>
            )}

            {hasRetryable && (
              <Button
                label="Retry Failed"
                size="sm"
                variant="secondary"
                isDisabled={isBusy}
                icon={<RotateCcw className="w-3.5 h-3.5" />}
                onClick={handleRetryGroup}
              />
            )}

            {hasActive && (
              <Button
                label="Cancel Active"
                size="sm"
                variant="secondary"
                isDisabled={isBusy}
                icon={<StopCircle className="w-3.5 h-3.5" />}
                onClick={handleCancelGroup}
              />
            )}

            <IconButton
              label="Delete group"
              size="sm"
              variant="ghost"
              tooltip="Remove group or delete files"
              isDisabled={isBusy}
              icon={<Trash2 className="w-4 h-4 text-[var(--color-text-error)]" />}
              onClick={() => setShowDeleteDialog(true)}
            />
          </div>
        </div>

        {/* Aggregate Progress Bar (hidden when entire group is completed) */}
        {!allCompleted && (
          <ProgressBar
            label={`${group.title} aggregate progress`}
            isLabelHidden
            value={group.overallPercent}
            max={100}
            hasValueLabel
            variant={
              group.failedCount > 0 && group.downloadingCount === 0
                ? 'error'
                : group.downloadingCount > 0
                  ? 'accent'
                  : 'neutral'
            }
          />
        )}

        {/* Expanded Nested Items */}
        {isExpanded && (
          <div className="pt-2 border-t border-[var(--color-border)] space-y-2">
            <div
              className={cn(
                viewMode === 'list'
                  ? 'flex flex-col gap-1.5'
                  : viewMode === 'compact'
                    ? 'grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2.5'
                    : 'grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3'
              )}
            >
              {visibleTasks.map((task) => (
                <TaskCard
                  key={task.id}
                  task={task}
                  viewMode={viewMode}
                  onRefresh={onRefresh}
                  onOpenCookies={onOpenCookies}
                />
              ))}
            </div>

            {remainingCount > 0 && (
              <div className="flex items-center justify-center gap-2 pt-1">
                <Button
                  label={`Show 24 More (${remainingCount} remaining)`}
                  size="sm"
                  variant="secondary"
                  onClick={() => setVisibleCount((prev) => prev + 24)}
                />
                {remainingCount > 24 && (
                  <Button
                    label={`Show All ${group.tasks.length}`}
                    size="sm"
                    variant="ghost"
                    onClick={() => setVisibleCount(group.tasks.length)}
                  />
                )}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Group Deletion Confirmation Dialog */}
      <Dialog
        isOpen={showDeleteDialog}
        onOpenChange={setShowDeleteDialog}
        width={440}
        purpose="info"
      >
        <div className="flex flex-col">
          <DialogHeader
            title="Remove Download Group"
            subtitle={group.title}
            onOpenChange={setShowDeleteDialog}
          />
          <div className="p-4 text-sm text-[var(--color-text-secondary)] space-y-2">
            <p>
              Choose whether to remove all {group.tasks.length} entries in this group from the
              queue only, or also delete their downloaded files from server storage.
            </p>
          </div>
          <div className="px-4 py-3 border-t border-[var(--color-border)] bg-[var(--color-background-surface)] flex flex-wrap items-center justify-end gap-2">
            <Button
              label="Cancel"
              size="sm"
              variant="secondary"
              icon={<X className="w-3.5 h-3.5" />}
              onClick={() => setShowDeleteDialog(false)}
            />
            <Button
              label="Queue Only"
              size="sm"
              variant="secondary"
              isLoading={isBusy}
              onClick={() => handleDeleteGroup(false)}
            />
            {group.completedCount > 0 && (
              <Button
                label="Delete Group & Files"
                size="sm"
                variant="destructive"
                isLoading={isBusy}
                icon={<Trash2 className="w-3.5 h-3.5" />}
                onClick={() => handleDeleteGroup(true)}
              />
            )}
          </div>
        </div>
      </Dialog>
    </Card>
  );
}
