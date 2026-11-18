'use client';
import Link from 'next/link';
import {usePathname} from 'next/navigation';
import {PageHeading} from '@/components/ui/primitives';
import type {ReactNode} from 'react';
const developer=[['/dashboard','Обзор'],['/applications','Мои отклики'],['/saved','Избранные компании'],['/contacts','История контактов'],['/profile/edit','Профиль и портфолио'],['/settings','Настройки']];
const company=[['/workspace','Обзор'],['/workspace/orders','Мои заказы'],['/workspace/profile','Профиль компании'],['/workspace/members','Участники'],['/settings','Настройки']];
const moderator=[['/moderation','На проверке'],['/moderation/import','Импорт данных'],['/moderation/history','Журнал действий'],['/settings','Настройки']];
export function AccountShell({title,description='Личный кабинет FirstOrder',role='developer',children}:{title:string;description?:string;role?:'developer'|'company'|'moderator';children:ReactNode}){const path=usePathname();return <section className="container page"><PageHeading title={title} description={description}/><div className="account-grid"><nav className="account-nav" aria-label="Разделы кабинета">{(role==='company'?company:role==='moderator'?moderator:developer).map(([href,label])=><Link key={href} href={href} aria-current={path===href?'page':undefined}>{label}</Link>)}</nav><div className="stack-lg">{children}</div></div></section>}
