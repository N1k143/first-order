'use client';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { useEffect, useRef, useState } from 'react';
import { DEMO, refreshAppState, updateDemo, useAppState } from '@/lib/state/store';
import { apiRequest } from '@/lib/api/client';
import { companies } from '@/data/demo';
import { Field, Notice, PageHeading } from '@/components/ui/primitives';
import { AuthLayout, type AuthView as View, type AuthRole } from './auth-layout';
import type { AppState } from '@/types/domain';

function accountHome(state: AppState) {
  if (state.profile.role === 'company' || state.profile.companies?.length) return '/workspace';
  return state.profile.role === 'moderator' ? '/companies' : '/applications';
}

const headings: Record<View, [string, string]> = {
  roles: ['Начнём с вашей роли', 'Создайте аккаунт, чтобы работать с компаниями и заказами.'],
  register: ['Создать аккаунт разработчика', 'Первый шаг к вашему первому проекту.'],
  company: ['Аккаунт представителя компании', 'Создайте профиль, чтобы разместить первый заказ.'],
  login: ['С возвращением', 'Войдите, чтобы продолжить работу.'],
  forgot: ['Восстановление пароля', 'Укажите email, с которым вы зарегистрировались.'],
  reset: ['Задайте новый пароль', 'Выберите пароль, который вы ещё не использовали.'],
  verify: ['Проверьте почту', 'Подтвердите email, чтобы начать работу.'],
};
export function AuthPage({ view, entryRole }: { view: View; entryRole?: AuthRole }) {
  const state = useAppState(),
    router = useRouter(),
    params = useSearchParams();
  const [error, setError] = useState(''),
    [message, setMessage] = useState(''),
    [busy, setBusy] = useState(false),
    [addCompany, setAddCompany] = useState(false);
  const role: AuthRole =
    view === 'company'
      ? 'company'
      : view === 'register'
        ? 'developer'
        : entryRole || (params.get('role') === 'company' ? 'company' : 'developer');
  const [defaultTitle, description] = headings[view];
  const title =
    view === 'login'
      ? role === 'company'
        ? 'Вход для компании'
        : 'Вход для разработчика'
      : defaultTitle;
  const verificationEmail =
    params.get('email') || (state.signedIn || DEMO ? state.profile.email : '');
  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError('');
    setMessage('');
    const f = new FormData(e.currentTarget);
    const values = Object.fromEntries(f);
    if (values.confirm && values.password !== values.confirm) {
      setError('Пароли не совпадают.');
      return;
    }
    setBusy(true);
    try {
      if (DEMO) {
        if (view === 'register' || view === 'company' || view === 'login') {
          const selected = companies.find((c) => c.name === values.company);
          if (view === 'company' && !selected && !addCompany) {
            throw new Error('Выберите компанию из списка или нажмите «Добавить данные компании».');
          }
          const newCompany =
            view === 'company' && !selected
              ? {
                  ...state.company,
                  id: crypto.randomUUID(),
                  name: String(values.company).trim(),
                  initials: String(values.company).trim().slice(0, 2).toUpperCase(),
                  city: String(values.city).trim(),
                  address: '',
                  email: String(values.email),
                  phone: '',
                  website: '',
                  checkedAt: null,
                  categoryId: null,
                  category: 'Без категории',
                  sourceUrl: null,
                  websiteStatus: 'needs_review' as const,
                }
              : undefined;
          updateDemo((s) => ({
            ...s,
            signedIn: true,
            profile: {
              ...s.profile,
              email: String(values.email),
              name: String(values.name || s.profile.name),
              role,
            },
            company: selected || newCompany || s.company,
          }));
          router.push(
            view === 'login'
              ? role === 'company'
                ? '/workspace'
                : '/dashboard'
              : `/verify-email?role=${role}&email=${encodeURIComponent(String(values.email))}`,
          );
        } else if (view === 'reset')
          setMessage('Деморежим: пароль проверен, но не сохранён и не отправлен на сервер.');
        else setMessage('Деморежим: письмо не отправляется. Можно проверить следующий экран.');
      } else {
        if (view === 'roles') return;
        const endpoints = {
          register: '/auth/register',
          company: '/auth/register',
          login: '/auth/login',
          forgot: '/auth/forgot-password',
          reset: '/auth/reset-password',
          verify: '/auth/resend-verification',
        };
        const body =
          view === 'verify'
            ? { email: String(values.email || verificationEmail) }
            : view === 'forgot'
              ? { email: values.email }
              : view === 'reset'
                ? { password: values.password, confirm: values.confirm }
                : view === 'login'
                  ? { email: values.email, password: values.password }
                  : { ...values, role: view === 'company' ? 'company' : 'developer' };
        const result = await apiRequest<{ status?: string }>(endpoints[view], {
          method: 'POST',
          body,
        });
        if (view === 'login') {
          const session = await refreshAppState();
          if (!session.signedIn) throw new Error('Не удалось загрузить аккаунт. Повторите вход.');
          router.push(accountHome(session));
        } else if (view === 'register' || view === 'company') {
          if (result?.status === 'authenticated') {
            const session = await refreshAppState();
            if (!session.signedIn) throw new Error('Не удалось загрузить аккаунт. Повторите вход.');
            router.push(accountHome(session));
          } else {
            router.push(
              `/verify-email?role=${role}&email=${encodeURIComponent(String(values.email))}`,
            );
          }
        } else if (view === 'reset') {
          await refreshAppState();
          setMessage('Пароль обновлён.');
        } else {
          setMessage('Запрос принят. Проверьте почту.');
        }
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось выполнить запрос.');
    } finally {
      setBusy(false);
    }
  }
  if (view === 'roles')
    return (
      <AuthLayout title={title} description={description} view={view} role={role}>
        <div className="stack-lg">
          {[
            {
              title: 'Я разработчик',
              text: 'Ищу первые проекты, хочу пополнить портфолио и работать с реальными компаниями.',
              meta: 'Каталог компаний · Отклики · Портфолио',
              href: '/register/developer',
              button: 'Стать разработчиком',
            },
            {
              title: 'Я представляю компанию',
              text: 'Хочу разместить задачу и найти разработчика для своего бизнеса.',
              meta: 'Публикация заказов · Выбор исполнителя',
              href: '/register/company',
              button: 'Зарегистрировать компанию',
            },
          ].map((r, i) => (
            <article className="card stack" key={r.href}>
              <h3>{r.title}</h3>
              <p className="small muted">{r.text}</p>
              <p className="small muted">{r.meta}</p>
              <Link className={`button align-start ${i ? 'secondary' : ''}`} href={r.href}>
                {r.button}
              </Link>
            </article>
          ))}
        </div>
        <Link className="text-link" href="/login">
          Уже есть аккаунт? Войти
        </Link>
      </AuthLayout>
    );
  if (view === 'verify')
    return (
      <AuthLayout title={title} description={description} view={view} role={role}>
        <div className="card stack auth-card">
          <h3>{verificationEmail ? `Письмо для ${verificationEmail}` : 'Подтверждение email'}</h3>
          <p className="muted">
            Откройте ссылку в письме. Если письма нет, проверьте папку «Спам» или отправьте его ещё
            раз.
          </p>
          <form className="stack" onSubmit={submit}>
            {verificationEmail ? (
              <input type="hidden" name="email" value={verificationEmail} />
            ) : (
              <Field label="Email" name="email" type="email" autoComplete="email" required />
            )}
            <button className="button" disabled={busy}>
              Отправить письмо повторно
            </button>
          </form>
          {DEMO && (
            <>
              <Notice>Деморежим: реальное письмо не отправляется.</Notice>
              <Link
                className="button secondary"
                href={role === 'company' ? '/workspace' : '/profile/edit'}
              >
                Продолжить в деморежиме
              </Link>
            </>
          )}
          {message && <Notice>{message}</Notice>}
          {error && <Notice error>{error}</Notice>}
        </div>
      </AuthLayout>
    );
  const register = view === 'register' || view === 'company';
  return (
    <AuthLayout title={title} description={description} view={view} role={role}>
      <>
        {view === 'login' && (
          <nav className="tabs" aria-label="Роль для входа">
            <Link href="/login/developer" aria-current={role === 'developer' ? 'page' : undefined}>
              Разработчик
            </Link>
            <Link href="/login/company" aria-current={role === 'company' ? 'page' : undefined}>
              Компания
            </Link>
          </nav>
        )}
        <form className="card stack mobile-flat auth-card" onSubmit={submit}>
          {register && (
            <Field
              label={view === 'company' ? 'Ваше имя' : 'Имя и фамилия'}
              name="name"
              autoComplete="name"
              placeholder={view === 'company' ? 'Имя и фамилия' : 'Как к вам обращаться'}
              required
            />
          )}
          {view !== 'reset' && (
            <Field
              label={view === 'company' ? 'Рабочий email' : 'Email'}
              type="email"
              name="email"
              autoComplete="email"
              placeholder={view === 'company' ? 'name@company.kz' : 'you@example.com'}
              required
            />
          )}
          {view !== 'forgot' && (
            <Field
              label={view === 'reset' ? 'Новый пароль' : 'Пароль'}
              type="password"
              name="password"
              autoComplete={view === 'login' ? 'current-password' : 'new-password'}
              placeholder={view === 'login' ? 'Введите пароль' : 'Не менее 8 символов'}
              minLength={view === 'login' ? 1 : 8}
              required
            />
          )}
          {(view === 'register' || view === 'reset') && (
            <Field
              label="Повторите пароль"
              type="password"
              name="confirm"
              autoComplete="new-password"
              placeholder="Введите пароль ещё раз"
              required
              minLength={8}
            />
          )}{' '}
          {view === 'company' && (
            <>
              <Field
                label={DEMO ? 'Компания' : 'Название новой компании'}
                name="company"
                list={DEMO ? 'company-options' : undefined}
                placeholder={
                  !DEMO || addCompany ? 'Название новой компании' : 'Найти компанию в каталоге'
                }
                required
              />
              {DEMO && (
                <>
                  <datalist id="company-options">
                    {companies.map((c) => (
                      <option key={c.id} value={c.name} />
                    ))}
                  </datalist>
                  <button
                    type="button"
                    className="text-button small"
                    onClick={() => setAddCompany((v) => !v)}
                  >
                    {addCompany
                      ? 'Выбрать существующую компанию'
                      : 'Компании нет в каталоге? Добавить данные компании'}
                  </button>
                </>
              )}
              {(!DEMO || addCompany) && (
                <Field label="Город компании" name="city" required placeholder="Астана" />
              )}
              {!DEMO && (
                <p className="small muted">
                  Для доступа к существующей компании обратитесь к модератору. Новая компания
                  появится в каталоге после проверки.
                </p>
              )}
            </>
          )}
          {view === 'login' && (
            <Link className="text-link small" href={`/forgot-password?role=${role}`}>
              Не помню пароль
            </Link>
          )}
          {register && (
            <p className="caption muted">
              {DEMO
                ? 'В деморежиме аккаунт сохраняется только в этом браузере. См. '
                : 'Создавая аккаунт, вы принимаете '}
              <Link href="/terms">условия использования</Link>
              <br className="desktop-only" /> и <Link href="/privacy">информацию о данных</Link>.
            </p>
          )}
          <button className={`button ${view === 'company' ? 'align-start' : ''}`} disabled={busy}>
            {busy
              ? 'Подождите…'
              : view === 'login'
                ? 'Войти'
                : view === 'forgot'
                  ? 'Отправить ссылку'
                  : view === 'reset'
                    ? 'Сохранить новый пароль'
                    : view === 'company'
                      ? 'Создать аккаунт компании'
                      : 'Создать аккаунт'}
          </button>
          {view === 'login' && (
            <>
              <hr className="desktop-only" />
              <Link className="button secondary" href="/register">
                Создать аккаунт
              </Link>
            </>
          )}
          {register && (
            <Link href={`/login/${role}`} className="text-link small">
              Уже есть аккаунт? Войти
            </Link>
          )}
          {message && (
            <Notice>
              {message}
              {DEMO && view === 'forgot' && (
                <>
                  {' '}
                  <Link href={`/reset-password?role=${role}`}>Открыть форму нового пароля →</Link>
                </>
              )}
            </Notice>
          )}
          {error && <Notice error>{error}</Notice>}
        </form>
      </>
    </AuthLayout>
  );
}

