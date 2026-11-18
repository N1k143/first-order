'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useAppState } from '@/lib/state/store';
import styles from './site-shell.module.css';
import { Avatar } from '@/components/ui/avatar';

export function SiteHeader() {
  const state = useAppState();
  const path = usePathname();
  const accountPage =
    /^\/(workspace|dashboard|applications|saved|contacts|profile|settings|moderation)(\/|$)/.test(
      path,
    );
  const dashboard =
    state.profile.role === 'company'
      ? '/workspace'
      : state.profile.role === 'moderator'
        ? '/moderation'
        : '/dashboard';
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
            <Link href="/workspace/orders/new" className={`button secondary ${styles.post}`}>
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
            <Link className={styles.avatarLink} href="/profile" aria-label="Мой профиль">
              <Avatar name={state.profile.name} src={state.profile.avatarUrl} />
            </Link>
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
                  <Link href="/applications">Мои отклики</Link>
                  <Link href="/saved">Избранное</Link>
                  <Link href="/contacts">История контактов</Link>
                  <Link href="/settings">Настройки</Link>
                </>
              )}
              <Link href="/workspace/orders/new">Разместить заказ</Link>
            </nav>
          </details>
        </div>
      </div>
    </header>
  );
}
