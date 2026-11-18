import Link from 'next/link';
import styles from './site-shell.module.css';

export function SiteHeader() {
  return <header className={styles.header}><div className={`container ${styles.headerInner}`}>
    <Link href="/" className={styles.brand} aria-label="FirstOrder — главная">FirstOrder</Link>
    <nav className={styles.nav} aria-label="Основная навигация"><Link href="/companies">Компании</Link><Link href="/orders">Заказы</Link><Link href="/#how-it-works">Как это работает</Link></nav>
    <div className={styles.actions}><Link href="/workspace/orders/new" className={`button secondary ${styles.post}`}>Разместить заказ</Link><Link href="/login">Войти</Link>
      <details className={styles.menu}><summary>Меню</summary><nav className={styles.mobileNav} aria-label="Мобильная навигация"><Link href="/companies">Компании</Link><Link href="/orders">Заказы</Link><Link href="/profile">Мой профиль</Link><Link href="/saved">Избранное</Link><Link href="/settings">Настройки</Link><Link href="/workspace/orders/new">Разместить заказ</Link></nav></details>
    </div>
  </div></header>;
}
