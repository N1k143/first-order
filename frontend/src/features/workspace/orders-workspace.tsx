'use client';
import Link from 'next/link';
import {useRouter} from 'next/navigation';
import {useState} from 'react';
import {apiRequest} from '@/lib/api/client';
import {mutate,useResource} from '@/lib/api/resources';
import {DEMO,refreshAppState,updateDemo,useAppState} from '@/lib/state/store';
import {ConfirmDialog} from '@/components/ui/confirm-dialog';
import {EmptyState,Field,Notice,PageHeading,ResourceState,Select,Textarea,dateLabel,money} from '@/components/ui/primitives';
import type {Application,Order,Profile} from '@/types/domain';
import {orderStatus,applicationStatus} from '@/lib/order-status';

function useMemberships(){
 const state=useAppState();
 return useResource<Profile>('/me',{...state.profile,companies:state.profile.companies||(state.profile.role==='company'?[{companyId:state.company.id,memberRole:'owner',name:state.company.name}]:[])});
}
function NoMembership(){return <EmptyState title="Нет доступа к компании" description="Для публикации заказов нужны права представителя. Создайте аккаунт новой компании или попросите владельца добавить вас в участники." href="/register/company" label="Регистрация компании"/>;}
function LoginRequired(){return <EmptyState title="Войдите в аккаунт" description="Для работы с заказами и откликами нужна сессия." href="/login" label="Войти"/>;}

export function WorkspaceOrdersPage(){
 const state=useAppState(),me=useMemberships();
 const resource=useResource<Order[]>('/orders?mine=true&limit=100',state.orders.filter(o=>o.companyId===state.company.id));
 const companies=me.data?.companies||[];
 return <section className="container page"><PageHeading title="Мои заказы" description="Черновики, публикации и выбор исполнителя."/>{!me.data?<><ResourceState error={me.error} reload={me.reload}/>{me.error&&<LoginRequired/>}</>:!companies.length?<NoMembership/>:<div className="stack-lg"><div><Link className="button" href="/workspace/orders/new">Создать заказ</Link></div>{resource.data?resource.data.length?resource.data.map(order=><article className="card stack" key={order.id}><div><span className="badge">{orderStatus[order.status]}</span></div><h2><Link href={`/workspace/orders/${order.id}`}>{order.title}</Link></h2><p className="muted">{order.companyName} · {money(order.budgetMin)} – {money(order.budgetMax)}</p><p>{order.applicationsCount} откликов</p><Link className="button secondary align-start" href={`/workspace/orders/${order.id}`}>Управлять заказом</Link></article>):<EmptyState title="Заказов пока нет" description="Создайте черновик, проверьте условия и опубликуйте задачу."/>:<ResourceState error={resource.error} reload={resource.reload}/>}</div>}</section>;
}

