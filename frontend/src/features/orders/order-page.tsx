'use client';
import { budgetLabel, deadlineLabel } from '@/lib/format';

import Link from 'next/link';
import { useState } from 'react';
import { useAppState, updateDemo, DEMO } from '@/lib/state/store';
import { mutate, useResource } from '@/lib/api/resources';
import { applicationFields } from '@/lib/api/form-fields';
import { orderLabels } from '@/lib/state/order-actions';
import { orderStatus } from '@/lib/order-status';
import { AccountAccess } from '@/features/account/account-access';
import {
  PageHeading,
  ResourceState,
  EmptyState,
  Textarea,
  Field,
  Notice,
  dateLabel,
  money,
} from '@/components/ui/primitives';

function DemoOrderPage({ id, apply = false }: { id: string; apply?: boolean }) {
  const state = useAppState();
  const resource = useResource(
    `/orders/${encodeURIComponent(id)}`,
    state.orders.find((o) => o.id === id && o.status !== 'draft' && o.visibility !== false),
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
    (a) => a.orderId === id && a.profileId === state.profile.id,
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
                  let fields;
                  try {
                    fields = applicationFields(f);
                  } catch (error) {
                    setError(error instanceof Error ? error.message : 'Проверьте поля отклика.');
                    return;
                  }
                  const application = {
                    id: crypto.randomUUID(),
                    orderId: id,
                    name: state.profile.name,
                    profileId: state.profile.id,
                    price: fields.proposed_price,
                    deadline: fields.proposed_deadline,
                    letter: fields.message,
                    status: 'pending' as const,
                  };
                  if (
                    application.letter.length < 20 ||
                    (application.deadline !== null &&
                      application.deadline < new Date().toISOString().slice(0, 10))
                  ) {
                    setError('Проверьте письмо, стоимость и срок.');
                    return;
                  }
                  setPending(true);
                  try {
                    await mutate(`/orders/${id}/applications`, 'POST', fields, () =>
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
                    min={0}
                    max={1000000000}
                    step="any"
                    placeholder="150 000"
                  />
                  <Field
                    label="Готово к дате"
                    name="deadline"
                    type="date"
                    min={new Date().toISOString().slice(0, 10)}
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
          <h2>{budgetLabel(order.budgetMin, order.budgetMax)}</h2>
          <p className="small muted">Бюджет проекта</p>
          <hr />
          <p>{deadlineLabel(order.deadline)}</p>
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

function ApiOrderPage({ id, apply = false }: { id: string; apply?: boolean }) {
  const state = useAppState(),
    resource = useResource(
      `/orders/${id}`,
      state.orders.find((o) => o.id === id),
    );
  const [error, setError] = useState(''),
    [sent, setSent] = useState(false),
    [pending, setPending] = useState(false);
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
  const existing = state.applications.find(
    (a) => a.orderId === id && a.profileId === state.profile.id,
  );
  const own =
    state.profile.companies?.some((c) => c.companyId === order.companyId) ||
    (DEMO && state.profile.role === 'company' && state.company.id === order.companyId);
  const action = own ? (
    <Link className="button" href={`/workspace/orders/${id}`}>
      Управлять заказом
    </Link>
  ) : existing ? (
    <Link className="button secondary" href="/applications">
      {existing.status === 'withdrawn' ? 'Отклик отозван' : 'Ваш отклик отправлен'}
    </Link>
  ) : order.status === 'open' ? (
    <Link className="button" href={state.signedIn ? `/orders/${id}/apply` : '/login'}>
      Откликнуться
    </Link>
  ) : (
    <span className="badge">Приём откликов закрыт</span>
  );
  if (apply && sent)
    return (
      <section className="container page">
        <PageHeading title="Отклик отправлен" />
        <div className="card stack">
          <Notice>Отклик отправлен. Решение заказчика появится в личном списке.</Notice>
          <Link className="button align-start" href="/applications">
            Мои отклики →
          </Link>
        </div>
      </section>
    );
  if (apply) {
    if (own || existing || !state.signedIn || order.status !== 'open')
      return (
        <section className="container page">
          <PageHeading title={order.title} />
          <div className="card stack">
            <p>
              {own
                ? 'Вы представляете компанию этого заказа.'
                : existing
                  ? 'На этот заказ уже существует ваш отклик.'
                  : !state.signedIn
                    ? 'Войдите, чтобы отправить предложение.'
                    : 'Приём откликов закрыт.'}
            </p>
            <div>{action}</div>
          </div>
        </section>
      );
    return (
      <section className="container page">
        <PageHeading
          title="Ваше предложение"
          description={`${order.title} · ${order.companyName}`}
        />
        <div className="two-columns">
          <form
            className="card stack mobile-flat"
            onSubmit={async (e) => {
              e.preventDefault();
              setError('');
              const f = new FormData(e.currentTarget);
              const a = {
                id: crypto.randomUUID(),
                orderId: id,
                name: state.profile.name,
                profileId: state.profile.id,
                price: Number(f.get('price')),
                deadline: String(f.get('deadline')),
                letter: String(f.get('letter')),
                status: 'pending' as const,
              };
              setPending(true);
              try {
                await mutate(
                  `/orders/${id}/applications`,
                  'POST',
                  { message: a.letter, proposed_price: a.price, proposed_deadline: a.deadline },
                  () =>
                    updateDemo((s) => ({
                      ...s,
                      applications: [...s.applications, a],
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
              maxLength={10000}
              placeholder="Опишите решение, этапы и результат работы."
            />
            <div className="form-grid">
              <Field
                label="Стоимость, ₸"
                name="price"
                type="number"
                min={0}
                max={9999999999.99}
                step="0.01"
                required
              />
              <Field
                label="Готово к дате"
                name="deadline"
                type="date"
                min={new Date().toISOString().slice(0, 10)}
                required
              />
            </div>
            <div className="button-row">
              <button className="button" disabled={pending || sent}>
                {pending ? 'Отправляем…' : 'Отправить отклик'}
              </button>
              <Link className="button secondary" href={`/orders/${id}`}>
                Отмена
              </Link>
            </div>
            {error && <Notice error>{error}</Notice>}
            {sent && (
              <Notice>
                Отклик отправлен. <Link href="/applications">Мои отклики →</Link>
              </Notice>
            )}
          </form>
          <aside className="card stack desktop-only">
            <h3>Хорошее предложение</h3>
            <p className="muted">
              Опишите решение конкретной задачи. Укажите, что входит в стоимость и сколько времени
              понадобится.
            </p>
            <p className="small muted">
              Один отклик на заказ. Отзыв не позволяет отправить новый отклик на тот же заказ.
            </p>
          </aside>
        </div>
      </section>
    );
  }
  return (
    <section className="container page">
      <PageHeading
        title={order.title}
        description={
          <>
            <Link href="/orders">Заказы</Link> / {order.companyName}
          </>
        }
      />
      <div className="two-columns">
        <div className="stack-lg">
          <article className="card stack">
            <div>
              <span className="badge">{orderStatus[order.status]}</span>
            </div>
            <h2>О задаче</h2>
            <p style={{ whiteSpace: 'pre-wrap' }}>{order.description}</p>
          </article>
          <article className="card stack">
            <h3>О заказчике</h3>
            <p>
              {order.companyName} · {order.city}
            </p>
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
          <p>Срок: {order.deadline ? `до ${dateLabel(order.deadline)}` : 'по договорённости'}</p>
          <p>{order.applicationsCount} откликов</p>
          <div>{action}</div>
        </aside>
      </div>
    </section>
  );
}

export function OrderPage(props: { id: string; apply?: boolean }) {
  return DEMO ? <DemoOrderPage {...props} /> : <ApiOrderPage {...props} />;
}
