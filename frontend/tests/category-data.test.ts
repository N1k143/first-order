import test from 'node:test';
import assert from 'node:assert/strict';
import { loadCategories } from '../src/lib/api/category-data';

test('category directory accepts deployed paged and legacy list responses', async () => {
  const original = globalThis.fetch;
  const items = [{ id: 'category', name: 'Образование' }];
  try {
    for (const payload of [items, { items, total: 1, page: 1, limit: 100 }]) {
      globalThis.fetch = async () => Response.json(payload);
      assert.deepEqual(await loadCategories('/categories'), items);
    }
  } finally {
    globalThis.fetch = original;
  }
});

test('category directory loads beyond the first page and rejects a broken response', async () => {
  const original = globalThis.fetch;
  const items = Array.from({ length: 101 }, (_, index) => ({
    id: String(index),
    name: `Сфера ${index}`,
  }));
  try {
    globalThis.fetch = async (input) => {
      const page = new URL(String(input), 'http://localhost').searchParams.get('page');
      return Response.json({
        items: page === '1' ? items.slice(0, 100) : items.slice(100),
        total: 101,
      });
    };
    assert.deepEqual(await loadCategories('/categories'), items);
    globalThis.fetch = async () => Response.json({ items: { error: 'invalid list' } });
    await assert.rejects(loadCategories('/categories'), /справочник/);
  } finally {
    globalThis.fetch = original;
  }
});
