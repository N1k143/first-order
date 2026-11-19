'use client';
import { useSyncExternalStore } from 'react';
import { initialState } from '@/data/demo';
import { apiRequest, ApiError } from '@/lib/api/client';
import type { AppState, Application, Company, Contact, Order, Portfolio, Profile } from '@/types/domain';
export const DEMO = process.env.NEXT_PUBLIC_DATA_MODE !== 'api';
const KEY = 'firstorder:demo:v1';
const emptyState: AppState = {
 profile: {id:'',name:'',email:'',role:'developer',city:'',bio:'',headline:'',skills:[],github:''},
 signedIn:false, favorites:[],portfolio:[],contacts:[],applications:[],orders:[],members:[],corrections:[],hiddenCompanies:[],history:[],
 company:{id:'',name:'',initials:'',city:'',category:'',address:'',description:'',websiteStatus:'needs_review',checkedAt:''},
};
const serverSnapshot = DEMO ? initialState : emptyState;
let state = serverSnapshot;
let initialized = false;
let generation = 0;
let loadingController: AbortController | undefined;
const listeners = new Set<() => void>();
function notify() { listeners.forEach(l => l()); }
function items<T>(value: T[] | {items:T[]}): T[] { return Array.isArray(value) ? value : value.items; }
type RemoteProfile = Profile & {companies?:{companyId:string;memberRole:string}[]};

/** Replace the API snapshot only with server data; failed requests never use fixtures. */
export async function refreshAppState(): Promise<void> {
 if (DEMO || typeof window === 'undefined') return;
 const version = ++generation;
 loadingController?.abort();
 const controller = new AbortController(); loadingController = controller;
 const signal = controller.signal;
 const next: AppState = structuredClone(emptyState);
 const orderTask = apiRequest<Order[] | {items:Order[]}>('/orders?limit=100', {signal})
  .then(value => { next.orders = items(value); }).catch(() => { next.orders = []; });
 try {
  const profile = await apiRequest<RemoteProfile>('/me', {signal});
  next.profile = {...emptyState.profile,...profile,skills:profile.skills || [],bio:profile.bio || '',github:profile.github || ''};
  next.signedIn = true;
  const tasks: Promise<void>[] = [];
  if (profile.role !== 'moderator') {
   tasks.push(apiRequest<Company[] | {items:Company[]}>('/me/favorites?limit=100',{signal}).then(v=>{next.favorites=items(v).map(c=>c.id);}).catch(()=>{next.favorites=[];}));
   tasks.push(apiRequest<Contact[] | {items:Contact[]}>('/me/contacts?limit=100',{signal}).then(v=>{next.contacts=items(v).map(c=>({...c,note:c.note||'',date:c.date||'',status:({planned:'Планирую написать',contacted:'Написал',replied:'Ответили',declined:'Не актуально'} as Record<string,string>)[c.status]||c.status}));}).catch(()=>{next.contacts=[];}));
   tasks.push(apiRequest<Application[] | {items:Application[]}>('/me/applications?limit=100',{signal}).then(v=>{next.applications=items(v);}).catch(()=>{next.applications=[];}));
   tasks.push(apiRequest<Portfolio[] | {items:Portfolio[]}>('/me/portfolio?limit=100',{signal}).then(v=>{next.portfolio=items(v);}).catch(()=>{next.portfolio=[];}));
  }
  const companyId = profile.companies?.[0]?.companyId;
  if(companyId) tasks.push(apiRequest<Company>(`/companies/${encodeURIComponent(companyId)}`,{signal}).then(v=>{next.company=v;}).catch(()=>{next.company=structuredClone(emptyState.company);}));
  await Promise.all(tasks);
 } catch(error) {
  // Signed-out and unavailable sessions both clear personal data. Resource hooks
  // report HTTP failures on their pages; no previous user's snapshot survives.
  if (!(error instanceof ApiError && error.status === 401) && signal.aborted) return;
 }
 await orderTask;
 if (version === generation && !signal.aborted) { state = next; notify(); }
}
function load() {
 if (initialized || typeof window === 'undefined') return; initialized = true;
 if (DEMO) {
  try { const value = JSON.parse(localStorage.getItem(KEY) || 'null'); if(value && value.profile && Array.isArray(value.orders) && Array.isArray(value.applications)) state = {...initialState,...value}; } catch { /* Unavailable demo storage: use fixtures. */ }
  queueMicrotask(notify);
 } else { void refreshAppState(); }
}
function subscribe(listener: () => void) { listeners.add(listener); load(); return () => { listeners.delete(listener); }; }
export function useAppState() { return useSyncExternalStore(subscribe, () => state, () => serverSnapshot); }
export function updateDemo(update: (current: AppState) => AppState) {
 if (!DEMO) throw new Error('Local mutations are disabled in API mode');
 state = update(state); try { localStorage.setItem(KEY, JSON.stringify(state)); } catch { /* In-memory demo remains available. */ } notify();
}
export function resetDemo() { updateDemo(() => structuredClone(initialState)); }
