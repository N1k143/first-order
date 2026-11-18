import { DemoIndicator } from './demo-indicator';
import Link from 'next/link';
import styles from './site-shell.module.css';
export function SiteFooter() {
  return (
    <>
      <DemoIndicator />
      <footer className={styles.footer}>
        <div className="container">
          <div className={styles.footerTop}>
            <Link href="/" className={styles.footerBrand}>
              FirstOrder
            </Link>
            <span className="small muted">© 2026</span>
          </div>
          <nav className={styles.footerNav} aria-label="Навигация в подвале">
            <Link href="/companies">Компании</Link>
            <Link href="/orders">Заказы</Link>
            <Link href="/about">О сервисе</Link>
            <Link href="/support">Поддержка</Link>
            <Link href="/privacy">Конфиденциальность</Link>
          </nav>
        </div>
      </footer>
    </>
  );
}
