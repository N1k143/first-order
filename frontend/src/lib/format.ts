export function money(value: number | null | undefined) {
  return value == null ? 'По договорённости' : new Intl.NumberFormat('ru-RU').format(value) + ' ₸';
}
export function budgetLabel(min: number | null, max: number | null) {
  if (min == null && max == null) return 'Бюджет по договорённости';
  if (min == null) return `До ${money(max)}`;
  if (max == null) return `От ${money(min)}`;
  return min === max ? money(min) : `${money(min)} – ${money(max)}`;
}
export function dateLabel(value: string | null | undefined) {
  if (!value) return 'Не указано';
  const date = new Date(value.length === 10 ? `${value}T12:00:00Z` : value);
  if (!Number.isFinite(date.getTime())) return 'Не указано';
  return date.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', timeZone: 'UTC' });
}
export function deadlineLabel(value: string | null | undefined) {
  return value ? `До ${dateLabel(value)}` : 'Срок по договорённости';
}
