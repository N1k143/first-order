import { apiRequest } from './client';

export interface Category {
  id: string;
  name: string;
}

/** Accept both deployed DTO versions; never pass a pagination object to a list UI. */
export async function loadCategories(path: string, signal?: AbortSignal): Promise<Category[]> {
  const rows: Category[] = [];
  const seen = new Set<string>();
  const limit = 100;
  for (let page = 1; page <= 1000; page++) {
    const query = new URLSearchParams({ limit: String(limit), page: String(page) });
    const value = await apiRequest<unknown>(`${path}?${query}`, { signal });
    const envelope = value as { items?: unknown; total?: unknown } | null;
    const items = Array.isArray(value) ? value : envelope?.items;
    if (
      !Array.isArray(items) ||
      items.some((item) => !item || typeof item.id !== 'string' || typeof item.name !== 'string')
    ) {
      throw new Error('Не удалось прочитать справочник сфер деятельности. Попробуйте ещё раз.');
    }
    const before = seen.size;
    for (const item of items as Category[]) {
      if (!seen.has(item.id)) {
        seen.add(item.id);
        rows.push(item);
      }
    }
    if (
      items.length < limit ||
      (typeof envelope?.total === 'number' && rows.length >= envelope.total)
    )
      return rows;
    if (seen.size === before)
      throw new Error('Не удалось загрузить следующую страницу справочника.');
  }
  throw new Error('Справочник слишком большой. Уточните данные на сервере.');
}
