'use client';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { AccountShell } from '@/components/layout/account-shell';
import { AccountAccess } from '@/features/account/account-access';
import { Field, Textarea, Notice, EmptyState, money, dateLabel } from '@/components/ui/primitives';
import { useAppState, updateDemo } from '@/lib/state/store';
import { validateOrder } from '@/lib/state/order-actions';
import type { Order } from '@/types/domain';
export function OrderEditor({ id, preview = false }: { id?: string; preview?: boolean }) {
  const state = useAppState(),
    router = useRouter();
  const [error, setError] = useState('');
  const order = state.orders.find((o) => o.id === id && o.companyId === state.company.id);
  function save(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const next: Order = {
      id: order?.id || crypto.randomUUID(),
      companyId: state.company.id,
      companyName: state.company.name,
      city: state.company.city,
      title: String(f.get('title')).trim(),
      description: String(f.get('description')).trim(),
      budgetMin: Number(f.get('min')),
      budgetMax: Number(f.get('max')),
      deadline: String(f.get('deadline')),
      status: 'draft',
      applicationsCount: 0,
      publishedAt: new Date().toISOString().slice(0, 10),
    };
    const issue = validateOrder(next);
    if (issue) {
      setError(issue);
      return;
    }
    updateDemo((s) => ({ ...s, orders: [next, ...s.orders.filter((o) => o.id !== next.id)] }));
    const intent = (e.nativeEvent as SubmitEvent).submitter?.getAttribute('value');
    router.push(intent === 'draft' ? '/workspace/orders' : `/workspace/orders/${next.id}/preview`);
  }
  return (
    <AccountShell
      title={preview ? 'Предпросмотр заказа' : id ? 'Редактирование заказа' : 'Создание заказа'}
      role="company"
    >
      <AccountAccess company>
        {id && (!order || order.status !== 'draft') ? (
          <EmptyState
            title="Черновик недоступен"
            description="Он уже опубликован или принадлежит другой компании."
            href="/workspace/orders"
            label="Мои заказы"
          />
        ) : preview && order ? (
          <>
            <article className="card stack">
              <span className="badge neutral align-start">Предпросмотр · Ещё не опубликован</span>
              <h2>{order.title}</h2>
              <p className="muted">
                {order.companyName} · {order.city}
              </p>
              <h3>О задаче</h3>
              <p className="preserve-lines">{order.description}</p>
              <hr />
              <h2>
                {money(order.budgetMin)} – {money(order.budgetMax)}
              </h2>
              <p>До {dateLabel(order.deadline)}</p>
            </article>
            <div className="button-row">
              <button
                className="button"
                onClick={() => {
                  const issue = validateOrder(order);
                  if (issue) {
                    setError(issue);
                    return;
                  }
                  updateDemo((s) => ({
                    ...s,
                    orders: s.orders.map((o) =>
                      o.id === order.id
                        ? {
                            ...o,
                            status: 'open',
                            publishedAt: new Date().toISOString().slice(0, 10),
                          }
                        : o,
                    ),
                  }));
                  router.push(`/workspace/orders/${order.id}`);
                }}
              >
                Опубликовать заказ
              </button>
              <Link className="button secondary" href={`/workspace/orders/${order.id}/edit`}>
                Вернуться к редактированию
              </Link>
            </div>
          </>
        ) : (
          <form className="card stack order-editor" onSubmit={save}>
            <h3>Расскажите о задаче</h3>
            <Field
              label="Название заказа"
              name="title"
              placeholder="Сайт для локальной кофейни"
              defaultValue={order?.title}
              required
              minLength={5}
              maxLength={160}
            />
            <Textarea
              label="Описание задачи"
              name="description"
              placeholder="Что нужно сделать, какой результат вы ожидаете и какие материалы готовы предоставить."
              defaultValue={order?.description}
              required
              minLength={20}
              maxLength={10000}
            />
            <div className="form-grid">
              <Field
                label="Бюджет от, ₸"
                name="min"
                type="number"
                min={1}
                max={1000000000}
                step={1}
                placeholder="120 000"
                defaultValue={order?.budgetMin}
                required
              />
              <Field
                label="Бюджет до, ₸"
                name="max"
                type="number"
                min={1}
                max={1000000000}
                step={1}
                placeholder="180 000"
                defaultValue={order?.budgetMax}
                required
              />
            </div>
            <div className="form-grid">
              <Field
                label="Желаемый срок"
                name="deadline"
                type="date"
                min={new Date().toISOString().slice(0, 10)}
                defaultValue={order?.deadline}
                required
              />
            </div>
            <p className="small muted">Компания: {state.company.name}</p>
            <hr />
            <div className="button-row">
              <button className="button" value="preview">
                Предпросмотр
              </button>
              <button className="button secondary" value="draft">
                Сохранить черновик
              </button>
            </div>
          </form>
        )}
        {error && <Notice error>{error}</Notice>}
      </AccountAccess>
    </AccountShell>
  );
}
