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
  mock.method(
    globalThis,
    'fetch',
    async () =>
      new Response('{"error":"unauthorized"}', {
        status: 401,
        headers: { 'content-type': 'application/json' },
      }),
  );
  await assert.rejects(
    apiRequest('/protected'),
    (e: unknown) => e instanceof ApiError && e.status === 401,
  );
});
test('empty successful mutations are supported', async () => {
  mock.method(globalThis, 'fetch', async () => new Response(null, { status: 204 }));
  assert.equal(await apiRequest('/resource', { method: 'DELETE' }), undefined);
});
test('malformed JSON is reported instead of returned as typed application data', async () => {
  mock.method(
    globalThis,
    'fetch',
    async () => new Response('{broken', { headers: { 'content-type': 'application/json' } }),
  );
  await assert.rejects(apiRequest('/resource'), ApiError);
});
test('external origins cannot be supplied to the authenticated client', async () => {
  const fetchMock = mock.method(globalThis, 'fetch');
  await assert.rejects(apiRequest('//untrusted.example'));
  assert.equal(fetchMock.mock.callCount(), 0);
});
