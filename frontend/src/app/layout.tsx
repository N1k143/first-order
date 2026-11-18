import type { Metadata } from 'next';
import { SiteHeader } from '@/components/layout/site-header';
import { SiteFooter } from '@/components/layout/site-footer';
import './globals.css';

export const metadata: Metadata = {
  title: { default: 'FirstOrder — ваш первый заказ', template: '%s · FirstOrder' },
  description: 'Находите компании, которым нужен сайт, и откликайтесь на задачи бизнеса.',
};
export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="ru">
      <body>
        <a className="skip-link button" href="#main">
          К содержимому
        </a>
        <SiteHeader />
        <main id="main">{children}</main>
        <SiteFooter />
      </body>
    </html>
  );
}
