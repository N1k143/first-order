'use client';
import { budgetLabel, deadlineLabel } from '@/lib/format';

import Link from 'next/link';
import { CompanyEditor } from './company-editor';
import { useState } from 'react';
import { AccountShell } from '@/components/layout/account-shell';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { EmptyState, Notice, money } from '@/components/ui/primitives';
import { AccountAccess } from '@/features/account/account-access';
import { DEMO, useAppState, updateDemo } from '@/lib/state/store';
import { orderLabels, selectContractor } from '@/lib/state/order-actions';
import { ManageOrderPage } from './orders-workspace';
import { ApiWorkspacePage } from './api-workspace';

function DemoWorkspacePage({
  view = 'overview',
  id,
}: {
  view?: 'overview' | 'orders' | 'order' | 'applications' | 'profile' | 'members';
  id?: string;
}) {
  const state = useAppState();
  const [filter, setFilter] = useState('active');
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const [selection, setSelection] = useState<string | null>(null);
  const [complete, setComplete] = useState(false);
  const orders = state.orders.filter((o) => o.companyId === state.company.id);
  const order = orders.find((o) => o.id === id);
  const applications = state.applications.filter(
    (a) =>
      orders.some((o) => o.id === a.orderId) &&
      (!id || a.orderId === id) &&
      a.status !== 'withdrawn',
  );
  const titles = {
    overview: 'Кабинет компании',
    orders: 'Мои заказы',
    order: order?.title || 'Заказ',
    applications: 'Отклики на заказы',
    profile: 'Профиль компании',
    members: 'Участники компании',
  };
  const filtered = orders.filter((o) =>
    filter === 'draft'
      ? o.status === 'draft'
      : filter === 'completed'
        ? o.status === 'completed'
        : o.status === 'open' || o.status === 'in_progress',
  );
  const orderList = (
    <>
      {filtered.length ? (
        filtered.map((o) => (
          <article className="card stack" key={o.id}>
            <div className="spread wrap">
              <h3>{o.title}</h3>
              <span className="badge">{orderLabels[o.status]}</span>
            </div>
            <p className="small muted">
              {budgetLabel(o.budgetMin, o.budgetMax)} · {deadlineLabel(o.deadline)}
            </p>
            <Link
              className="text-link"
              href={
                o.status === 'draft'
                  ? `/workspace/orders/${o.id}/edit`
                  : `/workspace/orders/${o.id}`
              }
            >
              {o.status === 'draft'
                ? 'Продолжить редактирование'
                : `Открыть заказ · ${o.applicationsCount} откликов`}{' '}
              →
            </Link>
          </article>
        ))
      ) : (
        <EmptyState
          title="Здесь пока нет заказов"
          description="Создайте задачу и расскажите, какой результат нужен вашему бизнесу."
          href="/workspace/orders/new"
          label="Разместить заказ"
        />
      )}
    </>
  );
  return (
    <AccountShell title={titles[view]} role="company">
      <AccountAccess company>
        {(view === 'overview' || view === 'orders') && (
          <>
            {view === 'overview' && (
              <div className="stats">
                {[
                  [orders.filter((o) => o.status === 'open').length, 'Открытых заказов'],
                  [applications.filter((a) => a.status === 'pending').length, 'Новых откликов'],
                  [orders.filter((o) => o.status === 'in_progress').length, 'В работе'],
                ].map(([n, label]) => (
                  <div className="card stat" key={label}>
                    <strong>{n}</strong>
                    <span className="small muted">{label}</span>
                  </div>
                ))}
              </div>
            )}
            <div className="card stack soft-card">
              <h3>Новая задача для бизнеса</h3>
              <p className="muted">
                Опишите результат, бюджет и сроки. Сравните предложения и выберите исполнителя.
              </p>
              <Link className="button align-start" href="/workspace/orders/new">
                Разместить заказ
              </Link>
            </div>
            <nav className="tabs" aria-label="Статус заказов">
              {[
                ['active', 'Активные'],
                ['draft', 'Черновики'],
                ['completed', 'Завершённые'],
              ].map(([value, label]) => (
                <button
                  key={value}
                  aria-current={filter === value ? 'page' : undefined}
                  onClick={() => setFilter(value)}
                >
                  {label}
                </button>
              ))}
            </nav>
            {orderList}
          </>
        )}
        {view === 'order' &&
          (order ? (
            <>
              <article className="card stack">
                <span className="badge align-start">{orderLabels[order.status]}</span>
                <h2>{order.title}</h2>
                <p className="preserve-lines">{order.description}</p>
                <p>
                  {budgetLabel(order.budgetMin, order.budgetMax)} · {deadlineLabel(order.deadline)}
                </p>
                <div className="button-row">
                  <Link className="button secondary" href={`/orders/${order.id}`}>
                    Открыть публичную страницу
                  </Link>
                  {order.status === 'open' && (
                    <Link className="button" href={`/workspace/orders/${order.id}/applications`}>
                      Посмотреть отклики
                    </Link>
                  )}
                  {order.status === 'in_progress' && (
                    <button className="button" onClick={() => setComplete(true)}>
                      Завершить заказ
                    </button>
                  )}
                </div>
              </article>
              {order.selectedApplicationId && (
                <article className="card stack">
                  <h3>Исполнитель</h3>
                  <p>
                    {
                      state.applications.find(
                        (a) => a.orderId === order.id && a.id === order.selectedApplicationId,
                      )?.name
                    }
                  </p>
                  <p className="small muted">
                    Контактами и материалами обменивайтесь напрямую. В FirstOrder нет встроенного
                    чата и оплаты.
                  </p>
                </article>
              )}
            </>
          ) : (
            <EmptyState
              title="Заказ не найден"
              description="Он недоступен в кабинете этой компании."
              href="/workspace/orders"
              label="Мои заказы"
            />
          ))}
        {view === 'applications' &&
          (applications.length ? (
            applications.map((a) => (
              <article className="card stack" key={a.id}>
                <div className="spread wrap">
                  <h2>{a.name}</h2>
                  <span className="badge">
                    {a.status === 'pending'
                      ? 'На рассмотрении'
                      : a.status === 'selected'
                        ? 'Исполнитель выбран'
                        : a.status === 'completed'
                          ? 'Работа завершена'
                          : 'Отклонён'}
                  </span>
                </div>
                <p className="small muted">{orders.find((o) => o.id === a.orderId)?.title}</p>
                <p>
                  {money(a.price)} · {deadlineLabel(a.deadline)}
                </p>
                <p className="preserve-lines">{a.letter}</p>
                {a.status === 'pending' &&
                  orders.find((o) => o.id === a.orderId)?.status === 'open' && (
                    <button className="button align-start" onClick={() => setSelection(a.id)}>
                      Выбрать исполнителя
                    </button>
                  )}
              </article>
            ))
          ) : (
            <EmptyState
              title="Откликов пока нет"
              description="Когда разработчики предложат решение, их предложения появятся здесь."
              href="/workspace/orders"
              label="Мои заказы"
            />
          ))}
        {view === 'profile' && <CompanyEditor />}
        {view === 'members' && (
          <>
            <div className="card stack">
              <h3>Команда {state.company.name}</h3>
              {state.members.map((m) => (
                <div className="spread wrap member-row" key={m.id}>
                  <div>
                    <p>{m.name}</p>
                    <p className="small muted">{m.email}</p>
                  </div>
                  <span className="badge neutral">
                    {m.role === 'owner' ? 'Владелец' : 'Участник'}
                  </span>
                </div>
              ))}
            </div>
            <div className="card stack">
              <h3>Пригласить участника</h3>
              <p className="muted">
                Приглашения появятся после подключения API участников и отправки email. Сейчас
                письма не отправляются.
              </p>
            </div>
          </>
        )}
        {message && <Notice>{message}</Notice>}
        {error && <Notice error>{error}</Notice>}
        <ConfirmDialog
          open={!!selection}
          title="Выбрать исполнителя?"
          onClose={() => setSelection(null)}
          onConfirm={() => {
            const app = state.applications.find((a) => a.id === selection);
            if (!app) return;
            try {
              updateDemo((s) => selectContractor(s, app.orderId, app.id));
              setSelection(null);
              setMessage('Исполнитель выбран. Заказ перешёл в работу.');
            } catch (e) {
              setError(e instanceof Error ? e.message : 'Не удалось выбрать исполнителя.');
              setSelection(null);
            }
          }}
        >
          Заказ перейдёт в работу. Остальные предложения будут отклонены.
        </ConfirmDialog>
        <ConfirmDialog
          open={complete}
          title="Завершить заказ?"
          onClose={() => setComplete(false)}
          onConfirm={() => {
            updateDemo((s) => ({
              ...s,
              orders: s.orders.map((o) =>
                o.id === id && o.status === 'in_progress' ? { ...o, status: 'completed' } : o,
              ),
              applications: s.applications.map((a) =>
                a.orderId === id && a.status === 'selected' ? { ...a, status: 'completed' } : a,
              ),
            }));
            setComplete(false);
          }}
        >
          Подтвердите, что результат работы получен и принят.
        </ConfirmDialog>
      </AccountAccess>
    </AccountShell>
  );
}

export function WorkspacePage(props: {
  view?: 'overview' | 'orders' | 'order' | 'applications' | 'profile' | 'members';
  id?: string;
}) {
  if (!DEMO) {
    if ((props.view === 'order' || props.view === 'applications') && props.id) {
      return <ManageOrderPage id={props.id} />;
    }
    return <ApiWorkspacePage view={props.view === 'order' ? 'orders' : props.view} />;
  }
  return <DemoWorkspacePage {...props} />;
}
