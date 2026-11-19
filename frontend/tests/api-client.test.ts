import { test, afterEach, mock } from 'node:test';
import assert from 'node:assert/strict';
import { apiRequest, ApiError, checkBackendHealth } from '../src/lib/api/client';
afterEach(() => mock.restoreAll());
test('health uses the existing Go route through the frontend proxy', async () => {
  const fetchMock = mock.method(globalThis, 'fetch', async () => new Response('OK'));
  assert.equal(await checkBackendHealth(), 'OK');
  assert.equal(fetchMock.mock.calls[0].arguments[0], '/api/health');
  assert.equal(fetchMock.mock.calls[0].arguments[1]?.credentials, 'include');
});
test('API failures stay failures, without silently substituting demo data', async () => {
  mock.method(globalThis, 'fetch', async () => new Response('{"error":"unauthorized"}', { status: 401, headers: { 'content-type': 'application/json' } }));
  await assert.rejects(apiRequest('/protected'), (e: unknown) => e instanceof ApiError && e.status === 401);
});
test('empty successful mutations are supported', async () => {
  mock.method(globalThis, 'fetch', async () => new Response(null, { status: 204 }));
  assert.equal(await apiRequest('/resource', { method: 'DELETE' }), undefined);
});
test('malformed JSON is reported instead of returned as typed application data', async () => {
  mock.method(globalThis, 'fetch', async () => new Response('{broken', { headers: { 'content-type': 'application/json' } }));
  await assert.rejects(apiRequest('/resource'), ApiError);
});
test('external origins cannot be supplied to the authenticated client', async () => {
  const fetchMock = mock.method(globalThis, 'fetch');
  await assert.rejects(apiRequest('//untrusted.example'));
  assert.equal(fetchMock.mock.callCount(), 0);
});
test('concurrent expired requests share one refresh and retry once', async () => {
 let refreshed=false,refreshes=0;
 const counts=new Map<string,number>();
 mock.method(globalThis,'fetch',async(input:RequestInfo|URL)=>{
  const path=String(input);counts.set(path,(counts.get(path)||0)+1);
  if(path==='/api/auth/refresh'){refreshes++;await new Promise(resolve=>setTimeout(resolve,10));refreshed=true;return new Response('{}',{headers:{'content-type':'application/json'}});}
  return new Response(JSON.stringify(refreshed?{ok:true}:{error:{message:'expired'}}),{status:refreshed?200:401,headers:{'content-type':'application/json'}});
 });
 const values=await Promise.all([apiRequest<{ok:boolean}>('/one'),apiRequest<{ok:boolean}>('/two')]);
 assert.equal(refreshes,1);assert.equal(counts.get('/api/one'),2);assert.equal(counts.get('/api/two'),2);assert.ok(values.every(v=>v.ok));
});
test('an unauthorized retry does not loop and login never refreshes',async()=>{
 const fetchMock=mock.method(globalThis,'fetch',async(input:RequestInfo|URL)=>String(input)==='/api/auth/refresh'?new Response('{}'):new Response('{}',{status:401,headers:{'content-type':'application/json'}}));
 await assert.rejects(apiRequest('/protected'),(e:unknown)=>e instanceof ApiError&&e.status===401);
 assert.equal(fetchMock.mock.callCount(),3);
 await assert.rejects(apiRequest('/auth/login',{method:'POST',body:{email:'a@example.com',password:'wrong'}}),ApiError);
 assert.equal(fetchMock.mock.callCount(),4);
});
