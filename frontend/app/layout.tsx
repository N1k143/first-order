import type { Metadata } from "next";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import "./globals.css";
import "@/styles/layout.css";
import "@/styles/home.css";
import "@/styles/auth.css";

export const metadata: Metadata = {
  title: "First Order — найди первый заказ",
  description:
    "Платформа для поиска компаний без сайта и первых заказов начинающими разработчиками.",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="ru">
      <body>
        <a className="skip" href="#main">
          К содержимому
        </a>
        <SiteHeader />
        {children}
        <SiteFooter />
      </body>
    </html>
  );
}
