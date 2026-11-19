'use client';

import { useState } from 'react';
import { categories } from '@/data/demo';
import { useResource } from '@/lib/api/resources';
import { companyFields } from '@/lib/api/form-fields';
import { useAppState, updateDemo } from '@/lib/state/store';
import { Field, Select, Notice, ResourceState } from '@/components/ui/primitives';

export function CompanyEditor() {
  const state = useAppState();
  const directory = useResource('/categories', categories);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  if (!directory.data) return <ResourceState error={directory.error} reload={directory.reload} />;
  return (
    <form
      className="card stack"
      onSubmit={(event) => {
        event.preventDefault();
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
        updateDemo((s) => ({
          ...s,
          company: {
            ...s.company,
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
          orders: s.orders.map((order) =>
            order.companyId === s.company.id
              ? { ...order, companyName: fields.name, city: fields.city }
              : order,
          ),
        }));
        setError('');
        setMessage('Данные компании сохранены в этом браузере.');
      }}
    >
      <h3>О компании</h3>
      <Field
        label="Название компании"
        name="name"
        defaultValue={state.company.name}
        required
        maxLength={120}
      />
      <Field label="Город" name="city" defaultValue={state.company.city} required maxLength={100} />
      <Select
        label="Сфера деятельности"
        name="category_id"
        defaultValue={state.company.categoryId || ''}
      >
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
        defaultValue={state.company.address}
        maxLength={300}
      />
      <div className="form-grid">
        <Field
          label="Email для связи"
          type="email"
          name="email"
          defaultValue={state.company.email}
        />
        <Field label="Телефон" type="tel" name="phone" defaultValue={state.company.phone} />
      </div>
      <Field
        label="Сайт компании"
        type="url"
        pattern="https?://.*"
        name="website"
        defaultValue={state.company.website}
        placeholder="https://example.com"
      />
      <Field
        label="Ссылка на источник"
        type="url"
        pattern="https?://.*"
        name="source_url"
        defaultValue={state.company.sourceUrl || ''}
        placeholder="https://example.com/company"
      />
      <p className="small muted">Статус проверки сайта и публикации меняется после модерации.</p>
      <button className="button align-start">Сохранить изменения</button>
      {error && <Notice error>{error}</Notice>}
      {message && <Notice>{message}</Notice>}
    </form>
  );
}