export function NewOrderPage(){
 const me=useMemberships(),router=useRouter(),state=useAppState();
 const [error,setError]=useState(''),[busy,setBusy]=useState(false);
 const companies=me.data?.companies||[];
 async function submit(event:React.FormEvent<HTMLFormElement>){
  event.preventDefault();setError('');const fields=new FormData(event.currentTarget);
  const body={company_id:String(fields.get('company')),title:String(fields.get('title')).trim(),description:String(fields.get('description')).trim(),budget_min:Number(fields.get('budget_min')),budget_max:Number(fields.get('budget_max')),deadline:String(fields.get('deadline'))};
  if(body.budget_max<body.budget_min){setError('Верхняя граница бюджета не может быть меньше нижней.');return;}
  setBusy(true);
  try{
   let id:string;
   if(DEMO){id=crypto.randomUUID();const order:Order={id,companyId:body.company_id,companyName:companies.find(c=>c.companyId===body.company_id)?.name||state.company.name,city:state.company.city,title:body.title,description:body.description,budgetMin:body.budget_min,budgetMax:body.budget_max,deadline:body.deadline,status:'draft',applicationsCount:0,publishedAt:new Date().toISOString()};updateDemo(s=>({...s,orders:[order,...s.orders]}));}
   else {const result=await apiRequest<Order>('/orders',{method:'POST',body});id=result.id;await refreshAppState();}
   router.push(`/workspace/orders/${encodeURIComponent(id)}`);
  }catch(e){setError(e instanceof Error?e.message:'Не удалось создать заказ.');}finally{setBusy(false);}
 }
 return <section className="container page"><PageHeading title="Новый заказ" description="Сначала создайте черновик. Публикация — отдельный шаг."/>{!me.data?<><ResourceState error={me.error} reload={me.reload}/>{me.error&&<LoginRequired/>}</>:!companies.length?<NoMembership/>:<form className="card stack auth-card" onSubmit={submit}><Select label="Компания" name="company" required>{companies.map(c=><option key={c.companyId} value={c.companyId}>{c.name||c.companyId}</option>)}</Select><Field label="Название заказа" name="title" required minLength={3} maxLength={200}/><Textarea label="Описание задачи" name="description" required minLength={20} maxLength={20000}/><div className="form-grid"><Field label="Бюджет от, ₸" name="budget_min" type="number" min={0} step="0.01" required/><Field label="Бюджет до, ₸" name="budget_max" type="number" min={0} step="0.01" required/></div><Field label="Срок выполнения" name="deadline" type="date" min={new Date().toISOString().slice(0,10)} required/><div className="button-row"><button className="button" disabled={busy}>{busy?'Создаём…':'Создать черновик'}</button><Link className="button secondary" href="/workspace/orders">Отмена</Link></div>{error&&<Notice error>{error}</Notice>}</form>}</section>;
}

export function ManageOrderPage({id}:{id:string}){
 const state=useAppState(),me=useMemberships();
 const orderResource=useResource<Order|undefined>(`/orders/${encodeURIComponent(id)}`,state.orders.find(o=>o.id===id));
 const applications=useResource<Application[]>(`/orders/${encodeURIComponent(id)}/applications?limit=100`,state.applications.filter(a=>a.orderId===id));
 const [error,setError]=useState(''),[message,setMessage]=useState(''),[busy,setBusy]=useState(false),[selected,setSelected]=useState<Application|null>(null);
 const order=orderResource.data;
 const canManage=Boolean(order&&me.data?.companies?.some(c=>c.companyId===order.companyId));
 async function transition(action:'publish'|'close'|'complete'){
  if(busy)return;setBusy(true);setError('');setMessage('');
  try{await mutate(`/orders/${id}/${action}`,'POST',undefined,()=>updateDemo(s=>({...s,orders:s.orders.map(o=>o.id===id?{...o,status:action==='publish'?'open':action==='complete'?'completed':'closed'}:o)})));setMessage(action==='publish'?'Заказ опубликован.':action==='complete'?'Заказ завершён.':'Заказ закрыт.');}
  catch(e){setError(e instanceof Error?e.message:'Не удалось изменить заказ.');}finally{setBusy(false);}
 }
 async function choose(){
  if(!selected||busy)return;setBusy(true);setError('');setMessage('');const choice=selected;
  try{await mutate(`/orders/${id}/select-application`,'POST',{application_id:choice.id},()=>updateDemo(s=>({...s,orders:s.orders.map(o=>o.id===id?{...o,status:'in_progress',selectedApplicationId:choice.id}:o),applications:s.applications.map(a=>a.orderId===id&&a.status==='pending'?{...a,status:a.id===choice.id?'selected':'rejected'}:a)})));setMessage('Исполнитель выбран. Остальные поданные отклики отклонены.');setSelected(null);}
  catch(e){setError(e instanceof Error?e.message:'Не удалось выбрать исполнителя.');setSelected(null);}finally{setBusy(false);}
 }
 if(!me.data)return <section className="container page"><ResourceState error={me.error} reload={me.reload}/>{me.error&&<LoginRequired/>}</section>;
 if(!order)return <section className="container page"><ResourceState error={orderResource.error} reload={orderResource.reload}/></section>;
 if(!canManage)return <section className="container page"><NoMembership/></section>;
 return <section className="container page"><PageHeading title={order.title} description={<Link href="/workspace/orders">← Мои заказы</Link>}/><div className="stack-lg"><article className="card stack"><div><span className="badge" data-testid="order-status">{orderStatus[order.status]}</span></div><p style={{whiteSpace:'pre-wrap'}}>{order.description}</p><p>{money(order.budgetMin)} – {money(order.budgetMax)} · До {dateLabel(order.deadline)}</p><div className="button-row">{order.status==='draft'&&<button className="button" disabled={busy} onClick={()=>void transition('publish')}>Опубликовать заказ</button>}{order.status==='in_progress'&&<button className="button" disabled={busy} onClick={()=>void transition('complete')}>Завершить заказ</button>}{['draft','open','in_progress'].includes(order.status)&&<button className="button secondary" disabled={busy} onClick={()=>void transition('close')}>Закрыть заказ</button>}<Link className="text-link" href={`/orders/${id}`}>Страница заказа</Link><a className="text-link" href="#order-applications">Отклики</a></div>{message&&<Notice>{message}</Notice>}{error&&<Notice error>{error}</Notice>}</article><section className="stack-lg" id="order-applications"><div className="button-row"><h2>Отклики</h2><button className="button secondary" onClick={applications.reload}>Обновить отклики</button></div>{applications.data?applications.data.length?applications.data.map(application=><article className="card stack" key={application.id} data-testid="application-card"><h3>{application.name||'Разработчик'}</h3><span className="badge align-start">{applicationStatus[application.status]}</span><p style={{whiteSpace:'pre-wrap'}}>{application.letter}</p><p>{money(application.price)} · До {dateLabel(application.deadline)}</p>{order.status==='open'&&application.status==='pending'&&<button className="button align-start" disabled={busy} onClick={()=>setSelected(application)}>Выбрать исполнителя</button>}</article>):<EmptyState title="Откликов пока нет" description={order.status==='draft'?'Опубликуйте заказ, чтобы разработчики могли откликнуться.':'Предложения разработчиков появятся здесь.'}/>:<ResourceState error={applications.error} reload={applications.reload}/>}</section></div><ConfirmDialog open={Boolean(selected)} title="Выбрать исполнителя?" description={`Вы выбираете ${selected?.name||'этого разработчика'}. Остальные поданные отклики будут отклонены.`} confirmLabel={busy?'Сохраняем…':'Подтвердить выбор'} onConfirm={()=>void choose()} onClose={()=>{if(!busy)setSelected(null);}}/></section>;
}

