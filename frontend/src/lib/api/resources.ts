'use client';
import { useEffect, useState } from 'react';
import { apiRequest } from './client';
import { DEMO } from '@/lib/state/store';
export function useResource<T>(path: string, demo: T) {
 const [remote, setRemote] = useState<{path:string;data?:T;error?:string}>({path:''});
 const [version,setVersion] = useState(0);
 useEffect(() => { if(DEMO) return; const controller=new AbortController();
 apiRequest<T>(path,{signal:controller.signal}).then(data=>setRemote({path,data})).catch(e=>{if(!controller.signal.aborted)setRemote({path,error:e instanceof Error?e.message:'Не удалось загрузить данные.'});});
 return ()=>controller.abort(); },[path,version]);
 return {data:DEMO?demo:remote.path===path?remote.data:undefined,error:DEMO?undefined:remote.path===path?remote.error:undefined,loading:!DEMO&&(remote.path!==path||(!remote.data&&!remote.error)),reload:()=>setVersion(v=>v+1)};
}
export async function mutate(path: string, method: 'POST'|'PATCH'|'DELETE', body: unknown, local: () => void) {
 if(DEMO){local();return;} await apiRequest(path,{method,body});
}
export function safeHref(value: string) { try { const url=new URL(value);return ['http:','https:'].includes(url.protocol)?url.href:undefined; }catch{return undefined;} }
