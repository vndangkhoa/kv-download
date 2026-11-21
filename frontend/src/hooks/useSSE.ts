'use client';

import { useEffect, useState, useCallback, useRef } from 'react';
import { DownloadTask } from '@/types';
import { fetchQueue } from '@/lib/api';

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

  // Compute speed & task array
  useEffect(() => {
    const activeTasks = Object.values(tasks).filter((t) => t.status === 'downloading' && t.speed);
    if (activeTasks.length > 0 && activeTasks[0].speed) {
      setGlobalSpeed(activeTasks[0].speed);
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
