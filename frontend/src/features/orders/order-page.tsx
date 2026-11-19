'use client';
import Link from 'next/link';
import {useState} from 'react';
import {useAppState,updateDemo,DEMO} from '@/lib/state/store';
import {mutate,useResource} from '@/lib/api/resources';
import {orderStatus} from '@/lib/order-status';
import {PageHeading,ResourceState,EmptyState,Textarea,Field,Notice,money,dateLabel} from '@/components/ui/primitives';
export function OrderPage({id,apply=false}:{id:string;apply?:boolean}){
 const state=useAppState(),resource=useResource(`/orders/${id}`,state.orders.find(o=>o.id===id));
 const [error,setError]=useState(''),[sent,setSent]=useState(false),[pending,setPending]=useState(false);
 const order=resource.data;
 if(!order)return <section className="container page">{resource.loading||resource.error?<ResourceState error={resource.error} reload={resource.reload}/>:<EmptyState title="Заказ недоступен" description="Заказ удалён или не опубликован." href="/orders" label="К заказам"/>}</section>;
 const existing=state.applications.find(a=>a.orderId===id&&a.profileId===state.profile.id);
 const own=state.profile.companies?.some(c=>c.companyId===order.companyId)||(DEMO&&state.profile.role==='company'&&state.company.id===order.companyId);
 const action=own?<Link className="button" href={`/workspace/orders/${id}`}>Управлять заказом</Link>:existing?<Link className="button secondary" href="/applications">{existing.status==='withdrawn'?'Отклик отозван':'Ваш отклик отправлен'}</Link>:order.status==='open'?<Link className="button" href={state.signedIn?`/orders/${id}/apply`:'/login'}>Откликнуться</Link>:<span className="badge">Приём откликов закрыт</span>;
 if(apply&&sent)return <section className="container page"><PageHeading title="Отклик отправлен"/><div className="card stack"><Notice>Отклик отправлен. Решение заказчика появится в личном списке.</Notice><Link className="button align-start" href="/applications">Мои отклики →</Link></div></section>;
 if(apply){
  if(own||existing||!state.signedIn||order.status!=='open')return <section className="container page"><PageHeading title={order.title}/><div className="card stack"><p>{own?'Вы представляете компанию этого заказа.':existing?'На этот заказ уже существует ваш отклик.':!state.signedIn?'Войдите, чтобы отправить предложение.':'Приём откликов закрыт.'}</p><div>{action}</div></div></section>;
  return <section className="container page"><PageHeading title="Ваше предложение" description={`${order.title} · ${order.companyName}`}/><div className="two-columns"><form className="card stack mobile-flat" onSubmit={async e=>{
   e.preventDefault();setError('');const f=new FormData(e.currentTarget);
   const a={id:crypto.randomUUID(),orderId:id,name:state.profile.name,profileId:state.profile.id,price:Number(f.get('price')),deadline:String(f.get('deadline')),letter:String(f.get('letter')),status:'pending' as const};
   setPending(true);
   try{await mutate(`/orders/${id}/applications`,'POST',{message:a.letter,proposed_price:a.price,proposed_deadline:a.deadline},()=>updateDemo(s=>({...s,applications:[...s.applications,a],orders:s.orders.map(o=>o.id===id?{...o,applicationsCount:o.applicationsCount+1}:o)})));setSent(true);}
   catch(e){setError(e instanceof Error?e.message:'Ошибка отправки.');}finally{setPending(false);}
  }}><h3>Расскажите, как решите задачу</h3><Textarea label="Сопроводительное письмо" name="letter" required minLength={20} maxLength={10000} placeholder="Опишите решение, этапы и результат работы."/><div className="form-grid"><Field label="Стоимость, ₸" name="price" type="number" min={0} max={9999999999.99} step="0.01" required/><Field label="Готово к дате" name="deadline" type="date" min={new Date().toISOString().slice(0,10)} required/></div><div className="button-row"><button className="button" disabled={pending||sent}>{pending?'Отправляем…':'Отправить отклик'}</button><Link className="button secondary" href={`/orders/${id}`}>Отмена</Link></div>{error&&<Notice error>{error}</Notice>}{sent&&<Notice>Отклик отправлен. <Link href="/applications">Мои отклики →</Link></Notice>}</form><aside className="card stack desktop-only"><h3>Хорошее предложение</h3><p className="muted">Опишите решение конкретной задачи. Укажите, что входит в стоимость и сколько времени понадобится.</p><p className="small muted">Один отклик на заказ. Отзыв не позволяет отправить новый отклик на тот же заказ.</p></aside></div></section>;
 }
 return <section className="container page"><PageHeading title={order.title} description={<><Link href="/orders">Заказы</Link> / {order.companyName}</>}/><div className="two-columns"><div className="stack-lg"><article className="card stack"><div><span className="badge">{orderStatus[order.status]}</span></div><h2>О задаче</h2><p style={{whiteSpace:'pre-wrap'}}>{order.description}</p></article><article className="card stack"><h3>О заказчике</h3><p>{order.companyName} · {order.city}</p><Link className="button secondary align-start" href={`/companies/${order.companyId}`}>Карточка компании</Link></article></div><aside className="card stack"><h2>{money(order.budgetMin)} – {money(order.budgetMax)}</h2><p className="small muted">Бюджет проекта</p><p>Срок: {order.deadline?`до ${dateLabel(order.deadline)}`:'по договорённости'}</p><p>{order.applicationsCount} откликов</p><div>{action}</div></aside></div></section>;
}
