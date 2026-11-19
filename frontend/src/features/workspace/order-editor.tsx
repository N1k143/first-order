'use client';
import { budgetLabel, deadlineLabel } from '@/lib/format';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { AccountShell } from '@/components/layout/account-shell';
import { AccountAccess } from '@/features/account/account-access';
import { Field, Textarea, Notice, EmptyState } from '@/components/ui/primitives';
import { DEMO, useAppState, updateDemo } from '@/lib/state/store';
import { validateOrder } from '@/lib/state/order-actions';
import { orderFields } from '@/lib/api/form-fields';
import type { Order } from '@/types/domain';
import { ApiDraftEditor, NewOrderPage } from './orders-workspace';
function DemoOrderEditor({ id, preview = false }: { id?: string; preview?: boolean }) {
  const state = useAppState(),
    router = useRouter();
  const [error, setError] = useState('');
  const order = state.orders.find((o) => o.id === id && o.companyId === state.company.id);
  function save(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    let fields;
    try {
      fields = orderFields(f);
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Проверьте поля заказа.');
      return;
    }
    const next: Order = {
      id: order?.id || crypto.randomUUID(),
      companyId: state.company.id,
      companyName: state.company.name,
      city: state.company.city,
      title: fields.title,
      description: fields.description,
      budgetMin: fields.budget_min,
      budgetMax: fields.budget_max,
      deadline: fields.deadline,
      status: 'draft',
      visibility: false,
      selectedApplicationId: null,
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
              <h2>{budgetLabel(order.budgetMin, order.budgetMax)}</h2>
              <p>{deadlineLabel(order.deadline)}</p>
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
                            visibility: true,
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
            <p className="small muted">
              Бюджет и срок можно оставить пустыми и согласовать с исполнителем.
            </p>
            <div className="form-grid">
              <Field
                label="Бюджет от, ₸"
                name="min"
                type="number"
                min={0}
                max={1000000000}
                step="any"
                placeholder="120 000"
                defaultValue={order?.budgetMin ?? ''}
              />
              <Field
                label="Бюджет до, ₸"
                name="max"
                type="number"
                min={0}
                max={1000000000}
                step="any"
                placeholder="180 000"
                defaultValue={order?.budgetMax ?? ''}
              />
            </div>
            <div className="form-grid">
              <Field
                label="Желаемый срок"
                name="deadline"
                type="date"
                min={new Date().toISOString().slice(0, 10)}
                defaultValue={order?.deadline ?? ''}
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

export function OrderEditor(props: { id?: string; preview?: boolean }) {
  if (!DEMO)
    return props.id ? <ApiDraftEditor id={props.id} preview={props.preview} /> : <NewOrderPage />;
  return <DemoOrderEditor {...props} />;
}
