'use client';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { apiRequest } from '@/lib/api/client';
import { DEMO, refreshAppState, updateDemo, useAppState } from '@/lib/state/store';
import styles from './site-shell.module.css';

export function SiteHeader() {
 const state=useAppState(),router=useRouter();
 const [busy,setBusy]=useState(false),[error,setError]=useState('');
 const company=state.profile.role==='company'||Boolean(state.profile.companies?.length);
 async function logout(){
  setBusy(true);setError('');
  try {
   if(DEMO) updateDemo(s=>({...s,signedIn:false}));
   else {await apiRequest('/auth/logout',{method:'POST'});await refreshAppState();}
   router.replace('/companies');
  }catch(e){setError(e instanceof Error?e.message:'Не удалось выйти.');}
  finally{setBusy(false);}
 }
 const workspace=company?'/workspace/orders':'/applications';
 const post=state.signedIn?'/workspace/orders/new':'/register/company';
 return <header className={styles.header}><div className={`container ${styles.headerInner}`}>
  <Link href="/" className={styles.brand} aria-label="FirstOrder — главная">FirstOrder</Link>
  <nav className={styles.nav} aria-label="Основная навигация"><Link href="/companies">Компании</Link><Link href="/orders">Заказы</Link><Link href="/#how-it-works">Как это работает</Link></nav>
  <div className={styles.actions}>
   <Link href={post} className={`button secondary ${styles.post}`}>Разместить заказ</Link>
   {state.signedIn?<><Link className="desktop-only" href={workspace}>{company?'Мои заказы':'Мои отклики'}</Link><button className="text-button" onClick={logout} disabled={busy}>{busy?'Выходим…':'Выйти'}</button></>:<Link href="/login">Войти</Link>}
   <details className={styles.menu}><summary>Меню</summary><nav className={styles.mobileNav} aria-label="Мобильная навигация"><Link href="/companies">Компании</Link><Link href="/orders">Заказы</Link>{state.signedIn&&<Link href={workspace}>{company?'Мои заказы':'Мои отклики'}</Link>}<Link href={post}>Разместить заказ</Link>{!state.signedIn&&<Link href="/register">Создать аккаунт</Link>}</nav></details>
  </div>
 </div>{error&&<p className="container error-text" role="alert">{error}</p>}</header>;
}
