'use client';

import Link from 'next/link';
import { useState } from 'react';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  EmptyState,
  Field,
  Notice,
  ResourceState,
  Select,
  money,
} from '@/components/ui/primitives';
import { mutate } from '@/lib/api/resources';
import { useAppState } from '@/lib/state/store';
import { budgetLabel, deadlineLabel } from '@/lib/format';
import { orderStatus, applicationStatus } from '@/lib/order-status';
import type { Application, Order } from '@/types/domain';
import { CompanyEditor } from './company-editor';
import { CompanyWorkspaceShell } from './workspace-shell';
import {
  useCompanyOrders,
  useWorkspaceCollection,
  workspaceApplication,
  type CompanyApplicationRow,
  type CompanyMemberRow,
} from './workspace-data';

export type CompanyView = 'overview' | 'orders' | 'applications' | 'profile' | 'members';
const titles: Record<CompanyView, string> = {
  overview: 'Кабинет компании',
  orders: 'Мои заказы',
  applications: 'Отклики на заказы',
  profile: 'Профиль компании',
  members: 'Участники компании',
};

function OrderSummary({ order }: { order: Order }) {
  return (
    <article className="card stack" data-testid="company-order">
      <div className="spread wrap">
        <h3>
          <Link href={`/workspace/orders/${order.id}`}>{order.title}</Link>
        </h3>
        <span className="badge">{orderStatus[order.status]}</span>
      </div>
      <p className="muted">
        {budgetLabel(order.budgetMin, order.budgetMax)} · {deadlineLabel(order.deadline)}
      </p>
      <p className="small muted">
        {order.applicationsCount} откликов
        {order.visibility === false ? ' · Скрыт из публичного каталога' : ''}
      </p>
      <div className="button-row">
        <Link className="button secondary" href={`/workspace/orders/${order.id}`}>
          Управлять заказом
        </Link>
        {order.status === 'draft' ? (
          <Link className="text-link" href={`/workspace/orders/${order.id}/edit`}>
            Редактировать черновик
          </Link>
        ) : (
          <Link className="text-link" href={`/workspace/orders/${order.id}/applications`}>
            Открыть отклики
          </Link>
        )}
      </div>
    </article>
  );
}

