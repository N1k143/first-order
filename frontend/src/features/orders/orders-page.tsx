'use client';

import { useSearchParams } from 'next/navigation';
import { useAppState } from '@/lib/state/store';
import { useResource } from '@/lib/api/resources';
import { PageHeading, Select, Field, ResourceState, EmptyState } from '@/components/ui/primitives';
import { isPublicOrder } from '@/lib/orders';
import { OrderCard } from './order-card';
import styles from '@/features/catalog/catalog.module.css';

export function OrdersPage() {
  const state = useAppState();
  const p = useSearchParams();

  function change(values: Record<string, string>) {
    const q = new URLSearchParams(window.location.search);

    for (const [key, value] of Object.entries(values)) {
      q.set(key, value);
    }

    window.history.replaceState(null, '', '/orders?' + q.toString());
  }

  const rows = state.orders.filter(
    (order) =>
      isPublicOrder(order) &&
      (!p.get('q') ||
        `${order.title} ${order.description}`.toLowerCase().includes(p.get('q')!.toLowerCase())) &&
      (!p.get('city') || p.get('city') === order.city) &&
      (!p.get('min') ||
        (order.budgetMax !== null
          ? order.budgetMax >= Number(p.get('min'))
          : order.budgetMin !== null)) &&
      (!p.get('max') ||
        (order.budgetMin !== null
          ? order.budgetMin <= Number(p.get('max'))
          : order.budgetMax !== null)) &&
      (!p.get('status') || order.status === p.get('status')),
  );

  const resource = useResource('/orders?' + p.toString(), rows);
  const visibleOrders = resource.data?.filter(isPublicOrder);

  const filters = (
    <form
      key={p.toString()}
      className="card stack"
      onSubmit={(event) => {
        event.preventDefault();

        const formData = new FormData(event.currentTarget);
        const values = Object.fromEntries(
          [...formData].map(([key, value]) => [key, String(value)]),
        );

        change(values);
      }}
    >
      <h3>Фильтры</h3>

      <Select name="city" label="Город" defaultValue={p.get('city') || ''}>
        <option value="">Все города</option>
        <option>Астана</option>
        <option>Алматы</option>
      </Select>

      <Field
        label="Бюджет от, ₸"
        name="min"
        type="number"
        min={0}
        placeholder="От 50 000"
        defaultValue={p.get('min') || ''}
      />

      <Field
        label="Бюджет до, ₸"
        name="max"
        type="number"
        min={0}
        placeholder="До 300 000"
        defaultValue={p.get('max') || ''}
      />

      <Select label="Статус заказа" name="status" defaultValue={p.get('status') || ''}>
        <option value="">Все опубликованные</option>
        <option value="open">Приём откликов</option>
        <option value="in_progress">В работе</option>
        <option value="completed">Завершён</option>
      </Select>

      <button type="submit" className="button">
        Применить
      </button>

      <button
        type="button"
        className="text-button small"
        onClick={() => window.history.replaceState(null, '', '/orders')}
      >
        Сбросить фильтры
      </button>
    </form>
  );

  return (
    <section className="container page">
      <PageHeading
        title="Заказы от бизнеса"
        description="Задачи с понятным бюджетом. Выберите проект и отправьте своё предложение."
        mobileTitle="Заказы"
        mobileDescription="Выберите задачу для первого проекта."
      />

      <form
        className="order-search"
        role="search"
        aria-label="Поиск заказов"
        onSubmit={(event) => {
          event.preventDefault();

          const formData = new FormData(event.currentTarget);
          change({ q: String(formData.get('q') || '') });
        }}
      >
        <input
          className="field"
          name="q"
          enterKeyHint="search"
          key={p.get('q')}
          aria-label="Поиск заказов"
          placeholder="Название или описание задачи"
          defaultValue={p.get('q') || ''}
        />

        <button type="submit" className="button">
          Найти
        </button>
      </form>

      <details className={`mobile-only ${styles.filterToggle}`} style={{ marginBottom: 24 }}>
        <summary className="button secondary">Фильтры заказов</summary>
        {filters}
      </details>

      <div className={styles.layout}>
        <aside className={styles.sidebar}>{filters}</aside>

        <div className={styles.results}>
          {resource.data ? (
            <>
              <p className="small muted">Найдено {visibleOrders!.length} заказов</p>

              {visibleOrders!.length ? (
                visibleOrders!.map((order) => <OrderCard key={order.id} order={order} />)
              ) : (
                <EmptyState
                  title="Заказы не найдены"
                  description="Измените запрос или сбросьте фильтры."
                  href="/orders"
                  label="Сбросить фильтры"
                />
              )}
            </>
          ) : (
            <ResourceState error={resource.error} reload={resource.reload} />
          )}
        </div>
      </div>
    </section>
  );
}
