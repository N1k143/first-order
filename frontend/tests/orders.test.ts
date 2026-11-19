import assert from 'node:assert/strict';
import test from 'node:test';
import { orders } from '../src/data/demo';
import { isPublicOrder, latestOpenOrder } from '../src/lib/orders';

test('latest order uses publication timestamp, not array order, without mutating input', () => {
  const older = { ...orders[0], id: 'older', publishedAt: '2026-11-19T08:00:00Z' };
  const newer = { ...older, id: 'newer', publishedAt: '2026-11-19T09:00:00Z' };
  const list = [older, newer];
  assert.equal(latestOpenOrder(list)?.id, 'newer');
  assert.equal(list[0], older);
});

test('empty, hidden, draft, closed and invalid records cannot become a featured order', () => {
  assert.equal(latestOpenOrder([]), undefined);
  for (const patch of [
    { visibility: false },
    { status: 'draft' as const },
    { status: 'closed' as const },
    { status: 'hidden' as const },
    { id: '' },
    { title: ' ' },
  ]) {
    assert.equal(latestOpenOrder([{ ...orders[0], ...patch }]), undefined);
  }
  assert.equal(isPublicOrder({ ...orders[0], status: 'hidden' }), false);
});
