'use client';
import Link from 'next/link';
import { useRef, useState, type ReactNode } from 'react';
import { AccountShell } from '@/components/layout/account-shell';
import {
  EmptyState,
  Field,
  Notice,
  ResourceState,
  Select,
  Textarea,
} from '@/components/ui/primitives';
import { AccountAccess } from '@/features/account/account-access';
import { apiRequest } from '@/lib/api/client';
import { safeHref, useResource } from '@/lib/api/resources';
import { useAppState } from '@/lib/state/store';
import { dateLabel } from '@/lib/format';

type View = 'queue' | 'import' | 'history';
interface Row {
  id: string;
  name?: string;
  company_name?: string;
  message?: string;
  revision?: number;
  proposed_company?: Record<string, string | null>;
  payload?: { source_url?: string; attribution?: string };
  status?: string;
  kind?: string;
  processed?: number;
  total?: number;
  attempt?: number;
  error_code?: string;
  action?: string;
  reason?: string;
  moderator_name?: string;
  created_at?: string;
}
interface Rows {
  items: Row[];
  total: number;
}
const emptyRows: Rows = { items: [], total: 0 };
async function loadRows(path: string, signal: AbortSignal): Promise<Rows> {
  const value = await apiRequest<Rows>(path, { signal });
  if (!value || !Array.isArray(value.items) || typeof value.total !== 'number')
    throw new Error('Не удалось прочитать список. Обновите данные.');
  return value;
}
type Mutation = (
  path: string,
  method: 'POST' | 'PATCH',
  body?: unknown,
  headers?: HeadersInit,
) => Promise<void>;
export function RealModerationPage({ view }: { view: View }) {
  const state = useAppState();
  return (
    <AccountShell
      title={
        view === 'queue' ? 'Модерация' : view === 'import' ? 'Сбор данных' : 'Журнал модерации'
      }
      role="moderator"
    >
      <AccountAccess>
        {state.profile.role === 'moderator' ? (
          <ModerationContent key={view} view={view} />
        ) : (
          <EmptyState
            title="Нужны права модератора"
            description="Доступ выдаётся владельцем проекта."
          />
        )}
      </AccountAccess>
    </AccountShell>
  );
}
function ModerationContent({ view }: { view: View }) {
  const [tab, setTab] = useState(
    view === 'queue' ? 'candidates' : view === 'history' ? 'candidate-actions' : 'jobs',
  );
  const [page, setPage] = useState(1),
    [version, setVersion] = useState(0);
  const summary = useResource<Record<string, number>>(
    `/v1/moderation/summary?refresh=${version}`,
    {},
  );
  const path =
    tab === 'candidates'
      ? '/imports/candidates?status=pending'
      : tab === 'corrections'
        ? '/moderation/correction-requests?status=pending'
        : tab === 'hidden'
          ? '/companies?visibility=hidden'
          : tab === 'jobs'
            ? '/collection/jobs?'
            : `/moderation/${tab}?`;
  const list = useResource<Rows>(
    `/v1${path}&page=${page}&limit=10&refresh=${version}`,
    emptyRows,
    loadRows,
  );
  const refresh = () => setVersion((value) => value + 1);
  const mutate: Mutation = async (path, method, body, headers) => {
    await apiRequest(`/v1${path}`, { method, body, headers });
    refresh();
  };
  const tabs =
    view === 'queue'
      ? [
          ['candidates', 'Собранные карточки'],
          ['corrections', 'Обращения'],
          ['hidden', 'Скрытые компании'],
        ]
      : view === 'history'
        ? [
            ['candidate-actions', 'Решения по карточкам'],
            ['actions', 'Компании и заказы'],
          ]
        : [];
  return (
    <>
      {summary.data ? (
        <p className="muted">
          На проверке: {summary.data.pending_candidates ?? 0} · Обращений:{' '}
          {summary.data.pending_corrections ?? 0} · Скрытых компаний:{' '}
          {summary.data.hidden_companies ?? 0}
        </p>
      ) : (
        <ResourceState error={summary.error} reload={summary.reload} />
      )}
      {tabs.length > 0 && (
        <nav className="button-row" aria-label="Раздел модерации">
          {tabs.map(([key, label]) => (
            <button
              key={key}
              className={`button ${tab === key ? '' : 'secondary'}`}
              aria-pressed={tab === key}
              onClick={() => {
                setTab(key);
                setPage(1);
              }}
            >
              {label}
            </button>
          ))}
        </nav>
      )}
      {view === 'import' && <CollectionControls mutate={mutate} version={version} />}
      <button className="button secondary align-start" onClick={refresh}>
        Обновить данные
      </button>
      {!list.data ? (
        <ResourceState error={list.error} reload={list.reload} />
      ) : (
        <>
          {!list.data.items.length && (
            <EmptyState
              title="Список пуст"
              description="Новые записи появятся здесь после сбора данных или действий пользователей."
            />
          )}
          {list.data.items.map((item) => {
            if (tab === 'candidates')
              return (
                <CandidateCard key={`${item.id}-${item.revision}`} item={item} mutate={mutate} />
              );
            if (tab === 'corrections')
              return (
                <DecisionForm
                  key={item.id}
                  title={item.company_name || 'Обращение'}
                  onSubmit={async (reason, decision) => {
                    await mutate(`/moderation/correction-requests/${item.id}`, 'PATCH', {
                      status: decision === 'approve' ? 'approved' : 'rejected',
                      resolution_note: reason,
                    });
                  }}
                >
                  <p className="preserve-lines">{item.message}</p>
                  <p className="muted">
                    Подтверждение сохраняет решение. Данные компании автоматически не меняются.
                  </p>
                </DecisionForm>
              );
            if (tab === 'hidden')
              return (
                <DecisionForm
                  key={item.id}
                  title={item.name || 'Компания'}
                  approveLabel="Опубликовать"
                  onSubmit={async (reason) => {
                    await mutate(`/companies/${item.id}/visibility`, 'PATCH', {
                      visibility: 'published',
                      reason,
                    });
                  }}
                >
                  <Link className="text-link" href={`/companies/${item.id}`}>
                    Посмотреть карточку
                  </Link>
                </DecisionForm>
              );
            if (tab === 'jobs')
              return <JobCard key={`${item.id}-${item.status}`} item={item} mutate={mutate} />;
            return (
              <article className="card stack" key={item.id}>
                <h3>{actionLabel(item.action)}</h3>
                <p className="preserve-lines">{item.reason || 'Без комментария'}</p>
                <p className="small muted">
                  {dateLabel(item.created_at || '')} · {item.moderator_name || 'Оператор сервера'}
                </p>
              </article>
            );
          })}
          <nav className="button-row" aria-label="Страницы модерации">
            <button
              className="button secondary"
              disabled={page <= 1}
              onClick={() => setPage((value) => value - 1)}
            >
              Назад
            </button>
            <span>
              Страница {page} из {Math.max(1, Math.ceil(list.data.total / 10))}
            </span>
            <button
              className="button secondary"
              disabled={page * 10 >= list.data.total}
              onClick={() => setPage((value) => value + 1)}
            >
              Далее
            </button>
          </nav>
        </>
      )}
    </>
  );
}
function DecisionForm({
  title,
  children,
  onSubmit,
  approveLabel,
}: {
  title: string;
  children?: ReactNode;
  onSubmit: (reason: string, decision: string) => Promise<void>;
  approveLabel?: string;
}) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState('');
  return (
    <form
      className="card stack"
      onSubmit={async (event) => {
        event.preventDefault();
        if (busy) return;
        const reason = String(new FormData(event.currentTarget).get('reason') || '').trim();
        if (reason.length < 5) {
          setError('Укажите основание решения: минимум 5 символов.');
          return;
        }
        const decision =
          (event.nativeEvent as SubmitEvent).submitter?.getAttribute('value') || 'approve';
        setBusy(true);
        setError('');
        try {
          await onSubmit(reason, decision);
        } catch (cause) {
          setError(cause instanceof Error ? cause.message : 'Не удалось сохранить решение.');
        } finally {
          setBusy(false);
        }
      }}
    >
      <h3>{title}</h3>
      {children}
      <Textarea
        label="Основание решения"
        name="reason"
        required
        minLength={5}
        maxLength={2000}
        disabled={busy}
      />
      {error && <Notice error>{error}</Notice>}
      <div className="button-row">
        <button className="button" value="approve" disabled={busy}>
          {busy ? 'Сохраняем…' : approveLabel || 'Одобрить карточку'}
        </button>
        {!approveLabel && (
          <button className="button secondary" value="reject" disabled={busy}>
            Отклонить
          </button>
        )}
      </div>
    </form>
  );
}
function CandidateCard({ item, mutate }: { item: Row; mutate: Mutation }) {
  const company = item.proposed_company || {},
    source = safeHref(item.payload?.source_url || '');
  if (!Number.isInteger(item.revision) || (item.revision || 0) < 1)
    return <Notice error>Не удалось прочитать версию карточки. Обновите данные.</Notice>;
  return (
    <div className="stack">
      <DecisionForm
        title={company.name || 'Карточка компании'}
        onSubmit={async (reason, decision) => {
          await mutate(`/imports/candidates/${item.id}/review`, 'POST', {
            decision,
            reason,
            expected_revision: item.revision,
          });
        }}
      >
        <p>{[company.city, company.address, company.category_name].filter(Boolean).join(' · ')}</p>
        <p>{[company.phone, company.email].filter(Boolean).join(' · ') || 'Контакты не указаны'}</p>
        <p className="muted">
          {company.website_status === 'found'
            ? 'Сайт найден'
            : company.website_status === 'not_found'
              ? 'Сайт не найден'
              : 'Наличие сайта требует проверки'}
          {company.website_url ? ` · ${company.website_url}` : ''}
        </p>
        {source && (
          <a className="text-link" href={source} target="_blank" rel="noreferrer">
            Проверить первоисточник
          </a>
        )}
        <p className="small muted">{item.payload?.attribution}</p>
        <p className="small muted">
          Одобрение создаёт скрытую компанию. Публикация выполняется отдельно в разделе «Скрытые
          компании».
        </p>
      </DecisionForm>
      <CandidateEditor item={item} mutate={mutate} />
    </div>
  );
}
function CollectionControls({ mutate, version }: { mutate: Mutation; version: number }) {
  const submission = useRef<{ payload: string; key: string } | null>(null);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState('');
  const policy = useResource<{
    enabled: boolean;
    minimum: number;
    target: number;
    counts: { available: number; pending: number };
  }>(`/v1/collection/replenishment?refresh=${version}`, {
    enabled: false,
    minimum: 0,
    target: 0,
    counts: { available: 0, pending: 0 },
  });
  return (
    <form
      className="card stack"
      onSubmit={async (event) => {
        event.preventDefault();
        if (busy) return;
        const data = new FormData(event.currentTarget);
        setBusy(true);
        setError('');
        try {
          const body = { kind: data.get('kind'), limit: Number(data.get('limit')) };
          const payload = JSON.stringify(body);
          if (!submission.current || submission.current.payload !== payload)
            submission.current = { payload, key: crypto.randomUUID() };
          await mutate('/collection/jobs', 'POST', body, {
            'Idempotency-Key': submission.current.key,
          });
          submission.current = null;
        } catch (cause) {
          setError(cause instanceof Error ? cause.message : 'Не удалось запустить сбор.');
        } finally {
          setBusy(false);
        }
      }}
    >
      <h2>Сбор и проверка компаний</h2>
      <p className="muted">
        Новые карточки OSM поступают на модерацию. Публикация выполняется отдельно.
      </p>
      {policy.data ? (
        <p>
          Автопополнение {policy.data.enabled ? 'включено' : 'выключено'}: порог{' '}
          {policy.data.minimum}, цель {policy.data.target}. Доступно {policy.data.counts.available},
          ожидают проверки {policy.data.counts.pending}.
        </p>
      ) : (
        <ResourceState error={policy.error} reload={policy.reload} />
      )}
      <Select label="Действие" name="kind" disabled={busy}>
        <option value="collect">Найти новые компании</option>
        <option value="recheck">Повторно проверить сайты</option>
      </Select>
      <Field
        label="Количество компаний"
        name="limit"
        type="number"
        min={1}
        max={100}
        defaultValue={25}
        required
        disabled={busy}
      />
      {error && <Notice error>{error}</Notice>}
      <button className="button align-start" disabled={busy}>
        {busy ? 'Ставим в очередь…' : 'Запустить задание'}
      </button>
    </form>
  );
}
function JobCard({ item, mutate }: { item: Row; mutate: Mutation }) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState('');
  const active = item.status === 'pending' || item.status === 'running';
  async function control(action: 'cancel' | 'retry') {
    setBusy(true);
    setError('');
    try {
      await mutate(`/collection/jobs/${item.id}/${action}`, 'POST');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось изменить задание.');
    } finally {
      setBusy(false);
    }
  }
  const statuses: Record<string, string> = {
    pending: 'В очереди',
    running: 'Выполняется',
    completed: 'Завершено',
    failed: 'Ошибка',
    canceled: 'Отменено',
  };
  return (
    <article className="card stack">
      <h3>
        {item.kind === 'recheck' ? 'Проверка сайтов' : 'Сбор компаний'} ·{' '}
        {statuses[item.status || ''] || item.status}
      </h3>
      <p>
        Обработано {item.processed || 0} из {item.total || 0}
      </p>
      {item.error_code && <p className="muted">{collectionError(item.error_code)}</p>}
      {error && <Notice error>{error}</Notice>}
      {active && (
        <button
          className="button secondary align-start"
          disabled={busy}
          onClick={() => void control('cancel')}
        >
          Отменить задание
        </button>
      )}
      {item.status === 'failed' && (item.attempt || 0) < 3 && (
        <button
          className="button secondary align-start"
          disabled={busy}
          onClick={() => void control('retry')}
        >
          Повторить задание
        </button>
      )}
    </article>
  );
}
function actionLabel(action?: string) {
  const labels: Record<string, string> = {
    edit: 'Карточка исправлена',
    correct: 'Компания исправлена',
    approve: 'Карточка одобрена',
    reject: 'Карточка отклонена',
    link: 'Источник связан с компанией',
    publish: 'Компания опубликована',
    hide: 'Компания скрыта',
    correction_approved: 'Обращение подтверждено',
    correction_rejected: 'Обращение отклонено',
  };
  return labels[action || ''] || action || 'Действие модератора';
}
function collectionError(code: string) {
  const messages: Record<string, string> = {
    SOURCE_UNAVAILABLE: 'Источник временно недоступен. Повторите задание позже.',
    SOURCE_INVALID: 'Источник вернул неполные данные. Требуется проверка.',
    CHECKPOINT_FAILED: 'Не удалось сохранить прогресс. Завершённые проверки сохранены.',
    WORKER_INTERRUPTED: 'Сбор был прерван. Можно продолжить с сохранённого места.',
    JOB_TIMEOUT: 'Проверка заняла слишком много времени. Можно повторить задание.',
  };
  return messages[code] || 'Задание завершилось с ошибкой. Проверьте историю и повторите позже.';
}

