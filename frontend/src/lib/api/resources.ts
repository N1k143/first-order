'use client';
import { useEffect, useState } from 'react';
import { apiRequest } from './client';
import { DEMO, refreshAppState } from '@/lib/state/store';

const invalidationListeners = new Set<() => void>();
export function useResource<T>(
  path: string,
  demo: T,
  loader?: (path: string, signal: AbortSignal) => Promise<T>,
) {
  const [remote, setRemote] = useState<{ path: string; data?: T; error?: string }>({ path: '' });
  const [version, setVersion] = useState(0);
  useEffect(() => {
    const invalidate = () => setVersion((value) => value + 1);
    invalidationListeners.add(invalidate);
    return () => {
      invalidationListeners.delete(invalidate);
    };
  }, []);
  useEffect(() => {
    if (DEMO) return;
    const controller = new AbortController();
    (loader ? loader(path, controller.signal) : apiRequest<T>(path, { signal: controller.signal }))
      .then((data) => {
        if (!controller.signal.aborted) setRemote({ path, data });
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setRemote({
            path,
            error: error instanceof Error ? error.message : 'Не удалось загрузить данные.',
          });
      });
    return () => controller.abort();
  }, [path, version, loader]);
  return {
    data: DEMO ? demo : remote.path === path ? remote.data : undefined,
    error: DEMO ? undefined : remote.path === path ? remote.error : undefined,
    loading: !DEMO && (remote.path !== path || (remote.data === undefined && !remote.error)),
    reload: () => setVersion((value) => value + 1),
  };
}
export async function mutate(
  path: string,
  method: 'POST' | 'PATCH' | 'DELETE' | 'PUT',
  body: unknown,
  local: () => void,
) {
  if (DEMO) {
    local();
    return;
  }
  await apiRequest(path, { method, body });
  invalidationListeners.forEach((listener) => listener());
  await refreshAppState();
}
export function safeHref(value: string) {
  try {
    const url = new URL(value);
    return ['http:', 'https:'].includes(url.protocol) ? url.href : undefined;
  } catch {
    return undefined;
  }
}
