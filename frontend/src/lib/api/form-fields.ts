import type {
  CompanyUpdate,
  ContactUpdate,
  OrderUpdate,
  ApplicationUpdate,
} from '@/types/database';

export function optionalText(value: FormDataEntryValue | null): string | null {
  return typeof value === 'string' ? value.trim() || null : null;
}
export function optionalNumber(value: FormDataEntryValue | null): number | null {
  const text = optionalText(value);
  if (text === null) return null;
  const number = Number(text);
  if (!Number.isFinite(number) || number < 0) throw new Error('Укажите неотрицательное число.');
  return number;
}
export function optionalDate(value: FormDataEntryValue | null): string | null {
  const text = optionalText(value);
  if (text === null) return null;
  if (
    !/^\d{4}-\d{2}-\d{2}$/.test(text) ||
    !Number.isFinite(Date.parse(text)) ||
    new Date(text).toISOString().slice(0, 10) !== text
  )
    throw new Error('Укажите корректную дату.');
  return text;
}
export function companyFields(form: FormData): CompanyUpdate {
  return {
    name: optionalText(form.get('name')) || '',
    city: optionalText(form.get('city')) || '',
    category_id: optionalText(form.get('category_id')),
    address: optionalText(form.get('address')),
    phone: optionalText(form.get('phone')),
    email: optionalText(form.get('email')),
    website_url: optionalText(form.get('website')),
    source_url: optionalText(form.get('source_url')),
  };
}
export function orderFields(form: FormData): OrderUpdate {
  return {
    title: optionalText(form.get('title')) || '',
    description: optionalText(form.get('description')) || '',
    budget_min: optionalNumber(form.get('min')),
    budget_max: optionalNumber(form.get('max')),
    deadline: optionalDate(form.get('deadline')),
  };
}
export function applicationFields(form: FormData): ApplicationUpdate {
  return {
    message: optionalText(form.get('letter')) || '',
    proposed_price: optionalNumber(form.get('price')),
    proposed_deadline: optionalDate(form.get('deadline')),
  };
}
export function contactFields(form: FormData, previousDate?: string | null): ContactUpdate {
  const date = optionalDate(form.get('contacted_at'));
  return {
    notes: optionalText(form.get('note')),
    contacted_at: date
      ? previousDate?.slice(0, 10) === date
        ? previousDate
        : `${date}T12:00:00.000Z`
      : null,
    status: optionalText(form.get('status')) || '',
  };
}