function CandidateEditor({ item, mutate }: { item: Row; mutate: Mutation }) {
  const company = item.proposed_company || {};
  const [busy, setBusy] = useState(false),
    [error, setError] = useState('');
  const fields = [
    ['name', 'Название'],
    ['city', 'Город компании'],
    ['category_name', 'Категория'],
    ['address', 'Адрес'],
    ['phone', 'Телефон'],
    ['email', 'Email компании'],
    ['website_url', 'Сайт компании'],
  ];
  return (
    <details className="card">
      <summary>Исправить карточку</summary>
      <form
        className="stack"
        onSubmit={async (event) => {
          event.preventDefault();
          if (busy) return;
          const data = new FormData(event.currentTarget),
            changes: Record<string, string | null> = {};
          for (const [key] of fields) {
            const value = String(data.get(key) || '').trim();
            if (value !== (company[key] || '')) changes[key] = value || null;
          }
          if ('category_name' in changes) changes.category_id = null;
          const status = String(data.get('website_status'));
          if (status !== company.website_status) changes.website_status = status;
          if (!Object.keys(changes).length) {
            setError('Нет изменений для сохранения.');
            return;
          }
          const reason = String(data.get('reason') || '').trim();
          if (reason.length < 5) {
            setError('Укажите основание исправления: минимум 5 символов.');
            return;
          }
          setBusy(true);
          setError('');
          try {
            await mutate(`/imports/candidates/${item.id}`, 'PATCH', {
              expected_revision: item.revision,
              reason,
              changes,
            });
          } catch (cause) {
            setError(cause instanceof Error ? cause.message : 'Не удалось сохранить исправления.');
          } finally {
            setBusy(false);
          }
        }}
      >
        {fields.map(([key, label]) => (
          <Field
            key={key}
            label={label}
            name={key}
            defaultValue={company[key] || ''}
            disabled={busy}
            required={key === 'name' || key === 'city'}
            type={key === 'email' ? 'email' : key === 'website_url' ? 'url' : 'text'}
            maxLength={key === 'name' || key === 'city' ? 240 : 4000}
          />
        ))}
        <Select
          label="Наличие сайта"
          name="website_status"
          defaultValue={company.website_status || 'needs_review'}
          disabled={busy}
        >
          <option value="needs_review">Нужна проверка</option>
          <option value="found">Сайт найден</option>
          <option value="not_found">Сайт не найден</option>
        </Select>
        <Textarea
          label="Основание исправления"
          name="reason"
          required
          minLength={5}
          maxLength={2000}
          disabled={busy}
        />
        {error && <Notice error>{error}</Notice>}
        <button className="button" disabled={busy}>
          {busy ? 'Сохраняем…' : 'Сохранить исправления'}
        </button>
      </form>
    </details>
  );
}
