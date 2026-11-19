'use client';

import { useEffect, useMemo, useState } from 'react';
import { apiRequest } from '@/lib/api/client';
import { useAppState } from '@/lib/state/store';
import type { Application, Order } from '@/types/domain';
import type { ApplicationRow, OrderRow } from '@/types/database';

export interface WorkspacePage<T> {
  items: T[];
  total: number;
  page: number;
  limit: number;
}
export type CompanyOrderRow = OrderRow & {
  company_name: string;
  city: string;
  applications_count: number;
};
export type CompanyApplicationRow = ApplicationRow & {
  developer_name: string;
  order_title: string;
  company_name: string;
};
export interface CompanyMemberRow {
  company_id: string;
  user_id: string;
  member_role: 'owner' | 'representative';
  name: string;
  created_at: string;
}

/** Read every server page so company totals and membership controls are not capped at 100. */
export async function fetchWorkspacePages<T>(path: string, signal: AbortSignal): Promise<T[]> {
  const rows: T[] = [];
  for (let page = 1; ; page++) {
    const response = await apiRequest<WorkspacePage<T>>(
      `${path}${path.includes('?') ? '&' : '?'}limit=100&page=${page}`,
      { signal },
    );
    if (!Array.isArray(response.items) || !Number.isFinite(response.total) || response.total < 0)
      throw new Error('Сервер вернул некорректный список.');
    rows.push(...response.items);
    if (rows.length >= response.total || response.items.length === 0) return rows;
  }
}

export function useWorkspaceCollection<T>(path: string | null) {
  const state = useAppState();
  const [version, setVersion] = useState(0);
  const request = useMemo(() => ({ path, state, version }), [path, state, version]);
  const [result, setResult] = useState<{ request: typeof request; data?: T[]; error?: string }>();
  useEffect(() => {
    if (!request.path) return;
    const controller = new AbortController();
    fetchWorkspacePages<T>(request.path, controller.signal)
      .then((data) => {
        if (!controller.signal.aborted) setResult({ request, data });
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setResult({
            request,
            error: error instanceof Error ? error.message : 'Не удалось загрузить данные компании.',
          });
      });
    return () => controller.abort();
  }, [request]);
  return {
    data: result?.request === request ? result.data : undefined,
    error: result?.request === request ? result.error : undefined,
    loading: Boolean(path) && result?.request !== request,
    reload: () => setVersion((value) => value + 1),
  };
}

export function workspaceOrder(row: CompanyOrderRow): Order {
  const status = row.status === 'contractor_selected' ? 'in_progress' : row.status;
  return {
    id: row.id,
    companyId: row.company_id,
    companyName: row.company_name,
    city: row.city,
    title: row.title,
    description: row.description,
    budgetMin: row.budget_min,
    budgetMax: row.budget_max,
    deadline: row.deadline,
    status: status as Order['status'],
    visibility: row.visibility,
    selectedApplicationId: row.selected_application_id,
    applicationsCount: Number(row.applications_count) || 0,
    publishedAt: row.created_at,
  };
}
export function workspaceApplication(row: CompanyApplicationRow): Application {
  const status =
    row.status === 'submitted' ? 'pending' : row.status === 'accepted' ? 'selected' : row.status;
  return {
    id: row.id,
    orderId: row.order_id,
    profileId: row.developer_id,
    name: row.developer_name,
    letter: row.message,
    price: row.proposed_price,
    deadline: row.proposed_deadline,
    status: status as Application['status'],
    orderTitle: row.order_title,
    companyName: row.company_name,
  };
}
export function useCompanyOrders(companyId: string) {
  const resource = useWorkspaceCollection<CompanyOrderRow>(
    `/v1/orders?mine=true&company_id=${encodeURIComponent(companyId)}`,
  );
  const orders = useMemo(() => resource.data?.map(workspaceOrder), [resource.data]);
  return { ...resource, data: orders };
}
