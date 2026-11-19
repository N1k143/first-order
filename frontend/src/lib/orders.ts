import type { Order } from '@/types/domain';

/** Public cards must never link to drafts, hidden records or missing IDs. */
export function isPublicOrder(order: Order) {
  return Boolean(
    order.id &&
    order.title?.trim() &&
    order.visibility !== false &&
    ['open', 'in_progress', 'completed', 'closed'].includes(order.status),
  );
}

export function latestOpenOrder(orders: Order[]) {
  return orders
    .filter((order) => isPublicOrder(order) && order.status === 'open')
    .reduce<Order | undefined>((latest, order) => {
      const timestamp = Date.parse(order.publishedAt) || 0;
      const previous = latest ? Date.parse(latest.publishedAt) || 0 : -1;
      return timestamp > previous ? order : latest;
    }, undefined);
}
