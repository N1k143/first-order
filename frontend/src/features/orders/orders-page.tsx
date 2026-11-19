'use client';
import { useSearchParams, useRouter } from 'next/navigation';
import { useAppState } from '@/lib/state/store';
import { useResource } from '@/lib/api/resources';
import { PageHeading, Select, Field, ResourceState, EmptyState } from '@/components/ui/primitives';
import { isPublicOrder } from '@/lib/orders';
import { OrderCard } from './order-card';
import styles from '@/features/catalog/catalog.module.css';
export function OrdersPage() {
  const state = useAppState(),
    p = useSearchParams(),
    router = useRouter();
  function change(values: Record<string, string>) {
    const q = new URLSearchParams(p.toString());
    for (const [k, v] of Object.entries(values)) q.set(k, v);
    router.replace('/orders?' + q.toString(), { scroll: false });
  }
  const rows = state.orders.filter(
    (o) =>
      isPublicOrder(o) &&
      (!p.get('q') ||
        `${o.title} ${o.description}`.toLowerCase().includes(p.get('q')!.toLowerCase())) &&
      (!p.get('city') || p.get('city') === o.city) &&
      (!p.get('min') ||
        (o.budgetMax !== null ? o.budgetMax >= Number(p.get('min')) : o.budgetMin !== null)) &&
      (!p.get('max') ||
        (o.budgetMin !== null ? o.budgetMin <= Number(p.get('max')) : o.budgetMax !== null)) &&
      (!p.get('status') || o.status === p.get('status')),
  );
  const resource = useResource('/orders?' + p.toString(), rows);
  const visibleOrders = resource.data?.filter(isPublicOrder);
  const filters = (
    <form
      key={p.toString()}
      className="card stack"
      onSubmit={(e) => {
        e.preventDefault();
        const f = new FormData(e.currentTarget);
        change(Object.fromEntries([...f].map(([k, v]) => [k, String(v)])));
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
      <button className="button">Применить</button>
      <button type="button" className="text-button small" onClick={() => router.replace('/orders')}>
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
        className="button-row"
        style={{ marginBottom: 32, flexWrap: 'nowrap' }}
        onSubmit={(e) => {
          e.preventDefault();
          change({ q: String(new FormData(e.currentTarget).get('q') || '') });
        }}
      >
        <input
          className="field"
          name="q"
          key={p.get('q')}
          aria-label="Поиск заказов"
          placeholder="Название или описание задачи"
          defaultValue={p.get('q') || ''}
        />
        <button className="button">Найти</button>
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
                visibleOrders!.map((o) => <OrderCard key={o.id} order={o} />)
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
