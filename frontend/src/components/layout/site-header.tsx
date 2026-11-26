'use client';

import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useRef, useState } from 'react';
import { apiRequest } from '@/lib/api/client';
import { DEMO, refreshAppState, updateDemo, useAppState, useSessionError } from '@/lib/state/store';
import styles from './site-shell.module.css';
import { Avatar } from '@/components/ui/avatar';

export function SiteHeader() {
  const state = useAppState();
  const sessionError = useSessionError();
  const [retrying, setRetrying] = useState(false);
  const path = usePathname();
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const mobileMenu = useRef<HTMLDetailsElement>(null);
  const company = state.profile.role === 'company' || Boolean(state.profile.companies?.length);
  const moderator = state.profile.role === 'moderator';
  const accountPage =
    /^\/(workspace|dashboard|applications|saved|contacts|profile|settings|moderation)(\/|$)/.test(
      path,
    );
  const dashboard = moderator ? '/moderation' : company ? '/workspace' : '/dashboard';
  const post = state.signedIn ? '/workspace/orders/new' : '/register/company';
  async function retrySession() {
    if (retrying) return;
    setRetrying(true);
    setError('');
    try {
      await refreshAppState();
    } catch {
      setError('Не удалось проверить сессию. Попробуйте ещё раз.');
    } finally {
      setRetrying(false);
    }
  }
  async function logout() {
    setBusy(true);
    setError('');
    try {
      if (DEMO) updateDemo((s) => ({ ...s, signedIn: false }));
      else {
        await apiRequest('/auth/logout', { method: 'POST' });
        await refreshAppState();
      }
      router.replace('/companies');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось выйти.');
    } finally {
      setBusy(false);
    }
  }
  return (
    <header className={styles.header}>
      <div className={`container ${styles.headerInner}`}>
        <Link href="/" className={styles.brand} aria-label="FirstOrder — главная">
          FirstOrder
        </Link>
        <nav className={styles.nav} aria-label="Основная навигация">
          <Link href="/companies">Компании</Link>
          <Link href="/orders">Заказы</Link>
          <Link href="/#how-it-works">Как это работает</Link>
        </nav>
        <div className={styles.actions}>
          {!sessionError && (!accountPage || !state.signedIn) && (
            <Link href={post} className={`button secondary ${styles.post}`}>
              Разместить заказ
            </Link>
          )}
          {!sessionError && (
            <Link
              className={`${styles.accountLink} ${state.signedIn && accountPage ? 'button quiet' : ''}`}
              href={state.signedIn ? dashboard : '/login'}
            >
              {state.signedIn ? 'Мой кабинет' : 'Войти'}
            </Link>
          )}
          {!sessionError && state.signedIn && (
            <>
              <Link className={styles.avatarLink} href="/profile" aria-label="Мой профиль">
                <Avatar name={state.profile.name} src={state.profile.avatarUrl} />
              </Link>
              <button
                type="button"
                className={`text-button ${styles.logout}`}
                onClick={logout}
                disabled={busy}
              >
                {busy ? 'Выходим…' : 'Выйти'}
              </button>
            </>
          )}
          <details key={path} ref={mobileMenu} className={styles.menu}>
            <summary>Меню</summary>
            <nav className={styles.mobileNav} aria-label="Мобильная навигация">
              <Link href="/companies">Компании</Link>
              <Link href="/orders">Заказы</Link>
              <Link
                href="/#how-it-works"
                onClick={() => mobileMenu.current?.removeAttribute('open')}
              >
                Как это работает
              </Link>
              {!sessionError && (
                <Link href={state.signedIn ? dashboard : '/login'}>
                  {state.signedIn ? 'Мой кабинет' : 'Войти'}
                </Link>
              )}
              {!sessionError && state.signedIn && (
                <>
                  <Link href="/profile">Мой профиль</Link>
                  {moderator ? (
                    <Link href="/moderation">Модерация</Link>
                  ) : company ? (
                    <>
                      <Link href="/workspace/orders">Мои заказы</Link>
                      <Link href="/workspace/applications">Отклики на заказы</Link>
                      <Link href="/workspace/profile">Профиль компании</Link>
                      <Link href="/workspace/members">Участники</Link>
                    </>
                  ) : (
                    <>
                      <Link href="/applications">Мои отклики</Link>
                      <Link href="/saved">Избранное</Link>
                      <Link href="/contacts">История контактов</Link>
                    </>
                  )}
                  <Link href="/settings">Настройки</Link>
                  <button
                    type="button"
                    className="text-button align-start"
                    onClick={logout}
                    disabled={busy}
                  >
                    {busy ? 'Выходим…' : 'Выйти'}
                  </button>
                </>
              )}
              {!sessionError && <Link href={post}>Разместить заказ</Link>}
              {!sessionError && !state.signedIn && <Link href="/register">Создать аккаунт</Link>}
            </nav>
          </details>
        </div>
      </div>
      {sessionError && (
        <div className="container button-row" role="alert">
          <span>Не удалось проверить сессию. Действия аккаунта временно недоступны.</span>
          <button
            className="text-button"
            type="button"
            disabled={retrying}
            onClick={() => void retrySession()}
          >
            {retrying ? 'Проверяем…' : 'Проверить сессию снова'}
          </button>
        </div>
      )}
      {error && (
        <p className="container error-text" role="alert">
          {error}
        </p>
      )}
    </header>
  );
}