/** Exchanges a one-time Auth redirect for server-owned HttpOnly session cookies. */
export function AuthCallbackPage() {
  const router = useRouter();
  const [error, setError] = useState('');
  const [pendingEmail, setPendingEmail] = useState(false);
  const task = useRef<Promise<string | null> | null>(null);
  useEffect(() => {
    let active = true;
    if (!task.current) {
      const query = new URLSearchParams(window.location.search);
      const fragment = new URLSearchParams(window.location.hash.slice(1));
      const accessToken = fragment.get('access_token');
      const refreshToken = fragment.get('refresh_token');
      const tokenHash = query.get('token_hash') || fragment.get('token_hash');
      const type = query.get('type') || fragment.get('type') || 'email';
      const invalidLink = query.has('error') || fragment.has('error');
      const pendingMessage =
        !invalidLink &&
        (fragment.has('message') || query.has('message')) &&
        (fragment.has('sb') || query.has('sb') || type === 'email_change');
      window.history.replaceState(window.history.state, '', window.location.pathname);
      task.current = (async () => {
        if (DEMO)
          throw new Error('Подтверждение по email доступно при подключении приложения к серверу.');
        if (invalidLink)
          throw new Error('Ссылка недействительна или истекла. Запросите новое письмо.');
        if (pendingMessage && !accessToken && !tokenHash) return null;
        if (accessToken) {
          await apiRequest('/auth/session', {
            method: 'POST',
            body: {
              access_token: accessToken,
              ...(refreshToken ? { refresh_token: refreshToken } : {}),
              expires_in: Number(fragment.get('expires_in')) || 3600,
            },
          });
        } else if (tokenHash) {
          const result = await apiRequest<{ status?: string }>('/auth/verify', {
            method: 'POST',
            body: { token_hash: tokenHash, type },
          });
          if (result?.status === 'second_confirmation_required') return null;
        } else throw new Error('В ссылке отсутствует подтверждение. Откройте актуальное письмо.');
        const session = await refreshAppState();
        if (!session.signedIn) throw new Error('Не удалось загрузить аккаунт. Повторите вход.');
        return type === 'recovery'
          ? '/reset-password'
          : type === 'email_change'
            ? '/settings'
            : accountHome(session);
      })();
    }
    // Reuse the exchange promise when development StrictMode replays this effect.
    task.current
      .then((destination) => {
        if (active && destination) router.replace(destination);
        else if (active) setPendingEmail(true);
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : 'Не удалось подтвердить аккаунт.');
      });
    return () => {
      active = false;
    };
  }, [router]);
  return (
    <section className="container page">
      <PageHeading
        title="Подтверждение аккаунта"
        description={
          error
            ? 'Откройте новое письмо или войдите в аккаунт.'
            : pendingEmail
              ? 'Остался ещё один шаг для смены email.'
              : 'Завершаем вход…'
        }
      />
      {pendingEmail ? (
        <div className="card stack auth-card" role="status">
          <h2>Подтвердите второй адрес</h2>
          <p>
            Откройте ссылку во втором письме: для смены email нужно подтвердить старый и новый
            адрес.
          </p>
          <Link className="button secondary" href="/settings">
            В настройки
          </Link>
        </div>
      ) : error ? (
        <div className="card stack auth-card">
          <Notice error>{error}</Notice>
          <Link className="button" href="/login">
            Ко входу
          </Link>
          <Link className="text-link" href="/verify-email">
            Отправить письмо повторно
          </Link>
        </div>
      ) : (
        <p role="status">Проверяем ссылку и сохраняем сессию.</p>
      )}
    </section>
  );
}
