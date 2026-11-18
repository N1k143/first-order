'use client';

import Link from 'next/link';
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { AccountShell } from '@/components/layout/account-shell';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { EmptyState, Field, Textarea, Notice, money, dateLabel } from '@/components/ui/primitives';
import { CompanyCard } from '@/features/catalog/company-card';
import { companies } from '@/data/demo';
import { useAppState, updateDemo, resetDemo } from '@/lib/state/store';
import { safeHref } from '@/lib/api/resources';
import { applicationLabels, withdrawApplication } from '@/lib/state/order-actions';
import { AccountAccess, BackendStatus } from './account-access';

type View =
  | 'dashboard'
  | 'applications'
  | 'saved'
  | 'contacts'
  | 'profile'
  | 'edit'
  | 'portfolio'
  | 'settings';
const titles: Record<View, string> = {
  dashboard: 'Кабинет разработчика',
  applications: 'Мои отклики',
  saved: 'Избранные компании',
  contacts: 'История контактов',
  profile: 'Мой профиль',
  edit: 'Редактирование профиля',
  portfolio: 'Редактор портфолио',
  settings: 'Настройки',
};

export function AccountPage({ view }: { view: View }) {
  const state = useAppState();
  const router = useRouter();
  const [message, setMessage] = useState('');
  const [filter, setFilter] = useState('all');
  const [withdraw, setWithdraw] = useState<string | null>(null);
  const [remove, setRemove] = useState<string | null>(null);
  const [reset, setReset] = useState(false);
  const applications = state.applications.filter((a) => a.profileId === state.profile.id);
  const visibleApplications = applications.filter((a) => filter === 'all' || a.status === filter);
  const profile = state.profile;
  const applicationCards = (
    view === 'dashboard' ? applications.slice(0, 3) : visibleApplications
  ).map((a) => {
    const order = state.orders.find((o) => o.id === a.orderId);
    return (
      <article className="card stack" key={a.id}>
        <span className="badge align-start">{applicationLabels[a.status]}</span>
        <h2>{order?.title || 'Заказ недоступен'}</h2>
        <p className="small muted">
          {order?.companyName} · До {dateLabel(a.deadline)}
        </p>
        <p>Ваше предложение: {money(a.price)}</p>
        <p className="preserve-lines">{a.letter}</p>
        <div className="button-row">
          <Link className="button secondary" href={`/orders/${a.orderId}`}>
            Открыть заказ
          </Link>
          {a.status === 'pending' && (
            <button className="button secondary" onClick={() => setWithdraw(a.id)}>
              Отозвать отклик
            </button>
          )}
          {a.status === 'selected' && order && (
            <Link className="button" href={`/companies/${order.companyId}/contacts`}>
              Контакты заказчика
            </Link>
          )}
        </div>
      </article>
    );
  });
  return (
    <AccountShell
      title={view === 'profile' ? profile.name : titles[view]}
      description={
        view === 'dashboard'
          ? `Здравствуйте, ${profile.name.split(' ')[0]}. Здесь ваши проекты и контакты.`
          : 'Личный кабинет FirstOrder'
      }
      role={view === 'settings' ? profile.role : 'developer'}
    >
      <AccountAccess>
        {view === 'dashboard' && (
          <>
            <div className="stats">
              {[
                [applications.length, 'Откликов'],
                [state.favorites.length, 'В избранном'],
                [state.contacts.length, 'Контактов'],
              ].map(([count, label]) => (
                <div className="card stat" key={label}>
                  <strong>{count}</strong>
                  <span className="small muted">{label}</span>
                </div>
              ))}
            </div>
            <article className="card stack soft-card">
              <h3>Покажите, что вы умеете</h3>
              <p className="muted">
                Заполните профиль и добавьте работы. Учебные проекты тоже помогут заказчику понять
                ваш подход.
              </p>
              <Link className="button align-start" href="/profile/edit">
                Редактировать профиль
              </Link>
            </article>
            <h2>Последние отклики</h2>
            {applicationCards.length ? (
              applicationCards
            ) : (
              <EmptyState
                title="Ваш первый проект впереди"
                description="Выберите задачу и предложите решение."
                href="/orders"
                label="Посмотреть заказы"
              />
            )}
            <Link className="text-link" href="/applications">
              Все отклики →
            </Link>
          </>
        )}
        {view === 'applications' && (
          <>
            <nav className="tabs" aria-label="Статус откликов">
              {[
                ['all', 'Все'],
                ['pending', 'На рассмотрении'],
                ['selected', 'Вы исполнитель'],
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
            {applicationCards.length ? (
              applicationCards
            ) : (
              <EmptyState
                title="Нет откликов"
                description="Здесь будут предложения с выбранным статусом."
                href="/orders"
                label="Найти заказ"
              />
            )}
          </>
        )}
        {view === 'saved' && (
          <>
            <p className="muted">Сохранено компаний: {state.favorites.length}</p>
            {state.favorites.length ? (
              companies
                .filter(
                  (c) => state.favorites.includes(c.id) && !state.hiddenCompanies.includes(c.id),
                )
                .map((c) => (
                  <CompanyCard key={c.id} company={c.id === state.company.id ? state.company : c} />
                ))
            ) : (
              <EmptyState
                title="Сохраните интересные компании"
                description="Они появятся здесь, чтобы вы могли вернуться к ним позже."
                href="/companies"
                label="Открыть каталог"
              />
            )}
          </>
        )}
        {view === 'contacts' &&
          (state.contacts.length ? (
            state.contacts.map((contact) => {
              const company = companies.find((c) => c.id === contact.companyId);
              return (
                <article className="card stack" key={contact.companyId}>
                  <div className="spread wrap">
                    <h3>{company?.name || 'Компания'}</h3>
                    <span className="badge neutral">{contact.status}</span>
                  </div>
                  <p className="small muted">Обновлено {dateLabel(contact.date)}</p>
                  <p className="preserve-lines">{contact.note || 'Заметка ещё не добавлена.'}</p>
                  <Link className="text-link" href={`/companies/${contact.companyId}/contacts`}>
                    Контакты и заметка →
                  </Link>
                </article>
              );
            })
          ) : (
            <EmptyState
              title="Контактов пока нет"
              description="Добавьте заметку в карточке компании, чтобы сохранить историю общения."
              href="/companies"
              label="Открыть каталог"
            />
          ))}
        {view === 'profile' && (
          <>
            <article className="card stack">
              <h2>{profile.headline || 'Разработчик'}</h2>
              <p className="muted">{profile.city}</p>
              <p className="preserve-lines">{profile.bio || 'Добавьте несколько слов о себе.'}</p>
              <div className="button-row">
                {profile.skills.map((skill) => (
                  <span className="badge" key={skill}>
                    {skill}
                  </span>
                ))}
              </div>
              {safeHref(profile.github) && (
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
            {state.portfolio.length ? (
              state.portfolio.map((p) => (
                <article className="card stack" key={p.id}>
                  <h3>{p.title}</h3>
                  <p className="small muted">{p.description}</p>
                  <div className="portfolio-cover">
                    <strong>Aa</strong>
                    <div>
                      <h2>{p.title}</h2>
                      <p className="small muted">Веб-разработка · Адаптивный интерфейс</p>
                    </div>
                  </div>
                  {safeHref(p.url) && (
                    <a
                      className="text-link"
                      href={safeHref(p.url)}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      Посмотреть проект ↗
                    </a>
                  )}
                </article>
              ))
            ) : (
              <EmptyState
                title="Добавьте первую работу"
                description="Расскажите о своей роли и результате проекта."
                href="/profile/portfolio"
                label="Добавить работу"
              />
            )}
          </>
        )}
        {view === 'edit' && (
          <>
            <nav className="tabs">
              <Link href="/profile/edit" aria-current="page">
                Основная информация
              </Link>
              <Link href="/profile/portfolio">Портфолио</Link>
              <Link href="/profile">Предпросмотр профиля</Link>
            </nav>
            <form
              className="card stack"
              onSubmit={(e) => {
                e.preventDefault();
                const f = new FormData(e.currentTarget);
                updateDemo((s) => ({
                  ...s,
                  profile: {
                    ...s.profile,
                    name: String(f.get('name')).trim(),
                    headline: String(f.get('headline')).trim(),
                    city: String(f.get('city')).trim(),
                    bio: String(f.get('bio')).trim(),
                    skills: [
                      ...new Set(
                        String(f.get('skills'))
                          .split(',')
                          .map((v) => v.trim())
                          .filter(Boolean),
                      ),
                    ],
                    github: String(f.get('github')).trim(),
                  },
                }));
                setMessage('Профиль сохранён в этом браузере.');
              }}
            >
              <Field
                label="Имя и фамилия"
                name="name"
                defaultValue={profile.name}
                required
                maxLength={100}
              />
              <Field
                label="Специализация"
                name="headline"
                defaultValue={profile.headline}
                maxLength={160}
              />
              <Field label="Город" name="city" defaultValue={profile.city} required />
              <Textarea label="О себе" name="bio" defaultValue={profile.bio} maxLength={2000} />
              <Field
                label="Навыки через запятую"
                name="skills"
                defaultValue={profile.skills.join(', ')}
                maxLength={300}
              />
              <Field
                label="GitHub или сайт"
                name="github"
                type="url"
                pattern="https?://.*"
                defaultValue={profile.github}
                placeholder="https://github.com/username"
              />
              <div className="button-row">
                <button className="button">Сохранить изменения</button>
                <Link className="button secondary" href="/profile/portfolio">
                  Добавить работу
                </Link>
              </div>
            </form>
          </>
        )}
        {view === 'portfolio' && (
          <>
            <form
              className="card stack"
              onSubmit={(e) => {
                e.preventDefault();
                const form = e.currentTarget;
                const f = new FormData(form);
                updateDemo((s) => ({
                  ...s,
                  portfolio: [
                    ...s.portfolio,
                    {
                      id: crypto.randomUUID(),
                      title: String(f.get('title')).trim(),
                      description: String(f.get('description')).trim(),
                      url: String(f.get('url')).trim(),
                    },
                  ],
                }));
                form.reset();
                setMessage('Работа добавлена в портфолио.');
              }}
            >
              <h3>Добавить работу</h3>
              <Field label="Название проекта" name="title" required minLength={3} maxLength={120} />
              <Textarea
                label="Описание и ваша роль"
                name="description"
                required
                minLength={10}
                maxLength={2000}
              />
              <Field
                label="Ссылка на проект"
                name="url"
                type="url"
                pattern="https?://.*"
                placeholder="https://example.com"
              />
              <p className="small muted">
                Можно добавить учебный проект. Укажите, что именно вы сделали.
              </p>
              <button className="button align-start">Добавить в портфолио</button>
            </form>
            {state.portfolio.map((p) => (
              <article className="card stack" key={p.id}>
                <h3>{p.title}</h3>
                <p>{p.description}</p>
                <button className="text-button" onClick={() => setRemove(p.id)}>
                  Удалить работу
                </button>
              </article>
            ))}
          </>
        )}
        {view === 'settings' && (
          <>
            <form
              className="card stack"
              onSubmit={(e) => {
                e.preventDefault();
                const f = new FormData(e.currentTarget);
                updateDemo((s) => ({
                  ...s,
                  profile: { ...s.profile, email: String(f.get('email')).trim() },
                }));
                setMessage(
                  'Email сохранён только в демопрофиле. Подтверждение по почте пока недоступно.',
                );
              }}
            >
              <h3>Аккаунт</h3>
              <Field
                label="Email"
                type="email"
                name="email"
                defaultValue={profile.email}
                required
              />
              <Link className="button secondary align-start" href="/reset-password">
                Изменить пароль
              </Link>
              <hr />
              <p className="small muted">
                Email-уведомления станут доступны после подключения серверной отправки писем.
              </p>
              <button className="button align-start">Сохранить изменения</button>
            </form>
            <div className="card stack">
              <h3>Демонстрация</h3>
              <Link className="text-link" href="/moderation">
                Проверить сценарий модерации →
              </Link>
              <p className="small muted">
                Изменения сохраняются только в этом браузере. Выберите роль, чтобы проверить другой
                сценарий.
              </p>
              <div className="button-row">
                <button
                  className="button secondary"
                  onClick={() => {
                    updateDemo((s) => ({ ...s, profile: { ...s.profile, role: 'developer' } }));
                    router.push('/dashboard');
                  }}
                >
                  Разработчик
                </button>
                <button
                  className="button secondary"
                  onClick={() => {
                    updateDemo((s) => ({ ...s, profile: { ...s.profile, role: 'company' } }));
                    router.push('/workspace');
                  }}
                >
                  Компания
                </button>
                <button className="button secondary" onClick={() => setReset(true)}>
                  Сбросить демоданные
                </button>
              </div>
              <hr />
              <BackendStatus />
            </div>
            <div className="card stack">
              <h3>Сессия</h3>
              <button
                className="button secondary align-start"
                onClick={() => {
                  updateDemo((s) => ({ ...s, signedIn: false }));
                  router.push('/login');
                }}
              >
                Выйти из аккаунта
              </button>
            </div>
          </>
        )}
        {message && <Notice>{message}</Notice>}
        <ConfirmDialog
          open={!!withdraw}
          title="Отозвать отклик?"
          onClose={() => setWithdraw(null)}
          onConfirm={() => {
            if (withdraw) updateDemo((s) => withdrawApplication(s, withdraw));
            setWithdraw(null);
          }}
        >
          Заказчик больше не сможет выбрать это предложение. Позже можно откликнуться заново.
        </ConfirmDialog>
        <ConfirmDialog
          open={!!remove}
          title="Удалить работу?"
          onClose={() => setRemove(null)}
          onConfirm={() => {
            updateDemo((s) => ({ ...s, portfolio: s.portfolio.filter((p) => p.id !== remove) }));
            setRemove(null);
          }}
        >
          Работа исчезнет из вашего портфолио.
        </ConfirmDialog>
        <ConfirmDialog
          open={reset}
          title="Сбросить демонстрацию?"
          onClose={() => setReset(false)}
          onConfirm={() => {
            resetDemo();
            setReset(false);
            setMessage('Восстановлены исходные демоданные.');
          }}
        >
          Созданные заказы, отклики, работы и заметки в этом браузере будут заменены исходными
          примерами.
        </ConfirmDialog>
      </AccountAccess>
    </AccountShell>
  );
}