export function MyApplicationsPage(){
 const state=useAppState();
 const resource=useResource<Application[]>('/me/applications?limit=100',state.applications.filter(a=>a.profileId===state.profile.id));
 const [error,setError]=useState(''),[busy,setBusy]=useState<string|null>(null);
 async function withdraw(id:string){setBusy(id);setError('');try{await mutate(`/applications/${id}/withdraw`,'POST',undefined,()=>updateDemo(s=>({...s,applications:s.applications.map(a=>a.id===id?{...a,status:'withdrawn'}:a)})));}catch(e){setError(e instanceof Error?e.message:'Не удалось отозвать отклик.');}finally{setBusy(null);}}
 return <section className="container page"><PageHeading title="Мои отклики" description="Предложения и решения заказчиков."/><div className="stack-lg"><div><button className="button secondary" onClick={resource.reload}>Обновить статусы</button></div>{error&&<Notice error>{error}</Notice>}{resource.data?resource.data.length?resource.data.map(a=><article className="card stack" key={a.id} data-testid="my-application"><h2><Link href={`/orders/${a.orderId}`}>{a.orderTitle||state.orders.find(o=>o.id===a.orderId)?.title||'Заказ'}</Link></h2><span className="badge align-start">{applicationStatus[a.status]}</span><p style={{whiteSpace:'pre-wrap'}}>{a.letter}</p><p>{money(a.price)} · До {dateLabel(a.deadline)}</p>{a.status==='pending'&&<button className="button secondary align-start" disabled={busy===a.id} onClick={()=>void withdraw(a.id)}>Отозвать отклик</button>}</article>):<EmptyState title="Вы ещё не откликались" description="Выберите открытый заказ и предложите решение." href="/orders" label="Посмотреть заказы"/>:<><ResourceState error={resource.error} reload={resource.reload}/>{resource.error&&!state.signedIn&&<LoginRequired/>}</>}</div></section>;
}
