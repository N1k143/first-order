'use client';

import Link from 'next/link';
import { useState } from 'react';
import { useAppState, updateDemo, DEMO } from '@/lib/state/store';
import { mutate, useResource } from '@/lib/api/resources';
import { orderLabels } from '@/lib/state/order-actions';
import { AccountAccess } from '@/features/account/account-access';
import {
  PageHeading,
  ResourceState,
  EmptyState,
  Textarea,
  Field,
  Notice,
  money,
  dateLabel,
} from '@/components/ui/primitives';

export function OrderPage({ id, apply = false }: { id: string; apply?: boolean }) {
  const state = useAppState();
  const resource = useResource(
    `/orders/${encodeURIComponent(id)}`,
    state.orders.find((o) => o.id === id && o.status !== 'draft'),
  );
  const [error, setError] = useState('');
  const [sent, setSent] = useState(false);
  const [pending, setPending] = useState(false);
  const order = resource.data;
  if (!order)
    return (
      <section className="container page">
        {resource.loading || resource.error ? (
          <ResourceState error={resource.error} reload={resource.reload} />
        ) : (
          <EmptyState
            title="Заказ недоступен"
            description="Заказ удалён или не опубликован."
            href="/orders"
            label="К заказам"
          />
        )}
      </section>
    );
  const active = state.applications.some(
    (a) => a.orderId === id && a.profileId === state.profile.id && a.status !== 'withdrawn',
  );
  const canApply = state.profile.role === 'developer';
  const action = (
    <Link
      className="button full-width"
      href={
        !state.signedIn
          ? '/login'
          : active
            ? '/applications'
            : canApply
              ? `/orders/${id}/apply`
              : '/settings'
      }
    >
      {!state.signedIn
        ? 'Войти и откликнуться'
        : active
          ? 'Ваш отклик отправлен'
          : canApply
            ? 'Откликнуться'
            : 'Перейти в роль разработчика'}
    </Link>
  );
  if (apply)
    return (
      <section className="container page">
        <PageHeading
          title="Ваше предложение"
          description={`${order.title} · ${order.companyName}`}
        />
        <AccountAccess>
          {!canApply ? (
            <EmptyState
              title="Отклики доступны разработчикам"
              description="Для проверки этого сценария выберите роль разработчика в настройках деморежима."
              href="/settings"
              label="Настройки"
            />
          ) : (
            <div className="two-columns">
              <form
                className="card stack mobile-flat"
                onSubmit={async (e) => {
                  e.preventDefault();
                  setError('');
                  if (active || sent) {
                    setError('На этот заказ уже отправлен отклик. Откройте «Мои отклики».');
                    return;
                  }
                  if (order.status !== 'open') {
                    setError('Приём откликов закрыт.');
                    return;
                  }
                  const f = new FormData(e.currentTarget);
                  const application = {
                    id: crypto.randomUUID(),
                    orderId: id,
                    name: state.profile.name,
                    profileId: state.profile.id,
                    price: Number(f.get('price')),
                    deadline: String(f.get('deadline')),
                    letter: String(f.get('letter')).trim(),
                    status: 'pending' as const,
                  };
                  if (
                    application.letter.length < 20 ||
                    !Number.isFinite(application.price) ||
                    application.price <= 0 ||
                    application.deadline < new Date().toISOString().slice(0, 10)
                  ) {
                    setError('Проверьте письмо, стоимость и срок.');
                    return;
                  }
                  setPending(true);
                  try {
                    await mutate(`/orders/${id}/applications`, 'POST', application, () =>
                      updateDemo((s) => ({
                        ...s,
                        applications: [...s.applications, application],
                        orders: s.orders.map((o) =>
                          o.id === id ? { ...o, applicationsCount: o.applicationsCount + 1 } : o,
                        ),
                      })),
                    );
                    setSent(true);
                  } catch (e) {
                    setError(e instanceof Error ? e.message : 'Ошибка отправки.');
                  } finally {
                    setPending(false);
                  }
                }}
              >
                <h3>Расскажите, как решите задачу</h3>
                <Textarea
                  label="Сопроводительное письмо"
                  name="letter"
                  required
                  minLength={20}
                  maxLength={5000}
                  placeholder="Опишите подход к задаче, этапы работы и что входит в стоимость."
                />
                <div className="form-grid">
                  <Field
                    label="Стоимость, ₸"
                    name="price"
                    type="number"
                    min={1}
                    max={1000000000}
                    required
                    placeholder="150 000"
                  />
                  <Field
                    label="Готово к дате"
                    name="deadline"
                    type="date"
                    min={new Date().toISOString().slice(0, 10)}
                    required
                  />
                </div>
                <p className="small muted">В отклике будет ссылка на ваш профиль и портфолио.</p>
                <hr />
                <div className="button-row">
                  <button
                    className="button"
                    disabled={pending || sent || active || order.status !== 'open'}
                  >
                    Отправить отклик
                  </button>
                  <Link className="button secondary" href={`/orders/${id}`}>
                    Назад к заказу
                  </Link>
                </div>
                {error && <Notice error>{error}</Notice>}
                {(sent || active) && (
                  <Notice>
                    {DEMO ? 'Отклик сохранён в деморежиме.' : 'Отклик отправлен.'}{' '}
                    <Link href="/applications">Мои отклики →</Link>
                  </Notice>
                )}
                {order.status !== 'open' && <Notice>Приём откликов закрыт.</Notice>}
              </form>
              <aside className="card stack desktop-only">
                <h3>Хорошее предложение</h3>
                <p className="muted">
                  Опишите решение конкретной задачи. Укажите, что входит в стоимость и сколько
                  времени понадобится.
                </p>
                <p className="small muted">
                  На каждый заказ можно отправить один отклик. Его можно отозвать до выбора
                  исполнителя.
                </p>
              </aside>
            </div>
          )}
        </AccountAccess>
      </section>
    );
  return (
    <section className="container page">
      <PageHeading
        title={order.title}
        description={
          <>
            <Link href="/orders">Заказы</Link> / {order.companyName}
          </>
        }
        mobileDescription={`${order.companyName} · ${order.city}`}
      />
      <div className="two-columns">
        <div className="stack-lg">
          <article className="card stack">
            <span className="badge align-start">{orderLabels[order.status]}</span>
            <h2>О задаче</h2>
            <p className="preserve-lines">{order.description}</p>
          </article>
          <article className="card stack">
            <h3>О заказчике</h3>
            <h3>{order.companyName}</h3>
            <p className="muted">{order.city}</p>
            <Link className="button secondary align-start" href={`/companies/${order.companyId}`}>
              Карточка компании
            </Link>
          </article>
        </div>
        <aside className="card stack">
          <h2>
            {money(order.budgetMin)} – {money(order.budgetMax)}
          </h2>
          <p className="small muted">Бюджет проекта</p>
          <hr />
          <p>Срок: до {dateLabel(order.deadline)}</p>
          <p className="small muted">
            {order.applicationsCount} откликов · опубликован {dateLabel(order.publishedAt)}
          </p>
          {order.status === 'open' ? (
            action
          ) : (
            <span className="badge align-start">Приём откликов закрыт</span>
          )}
          <p className="small muted">В предложении укажите стоимость, срок и подход к задаче.</p>
        </aside>
      </div>
    </section>
  );
}
