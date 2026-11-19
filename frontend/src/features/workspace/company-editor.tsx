'use client';

import { useState } from 'react';
import Link from 'next/link';
import { mutate, useResource } from '@/lib/api/resources';
import { useCategories } from '@/lib/api/categories';
import { companyFields } from '@/lib/api/form-fields';
import { DEMO, useAppState, updateDemo } from '@/lib/state/store';
import { Field, Select, Notice, ResourceState, EmptyState } from '@/components/ui/primitives';
import type { Company } from '@/types/domain';

function CompanyForm({ company }: { company: Company }) {
  const directory = useCategories();
  const [message, setMessage] = useState(''),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false);
  async function save(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setMessage('');
    setError('');
    const fields = companyFields(new FormData(event.currentTarget));
    if (!fields.name || !fields.city) {
      setError('Укажите название компании и город.');
      return;
    }
    const category = directory.data?.find((item) => item.id === fields.category_id);
    if (fields.category_id && !category) {
      setError('Выберите категорию из справочника.');
      return;
    }
    setBusy(true);
    try {
      await mutate(`/companies/${encodeURIComponent(company.id)}`, 'PATCH', fields, () =>
        updateDemo((state) => ({
          ...state,
          company: {
            ...state.company,
            name: fields.name,
            city: fields.city,
            categoryId: fields.category_id,
            category: category?.name || 'Без категории',
            address: fields.address || '',
            phone: fields.phone || '',
            email: fields.email || '',
            website: fields.website_url || '',
            sourceUrl: fields.source_url,
            initials: fields.name
              .split(/\s+/)
              .map((part) => part[0])
              .slice(0, 2)
              .join('')
              .toUpperCase(),
          },
          orders: state.orders.map((order) =>
            order.companyId === company.id
              ? { ...order, companyName: fields.name, city: fields.city }
              : order,
          ),
        })),
      );
      setMessage(
        DEMO ? 'Данные компании сохранены в этом браузере.' : 'Данные компании сохранены.',
      );
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Не удалось сохранить компанию.');
    } finally {
      setBusy(false);
    }
  }
  if (!directory.data) return <ResourceState error={directory.error} reload={directory.reload} />;
  return (
    <form className="card stack" onSubmit={save}>
      <h3>О компании</h3>
      <Field
        label="Название компании"
        name="name"
        defaultValue={company.name}
        required
        maxLength={240}
      />
      <Field label="Город" name="city" defaultValue={company.city} required maxLength={240} />
      <Select label="Сфера деятельности" name="category_id" defaultValue={company.categoryId || ''}>
        <option value="">Без категории</option>
        {directory.data.map((category) => (
          <option key={category.id} value={category.id}>
            {category.name}
          </option>
        ))}
      </Select>
      <Field
        label="Адрес компании"
        name="address"
        defaultValue={company.address || ''}
        maxLength={4000}
      />
      <div className="form-grid">
        <Field
          label="Email для связи"
          type="email"
          name="email"
          defaultValue={company.email || ''}
        />
        <Field label="Телефон" type="tel" name="phone" defaultValue={company.phone || ''} />
      </div>
      <Field
        label="Сайт компании"
        type="url"
        pattern="https?://.*"
        name="website"
        defaultValue={company.website || ''}
        placeholder="https://example.com"
      />
      <Field
        label="Ссылка на источник"
        type="url"
        pattern="https?://.*"
        name="source_url"
        defaultValue={company.sourceUrl || ''}
        placeholder="https://example.com/company"
      />
      <p className="small muted">Статус проверки сайта и публикации меняется после модерации.</p>
      <div className="button-row">
        <button className="button align-start" disabled={busy}>
          {busy ? 'Сохраняем…' : 'Сохранить изменения'}
        </button>
        <Link className="button secondary" href={`/companies/${company.id}`}>
          Открыть карточку компании
        </Link>
      </div>
      {error && <Notice error>{error}</Notice>}
      {message && <Notice>{message}</Notice>}
    </form>
  );
}
function ApiCompanyEditor({ companyId }: { companyId: string }) {
  const company = useResource<Company | undefined>(
    `/companies/${encodeURIComponent(companyId)}`,
    undefined,
  );
  return company.data ? (
    <CompanyForm key={companyId} company={company.data} />
  ) : (
    <ResourceState error={company.error} reload={company.reload} />
  );
}
export function CompanyEditor({ companyId }: { companyId?: string }) {
  const state = useAppState();
  if (DEMO) return <CompanyForm company={state.company} />;
  return companyId ? (
    <ApiCompanyEditor companyId={companyId} />
  ) : (
    <EmptyState
      title="Выберите компанию"
      description="Редактирование доступно участникам компании."
    />
  );
}
