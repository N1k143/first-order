'use client';

import Link from 'next/link';
import { useState, type FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import { AccountShell } from '@/components/layout/account-shell';
import { Avatar } from '@/components/ui/avatar';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  EmptyState,
  Field,
  Textarea,
  Select,
  Notice,
  ResourceState,
  money,
  dateLabel,
} from '@/components/ui/primitives';
import { useAppState, refreshAppState } from '@/lib/state/store';
import { apiRequest } from '@/lib/api/client';
import { mutate, safeHref, useResource } from '@/lib/api/resources';
import { portfolioFields } from '@/lib/api/profile-fields';
import { contactFields } from '@/lib/api/form-fields';
import { applicationLabels } from '@/lib/state/order-actions';
import type { Application, Company, Contact, Portfolio, Profile } from '@/types/domain';
import type { AccountView } from './account-page';
import { AccountAccess } from './account-access';
import { ProfileEditor } from './profile-editor';

const titles: Record<AccountView, string> = {
  dashboard: 'Мой кабинет',
  applications: 'Мои отклики',
  saved: 'Избранные компании',
  contacts: 'История контактов',
  profile: 'Мой профиль',
  edit: 'Редактирование профиля',
  portfolio: 'Редактор портфолио',
  settings: 'Настройки',
};
const noLocalMutation = () => {};
function errorMessage(error: unknown) {
  return error instanceof Error
    ? error.message
    : 'Не удалось сохранить изменения. Попробуйте ещё раз.';
}
function useList<T>(path: string) {
  const [page, setPage] = useState(1);
  const resource = useResource<T[] | { items: T[]; total: number }>(
    `${path}?limit=100&page=${page}`,
    [],
  );
  const data = resource.data;
  const rows = data ? (Array.isArray(data) ? data : data.items) : [];
  const total = data && !Array.isArray(data) ? data.total : undefined;
  return {
    ...resource,
    rows,
    total,
    page,
    setPage,
    hasNext: total === undefined ? rows.length === 100 : page * 100 < total,
  };
}
function Pages({
  page,
  setPage,
  hasNext,
}: {
  page: number;
  setPage: (page: number) => void;
  hasNext: boolean;
}) {
  return page > 1 || hasNext ? (
    <nav className="button-row" aria-label="Страницы списка">
      <button className="button secondary" disabled={page === 1} onClick={() => setPage(page - 1)}>
        Назад
      </button>
      <span>Страница {page}</span>
      <button className="button secondary" disabled={!hasNext} onClick={() => setPage(page + 1)}>
        Далее
      </button>
    </nav>
  ) : null;
}
export function RealAccountPage({ view }: { view: AccountView }) {
  const state = useAppState();
  return (
    <AccountShell title={titles[view]} role={state.profile.role}>
      <AccountAccess>
        <AccountContent view={view} />
      </AccountAccess>
    </AccountShell>
  );
}
function AccountContent({ view }: { view: AccountView }) {
  if (view === 'saved') return <SavedCompanies />;
  if (view === 'contacts') return <ContactHistory />;
  if (view === 'portfolio') return <PortfolioList editable />;
  if (view === 'settings') return <Settings />;
  if (view === 'profile' || view === 'edit') return <ProfileScreen edit={view === 'edit'} />;
  return <Dashboard />;
}
function ProfileScreen({ edit }: { edit: boolean }) {
  const state = useAppState();
  const resource = useResource<Profile>('/me', state.profile);
  const profile = resource.data;
  if (!profile) return <ResourceState error={resource.error} reload={resource.reload} />;
  if (edit)
    return (
      <>
        <nav className="tabs">
          <Link href="/profile/edit" aria-current="page">
            Основная информация
          </Link>
          <Link href="/profile/portfolio">Портфолио</Link>
          <Link href="/profile">Предпросмотр профиля</Link>
        </nav>
        <ProfileEditor profile={profile} key={profile.id} />
      </>
    );
  return (
    <>
      <article className="card stack">
        <Avatar name={profile.name} src={profile.avatarUrl} size={80} />
        <h2>{profile.name}</h2>
        <p className="preserve-lines">{profile.bio || 'Добавьте несколько слов о себе.'}</p>
        <div className="button-row">
          {(profile.skills || []).map((skill) => (
            <span className="badge" key={skill}>
              {skill}
            </span>
          ))}
        </div>
        {safeHref(profile.github || '') && (
          <a
            className="text-link"
            href={safeHref(profile.github)}
            target="_blank"
            rel="noopener noreferrer"
          >
            GitHub ↗
          </a>
        )}
        <Link className="button secondary align-start" href="/profile/edit">
          Редактировать профиль
        </Link>
      </article>
      <h2>Портфолио</h2>
      <PortfolioList />
    </>
  );
}
function PortfolioList({ editable = false }: { editable?: boolean }) {
  const resource = useList<Portfolio>('/me/portfolio');
  return (
    <>
      {editable && <PortfolioForm />}
      {resource.data ? (
        <>
          {resource.rows.length ? (
            resource.rows.map((item) =>
              editable ? (
                <PortfolioForm key={item.id} item={item} />
              ) : (
                <article className="card stack" key={item.id}>
                  <h3>{item.title}</h3>
                  <p className="preserve-lines">{item.description}</p>
                  {safeHref(item.url || '') && (
                    <a
                      className="text-link"
                      href={safeHref(item.url)}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      Посмотреть проект ↗
                    </a>
                  )}
                </article>
              ),
            )
          ) : (
            <EmptyState
              title="Портфолио пока пусто"
              description="Добавьте учебный или рабочий проект и расскажите о своей роли."
              {...(!editable ? { href: '/profile/portfolio', label: 'Добавить работу' } : {})}
            />
          )}
          <Pages {...resource} />
        </>
      ) : (
        <ResourceState error={resource.error} reload={resource.reload} />
      )}
    </>
  );
}
function PortfolioForm({ item }: { item?: Portfolio }) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [message, setMessage] = useState(''),
    [remove, setRemove] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    const form = event.currentTarget,
      fields = portfolioFields(new FormData(form));
    if (!fields.title) {
      setError('Укажите название работы.');
      return;
    }
    setBusy(true);
    setError('');
    setMessage('');
    try {
      await mutate(
        item ? `/me/portfolio/${encodeURIComponent(item.id)}` : '/me/portfolio',
        item ? 'PATCH' : 'POST',
        fields,
        noLocalMutation,
      );
      if (!item) form.reset();
      setMessage(item ? 'Работа обновлена.' : 'Работа добавлена в портфолио.');
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }
  async function removeItem() {
    if (!item || busy) return;
    setBusy(true);
    setError('');
    try {
      await mutate(
        `/me/portfolio/${encodeURIComponent(item.id)}`,
        'DELETE',
        undefined,
        noLocalMutation,
      );
      setRemove(false);
    } catch (cause) {
      setError(errorMessage(cause));
      setRemove(false);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <form
        className="card stack"
        onSubmit={submit}
        data-testid={item ? 'portfolio-item' : 'portfolio-create'}
      >
        <h3>{item ? item.title : 'Добавить работу'}</h3>
        <Field
          label={item ? 'Название работы' : 'Название проекта'}
          name="title"
          defaultValue={item?.title || ''}
          required
          maxLength={120}
          disabled={busy}
        />
        <Textarea
          label={item ? 'Описание работы' : 'Описание и ваша роль'}
          name="description"
          defaultValue={item?.description || ''}
          maxLength={2000}
          disabled={busy}
        />
        <Field
          label={item ? 'Адрес проекта' : 'Ссылка на проект'}
          name="url"
          type="url"
          pattern="https?://.*"
          defaultValue={item?.url || ''}
          disabled={busy}
        />
        <div className="button-row">
          <button className="button" disabled={busy}>
            {busy ? 'Сохраняем…' : item ? 'Сохранить работу' : 'Добавить в портфолио'}
          </button>
          {item && (
            <button
              type="button"
              className="text-button"
              disabled={busy}
              onClick={() => setRemove(true)}
            >
              Удалить работу
            </button>
          )}
        </div>
        {error && <Notice error>{error}</Notice>}
        {message && <Notice>{message}</Notice>}
      </form>
      <ConfirmDialog
        open={remove}
        title="Удалить работу?"
        description="Работа будет удалена из вашего портфолио."
        confirmLabel={busy ? 'Удаляем…' : 'Удалить работу'}
        onClose={() => {
          if (!busy) setRemove(false);
        }}
        onConfirm={() => void removeItem()}
      />
    </>
  );
}
function SavedCompanies() {
  const resource = useList<Company>('/me/favorites');
  return resource.data ? (
    <>
      <p className="muted">Сохранено компаний: {resource.total ?? resource.rows.length}</p>
      {resource.rows.length ? (
        resource.rows.map((company) => <SavedCompany key={company.id} company={company} />)
      ) : (
        <EmptyState
          title="Сохраните интересные компании"
          description="Они появятся здесь, чтобы вы могли вернуться к ним позже."
          href="/companies"
          label="Открыть каталог"
        />
      )}
      <Pages {...resource} />
    </>
  ) : (
    <ResourceState error={resource.error} reload={resource.reload} />
  );
}
function SavedCompany({ company }: { company: Company }) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState('');
  async function remove() {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      await mutate(
        `/me/favorites/${encodeURIComponent(company.id)}`,
        'DELETE',
        undefined,
        noLocalMutation,
      );
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }
  return (
    <article className="card stack" data-testid="saved-company">
      <h3>
        <Link href={`/companies/${company.id}`}>{company.name}</Link>
      </h3>
      <p className="muted">
        {company.category} · {company.city}
      </p>
      <p>{company.address || 'Адрес не указан'}</p>
      <div className="button-row">
        <Link className="button secondary" href={`/companies/${company.id}`}>
          О компании
        </Link>
        <button className="text-button" disabled={busy} onClick={() => void remove()}>
          {busy ? 'Удаляем…' : 'Убрать из избранного'}
        </button>
      </div>
      {error && <Notice error>{error}</Notice>}
    </article>
  );
}
function ContactHistory() {
  const resource = useList<Contact>('/me/contacts');
  return resource.data ? (
    <>
      {resource.rows.length ? (
        resource.rows.map((contact) => <ContactEntry key={contact.companyId} contact={contact} />)
      ) : (
        <EmptyState
          title="Контактов пока нет"
          description="Добавьте заметку в карточке компании, чтобы сохранить историю общения."
          href="/companies"
          label="Открыть каталог"
        />
      )}
      <Pages {...resource} />
    </>
  ) : (
    <ResourceState error={resource.error} reload={resource.reload} />
  );
}
const contactOptions = {
  planned: 'Планирую написать',
  contacted: 'Написал',
  replied: 'Ответили',
  declined: 'Не актуально',
};
function ContactEntry({ contact }: { contact: Contact }) {
  const company = useResource<Company | undefined>(
    `/companies/${encodeURIComponent(contact.companyId)}`,
    undefined,
  );
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [message, setMessage] = useState('');
  const status =
    Object.entries(contactOptions).find(
      ([key, label]) => contact.status === key || contact.status === label,
    )?.[0] || 'planned';
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    const fields = contactFields(
      new FormData(event.currentTarget),
      contact.contactedAt || contact.date,
    );
    setBusy(true);
    setError('');
    setMessage('');
    try {
      await mutate(
        `/me/contacts/${encodeURIComponent(contact.companyId)}`,
        'PUT',
        fields,
        noLocalMutation,
      );
      setMessage('Заметка сохранена.');
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }
  return (
    <form className="card stack" onSubmit={submit} data-testid="contact-entry">
      <h3>
        {company.data?.name || (company.error ? 'Компания недоступна' : 'Загрузка компании…')}
      </h3>
      {company.error && <Notice error>{company.error}</Notice>}
      <Select label="Статус контакта" name="status" defaultValue={status} disabled={busy}>
        {Object.entries(contactOptions).map(([value, label]) => (
          <option key={value} value={value}>
            {label}
          </option>
        ))}
      </Select>
      <Field
        label="Дата контакта"
        name="contacted_at"
        type="date"
        defaultValue={(contact.contactedAt || contact.date || '').slice(0, 10)}
        disabled={busy}
      />
      <Textarea
        label="Моя заметка"
        name="note"
        defaultValue={contact.note || ''}
        maxLength={10000}
        disabled={busy}
      />
      <div className="button-row">
        <button className="button secondary" disabled={busy}>
          {busy ? 'Сохраняем…' : 'Сохранить заметку'}
        </button>
        <Link href={`/companies/${contact.companyId}/contacts`} className="text-link">
          Контакты компании →
        </Link>
      </div>
      {error && <Notice error>{error}</Notice>}
      {message && <Notice>{message}</Notice>}
    </form>
  );
}
function Dashboard() {
  const state = useAppState();
  const applications = useList<Application>('/me/applications');
  const favorites = useList<Company>('/me/favorites');
  const contacts = useList<Contact>('/me/contacts');
  return (
    <>
      <p>Здравствуйте, {state.profile.name}. Здесь ваши проекты и контакты.</p>
      <div className="stats">
        {[
          { resource: applications, label: 'Откликов' },
          { resource: favorites, label: 'В избранном' },
          { resource: contacts, label: 'Контактов' },
        ].map(({ resource, label }) => (
          <div className="card stat" key={label}>
            <strong>{resource.data ? (resource.total ?? resource.rows.length) : '—'}</strong>
            <span>{label}</span>
            {resource.error && <Notice error>{resource.error}</Notice>}
          </div>
        ))}
      </div>
      <div className="button-row">
        <Link className="button" href="/profile/edit">
          Редактировать профиль
        </Link>
        <Link className="button secondary" href="/profile/portfolio">
          Добавить работу
        </Link>
      </div>
      <h2>Последние отклики</h2>
      {applications.data ? (
        applications.rows.length ? (
          applications.rows.slice(0, 3).map((application) => (
            <article className="card stack" key={application.id}>
              <span className="badge align-start">{applicationLabels[application.status]}</span>
              <h3>{application.orderTitle || 'Заказ'}</h3>
              <p>
                {application.companyName} · {money(application.price)}
              </p>
              {application.deadline && <p>Срок: {dateLabel(application.deadline)}</p>}
              <Link href={`/orders/${application.orderId}`} className="text-link">
                Открыть заказ →
              </Link>
            </article>
          ))
        ) : (
          <EmptyState
            title="Ваш первый проект впереди"
            description="Выберите задачу и предложите решение."
            href="/orders"
            label="Посмотреть заказы"
          />
        )
      ) : (
        <ResourceState error={applications.error} reload={applications.reload} />
      )}
      <Link href="/applications" className="text-link">
        Все отклики →
      </Link>
    </>
  );
}
function Settings() {
  const state = useAppState(),
    router = useRouter();
  const profile = useResource<Profile>('/me', state.profile);
  const [busy, setBusy] = useState(''),
    [error, setError] = useState(''),
    [message, setMessage] = useState('');
  async function changeEmail(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    const email = String(new FormData(event.currentTarget).get('email') || '').trim();
    setBusy('email');
    setError('');
    setMessage('');
    try {
      const result = await apiRequest<{ status: string }>('/auth/change-email', {
        method: 'POST',
        body: { email },
      });
      if (result.status === 'updated') {
        await refreshAppState();
        profile.reload();
        setMessage('Email обновлён.');
      } else if (result.status === 'confirmation_required') {
        setMessage(
          'Проверьте почту. Для смены адреса может потребоваться подтверждение по ссылкам в обоих почтовых ящиках. До завершения подтверждения действует прежний email.',
        );
      } else throw new Error('Не удалось подтвердить результат смены email. Обновите страницу.');
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy('');
    }
  }
  async function changePassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    const form = event.currentTarget,
      fields = new FormData(form),
      password = String(fields.get('password')),
      confirm = String(fields.get('confirm'));
    if (password !== confirm) {
      setError('Пароли не совпадают.');
      return;
    }
    setBusy('password');
    setError('');
    setMessage('');
    try {
      await apiRequest('/auth/reset-password', { method: 'POST', body: { password, confirm } });
      form.reset();
      setMessage('Пароль обновлён.');
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy('');
    }
  }
  async function logout() {
    if (busy) return;
    setBusy('logout');
    setError('');
    try {
      await apiRequest('/auth/logout', { method: 'POST' });
      await refreshAppState();
      router.replace('/login');
    } catch (cause) {
      setError(errorMessage(cause));
      setBusy('');
    }
  }
  if (!profile.data) return <ResourceState error={profile.error} reload={profile.reload} />;
  return (
    <>
      <form className="card stack" onSubmit={changeEmail}>
        <h3>Email аккаунта</h3>
        <p>
          Текущий адрес: <strong data-testid="current-account-email">{profile.data.email}</strong>
        </p>
        <Field
          label="Новый email"
          name="email"
          type="email"
          required
          defaultValue={profile.data.email}
          disabled={!!busy}
        />
        <button className="button align-start" disabled={!!busy}>
          {busy === 'email' ? 'Отправляем…' : 'Отправить подтверждение'}
        </button>
      </form>
      <form className="card stack" onSubmit={changePassword}>
        <h3>Пароль</h3>
        <Field
          label="Новый пароль"
          name="password"
          type="password"
          autoComplete="new-password"
          required
          minLength={8}
          maxLength={256}
          disabled={!!busy}
        />
        <Field
          label="Повторите пароль"
          name="confirm"
          type="password"
          autoComplete="new-password"
          required
          minLength={8}
          maxLength={256}
          disabled={!!busy}
        />
        <button className="button align-start" disabled={!!busy}>
          {busy === 'password' ? 'Сохраняем…' : 'Изменить пароль'}
        </button>
      </form>
      <section className="card stack">
        <h3>Сессия</h3>
        <button
          className="button secondary align-start"
          disabled={!!busy}
          onClick={() => void logout()}
        >
          {busy === 'logout' ? 'Выходим…' : 'Выйти из аккаунта'}
        </button>
      </section>
      {error && <Notice error>{error}</Notice>}
      {message && <Notice>{message}</Notice>}
    </>
  );
}
