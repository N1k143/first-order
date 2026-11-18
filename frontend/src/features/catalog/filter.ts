import type { Company } from '@/types/domain';
export function filterCompanies(items: Company[], params: URLSearchParams) {
  const query = (params.get('q') || '').trim().toLocaleLowerCase('ru');
  const city = params.get('city') ?? 'Астана',
    category = params.get('category') || '',
    status = params.get('status') ?? 'not_found';
  const result = items.filter(
    (c) =>
      (!city || c.city === city) &&
      (!category || c.category === category) &&
      (!status || c.websiteStatus === status) &&
      (!query ||
        `${c.name} ${c.category} ${c.description}`.toLocaleLowerCase('ru').includes(query)),
  );
  return result.sort(
    params.get('sort') === 'name'
      ? (a, b) => a.name.localeCompare(b.name, 'ru')
      : (a, b) => b.checkedAt.localeCompare(a.checkedAt),
  );
}
