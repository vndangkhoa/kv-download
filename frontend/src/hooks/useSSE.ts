'use client';

import { useEffect, useState, useCallback, useRef } from 'react';
import { DownloadTask } from '@/types';
import { fetchQueue } from '@/lib/api';

function parseSpeedBytesPerSec(raw?: string): number {
  if (!raw) return 0;
  const m = raw.trim().match(/^([0-9]+(?:\.[0-9]+)?)\s*(GiB|GB|MiB|MB|KiB|KB|B)\s*\/?\s*s?$/i);
  if (!m) return 0;
  const val = parseFloat(m[1]);
  if (!isFinite(val) || val <= 0) return 0;
  const unit = m[2].toLowerCase();
  switch (unit) {
    case 'gib':
      return val * 1024 * 1024 * 1024;
    case 'gb':
      return val * 1000 * 1000 * 1000;
    case 'mib':
      return val * 1024 * 1024;
    case 'mb':
      return val * 1000 * 1000;
    case 'kib':
      return val * 1024;
    case 'kb':
      return val * 1000;
    case 'b':
      return val;
    default:
      return 0;
  }
}

function formatAggregatedSpeed(bps: number): string {
  const gib = 1024 * 1024 * 1024;
  const mib = 1024 * 1024;
  const kib = 1024;
  if (bps >= gib) return `${(bps / gib).toFixed(2)} GiB/s`;
  if (bps >= mib) return `${(bps / mib).toFixed(2)} MiB/s`;
  if (bps >= kib) return `${(bps / kib).toFixed(1)} KiB/s`;
  return `${Math.round(bps)} B/s`;
}

function sumActiveSpeeds(activeTasks: DownloadTask[]): string {
  if (activeTasks.length === 0) return '0 KB/s';
  let totalBps = 0;
  for (const t of activeTasks) {
    totalBps += parseSpeedBytesPerSec(t.speed);
  }
  if (totalBps > 0) {
    return formatAggregatedSpeed(totalBps);
  }
  return activeTasks[0]?.speed || '0 KB/s';
}

export function useSSE() {
  const [tasks, setTasks] = useState<Record<string, DownloadTask>>({});
  const [isConnected, setIsConnected] = useState(false);
  const [globalSpeed, setGlobalSpeed] = useState('0 KB/s');
  const eventSourceRef = useRef<EventSource | null>(null);

  const syncQueue = useCallback(async () => {
    try {
      const q = await fetchQueue();
      const map: Record<string, DownloadTask> = {};
      q.forEach((t) => {
        map[t.id] = t;
      });
      setTasks(map);
    } catch (e) {
      console.error('Failed to sync queue:', e);
    }
  }, []);

  useEffect(() => {
    syncQueue();

    let es: EventSource | null = null;
    let reconnectTimeout: NodeJS.Timeout;

    const connect = () => {
      es = new EventSource('/api/events');
      eventSourceRef.current = es;

      es.onopen = () => {
        setIsConnected(true);
      };

      es.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (data.type === 'init' && Array.isArray(data.tasks)) {
            const map: Record<string, DownloadTask> = {};
            data.tasks.forEach((t: DownloadTask) => {
              map[t.id] = t;
            });
            setTasks(map);
          } else if (data.type === 'task_update' && data.task) {
            setTasks((prev) => ({
              ...prev,
              [data.task.id]: data.task,
            }));
          } else if (data.type === 'task_deleted' && data.taskId) {
            setTasks((prev) => {
              const copy = { ...prev };
              delete copy[data.taskId];
              return copy;
            });
          }
        } catch (err) {
          console.error('SSE JSON error:', err);
        }
      };

      es.onerror = () => {
        setIsConnected(false);
        es?.close();
        reconnectTimeout = setTimeout(connect, 3000);
      };
    };

    connect();

    return () => {
      clearTimeout(reconnectTimeout);
      es?.close();
    };
  }, [syncQueue]);

  // Compute aggregated active download speed across all active tasks
  useEffect(() => {
    const activeTasks = Object.values(tasks).filter((t) => t.status === 'downloading' && t.speed);
    if (activeTasks.length > 0) {
      setGlobalSpeed(sumActiveSpeeds(activeTasks));
    } else {
      setGlobalSpeed('0 KB/s');
    }
  }, [tasks]);

  const taskList = Object.values(tasks).sort(
    (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
  );

  return {
    tasks,
    taskList,
    isConnected,
    globalSpeed,
    syncQueue,
    setTasks,
  };
}
