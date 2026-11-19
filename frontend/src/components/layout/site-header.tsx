'use client';

import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useState } from 'react';
import { apiRequest } from '@/lib/api/client';
import { DEMO, refreshAppState, updateDemo, useAppState } from '@/lib/state/store';
import styles from './site-shell.module.css';
import { Avatar } from '@/components/ui/avatar';

export function SiteHeader() {
  const state = useAppState();
  const path = usePathname();
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const company = state.profile.role === 'company' || Boolean(state.profile.companies?.length);
  const moderator = state.profile.role === 'moderator';
  const accountPage =
    /^\/(workspace|dashboard|applications|saved|contacts|profile|settings|moderation)(\/|$)/.test(
      path,
    );
  const dashboard = company
    ? '/workspace'
    : moderator
      ? DEMO
        ? '/moderation'
        : '/companies'
      : DEMO
        ? '/dashboard'
        : '/applications';
  const post = state.signedIn ? '/workspace/orders/new' : '/register/company';
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
          {(!accountPage || !state.signedIn) && (
            <Link href={post} className={`button secondary ${styles.post}`}>
              Разместить заказ
            </Link>
          )}
          <Link
            className={`${styles.accountLink} ${state.signedIn && accountPage ? 'button quiet' : ''}`}
            href={state.signedIn ? dashboard : '/login'}
          >
            {state.signedIn ? 'Мой кабинет' : 'Войти'}
          </Link>
          {state.signedIn && (
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
          <details key={path} className={styles.menu}>
            <summary>Меню</summary>
            <nav className={styles.mobileNav} aria-label="Мобильная навигация">
              <Link href="/companies">Компании</Link>
              <Link href="/orders">Заказы</Link>
              <Link href={state.signedIn ? dashboard : '/login'}>
                {state.signedIn ? 'Мой кабинет' : 'Войти'}
              </Link>
              {state.signedIn && (
                <>
                  <Link href="/profile">Мой профиль</Link>
                  {company ? (
                    <>
                      <Link href="/workspace/orders">Мои заказы</Link>
                      <Link href="/workspace/applications">Отклики на заказы</Link>
                      <Link href="/workspace/profile">Профиль компании</Link>
                      <Link href="/workspace/members">Участники</Link>
                    </>
                  ) : moderator ? (
                    <Link href="/moderation">Модерация</Link>
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
              <Link href={post}>Разместить заказ</Link>
              {!state.signedIn && <Link href="/register">Создать аккаунт</Link>}
            </nav>
          </details>
        </div>
      </div>
      {error && (
        <p className="container error-text" role="alert">
          {error}
        </p>
      )}
    </header>
  );
}