function CompanyOrders({ companyId, overview = false }: { companyId: string; overview?: boolean }) {
  const resource = useCompanyOrders(companyId);
  const [filter, setFilter] = useState('all'),
    [query, setQuery] = useState(''),
    [page, setPage] = useState(1);
  const orders = resource.data;
  if (!orders) return <ResourceState error={resource.error} reload={resource.reload} />;
  const matching = orders.filter(
    (order) =>
      (filter === 'all' || order.status === filter) &&
      `${order.title} ${order.description}`
        .toLocaleLowerCase('ru')
        .includes(query.trim().toLocaleLowerCase('ru')),
  );
  const pageSize = 12,
    pages = Math.ceil(matching.length / pageSize),
    currentPage = Math.min(page, Math.max(1, pages));
  const visible = overview
    ? orders.slice(0, 5)
    : matching.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  return (
    <>
      {overview && (
        <div className="stats" data-testid="company-statistics">
          {[
            [orders.filter((order) => order.status === 'open').length, 'Открытых заказов'],
            [orders.filter((order) => order.status === 'draft').length, 'Черновиков'],
            [orders.filter((order) => order.status === 'in_progress').length, 'В работе'],
            [orders.filter((order) => order.status === 'completed').length, 'Завершено'],
            [orders.reduce((total, order) => total + order.applicationsCount, 0), 'Всего откликов'],
          ].map(([count, label]) => (
            <div className="card stat" key={label}>
              <strong>{count}</strong>
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
        <div className="button-row">
          <Link className="button" href="/workspace/orders/new">
            Создать заказ
          </Link>
          {overview && (
            <>
              <Link className="button secondary" href="/workspace/profile">
                Профиль компании
              </Link>
              <Link className="button secondary" href="/workspace/members">
                Участники компании
              </Link>
            </>
          )}
        </div>
      </div>
      {overview ? (
        <div className="spread wrap">
          <h2>Последние заказы</h2>
          <Link className="text-link" href="/workspace/orders">
            Все заказы ({orders.length})
          </Link>
        </div>
      ) : (
        <div className="card form-grid">
          <Select
            label="Статус заказа"
            value={filter}
            onChange={(event) => {
              setFilter(event.target.value);
              setPage(1);
            }}
          >
            <option value="all">Все заказы</option>
            {Object.entries(orderStatus)
              .filter(([value]) => value !== 'hidden')
              .map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
          </Select>
          <Field
            label="Поиск моих заказов"
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setPage(1);
            }}
            placeholder="Название или описание"
          />
        </div>
      )}
      <div className="spread wrap">
        <p className="small muted">
          {overview
            ? `${orders.length} заказов у выбранной компании`
            : `Найдено ${matching.length} заказов`}
        </p>
        <button className="text-button" onClick={resource.reload}>
          Обновить заказы
        </button>
      </div>
      {visible.length ? (
        visible.map((order) => <OrderSummary key={order.id} order={order} />)
      ) : (
        <EmptyState
          title={orders.length ? 'Заказы не найдены' : 'Заказов пока нет'}
          description={
            orders.length
              ? 'Измените поиск или статус заказа.'
              : 'Создайте черновик и опубликуйте первую задачу.'
          }
        />
      )}
      {!overview && pages > 1 && (
        <nav className="button-row" aria-label="Страницы заказов">
          <button
            className="button secondary"
            disabled={currentPage === 1}
            onClick={() => setPage(currentPage - 1)}
          >
            Назад
          </button>
          <span>
            Страница {currentPage} из {pages}
          </span>
          <button
            className="button secondary"
            disabled={currentPage === pages}
            onClick={() => setPage(currentPage + 1)}
          >
            Далее
          </button>
        </nav>
      )}
    </>
  );
}

function OrderApplications({
  order,
  filter,
  busy,
  onChoose,
}: {
  order: Order;
  filter: string;
  busy: boolean;
  onChoose: (order: Order, application: Application) => void;
}) {
  const resource = useWorkspaceCollection<CompanyApplicationRow>(
    `/v1/orders/${encodeURIComponent(order.id)}/applications`,
  );
  const applications = resource.data
    ?.map(workspaceApplication)
    .filter((application) => filter === 'all' || application.status === filter);
  return (
    <section className="stack" data-testid="company-order-applications">
      <div className="spread wrap">
        <h2>
          <Link href={`/workspace/orders/${order.id}/applications`}>{order.title}</Link>
        </h2>
        <span className="badge">{orderStatus[order.status]}</span>
      </div>
      {!applications ? (
        <ResourceState error={resource.error} reload={resource.reload} />
      ) : applications.length ? (
        applications.map((application) => (
          <article className="card stack" key={application.id} data-testid="application-card">
            <div className="spread wrap">
              <h3>{application.name || 'Разработчик'}</h3>
              <span className="badge">{applicationStatus[application.status]}</span>
            </div>
            <p className="preserve-lines">{application.letter}</p>
            <p>
              {money(application.price)} · {deadlineLabel(application.deadline)}
            </p>
            {order.status === 'open' && application.status === 'pending' && (
              <button
                className="button align-start"
                disabled={busy}
                onClick={() => onChoose(order, application)}
              >
                Выбрать исполнителя
              </button>
            )}
          </article>
        ))
      ) : (
        <p className="muted">Откликов с выбранным статусом нет.</p>
      )}
    </section>
  );
}
function CompanyApplications({ companyId }: { companyId: string }) {
  const orders = useCompanyOrders(companyId);
  const [filter, setFilter] = useState('all'),
    [page, setPage] = useState(1),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [message, setMessage] = useState('');
  const [selected, setSelected] = useState<{ order: Order; application: Application } | null>(null);
  async function choose() {
    if (!selected || busy) return;
    setBusy(true);
    setError('');
    setMessage('');
    try {
      await mutate(
        `/orders/${encodeURIComponent(selected.order.id)}/select-application`,
        'POST',
        { application_id: selected.application.id },
        () => {},
      );
      setMessage('Исполнитель выбран. Остальные поданные отклики отклонены.');
      setSelected(null);
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Не удалось выбрать исполнителя.');
      setSelected(null);
      orders.reload();
    } finally {
      setBusy(false);
    }
  }
  if (!orders.data) return <ResourceState error={orders.error} reload={orders.reload} />;
  const active = orders.data.filter((order) => order.applicationsCount > 0);
  const pages = Math.ceil(active.length / 5),
    currentPage = Math.min(page, Math.max(1, pages));
  const visible = active.slice((currentPage - 1) * 5, currentPage * 5);
  return (
    <>
      <div className="card stack">
        <Select
          label="Статус отклика"
          value={filter}
          onChange={(event) => {
            setFilter(event.target.value);
            setPage(1);
          }}
        >
          <option value="all">Все отклики</option>
          {Object.entries(applicationStatus).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </Select>
        <button className="button secondary align-start" disabled={busy} onClick={orders.reload}>
          Обновить отклики
        </button>
      </div>
      {message && <Notice>{message}</Notice>}
      {error && <Notice error>{error}</Notice>}
      {active.length ? (
        visible.map((order) => (
          <OrderApplications
            key={order.id}
            order={order}
            filter={filter}
            busy={busy}
            onChoose={(order, application) => setSelected({ order, application })}
          />
        ))
      ) : (
        <EmptyState
          title="Откликов пока нет"
          description="Предложения разработчиков появятся после публикации заказов."
          href="/workspace/orders"
          label="Мои заказы"
        />
      )}
      {pages > 1 && (
        <nav className="button-row" aria-label="Страницы заказов с откликами">
          <button
            className="button secondary"
            disabled={currentPage === 1}
            onClick={() => setPage(currentPage - 1)}
          >
            Назад
          </button>
          <span>
            Страница {currentPage} из {pages}
          </span>
          <button
            className="button secondary"
            disabled={currentPage === pages}
            onClick={() => setPage(currentPage + 1)}
          >
            Далее
          </button>
        </nav>
      )}
      <ConfirmDialog
        open={Boolean(selected)}
        title="Выбрать исполнителя?"
        description={`Вы выбираете ${selected?.application.name || 'этого разработчика'} для заказа «${selected?.order.title || ''}». Остальные поданные отклики будут отклонены.`}
        confirmLabel={busy ? 'Сохраняем…' : 'Подтвердить выбор'}
        onConfirm={() => void choose()}
        onClose={() => {
          if (!busy) setSelected(null);
        }}
      />
    </>
  );
}

function CompanyMembers({ companyId }: { companyId: string }) {
  const state = useAppState();
  const resource = useWorkspaceCollection<CompanyMemberRow>(
    `/v1/companies/${encodeURIComponent(companyId)}/members`,
  );
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [message, setMessage] = useState(''),
    [removing, setRemoving] = useState<CompanyMemberRow | null>(null);
  const [userId, setUserId] = useState(''),
    [role, setRole] = useState('representative');
  const members = resource.data;
  const owner = Boolean(
    members?.some(
      (member) => member.user_id === state.profile.id && member.member_role === 'owner',
    ),
  );
  const owners = members?.filter((member) => member.member_role === 'owner').length || 0;
  async function add(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!owner || busy) return;
    setBusy(true);
    setError('');
    setMessage('');
    try {
      await mutate(
        `/v1/companies/${encodeURIComponent(companyId)}/members`,
        'POST',
        { user_id: userId.trim(), member_role: role },
        () => {},
      );
      setMessage('Участник добавлен в компанию.');
      setUserId('');
      setRole('representative');
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Не удалось добавить участника.');
      resource.reload();
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    if (!removing || !owner || busy) return;
    setBusy(true);
    setError('');
    setMessage('');
    try {
      await mutate(
        `/v1/companies/${encodeURIComponent(companyId)}/members/${encodeURIComponent(removing.user_id)}`,
        'DELETE',
        undefined,
        () => {},
      );
      setMessage('Участник удалён из компании.');
      setRemoving(null);
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Не удалось удалить участника.');
      setRemoving(null);
      resource.reload();
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      {message && <Notice>{message}</Notice>}
      {error && <Notice error>{error}</Notice>}
      {!members ? (
        <ResourceState error={resource.error} reload={resource.reload} />
      ) : (
        <>
          <div className="card stack">
            <div className="spread wrap">
              <h2>Команда компании</h2>
              <button className="text-button" disabled={busy} onClick={resource.reload}>
                Обновить участников
              </button>
            </div>
            {members.map((member) => (
              <article
                className="spread wrap member-row"
                key={member.user_id}
                data-testid="company-member"
              >
                <div>
                  <h3>
                    {member.name || 'Участник'}
                    {member.user_id === state.profile.id ? ' (вы)' : ''}
                  </h3>
                  <p className="small muted" style={{ overflowWrap: 'anywhere' }}>
                    ID: {member.user_id}
                  </p>
                </div>
                <div className="button-row">
                  <span className="badge neutral">
                    {member.member_role === 'owner' ? 'Владелец' : 'Представитель'}
                  </span>
                  {owner &&
                    (member.member_role === 'owner' && owners === 1 ? (
                      <span className="small muted">Последний владелец</span>
                    ) : (
                      <button
                        className="button secondary"
                        disabled={busy}
                        onClick={() => setRemoving(member)}
                      >
                        Удалить участника
                      </button>
                    ))}
                </div>
              </article>
            ))}
            {!owner && (
              <p className="muted">
                Добавлять и удалять участников может только владелец компании.
              </p>
            )}
          </div>
          {owner && (
            <form className="card stack" onSubmit={add}>
              <h2>Добавить участника</h2>
              <p className="muted">
                Укажите ID уже зарегистрированного пользователя. Он должен подтвердить почту и войти
                в аккаунт. Это добавление доступа к компании; письмо-приглашение не отправляется.
              </p>
              <Field
                label="ID пользователя"
                name="user_id"
                value={userId}
                onChange={(event) => setUserId(event.target.value)}
                required
                pattern="[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}"
                placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              />
              <Select
                label="Роль в компании"
                name="member_role"
                value={role}
                onChange={(event) => setRole(event.target.value)}
              >
                <option value="representative">Представитель — профиль компании и заказы</option>
                <option value="owner">Владелец — также управление участниками</option>
              </Select>
              <button className="button align-start" disabled={busy}>
                {busy ? 'Сохраняем…' : 'Добавить участника'}
              </button>
            </form>
          )}
        </>
      )}
      <ConfirmDialog
        open={Boolean(removing)}
        title="Удалить участника?"
        description={`${removing?.name || 'Пользователь'} потеряет доступ к компании. Его аккаунт останется на платформе.`}
        confirmLabel={busy ? 'Удаляем…' : 'Удалить участника'}
        onConfirm={() => void remove()}
        onClose={() => {
          if (!busy) setRemoving(null);
        }}
      />
    </>
  );
}

export function ApiWorkspacePage({ view = 'overview' }: { view?: CompanyView }) {
  return (
    <CompanyWorkspaceShell title={titles[view]}>
      {(companyId) =>
        view === 'profile' ? (
          <CompanyEditor companyId={companyId} />
        ) : view === 'members' ? (
          <CompanyMembers companyId={companyId} />
        ) : view === 'applications' ? (
          <CompanyApplications companyId={companyId} />
        ) : (
          <CompanyOrders companyId={companyId} overview={view === 'overview'} />
        )
      }
    </CompanyWorkspaceShell>
  );
}
