'use client';

import { useEffect, useState } from 'react';
import { apiRequest } from '@/lib/api/client';
import { DEMO, useAppState, useStoreReady } from '@/lib/state/store';
import { latestOpenOrder } from '@/lib/orders';
import type { Order } from '@/types/domain';

/** The API sorts by creation time; skip private records visible to their owner. */
export function useLatestOrder() {
  const state = useAppState();
  const ready = useStoreReady();
  const [remote, setRemote] = useState<Order>();
  useEffect(() => {
    if (DEMO) return;
    const controller = new AbortController();
    async function load() {
      const limit = 100;
      for (let page = 1; !controller.signal.aborted; page++) {
        const rows = await apiRequest<Order[]>(`/orders?status=open&limit=${limit}&page=${page}`, {
          signal: controller.signal,
        });
        const latest = latestOpenOrder(rows);
        if (latest || rows.length < limit) {
          if (!controller.signal.aborted) setRemote(latest);
          return;
        }
      }
    }
    void load().catch(() => {
      if (!controller.signal.aborted) setRemote(undefined);
    });
    return () => controller.abort();
  }, []);
  return DEMO ? (ready ? latestOpenOrder(state.orders) : undefined) : remote;
}
